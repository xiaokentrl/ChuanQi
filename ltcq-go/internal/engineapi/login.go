package engineapi

import (
	"database/sql"
	"encoding/binary"
	"fmt"
	"log/slog"
	"strings"

	_ "github.com/go-sql-driver/mysql"
)

const (
	cmdLogin             = 0xff
	loginSubCmdResult    = 0x01
	loginSubCmdCreate    = 0x02
	loginSubCmdRoleList  = 0x04
	loginSubCmdEnterGame = 0x05

	loginResultSuccess   = 0x00
	loginResultBadPass   = 0x01
	loginResultNoAccount = 0x02
)

const actorColumns = `
	actorid, COALESCE(accountid,0), COALESCE(accountname,''), COALESCE(actorname,''),
	COALESCE(job,0), COALESCE(sex,0), COALESCE(level,1), COALESCE(exp,0),
	COALESCE(sceneid,0), COALESCE(gold,0), COALESCE(yuanbao,0), COALESCE(totalpower,0),
	COALESCE(baggridcount,0), COALESCE(vip_level,0), COALESCE(zhuansheng_lv,0),
	COALESCE(feats,0), COALESCE(shatter,0), COALESCE(spcshatter,0),
	COALESCE(togeatter,0), COALESCE(togeatterhigh,0), COALESCE(prestige_exp,0), COALESCE(status,0),
	COALESCE(recharge,0), COALESCE(UNIX_TIMESTAMP(createtime),0),
	COALESCE(appid,''), COALESCE(pfid,''), COALESCE(guildid,0), COALESCE(guild_name,''),
	COALESCE(chapterlevel,0), COALESCE(monthcard,0), COALESCE(reincarnate_lv,0)`

type LoginFrame struct {
	Flag     uint32
	Cmd      byte
	SubCmd   byte
	ServerID uint32
	Account  string
	Password string
}

func ParseLoginFrame(data []byte) (*LoginFrame, error) {
	if len(data) < 16 {
		return nil, fmt.Errorf("登录帧过短: %d", len(data))
	}
	if data[4] != cmdLogin || data[5] != loginSubCmdResult {
		return nil, fmt.Errorf("非登录请求: %02x %02x", data[4], data[5])
	}
	off := 10
	account, next, err := parseEgretString(data, off)
	if err != nil {
		return nil, fmt.Errorf("登录帧账号: %w", err)
	}
	if account == "" || len(account) > 64 {
		return nil, fmt.Errorf("登录帧账号长度非法")
	}
	off = next
	if off+2 > len(data) {
		return nil, fmt.Errorf("登录帧缺少密码长度")
	}
	n := int(binary.LittleEndian.Uint16(data[off : off+2]))
	off += 2
	if n != 32 || off+n+1 > len(data) || data[off+n] != 0 {
		return nil, fmt.Errorf("登录帧密码格式非法")
	}
	return &LoginFrame{
		Flag:     binary.LittleEndian.Uint32(data[0:4]),
		Cmd:      data[4],
		SubCmd:   data[5],
		ServerID: binary.LittleEndian.Uint32(data[6:10]),
		Account:  account,
		Password: string(data[off : off+32]),
	}, nil
}

func parseEgretString(data []byte, off int) (string, int, error) {
	if off < 0 || off+2 > len(data) {
		return "", off, fmt.Errorf("字符串长度缺失")
	}
	n := int(binary.LittleEndian.Uint16(data[off : off+2]))
	off += 2
	if off+n+1 > len(data) || data[off+n] != 0 {
		return "", off, fmt.Errorf("UTF-8 字符串格式非法: len=%d", n)
	}
	return string(data[off : off+n]), off + n + 1, nil
}

func appendEgretString(p []byte, value string) []byte {
	name := []byte(value)
	p = binary.LittleEndian.AppendUint16(p, uint16(len(name)))
	p = append(p, name...)
	return append(p, 0)
}

type AccountStore interface {
	CheckPassword(account, md5pass string) (int, bool, error)
	ListActors(account string) ([]ActorData, error)
	LoadActorData(actorID int) (*ActorData, error)
	CreateActor(account string, accountID int, name string, job, sex int) (int, error)
	SaveProgression(actorID int, level int, exp, gold int64) error
	Close()
}

type MySQLAccountStore struct {
	db *sql.DB
}

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
	return userID, closed == 0 && strings.EqualFold(passwd, md5pass), nil
}

