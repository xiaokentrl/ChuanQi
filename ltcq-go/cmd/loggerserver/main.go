// loggerserver 的 Go 版实现（里程碑 M1）。
// 协议规格：docs/loggerserver-spec.md（DWARF 结构布局 + golden 抓包实证）。
//
// 模式：
//
//	serve   监听 7001，握手→帧循环→按 cmd 写 MySQL（默认）
//	capture 抓包模式：占 7001 落盘原始字节（对拍工具）
//	replay  离线解析 golden 文件（-dry 只统计；连库时真实写入）
package main

import (
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"time"

	_ "github.com/go-sql-driver/mysql"

	"github.com/xiaokentrl/ChuanQi/ltcq-go/internal/netframe"
	"github.com/xiaokentrl/ChuanQi/ltcq-go/internal/storage"
)

func main() {
	mode := flag.String("mode", "serve", "运行模式: serve | capture | replay")
	addr := flag.String("addr", "127.0.0.1:7001", "serve 模式监听地址")
	dsn := flag.String("dsn", "root:123456@tcp(127.0.0.1:3306)/?charset=utf8&parseTime=false", "MySQL DSN")
	dbname := flag.String("db", "log_s1", "日志库名（对拍时 Go 用 log_s1_go）")
	file := flag.String("file", "", "replay/capture 的输入/输出文件")
	dry := flag.Bool("dry", false, "replay 只解析统计，不写库")
	flag.Parse()

	switch *mode {
	case "serve":
		serve(*addr, *dsn, *dbname)
	case "capture":
		capture(*addr, *file)
	case "replay":
		replay(*file, *dsn, *dbname, *dry)
	default:
		slog.Error("未知模式", "mode", *mode)
		os.Exit(1)
	}
}

// ---- serve ----

func serve(addr, dsn, dbname string) {
	store, err := storage.Open(dsn, dbname)
	if err != nil {
		slog.Error("连接 MySQL 失败", "err", err)
		os.Exit(1)
	}
	defer store.Close()
	slog.Info("MySQL 已连接", "db", dbname)

	if err := store.EnsureTables(); err != nil {
		slog.Error("建表失败（复刻原版 100 月循环）", "err", err)
		os.Exit(1)
	}
	slog.Info("建表完成（100 个月 drop+create，对齐原版）")

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		slog.Error("监听失败", "addr", addr, "err", err)
		os.Exit(1)
	}
	slog.Info("loggerserver(go) 监听", "addr", addr)

	for {
		conn, err := ln.Accept()
		if err != nil {
			slog.Error("accept 失败", "err", err)
			continue
		}
		go handleServe(conn, store)
	}
}

func handleServe(conn net.Conn, store *storage.Store) {
	defer conn.Close()
	remote := conn.RemoteAddr().String()
	slog.Info("连接进入", "remote", remote)

	// 握手 80B
	hs := make([]byte, netframe.HandshakeSize)
	if _, err := io.ReadFull(conn, hs); err != nil {
		slog.Error("读握手失败", "remote", remote, "err", err)
		return
	}
	h, err := netframe.ParseHandshake(hs)
	if err != nil {
		slog.Error("握手校验失败", "remote", remote, "err", err)
		return
	}
	slog.Info("握手通过", "remote", remote, "serverId", h.ServerID, "name", h.Name)

	fr := netframe.NewFrameReader(conn)
	stats := map[uint16]int{}
	for {
		f, err := fr.ReadFrame()
		if err != nil {
			slog.Info("连接关闭", "remote", remote, "stats", fmt.Sprintf("%v", stats))
			return
		}
		stats[f.Cmd]++
		if err := dispatch(store, h.ServerID, f); err != nil {
			slog.Error("处理失败", "cmd", f.Cmd, "err", err)
		}
	}
}

