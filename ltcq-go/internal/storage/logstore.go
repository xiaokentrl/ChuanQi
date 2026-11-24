// Package storage：loggerserver 的 MySQL 写入层。
// 建表/INSERT 模板照抄原版 data/loggerserver/data/main.lua（对拍一致性优先）。
// 原版语义：启动时从当前月起循环 100 个月 drop+create（首月不 drop）。
package storage

import (
	"database/sql"
	"fmt"
	"time"

	_ "github.com/go-sql-driver/mysql"

	"github.com/xiaokentrl/ChuanQi/ltcq-go/internal/netframe"
)

// Store 封装 log 库连接。dbname 独立可配（对拍时 Go 用 log_s1_go，原版用 log_s1）。
type Store struct {
	DB       *sql.DB
	dbname   string
	logger   *logLogger
	sendTobi bool
}

type logLogger struct{ db *sql.DB }

func (l *logLogger) getDate() (int, int) {
	n := time.Now()
	return n.Year(), int(n.Month())
}

// Open 连接 MySQL 并选定库。
func Open(dsn, dbname string) (*Store, error) {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		return nil, err
	}
	if _, err := db.Exec("USE `" + dbname + "`"); err != nil {
		return nil, fmt.Errorf("选库 %s 失败: %w", dbname, err)
	}
	s := &Store{DB: db, dbname: dbname}
	s.logger = &logLogger{db: db}
	return s, nil
}

// Close 关闭连接。
func (s *Store) Close() error { return s.DB.Close() }

// EnsureTables 复刻原版 CreateTablesThreadRoutine：从当前月起 100 个月，首月不 drop，逐月 drop+create。
func (s *Store) EnsureTables() error {
	now := time.Now()
	for i := 0; i < 100; i++ {
		t := now.AddDate(0, i, 0)
		y, m := t.Year(), int(t.Month())
		drops := []string{sqlDropCounter, sqlDropInstall, sqlDropDau, sqlDropEconomy,
			sqlDropMilestone, sqlDropPayment, sqlDropGif}
		if i != 0 { // 原版：第 i==1 轮起才 drop（首月跳过）
			for _, tpl := range drops {
				if _, err := s.DB.Exec(fmt.Sprintf(tpl, y, m)); err != nil {
					return fmt.Errorf("drop %d%02d: %w", y, m, err)
				}
			}
		}
		creates := []string{sqlCreateCounter, sqlCreatePayment, sqlCreateGif, sqlCreateDau,
			sqlCreateInstall, sqlCreateEconomy, sqlCreateMilestone, sqlCreateOnline}
		for _, tpl := range creates {
			if _, err := s.DB.Exec(fmt.Sprintf(tpl, y, m)); err != nil {
				return fmt.Errorf("create %d%02d: %w", y, m, err)
			}
		}
	}
	// singlegif 只 create 不 drop
	if _, err := s.DB.Exec(sqlCreateSingleGif); err != nil {
		return err
	}
	return nil
}

func (s *Store) ym() (int, int) { return s.logger.getDate() }

// InsertOnline 等价原版 cmd=9（Lua precessLog）：写 log_online_YYYYMM。
func (s *Store) InsertOnline(serverID int32, cnt int32) error {
	y, m := s.ym()
	_, err := s.DB.Exec(fmt.Sprintf(sqlLogOnline, y, m, serverID, cnt))
	return err
}

// InsertCounter 等价原版 cmd=1。
func (s *Store) InsertCounter(serverID int32, r netframe.CounterRec) error {
	y, m := s.ym()
	_, err := s.DB.Exec(fmt.Sprintf(sqlLogCounter, y, m, r.ActorID, r.UserLevel, r.Counter,
		r.Value, r.Extra, r.Kingdom, r.Phylum, r.Classfield, r.Family, r.Genus, serverID))
	return err
}

// InsertInstall 等价原版 cmd=2（actorid 列 = aid_ 字符串，对齐 main.lua 的 '%s'）。
func (s *Store) InsertInstall(serverID int32, r netframe.InstallRec) error {
	y, m := s.ym()
	_, err := s.DB.Exec(fmt.Sprintf(sqlLogInstall, y, m, r.Aid, r.Source, r.Affiliate,
		r.Creative, r.Family, r.Genus, r.FromUID, r.Extra, serverID))
	return err
}

