package main

import (
	"database/sql"
	"flag"
	"fmt"
	"os"
	"regexp"
	"strings"

	_ "github.com/go-sql-driver/mysql"
)

func main() {
	mode := flag.String("mode", "setup", "setup|status|drop")
	database := flag.String("database", "actor_s1_m5d_20261125", "isolated database name")
	account := flag.String("account", "haohaiyao", "account to clone")
	actorID := flag.Int("actor", 65537, "actor to clone")
	flag.Parse()
	if !regexp.MustCompile(`^[a-zA-Z0-9_]+$`).MatchString(*database) || !strings.HasPrefix(*database, "actor_s1_m5d_") {
		panic("unsafe test database name")
	}
	dsn := os.Getenv("M5D_DSN")
	if dsn == "" {
		panic("M5D_DSN is required")
	}
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		panic(err)
	}
	defer db.Close()
	if err := db.Ping(); err != nil {
		panic(err)
	}
	switch *mode {
	case "setup":
		setup(db, *database, *account, *actorID)
	case "status":
		status(db, *database, *actorID)
	case "drop":
		drop(db, *database)
	default:
		panic("unknown mode")
	}
}

func setup(db *sql.DB, database, account string, actorID int) {
	var exists int
	if err := db.QueryRow("SELECT COUNT(*) FROM information_schema.SCHEMATA WHERE SCHEMA_NAME=?", database).Scan(&exists); err != nil {
		panic(err)
	}
	if exists != 0 {
		panic("isolated database already exists")
	}
	var source string
	if err := db.QueryRow("SELECT DATABASE()").Scan(&source); err != nil {
		panic(err)
	}
	if _, err := db.Exec("CREATE DATABASE `" + database + "` CHARACTER SET utf8mb4"); err != nil {
		panic(err)
	}
	if _, err := db.Exec("USE `" + database + "`"); err != nil {
		panic(err)
	}
	if _, err := db.Exec("SET FOREIGN_KEY_CHECKS=0"); err != nil {
		panic(err)
	}
	for _, table := range []string{"globaluser", "actors", "roles", "actorvariable", "actorbinarydata"} {
		var createSQL string
		if err := db.QueryRow("SHOW CREATE TABLE `" + source + "`.`" + table + "`").Scan(&table, &createSQL); err != nil {
			panic(err)
		}
		createSQL = strings.Replace(createSQL, "CREATE TABLE `"+table+"`", "CREATE TABLE `"+database+"`.`"+table+"`", 1)
		if _, err := db.Exec(createSQL); err != nil {
			panic(err)
		}
	}
	queries := []struct {
		query string
		args  []any
	}{
		{"INSERT INTO `" + database + "`.globaluser SELECT * FROM `" + source + "`.globaluser WHERE account=?", []any{account}},
		{"INSERT INTO `" + database + "`.actors SELECT * FROM `" + source + "`.actors WHERE actorid=? AND accountname=?", []any{actorID, account}},
		{"INSERT INTO `" + database + "`.roles SELECT * FROM `" + source + "`.roles WHERE actorid=?", []any{actorID}},
		{"INSERT INTO `" + database + "`.actorvariable SELECT * FROM `" + source + "`.actorvariable WHERE actorid=?", []any{actorID}},
		{"INSERT INTO `" + database + "`.actorbinarydata SELECT * FROM `" + source + "`.actorbinarydata WHERE actorid=?", []any{actorID}},
	}
	for _, item := range queries {
		res, err := db.Exec(item.query, item.args...)
		if err != nil {
			panic(err)
		}
		if table := strings.Contains(item.query, ".globaluser"); table {
			rows, _ := res.RowsAffected()
			if rows != 1 {
				panic("globaluser clone row count is not 1")
			}
		}
		if strings.Contains(item.query, ".actors") {
			rows, _ := res.RowsAffected()
			if rows != 1 {
				panic("actors clone row count is not 1")
			}
		}
	}
	if _, err := db.Exec("UPDATE `"+database+"`.actors SET level=51, exp=0, gold=0, sceneid=2 WHERE actorid=?", actorID); err != nil {
		panic(err)
	}
	status(db, database, actorID)
}

func status(db *sql.DB, database string, actorID int) {
	var level int
	var exp, gold int64
	var sceneID int
	if err := db.QueryRow("SELECT level,exp,gold,sceneid FROM `"+database+"`.actors WHERE actorid=?", actorID).Scan(&level, &exp, &gold, &sceneID); err != nil {
		panic(err)
	}
	fmt.Printf("database=%s actor=%d level=%d exp=%d gold=%d scene=%d\n", database, actorID, level, exp, gold, sceneID)
}

func drop(db *sql.DB, database string) {
	if _, err := db.Exec("DROP DATABASE `" + database + "`"); err != nil {
		panic(err)
	}
	fmt.Printf("dropped %s\n", database)
}
