// gateway 的 Go 版实现（里程碑 M2）。
// 职责（对齐原版 GateWay.txt 语义）：
//   9001 接受客户端 WebSocket → 校验握手 → 透明转发到 gameworld 6001
//   双向原始字节录制到 reference/captures/（对拍 ADR-007）
//
// WS 层：手写 RFC6455 服务端子集（ADR-001，零依赖、字节全控）：
//   握手响应复刻原版模板（Sec-WebSocket-Accept/101）
//   客户端帧解码（含掩码还原）→ 裸 payload 转发后端
//   后端 TCP 字节 → 二进制帧（FIN+opcode 2，无掩码）回客户端
//
// 用法: gateway -mode serve [-listen :9001] [-backend 127.0.0.1:6001] [-recdir ../reference/captures]
package main

import (
	"bufio"
	"crypto/sha1"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"encoding/base64"
	"encoding/binary"
	"strings"
	"sync"
	"time"
)

const wsGUID = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"

func main() {
	mode := flag.String("mode", "serve", "运行模式: serve")
	listen := flag.String("listen", "127.0.0.1:9001", "客户端 WS 监听地址")
	backend := flag.String("backend", "127.0.0.1:6001", "gameworld 后端地址")
	recdir := flag.String("recdir", "", "字节录制目录（空=不录制）")
	flag.Parse()

	switch *mode {
	case "serve":
		serve(*listen, *backend, *recdir)
	default:
		slog.Error("未知模式", "mode", *mode)
		os.Exit(1)
	}
}

func serve(listen, backend, recdir string) {
	ln, err := net.Listen("tcp", listen)
	if err != nil {
		slog.Error("监听失败", "addr", listen, "err", err)
		os.Exit(1)
	}
	slog.Info("gateway(go) 监听", "addr", listen, "backend", backend, "recdir", recdir)

	var mu sync.Mutex
	var seq int
	for {
		conn, err := ln.Accept()
		if err != nil {
			slog.Error("accept 失败", "err", err)
			continue
		}
		mu.Lock()
		seq++
		id := seq
		mu.Unlock()
		go handleClient(conn, backend, recdir, id)
	}
}

// ---- WS 握手 ----

// readWSHandshake 读取 HTTP 升级请求头，返回 Sec-WebSocket-Key。
func readWSHandshake(r *bufio.Reader) (string, error) {
	var req strings.Builder
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return "", err
		}
		req.WriteString(line)
		if line == "\r\n" || line == "\n" {
			break
		}
		if len(req.String()) > 16384 {
			return "", fmt.Errorf("握手头过大")
		}
	}
	key := ""
	for _, line := range strings.Split(req.String(), "\n") {
		if strings.HasPrefix(strings.ToLower(line), "sec-websocket-key:") {
			key = strings.TrimSpace(line[len("Sec-WebSocket-Key:"):])
		}
	}
	if key == "" {
		return "", fmt.Errorf("缺少 Sec-WebSocket-Key")
	}
	slog.Info("WS 握手请求", "key", key, "reqHead", firstLine(req.String()))
	return key, nil
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return strings.TrimSpace(s[:i])
	}
	return strings.TrimSpace(s)
}

// wsAccept 计算 Sec-WebSocket-Accept（RFC6455 §4.2.2）。
func wsAccept(key string) string {
	h := sha1.Sum([]byte(key + wsGUID))
	return base64.StdEncoding.EncodeToString(h[:])
}

// ---- WS 帧读写（服务端子集） ----

// readWSFrame 读一帧客户端数据（客户端帧带掩码，此处还原 payload）。
// 返回 opcode 与还原后的 payload。
func readWSFrame(r *bufio.Reader) (byte, []byte, error) {
	var h [2]byte
	if _, err := io.ReadFull(r, h[:]); err != nil {
		return 0, nil, err
	}
	opcode := h[0] & 0x0F
	masked := h[1]&0x80 != 0
	l := uint64(h[1] & 0x7F)
	switch l {
	case 126:
		var ext [2]byte
		if _, err := io.ReadFull(r, ext[:]); err != nil {
			return 0, nil, err
		}
		l = uint64(binary.BigEndian.Uint16(ext[:]))
	case 127:
		var ext [8]byte
		if _, err := io.ReadFull(r, ext[:]); err != nil {
			return 0, nil, err
		}
		l = binary.BigEndian.Uint64(ext[:])
	}
	if l > 1<<24 { // 16MB 上限，防畸形帧
		return 0, nil, fmt.Errorf("WS 帧过大 %d", l)
	}
	var mask [4]byte
	if masked {
		if _, err := io.ReadFull(r, mask[:]); err != nil {
			return 0, nil, err
		}
	}
	payload := make([]byte, l)
	if _, err := io.ReadFull(r, payload); err != nil {
		return 0, nil, err
	}
	if masked {
		for i := range payload {
			payload[i] ^= mask[i%4]
		}
	}
	return opcode, payload, nil
}

