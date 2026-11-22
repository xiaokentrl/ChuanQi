// loggerserver capture 模式：占住 7001 端口，把原版 gameworld 发来的原始字节落盘。
// 用途：实证 loggerserver 协议帧结构（M1 规格输入，docs/loggerserver-spec.md）。
// 用法: loggerserver -capture -out captured.raw
package main

import (
	"flag"
	"fmt"
	"log/slog"
	"net"
	"os"
	"time"
)

func main() {
	capture := flag.Bool("capture", false, "以抓包模式运行（占用 7001，落盘原始字节）")
	out := flag.String("out", "captured.raw", "抓包输出文件（纯原始字节）")
	logf := flag.String("log", "capture.log", "chunk 元信息日志")
	flag.Parse()

	if !*capture {
		slog.Info("loggerserver stub", "phase", "M1 pending", "port", 7001)
		return
	}

	ln, err := net.Listen("tcp", "127.0.0.1:7001")
	if err != nil {
		slog.Error("listen failed", "err", err)
		os.Exit(1)
	}
	slog.Info("capture listening", "addr", ln.Addr().String())

	raw, err := os.OpenFile(*out, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		slog.Error("open out failed", "err", err)
		os.Exit(1)
	}
	defer raw.Close()
	meta, _ := os.OpenFile(*logf, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	defer meta.Close()

	for {
		conn, err := ln.Accept()
		if err != nil {
			slog.Error("accept failed", "err", err)
			continue
		}
		slog.Info("connection accepted", "remote", conn.RemoteAddr().String())
		go handle(conn, raw, meta)
	}
}

func handle(conn net.Conn, raw *os.File, meta *os.File) {
	defer conn.Close()
	buf := make([]byte, 1<<20)
	seq := 0
	var total int
	for {
		conn.SetReadDeadline(time.Now().Add(120 * time.Second))
		n, err := conn.Read(buf)
		if n > 0 {
			seq++
			total += n
			raw.Write(buf[:n])
			fmt.Fprintf(meta, "%s chunk=%d len=%d fileoff=%d\n",
				time.Now().Format("15:04:05.000"), seq, n, total-n)
		}
		if err != nil {
			meta.WriteString(fmt.Sprintf("closed after %d bytes\n", total))
			slog.Info("connection closed", "total", total)
			return
		}
	}
}
