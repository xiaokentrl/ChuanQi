// loggerserver 的 Go 版占位实现（阶段 1 正式开发）。
// 原版职责：监听 7001，接收 gameworld 的日志包，按月写入 MySQL log_s1。
// 建表 SQL 的权威来源：原版 data/loggerserver/data/main.lua。
package main

import "log/slog"

func main() {
	slog.Info("loggerserver stub", "phase", "M1 pending", "port", 7001)
}
