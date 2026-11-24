package storage

import (
	"fmt"
	"strings"
	"testing"
)

// SQL 模板的 golden 基准：表名 YYYYMM 格式与列序是对拍一致性的核心。
// 这些常量照抄原版 main.lua（logstore.go 注释），误改会导致入库列错位。

func TestSQLTemplates_YYYYMM(t *testing.T) {
	y, m := 2026, 11
	tests := []struct {
		name string
		tpl  string
		want string
	}{
		{"drop counter", sqlDropCounter, "drop table if exists `log_counter_202611`;"},
		{"drop gif", sqlDropGif, "drop table if exists `log_gif_202611`;"},
		{"create online", sqlCreateOnline, "create table if not exists `log_online_202611` (`logdate` datetime not null,`serverid` int default 0,`count` int default 0,KEY `idx` (`logdate`)) engine=myisam default charset=utf8;"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := fmt.Sprintf(tt.tpl, y, m)
			if got != tt.want {
				t.Errorf("SQL 不符\n got: %s\nwant: %s", got, tt.want)
			}
		})
	}
	// 月格式必须补零：2026-01 → 202601
	if got := fmt.Sprintf(sqlDropCounter, 2026, 1); !strings.Contains(got, "log_counter_202601") {
		t.Errorf("月份补零失败: %s", got)
	}
}

func TestInsertTemplates_Columns(t *testing.T) {
	y, m := 2026, 11
	tests := []struct {
		name      string
		sql       string
		wantCols  []string
		wantTable string
	}{
		{
			"online", fmt.Sprintf(sqlLogOnline, y, m, 1, 100),
			[]string{"logdate", "serverid", "count"}, "log_online_202611",
		},
		{
			"counter", fmt.Sprintf(sqlLogCounter, y, m, 1, "L10", "login", "1", "e", "k", "p", "cf", "f", "g", 1),
			[]string{"logdate", "actorid", "user_level", "counter", "value", "extra", "kingdom", "phylum", "classfield", "family", "genus", "serverid"},
			"log_counter_202611",
		},
		{
			"milestone", fmt.Sprintf(sqlLogMilestone, y, m, 1, "firstLogin", "1", "e", 1),
			[]string{"logdate", "actorid", "milestone", "value", "extra", "serverid"},
			"log_milestone_202611",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !strings.Contains(tt.sql, tt.wantTable) {
				t.Errorf("表名缺失 %s: %s", tt.wantTable, tt.sql)
			}
			open := strings.Index(tt.sql, "(")
			close := strings.Index(tt.sql, ") values")
			if open < 0 || close < 0 {
				t.Fatalf("列清单解析失败: %s", tt.sql)
			}
			cols := tt.sql[open+1 : close]
			for _, c := range tt.wantCols {
				if !strings.Contains(cols, c) {
					t.Errorf("列 %s 缺失: %s", c, cols)
				}
			}
		})
	}
}

// TestSQLTemplates_NoPlaceholderLeak 确保 fmt.Sprintf 后无残留 %d/%s 占位符。
func TestSQLTemplates_NoPlaceholderLeak(t *testing.T) {
	y, m := 2026, 11
	all := []string{
		fmt.Sprintf(sqlLogOnline, y, m, 1, 1),
		fmt.Sprintf(sqlLogCounter, y, m, 1, "a", "b", "c", "d", "e", "f", "g", "h", "i", 1),
		fmt.Sprintf(sqlLogInstall, y, m, "a", "b", "c", "d", "e", "f", "g", "h", 1),
		fmt.Sprintf(sqlLogDau, y, m, "a", "b", "c", "d", "e", "f", "g", "h", "i", 1),
		fmt.Sprintf(sqlLogEconomy, y, m, 1, "a", "b", "c", "d", "e", "f", "g", "h", "i", "j", 1),
		fmt.Sprintf(sqlLogMilestone, y, m, 1, "a", "b", "c", 1),
		fmt.Sprintf(sqlLogChat, y, m, 1, 1, 0, 0, "ip", "pf", "aid", "msg", 1, 0, 0, 0, 0, 0, 1),
	}
	for i, sql := range all {
		if strings.Contains(sql, "%d") || strings.Contains(sql, "%s") {
			t.Errorf("SQL[%d] 占位符残留: %s", i, sql)
		}
	}
}

// TestInsertTemplates_SQLInjectionRisk 记录当前 fmt.Sprintf 拼接的注入面。
// 这是已知债务（见 ROADMAP §8 安全五层）：网络解出的字符串未转义直接入 SQL。
// 该测试锁定当前行为，待改为参数化查询后应更新此测试。
func TestInsertTemplates_SQLInjectionRisk(t *testing.T) {
	y, m := 2026, 11
	malicious := "x'); drop table log_counter_202611; --"
	sql := fmt.Sprintf(sqlLogCounter, y, m, 1, malicious, "b", "c", "d", "e", "f", "g", "h", "i", 1)
	if !strings.Contains(sql, malicious) {
		t.Fatal("测试前提失效：恶意串未进入 SQL")
	}
	// 记录该风险：SQL 中出现注入的 drop 语句片段（当前实现未参数化）。
	if !strings.Contains(sql, "drop table") {
		t.Error("预期记录注入面：当前 fmt.Sprintf 拼接会原样带入 drop table 片段")
	}
}
