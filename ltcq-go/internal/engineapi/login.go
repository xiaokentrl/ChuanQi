package engineapi

import (
	"database/sql"
	"encoding/binary"
	"fmt"
	"log/slog"
	"strings"

	_ "github.com/go-sql-driver/mysql"
)

// login.go：C++ 引擎级登录状态机复刻（M2 实测修订 / ADR 待立项）。
//
// 权威依据：
//   - 登录帧 54B 结构：docs/RUNBOOK.md §七.7；golden session005_c2g_084447.raw
//   - 账号表 actor_s1.globaluser(account,passwd,closed)：www/backup/database/actor_s1.sql:309-328
//   - 密码 md5 明文比对（无加盐）：data/h5game/web/s10001/Login.php:27,41
//   - 回包 ff 01 00 = CMD_Login(255)/子命令1/成功：RUNBOOK §七.7 探针实测
//
// 登录鉴权不在 Lua（CMD_Login=255 无 netmsgdispatcher.reg），故与 OnNetMsg 分流。

const (
	cmdLogin             = 0xff // CMD_Login（protocol.lua:96）
	loginSubCmdResult    = 0x01 // 登录结果子命令
	loginResultSuccess   = 0x00 // ff 01 00
	loginResultBadPass   = 0x01 // ff 01 01
	loginResultNoAccount = 0x02 // ff 01 02
)

// LoginFrame 是从 cmd=5 body（NetId 之后）解出的登录请求。
type LoginFrame struct {
	Flag     uint32 // +0 4B 登录类型标志（golden 首帧=1）
	Cmd      byte   // +4 CMD_Login=255
	SubCmd   byte   // +5 子命令=1
	Account  string // 账号明文 UTF-8
	Password string // 32B md5（小写 hex）
}

// ParseLoginFrame 解析登录帧。data 为 NetId 之后的字节。
// 布局：[4B 标志][ff][01][01 00 00 00][2B 账号长][账号][00 20 00][32B md5][00]
func ParseLoginFrame(data []byte) (*LoginFrame, error) {
	if len(data) < 4+1+1+4+2+1+32+1 {
		return nil, fmt.Errorf("登录帧过短: %d", len(data))
	}
	f := &LoginFrame{Flag: binary.LittleEndian.Uint32(data[0:4]), Cmd: data[4], SubCmd: data[5]}
	if f.Cmd != cmdLogin {
		return nil, fmt.Errorf("非登录帧: cmd=%02x", f.Cmd)
	}
	off := 10 // 4标志 + ff + 子命令 + 4B 保留
	n := int(binary.LittleEndian.Uint16(data[off : off+2]))
	off += 2
	if n == 0 || n > 64 || off+n+3+32 > len(data) {
		return nil, fmt.Errorf("登录帧账号长度非法: n=%d len=%d", n, len(data))
	}
	f.Account = string(data[off : off+n])
	off += n
	// 3B 分隔符段（00 20 00），跳过
	off += 3
	f.Password = string(data[off : off+32])
	return f, nil
}

// AccountStore 是登录校验所需的账号存储（便于单测 mock）。
type AccountStore interface {
	// CheckPassword 返回 (accountID, 是否通过, error)。closed 账号视为不通过。
	CheckPassword(account, md5pass string) (int, bool, error)
	// ActorIDByAccount 返回账号对应的角色 id；不存在返回 0,nil（由调用方决定是否建号）。
	ActorIDByAccount(account string) (int, error)
	Close()
}

// MySQLAccountStore 用 actor_s1 库实现 AccountStore。
type MySQLAccountStore struct {
	db *sql.DB
}

// NewMySQLAccountStore 连接 MySQL（dsn 示例 root:123456@tcp(127.0.0.1:3306)/actor_s1?charset=utf8）。
func NewMySQLAccountStore(dsn string) (*MySQLAccountStore, error) {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("MySQL 连接失败: %w", err)
	}
	return &MySQLAccountStore{db: db}, nil
}

func (s *MySQLAccountStore) CheckPassword(account, md5pass string) (int, bool, error) {
	var userID int
	var passwd string
	var closed int
	err := s.db.QueryRow(
		"SELECT userid, passwd, COALESCE(closed,0) FROM globaluser WHERE account=?", account,
	).Scan(&userID, &passwd, &closed)
	if err == sql.ErrNoRows {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	if closed != 0 {
		return userID, false, nil
	}
	return userID, strings.EqualFold(passwd, md5pass), nil
}

func (s *MySQLAccountStore) ActorIDByAccount(account string) (int, error) {
	var actorID int
	err := s.db.QueryRow(
		"SELECT actorid FROM actors WHERE accountname=? LIMIT 1", account,
	).Scan(&actorID)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return actorID, err
}

func (s *MySQLAccountStore) Close() { _ = s.db.Close() }

// LoginManager 驱动登录状态机：解析帧 → 校验 → openuser → 绑定 actor → 回包。
type LoginManager struct {
	store  AccountStore
	router *NetRouter
}

func NewLoginManager(store AccountStore, router *NetRouter) *LoginManager {
	return &LoginManager{store: store, router: router}
}

// HandleLogin 实现 NetRouter 的 login handler 签名。
func (m *LoginManager) HandleLogin(sid uint16, data []byte) error {
	frame, err := ParseLoginFrame(data)
	if err != nil {
		slog.Warn("登录帧解析失败", "sid", sid, "err", err)
		return m.reply(sid, loginResultBadPass)
	}
	accountID, ok, err := m.store.CheckPassword(frame.Account, frame.Password)
	if err != nil {
		slog.Error("账号校验失败", "account", frame.Account, "err", err)
		return m.reply(sid, loginResultBadPass)
	}
	if !ok {
		slog.Info("密码校验失败", "account", frame.Account, "accountID", accountID)
		return m.reply(sid, loginResultNoAccount)
	}
	slog.Info("密码校验通过", "account", frame.Account, "accountID", accountID)

	actorID, err := m.store.ActorIDByAccount(frame.Account)
	if err != nil {
		slog.Error("查询角色失败", "account", frame.Account, "err", err)
		actorID = 0
	}
	if actorID != 0 {
		actor := DefaultActorManager.GetOrCreate(actorID)
		actor.Name = frame.Account
		m.router.BindActor(sid, actorID)
		slog.Info("登录绑定 actor", "sid", sid, "actorID", actorID)
	}
	return m.reply(sid, loginResultSuccess)
}

// reply 发送登录结果帧：[ff][01][result]（CMD_Login/子命令1/结果码）。
func (m *LoginManager) reply(sid uint16, result byte) error {
	packet := []byte{cmdLogin, loginSubCmdResult, result}
	return m.router.SendToSession(sid, packet)
}