func scanActor(row interface{ Scan(...any) error }) (*ActorData, error) {
	d := &ActorData{}
	err := row.Scan(&d.ActorID, &d.AccountID, &d.Account, &d.Name,
		&d.Job, &d.Sex, &d.Level, &d.Exp, &d.SceneID, &d.Gold, &d.YuanBao,
		&d.TotalPower, &d.BagGridCount, &d.VIPLevel, &d.ZhuanSheng,
		&d.Feats, &d.RuneShatter, &d.RuneExchange,
		&d.Together1, &d.Together2, &d.WeiWang, &d.Status,
		&d.Recharge, &d.CreateTime, &d.AppID, &d.PfID,
		&d.GuildID, &d.GuildName, &d.ChapterLevel, &d.MonthCard, &d.Reincarnate)
	return d, err
}

func (s *MySQLAccountStore) ListActors(account string) ([]ActorData, error) {
	rows, err := s.db.Query("SELECT "+actorColumns+" FROM actors WHERE accountname=? ORDER BY actorid", account)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var actors []ActorData
	for rows.Next() {
		d, err := scanActor(rows)
		if err != nil {
			return nil, err
		}
		actors = append(actors, *d)
	}
	return actors, rows.Err()
}

func (s *MySQLAccountStore) LoadActorData(actorID int) (*ActorData, error) {
	return scanActor(s.db.QueryRow("SELECT "+actorColumns+" FROM actors WHERE actorid=?", actorID))
}

func (s *MySQLAccountStore) CreateActor(account string, accountID int, name string, job, sex int) (int, error) {
	var exists int
	if err := s.db.QueryRow("SELECT 1 FROM actors WHERE actorname=? LIMIT 1", name).Scan(&exists); err == nil {
		return 0, fmt.Errorf("角色名已存在")
	} else if err != sql.ErrNoRows {
		return 0, err
	}
	res, err := s.db.Exec(`
		INSERT INTO actors(accountid,accountname,actorname,status,job,sex,serverindex,createtime,updatetime)
		VALUES(?,?,?,2,?,?,1,NOW(),NOW())`, accountID, account, name, job, sex)
	if err != nil {
		return 0, err
	}
	id64, err := res.LastInsertId()
	if err != nil || id64 <= 0 || uint64(id64) > 0xffffffff {
		return 0, fmt.Errorf("角色 ID 非法: %d %v", id64, err)
	}
	id := int(id64)
	cleanup := func() {
		for _, table := range []string{"actorvariable", "actorbinarydata", "roles", "actors"} {
			_, _ = s.db.Exec("DELETE FROM "+table+" WHERE actorid=?", id)
		}
	}
	if _, err := s.db.Exec("INSERT INTO roles(actorid,roleid,job,sex) VALUES(?,0,?,?)", id, job, sex); err != nil {
		cleanup()
		return 0, err
	}
	if _, err := s.db.Exec("INSERT INTO actorbinarydata(actorid) VALUES(?)", id); err != nil {
		cleanup()
		return 0, err
	}
	if _, err := s.db.Exec("INSERT INTO actorvariable(actorid) VALUES(?)", id); err != nil {
		cleanup()
		return 0, err
	}
	return id, nil
}

func (s *MySQLAccountStore) SaveProgression(actorID int, level int, exp, gold int64) error {
	if actorID <= 0 || level <= 0 || exp < 0 || gold < 0 {
		return fmt.Errorf("角色进度非法: actor=%d level=%d exp=%d gold=%d", actorID, level, exp, gold)
	}
	res, err := s.db.Exec("UPDATE actors SET level=?, exp=?, gold=?, updatetime=NOW() WHERE actorid=?", level, exp, gold, actorID)
	if err != nil {
		return err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		var exists int
		if err := s.db.QueryRow("SELECT 1 FROM actors WHERE actorid=?", actorID).Scan(&exists); err != nil {
			return err
		}
		if exists != 1 {
			return fmt.Errorf("角色进度保存未命中: actor=%d", actorID)
		}
	}
	return nil
}