// dispatch 等价原版 LoggerServer::PrecessData 的 switch（cmd 分布见反编译）。
func dispatch(store *storage.Store, serverID int32, f netframe.LogFrame) error {
	switch f.Cmd {
	case 0, 6:
		return nil // 原版 break 忽略
	case 1: // InsertCounter
		r, err := netframe.DecodeCounter(f.Body)
		if err != nil {
			return err
		}
		if r.Flag&1 == 0 {
			return nil
		}
		return store.InsertCounter(serverID, r)
	case 2: // InsertInstall
		r, err := netframe.DecodeInstall(f.Body)
		if err != nil {
			return err
		}
		if r.Flag&1 == 0 {
			return nil
		}
		return store.InsertInstall(serverID, r)
	case 3: // InsertDau
		r, err := netframe.DecodeDau(f.Body)
		if err != nil {
			return err
		}
		if r.Flag&1 == 0 {
			return nil
		}
		return store.InsertDau(serverID, r)
	case 4: // InsertEconomy
		r, err := netframe.DecodeEconomy(f.Body)
		if err != nil {
			return err
		}
		if r.Flag&1 == 0 {
			return nil
		}
		return store.InsertEconomy(serverID, r)
	case 5: // InsertMilestone
		r, err := netframe.DecodeMilestone(f.Body)
		if err != nil {
			return err
		}
		return store.InsertMilestone(serverID, r)
	case 9: // Lua precessLog 等价：在线人数
		r, err := netframe.DecodeOnline(f.Body)
		if err != nil {
			return err
		}
		return store.InsertOnline(serverID, r.Count)
	case 14: // InsertChat
		r, err := netframe.DecodeChat(f.Body)
		if err != nil {
			return err
		}
		return store.InsertChat(serverID, r)
	case 10, 11, 12, 13, 15:
		return nil // doSendDcApi*/ProcessTLog/WriteDKMLog：BI 通道，sendtobi=false，首版跳过
	default:
		return nil
	}
}

// ---- capture（对拍工具，保留） ----

func capture(addr, file string) {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		slog.Error("listen failed", "err", err)
		os.Exit(1)
	}
	slog.Info("capture listening", "addr", ln.Addr().String())

	raw, err := os.OpenFile(file, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		slog.Error("open out failed", "err", err)
		os.Exit(1)
	}
	defer raw.Close()

	for {
		conn, err := ln.Accept()
		if err != nil {
			slog.Error("accept failed", "err", err)
			continue
		}
		slog.Info("connection accepted", "remote", conn.RemoteAddr().String())
		go func(c net.Conn) {
			defer c.Close()
			buf := make([]byte, 1<<20)
			total := 0
			for {
				c.SetReadDeadline(time.Now().Add(120 * time.Second))
				n, err := c.Read(buf)
				if n > 0 {
					raw.Write(buf[:n])
					total += n
				}
				if err != nil {
					slog.Info("connection closed", "total", total)
					return
				}
			}
		}(conn)
	}
}

// ---- replay（离线解析 golden，M1 对拍入口） ----

func replay(file, dsn, dbname string, dry bool) {
	if file == "" {
		slog.Error("replay 需要 -file 指定 golden 文件")
		os.Exit(1)
	}
	f, err := os.Open(file)
	if err != nil {
		slog.Error("打开 golden 失败", "err", err)
		os.Exit(1)
	}
	defer f.Close()

	hs := make([]byte, netframe.HandshakeSize)
	if _, err := io.ReadFull(f, hs); err != nil {
		slog.Error("读握手失败", "err", err)
		os.Exit(1)
	}
	h, err := netframe.ParseHandshake(hs)
	if err != nil {
		slog.Error("握手解析失败", "err", err)
		os.Exit(1)
	}
	slog.Info("golden 握手", "serverId", h.ServerID, "name", h.Name)

	var store *storage.Store
	if !dry {
		store, err = storage.Open(dsn, dbname)
		if err != nil {
			slog.Error("连库失败", "err", err)
			os.Exit(1)
		}
		defer store.Close()
		if err := store.EnsureTables(); err != nil {
			slog.Error("建表失败", "err", err)
			os.Exit(1)
		}
	}

	fr := netframe.NewFrameReader(f)
	stats := map[uint16]int{}
	var errs int
	for {
		frm, err := fr.ReadFrame()
		if err == io.EOF {
			break
		}
		if err != nil {
			slog.Error("解析中断", "err", err)
			errs++
			break
		}
		stats[frm.Cmd]++
		if store != nil {
			if err := dispatch(store, h.ServerID, frm); err != nil {
				errs++
				if errs <= 5 {
					slog.Error("写入失败", "cmd", frm.Cmd, "err", err)
				}
			}
		}
	}
	total := 0
	for _, c := range stats {
		total += c
	}
	slog.Info("replay 完成", "总帧数", total, "cmd分布", fmt.Sprintf("%v", stats), "错误", errs)
	if dry {
		for _, cmd := range []uint16{1, 2, 3, 4, 5, 9, 14, 15} {
			if n := stats[cmd]; n > 0 {
				fmt.Printf("cmd=%-3d %d 帧\n", cmd, n)
			}
		}
	}
}

// 保留十六进制辅助（诊断用）
var _ = hex.Dump