// InsertDau 等价原版 cmd=3。
func (s *Store) InsertDau(serverID int32, r netframe.DauRec) error {
	y, m := s.ym()
	_, err := s.DB.Exec(fmt.Sprintf(sqlLogDau, y, m, r.Aid, r.Source, r.Affiliate,
		r.Creative, r.Family, r.Genus, r.IP, r.FromUID, r.Extra, serverID))
	return err
}

// InsertEconomy 等价原版 cmd=4。
func (s *Store) InsertEconomy(serverID int32, r netframe.EconomyRec) error {
	y, m := s.ym()
	_, err := s.DB.Exec(fmt.Sprintf(sqlLogEconomy, y, m, r.ActorID, r.Currency, r.Amount,
		r.Value, r.Kingdom, r.Phylum, r.Classfield, r.Family, r.Genus, r.Extra, r.Extra2, serverID))
	return err
}

// InsertMilestone 等价原版 cmd=5。
func (s *Store) InsertMilestone(serverID int32, r netframe.MilestoneRec) error {
	y, m := s.ym()
	_, err := s.DB.Exec(fmt.Sprintf(sqlLogMilestone, y, m, r.ActorID, r.Milestone, r.Value, r.Extra, serverID))
	return err
}

// InsertChat 等价原版 cmd=14。原版 main.lua 无 log_chat 模板；列序对照 LChat 字段与 payment 同构，
// 首版对齐 log_gif 列序（logdate,actorid,ret,amount,value2,kingdom,phylum,transactionid,extra,msgtype,p1..p5,serverid）
// —— 待对拍校准，见 docs/loggerserver-spec.md 遗留歧义。
func (s *Store) InsertChat(serverID int32, r netframe.ChatRec) error {
	y, m := s.ym()
	// 列序：actorid,ret,amount,value2,kingdom,phylum,transactionid,extra,msgtype,p1..p5,serverid
	// 原占位实现把 %s 列传了 int（vet 报错），此处按字段类型对齐；具体语义待对拍校准。
	_, err := s.DB.Exec(fmt.Sprintf(sqlLogChat, y, m, r.ActorID, r.Type, 0, 0,
		r.IP, r.PF, r.Aid, r.Content, r.Type, 0, 0, 0, 0, 0, serverID))
	return err
}

// ---- SQL 模板（照抄 main.lua；占位符改为 fmt 风格） ----

const sqlDropCounter = "drop table if exists `log_counter_%d%02d`;"
const sqlDropInstall = "drop table if exists `log_install_%d%02d`;"
const sqlDropDau = "drop table if exists `log_dau_%d%02d`;"
const sqlDropEconomy = "drop table if exists `log_economy_%d%02d`;"
const sqlDropMilestone = "drop table if exists `log_milestone_%d%02d`;"
const sqlDropPayment = "drop table if exists `log_payment_%d%02d`;"
const sqlDropGif = "drop table if exists `log_gif_%d%02d`;"
const sqlDropOnline = "drop table if exists `log_online_%d%02d`;"

const sqlCreateCounter = "create table if not exists `log_counter_%d%02d` (`logdate` datetime not null,`actorid` bigint(10) default null,`user_level` varchar(32),`counter` varchar(32),`value` varchar(32),`extra` varchar(32),`kingdom` varchar(32),`phylum` varchar(32),`classfield` varchar(32),`family` varchar(32),`genus` varchar(32),`serverid` int default 0,KEY `idx` (`logdate`,`counter`)) engine=myisam default charset=utf8;"

const sqlCreateInstall = "create table if not exists `log_install_%d%02d` (`logdate` datetime not null,`actorid` varchar(64),`source` varchar(32),`affiliate` varchar(32),`creative` varchar(32),`family` varchar(32),`genus` varchar(32),`from_uid` varchar(32),`extra` varchar(32),`serverid` int default 0,KEY `idx` (`logdate`)) engine=myisam default charset=utf8;"

const sqlCreateDau = "create table if not exists `log_dau_%d%02d` (`logdate` datetime not null,`actorid` varchar(64),`source` varchar(32),`affiliate` varchar(32),`creative` varchar(32),`family` varchar(32),`genus` varchar(32),`ip` varchar(32),`from_uid` varchar(32),`extra` varchar(32),`serverid` int default 0,KEY `idx` (`logdate`)) engine=myisam default charset=utf8;"

