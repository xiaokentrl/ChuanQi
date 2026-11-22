// tcpcap：通用 TCP 透传抓包代理（协议逆向用，ADR-007）。
// 监听 -listen，转发到 -to，双向原始字节带方向标签落盘。
// 用法: tcpcap -listen :6001 -to 127.0.0.1:6002 -out gw_capture.bin
package main

import (
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"sync"
	"time"
)

func main() {
	listen := flag.String("listen", ":6001", "监听地址")
	to := flag.String("to", "127.0.0.1:6002", "转发目标")
	out := flag.String("out", "tcpcap.bin", "输出文件（双方字节按到达顺序）")
	flag.Parse()

	ln, err := net.Listen("tcp", *listen)
	if err != nil {
		slog.Error("监听失败", "addr", *listen, "err", err)
		os.Exit(1)
	}
	slog.Info("tcpcap 透传", "listen", *listen, "to", *to, "out", *out)

	raw, err := os.OpenFile(*out, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		slog.Error("打开输出失败", "err", err)
		os.Exit(1)
	}
	defer raw.Close()

	for {
		conn, err := ln.Accept()
		if err != nil {
			slog.Error("accept", "err", err)
			continue
		}
		up, err := net.Dial("tcp", *to)
		if err != nil {
			slog.Error("上游连接失败", "to", *to, "err", err)
			conn.Close()
			continue
		}
		slog.Info("链路建立", "client", conn.RemoteAddr().String())
		go pump(conn, up, raw, 'C') // 客户端侧 → 上游
		go pump(up, conn, raw, 'S') // 上游 → 客户端侧
	}
}

func pump(src, dst net.Conn, raw io.Writer, tag byte) {
	buf := make([]byte, 1<<16)
	var mu sync.Mutex
	total := 0
	for {
		src.SetReadDeadline(time.Now().Add(600 * time.Second))
		n, err := src.Read(buf)
		if n > 0 {
			mu.Lock()
			fmt.Fprintf(raw, "@%s [%c] len=%d\n", time.Now().Format("15:04:05.000"), tag, n)
			raw.Write(buf[:n])
			total += n
			mu.Unlock()
			if _, werr := dst.Write(buf[:n]); werr != nil {
				src.Close()
				return
			}
		}
		if err != nil {
			mu.Lock()
			fmt.Fprintf(raw, "@%s [%c] closed total=%d\n", time.Now().Format("15:04:05.000"), tag, total)
			mu.Unlock()
			slog.Info("pump 结束", "tag", string(tag), "total", total, "err", err)
			dst.Close()
			src.Close()
			return
		}
	}
}
