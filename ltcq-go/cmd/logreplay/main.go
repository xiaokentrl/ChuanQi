// logreplay：把 golden 字节流原样发送到 7001 服务端（原版 loggerserver 或 Go 版）。
// 用途：差分对拍（ADR-007）——同一份字节分别喂两个实现，比对入库结果。
// 用法: logreplay -addr 127.0.0.1:7001 -file golden.raw [-chunk 4096]
package main

import (
	"flag"
	"io"
	"log/slog"
	"net"
	"os"
	"time"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:7001", "目标服务端")
	file := flag.String("file", "", "golden 字节文件")
	chunk := flag.Int("chunk", 4096, "发送块大小")
	flag.Parse()

	if *file == "" {
		slog.Error("需要 -file")
		os.Exit(1)
	}
	f, err := os.Open(*file)
	if err != nil {
		slog.Error("打开文件失败", "err", err)
		os.Exit(1)
	}
	defer f.Close()

	conn, err := net.Dial("tcp", *addr)
	if err != nil {
		slog.Error("连接失败", "err", err)
		os.Exit(1)
	}
	defer conn.Close()

	buf := make([]byte, *chunk)
	total := 0
	for {
		n, err := f.Read(buf)
		if n > 0 {
			if _, werr := conn.Write(buf[:n]); werr != nil {
				slog.Error("发送失败", "err", werr, "total", total)
				os.Exit(1)
			}
			total += n
			time.Sleep(2 * time.Millisecond) // 原版 C++ 处理节奏，避免淹没
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			slog.Error("读文件失败", "err", err)
			os.Exit(1)
		}
	}
	slog.Info("发送完成", "bytes", total)
}