const sqlCreateEconomy = "create table if not exists `log_economy_%d%02d` (`logdate` datetime not null,`actorid` bigint(10),`currency` varchar(32),`amount` varchar(32),`value` varchar(32),`kingdom` varchar(32),`phylum` varchar(32),`classfield` varchar(32),`family` varchar(32),`genus` varchar(32),`extra` varchar(128),`extra2` varchar(32),`serverid` int default 0,KEY `idx` (`logdate`)) engine=myisam default charset=utf8;"

const sqlCreateMilestone = "create table if not exists `log_milestone_%d%02d` (`logdate` datetime not null,`actorid` bigint(10),`milestone` varchar(32),`value` varchar(32),`extra` varchar(32),`serverid` int default 0,KEY `idx` (`logdate`)) engine=myisam default charset=utf8;"

const sqlCreatePayment = "create table if not exists `log_payment_%d%02d` (`logdate` datetime not null,`actorid` bigint(10),`ret` int default 0,`amount` int default 0,`value2` int default 0,`kingdom` varchar(32),`phylum` varchar(32),`transactionid` varchar(64),`extra` varchar(64),`msgtype` int default 0,`p1` int default 0,`p2` int default 0,`p3` int default 0,`p4` int default 0,`p5` int default 0,`serverid` int default 0,KEY `idx` (`logdate`,`transactionid`,`extra`)) engine=myisam default charset=utf8;"

const sqlCreateGif = "create table if not exists `log_gif_%d%02d` (`logdate` datetime not null,`actorid` bigint(10),`ret` int default 0,`amount` int default 0,`value2` int default 0,`kingdom` varchar(32),`phylum` varchar(32),`transactionid` varchar(64),`extra` varchar(64),`msgtype` int default 0,`p1` int default 0,`p2` int default 0,`p3` int default 0,`p4` int default 0,`p5` int default 0,`serverid` int default 0,KEY `idx` (`logdate`)) engine=myisam default charset=utf8;"

const sqlCreateOnline = "create table if not exists `log_online_%d%02d` (`logdate` datetime not null,`serverid` int default 0,`count` int default 0,KEY `idx` (`logdate`)) engine=myisam default charset=utf8;"

const sqlCreateSingleGif = "create table if not exists `log_gif` (`logdate` datetime NOT NULL,`actorid` bigint(10) default NULL,`ret` int(11) default '0',`amount` int(11) default '0',`value2` int(11) default '0',`kingdom` varchar(32) default NULL,`phylum` varchar(32) default NULL,`transactionid` varchar(64) default NULL,`extra` varchar(64) default NULL,`msgtype` int(11) default '0',`p1` int(11) default '0',`p2` int(11) default '0',`p3` int(11) default '0',`p4` int(11) default '0',`p5` int(11) default '0',`serverid` int(11) default '0',KEY `idx` (`transactionid`,`logdate`)) engine=myisam default charset=utf8;"

const sqlLogOnline = "insert into log_online_%d%02d(logdate, serverid, count) values(now(), %d, %d);"

const sqlLogCounter = "insert into log_counter_%d%02d(logdate,actorid,user_level,counter,value,extra,kingdom,phylum,classfield,family,genus,serverid) values(now(),%d,'%s','%s','%s','%s','%s','%s','%s','%s','%s',%d);"

const sqlLogInstall = "insert into log_install_%d%02d(logdate,actorid,source,affiliate,creative,family,genus,from_uid,extra,serverid) values(now(),'%s','%s','%s','%s','%s','%s','%s','%s',%d);"

const sqlLogDau = "insert into log_dau_%d%02d(logdate,actorid,source,affiliate,creative,family,genus,ip,from_uid,extra,serverid) values(now(),'%s','%s','%s','%s','%s','%s','%s','%s','%s',%d);"

const sqlLogEconomy = "insert into log_economy_%d%02d(logdate,actorid,currency,amount,value,kingdom,phylum,classfield,family,genus,extra,extra2,serverid) values(now(),%d,'%s','%s','%s','%s','%s','%s','%s','%s','%s','%s',%d);"

const sqlLogMilestone = "insert into log_milestone_%d%02d(logdate,actorid,milestone,value,extra,serverid) values(now(),%d,'%s','%s','%s',%d);"

// logChat 列序对照 LChat（actorid/type/内容与 payment 同构布局），首版占位，对拍校准。
const sqlLogChat = "insert into log_gif_%d%02d(logdate,actorid,ret,amount,value2,kingdom,phylum,transactionid,extra,msgtype,p1,p2,p3,p4,p5,serverid) values(now(),%d,%d,%d,%d,'%s','%s','%s','%s',%d,%d,%d,%d,%d,%d,%d);"