func (s *MySQLAccountStore) SaveVIPLevel(actorID, level int) error {
	if actorID <= 0 || level < 0 {
		return fmt.Errorf("VIP 等级参数非法: actor=%d level=%d", actorID, level)
	}
	res, err := s.db.Exec("UPDATE actors SET vip_level=?, updatetime=NOW() WHERE actorid=?", level, actorID)
	if err != nil {
		return err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 1 {
		return nil
	}
	var exists int
	if err := s.db.QueryRow("SELECT 1 FROM actors WHERE actorid=?", actorID).Scan(&exists); err != nil {
		return err
	}
	if exists != 1 {
		return fmt.Errorf("VIP 等级保存未命中: actor=%d", actorID)
	}
	return nil
}

func (s *MySQLAccountStore) SaveCurrency(actorID, currencyType int, amount int64) error {
	if actorID <= 0 || amount < 0 {
		return fmt.Errorf("货币存档参数非法: actor=%d type=%d amount=%d", actorID, currencyType, amount)
	}
	var column string
	switch currencyType {
	case currencyGold:
		column = "gold"
	case currencyYuanBao:
		column = "yuanbao"
	default:
		return fmt.Errorf("货币类型不支持持久化: %d", currencyType)
	}
	res, err := s.db.Exec("UPDATE actors SET "+column+"=?, updatetime=NOW() WHERE actorid=?", amount, actorID)
	if err != nil {
		return err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 1 {
		return nil
	}
	var exists int
	if err := s.db.QueryRow("SELECT 1 FROM actors WHERE actorid=?", actorID).Scan(&exists); err != nil {
		return err
	}
	if exists != 1 {
		return fmt.Errorf("货币存档未命中: actor=%d type=%d", actorID, currencyType)
	}
	return nil
}

func (s *MySQLAccountStore) Close() { _ = s.db.Close() }

type LoginManager struct {
	store  AccountStore
	router *NetRouter
	post   PostLoginHandler
}

func NewLoginManager(store AccountStore, router *NetRouter) *LoginManager {
	return &LoginManager{store: store, router: router}
}

func (m *LoginManager) SetPostLoginHandler(h PostLoginHandler) { m.post = h }

func (m *LoginManager) HandleLogin(sid uint16, data []byte) error {
	if len(data) < 6 || data[4] != cmdLogin {
		return fmt.Errorf("登录帧过短或命令非法: %d", len(data))
	}
	switch data[5] {
	case loginSubCmdResult:
		return m.authenticate(sid, data)
	case loginSubCmdRoleList:
		return m.listRoles(sid)
	case loginSubCmdCreate:
		return m.createRole(sid, data)
	case loginSubCmdEnterGame:
		return m.enterGame(sid, data)
	default:
		return fmt.Errorf("未知登录子命令: %02x", data[5])
	}
}

func (m *LoginManager) authenticate(sid uint16, data []byte) error {
	frame, err := ParseLoginFrame(data)
	if err != nil {
		slog.Warn("登录帧解析失败", "sid", sid, "err", err)
		return m.replyLogin(sid, loginResultBadPass)
	}
	accountID, ok, err := m.store.CheckPassword(frame.Account, frame.Password)
	if err != nil {
		slog.Error("账号校验失败", "account", frame.Account, "err", err)
		return m.replyLogin(sid, loginResultBadPass)
	}
	if !ok {
		return m.replyLogin(sid, loginResultNoAccount)
	}
	m.router.SetAccount(sid, frame.Account, accountID, frame.ServerID)
	slog.Info("账号鉴权通过", "sid", sid, "account", frame.Account, "accountID", accountID)
	return m.replyLogin(sid, loginResultSuccess)
}

func (m *LoginManager) authenticated(sid uint16) (*GateSession, error) {
	sess := m.router.AuthenticatedSession(sid)
	if sess == nil {
		return nil, fmt.Errorf("会话尚未鉴权: sid=%d", sid)
	}
	return sess, nil
}

func (m *LoginManager) listRoles(sid uint16) error {
	sess, err := m.authenticated(sid)
	if err != nil {
		return err
	}
	actors, err := m.store.ListActors(sess.Account)
	if err != nil {
		slog.Error("角色列表查询失败", "account", sess.Account, "err", err)
		return m.replyRoleList(sid, sess.AccountID, nil, 2)
	}
	status := byte(1)
	if len(actors) == 0 {
		status = 0
	}
	return m.replyRoleList(sid, sess.AccountID, actors, status)
}

func (m *LoginManager) replyRoleList(sid uint16, accountID int, actors []ActorData, status byte) error {
	p := []byte{cmdLogin, loginSubCmdRoleList}
	p = binary.LittleEndian.AppendUint32(p, uint32(accountID))
	p = append(p, status)
	if status == 1 {
		p = binary.LittleEndian.AppendUint32(p, uint32(len(actors)))
		for i := range actors {
			p = appendActorSummary(p, &actors[i])
		}
	}
	return m.router.SendToSession(sid, p)
}

func appendActorSummary(p []byte, d *ActorData) []byte {
	p = binary.LittleEndian.AppendUint32(p, uint32(d.ActorID))
	p = appendEgretString(p, d.Name)
	p = binary.LittleEndian.AppendUint32(p, uint32(d.Job))
	p = binary.LittleEndian.AppendUint32(p, uint32(d.ZhuanSheng))
	p = binary.LittleEndian.AppendUint32(p, uint32(d.Level))
	p = binary.LittleEndian.AppendUint64(p, uint64(d.TotalPower))
	p = binary.LittleEndian.AppendUint32(p, uint32(d.VIPLevel))
	p = binary.LittleEndian.AppendUint32(p, uint32(d.Sex))
	return p
}

type createRoleFrame struct {
	Name    string
	Sex     int
	Job     int
	Channel [3]string
}

func parseCreateRoleFrame(data []byte) (*createRoleFrame, error) {
	if len(data) < 9 {
		return nil, fmt.Errorf("创角帧过短")
	}
	name, off, err := parseEgretString(data, 6)
	if err != nil {
		return nil, err
	}
	if name == "" || len([]rune(name)) > 12 || off+3 > len(data) {
		return nil, fmt.Errorf("角色名或属性非法")
	}
	f := &createRoleFrame{Name: name, Sex: int(data[off]), Job: int(data[off+1])}
	off += 3
	if f.Sex < 0 || f.Sex > 1 || f.Job < 1 || f.Job > 3 {
		return nil, fmt.Errorf("职业或性别非法")
	}
	for i := range f.Channel {
		value, next, err := parseEgretString(data, off)
		if err != nil {
			return nil, err
		}
		f.Channel[i] = value
		off = next
	}
	return f, nil
}

func (m *LoginManager) createRole(sid uint16, data []byte) error {
	sess, err := m.authenticated(sid)
	if err != nil {
		return err
	}
	frame, err := parseCreateRoleFrame(data)
	if err != nil {
		return m.replyCreate(sid, 0, loginResultBadPass)
	}
	actorID, err := m.store.CreateActor(sess.Account, sess.AccountID, frame.Name, frame.Job, frame.Sex)
	if err != nil {
		slog.Warn("创角失败", "account", sess.Account, "name", frame.Name, "err", err)
		code := byte(loginResultBadPass)
		if strings.Contains(err.Error(), "角色名已存在") {
			code = 6
		}
		return m.replyCreate(sid, 0, code)
	}
	slog.Info("创角完成", "actorID", actorID, "account", sess.Account, "name", frame.Name)
	return m.replyCreate(sid, actorID, 0)
}

func (m *LoginManager) replyCreate(sid uint16, actorID int, result byte) error {
	p := []byte{cmdLogin, loginSubCmdCreate}
	p = binary.LittleEndian.AppendUint32(p, uint32(actorID))
	return m.router.SendToSession(sid, append(p, result))
}

func (m *LoginManager) enterGame(sid uint16, data []byte) error {
	sess, err := m.authenticated(sid)
	if err != nil {
		return err
	}
	if len(data) < 10 {
		return fmt.Errorf("进游戏帧过短")
	}
	actorID := int(int32(binary.LittleEndian.Uint32(data[6:10])))
	if actorID <= 0 {
		return m.replyEnter(sid, 0)
	}
	off := 10
	for range 3 {
		_, next, err := parseEgretString(data, off)
		if err != nil {
			return fmt.Errorf("进游戏渠道字段非法: %w", err)
		}
		off = next
	}
	actor, err := m.store.LoadActorData(actorID)
	if err != nil || actor.Account != sess.Account || actor.AccountID != sess.AccountID {
		return m.replyEnter(sid, 0)
	}
	m.router.BindActor(sid, actorID)
	if m.post != nil {
		if err := m.post.HandlePostLogin(sid, actorID, actor.Account, func() error {
			return m.replyEnter(sid, 1)
		}); err != nil {
			m.router.UnbindActor(sid)
			return m.replyEnter(sid, 0)
		}
		return nil
	}
	return m.replyEnter(sid, 1)
}

func (m *LoginManager) replyEnter(sid uint16, result byte) error {
	return m.router.SendToSession(sid, []byte{cmdLogin, loginSubCmdEnterGame, result})
}

func (m *LoginManager) replyLogin(sid uint16, result byte) error {
	return m.router.SendToSession(sid, []byte{cmdLogin, loginSubCmdResult, result})
}