// writeWSFrame 写一帧到客户端（服务端帧不掩码）。
func writeWSFrame(w io.Writer, opcode byte, payload []byte) error {
	var head [10]byte
	head[0] = 0x80 | opcode // FIN + opcode
	n := len(payload)
	idx := 2
	switch {
	case n < 126:
		head[1] = byte(n)
	case n <= 0xFFFF:
		head[1] = 126
		binary.BigEndian.PutUint16(head[2:4], uint16(n))
		idx = 4
	default:
		head[1] = 127
		binary.BigEndian.PutUint64(head[2:10], uint64(n))
		idx = 10
	}
	if _, err := w.Write(head[:idx]); err != nil {
		return err
	}
	_, err := w.Write(payload)
	return err
}

// ---- 会话中继 ----

func handleClient(client net.Conn, backend string, recdir string, id int) {
	defer client.Close()
	remote := client.RemoteAddr().String()
	start := time.Now()

	r := bufio.NewReaderSize(client, 1<<16)
	key, err := readWSHandshake(r)
	if err != nil {
		slog.Error("WS 握手失败", "remote", remote, "err", err)
		return
	}
	resp := fmt.Sprintf("HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: %s\r\n\r\n", wsAccept(key))
	if _, err := client.Write([]byte(resp)); err != nil {
		slog.Error("101 响应失败", "err", err)
		return
	}
	slog.Info("WS 握手通过", "remote", remote, "session", id)

	// 连接后端
	game, err := net.Dial("tcp", backend)
	if err != nil {
		slog.Error("后端连接失败", "backend", backend, "err", err)
		return
	}
	defer game.Close()

	// 录制器
	var recC2G, recG2C *os.File
	if recdir != "" {
		os.MkdirAll(recdir, 0755)
		recC2G, _ = os.OpenFile(fmt.Sprintf("%s/session%03d_c2g_%s.raw", recdir, id, start.Format("150405")), os.O_CREATE|os.O_WRONLY, 0644)
		recG2C, _ = os.OpenFile(fmt.Sprintf("%s/session%03d_g2c_%s.raw", recdir, id, start.Format("150405")), os.O_CREATE|os.O_WRONLY, 0644)
		if recC2G != nil {
			defer recC2G.Close()
		}
		if recG2C != nil {
			defer recG2C.Close()
		}
	}

	var wg sync.WaitGroup
	done := make(chan struct{})

	// 客户端 → 后端：WS 帧 → 裸 payload
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer close(done)
		for {
			opcode, payload, err := readWSFrame(r)
			if err != nil {
				slog.Info("客户端侧关闭", "session", id, "err", err)
				game.Close()
				return
			}
			switch opcode {
			case 0x8: // close
				slog.Info("客户端 close 帧", "session", id)
				game.Close()
				return
			case 0x9: // ping → pong（原版网关语义待对拍，先本地应答）
				if err := writeWSFrame(client, 0xA, payload); err != nil {
					return
				}
				continue
			case 0xA:
				continue // pong 忽略
			}
			if recC2G != nil {
				recC2G.Write(payload)
			}
			if _, err := game.Write(payload); err != nil {
				slog.Error("转发后端失败", "session", id, "err", err)
				client.Close()
				return
			}
		}
	}()

	// 后端 → 客户端：TCP 字节 → 二进制帧
	wg.Add(1)
	go func() {
		defer wg.Done()
		buf := make([]byte, 1<<16)
		for {
			n, err := game.Read(buf)
			if n > 0 {
				if recG2C != nil {
					recG2C.Write(buf[:n])
				}
				if err := writeWSFrame(client, 0x2, buf[:n]); err != nil {
					slog.Info("回写客户端失败", "session", id, "err", err)
					game.Close()
					return
				}
			}
			if err != nil {
				slog.Info("后端侧关闭", "session", id, "err", err)
				client.Close()
				return
			}
		}
	}()

	wg.Wait()
	slog.Info("会话结束", "session", id, "duration", time.Since(start))
}

var _ = io.Discard
