// wsprobe：模拟 egret 客户端走 gateway 完整流程，逐步打印（M2 调试探针）。
// 流程：WS 握手 → auth 帧(0xCA0FFFFF) → 收 skey → 登录帧(skey+数据) → 观察 5 秒响应。
package main

import (
	"bufio"
	"encoding/binary"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"math/rand"
	"net"
	"os"
	"time"
)

func writeClientFrame(w io.Writer, opcode byte, payload []byte, skey []byte) error {
	// 客户端帧：payload = skey(4B) + 数据，必须掩码
	full := make([]byte, 0, 4+len(payload))
	if skey != nil {
		full = append(full, skey...)
	}
	full = append(full, payload...)

	var head [14]byte
	head[0] = 0x80 | opcode
	n := len(full)
	idx := 2
	maskBit := byte(0x80)
	switch {
	case n < 126:
		head[1] = maskBit | byte(n)
	case n <= 0xFFFF:
		head[1] = maskBit | 126
		binary.BigEndian.PutUint16(head[2:4], uint16(n))
		idx = 6
	default:
		head[1] = maskBit | 127
		binary.BigEndian.PutUint64(head[2:10], uint64(n))
		idx = 10
	}
	var mask [4]byte
	for i := range mask {
		mask[i] = byte(rand.Intn(256))
	}
	copy(head[idx:idx+4], mask[:])
	idx += 4
	for i := range full {
		full[i] ^= mask[i%4]
	}
	out := append(head[:idx], full...)
	_, err := w.Write(out)
	return err
}

func readServerFrame(r *bufio.Reader) (byte, []byte, error) {
	var h [2]byte
	if _, err := io.ReadFull(r, h[:]); err != nil {
		return 0, nil, err
	}
	opcode := h[0] & 0x0F
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
	payload := make([]byte, l)
	if _, err := io.ReadFull(r, payload); err != nil {
		return 0, nil, err
	}
	return opcode, payload, nil
}

func main() {
	addr := flag.String("addr", "127.0.0.1:9002", "gateway 地址（s1=9002）")
	account := flag.String("account", "帅哥", "账号")
	flag.Parse()

	conn, err := net.Dial("tcp", *addr)
	if err != nil {
		fmt.Println("连接失败:", err)
		os.Exit(1)
	}
	defer conn.Close()
	r := bufio.NewReader(conn)

	// 1. WS 握手
	key := "dGhlIHNhbXBsZSBub25jZQ==" // 固定 test key（RFC 示例）
	req := fmt.Sprintf("GET / HTTP/1.1\r\nHost: %s\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Key: %s\r\nSec-WebSocket-Version: 13\r\n\r\n", *addr, key)
	if _, err := conn.Write([]byte(req)); err != nil {
		fmt.Println("握手发送失败:", err)
		return
	}
	// 读 101 响应
	var resp []byte
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			fmt.Println("读 101 失败:", err)
			return
		}
		resp = append(resp, line...)
		if line == "\r\n" {
			break
		}
	}
	fmt.Printf("101 响应 %d B: %q\n", len(resp), string(resp[:60])+"...")

	// 2. auth 帧（0xCA0FFFFF，无 skey 前缀）
	auth := make([]byte, 4)
	binary.LittleEndian.PutUint32(auth, 0xCA0FFFFF)
	if err := writeClientFrame(conn, 0x2, auth, nil); err != nil {
		fmt.Println("auth 发送失败:", err)
		return
	}
	fmt.Println("auth 帧已发: ffffffff...0xCA0FFFFF")

	// 3. 读 skey 回发
	conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	op, payload, err := readServerFrame(r)
	if err != nil {
		fmt.Println("等 skey 超时/失败:", err)
		return
	}
	fmt.Printf("收到帧 opcode=%d payload=%s (skey=%08x)\n", op, hex.Dump(payload), binary.LittleEndian.Uint32(payload))

	// 4. 登录帧（skey 前缀 + 模拟登录数据：对齐 golden cmd=5 数据部分）
	login := make([]byte, 0, 57)
	login = append(login, 1, 0, 0, 0)                                    // 4B golden: 01 00 00 00（勿用 00 00 00 00，gameworld 会拒登）
	login = append(login, 0xff, 0x01, 0x01, 0, 0, 0)                     // 6B
	login = append(login, 0x06, 0x00)                                    // 2B
	login = append(login, []byte(*account)...)                           // 账号 UTF-8
	login = append(login, 0, 0x20, 0x00)                                 // 3B
	login = append(login, []byte("e10adc3949ba59abbe56e057f20f883e")...) // md5 32B
	login = append(login, 0)                                             // 1B
	fmt.Printf("登录帧数据 %d B: %s\n", len(login), hex.Dump(login))
	if err := writeClientFrame(conn, 0x2, login, payload); err != nil {
		fmt.Println("登录发送失败:", err)
		return
	}
	fmt.Println("登录帧已发（skey 前缀）")

	// 5. 观察 5 秒响应
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	for i := 0; i < 20; i++ {
		op, payload, err := readServerFrame(r)
		if err != nil {
			fmt.Println("观察结束:", err)
			return
		}
		fmt.Printf("响应帧 #%d opcode=%d len=%d: %s\n", i+1, op, len(payload), hex.Dump(payload[:min(48, len(payload))]))
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
