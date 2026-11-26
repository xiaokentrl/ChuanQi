// wsprobe：模拟 egret 客户端走 gateway 完整流程，逐步打印（M2 调试探针）。
// 流程：WS 握手 → auth 帧(0xCA0FFFFF) → 收 skey → 登录帧(skey+数据) → 观察响应。
// 支持 count/concurrency 批量并发，用于 B2 soak 与 M6 机器人 burst 准备。
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
	"sync"
	"time"
)

func writeClientFrame(w io.Writer, opcode byte, payload []byte, skey []byte) error {
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

func probe(addr, account string, observe time.Duration, quiet bool) error {
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return fmt.Errorf("连接失败: %w", err)
	}
	defer conn.Close()
	r := bufio.NewReader(conn)

	key := "dGhlIHNhbXBsZSBub25jZQ=="
	req := fmt.Sprintf("GET / HTTP/1.1\r\nHost: %s\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Key: %s\r\nSec-WebSocket-Version: 13\r\n\r\n", addr, key)
	if _, err := conn.Write([]byte(req)); err != nil {
		return fmt.Errorf("握手发送失败: %w", err)
	}
	var resp []byte
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return fmt.Errorf("读 101 失败: %w", err)
		}
		resp = append(resp, line...)
		if line == "\r\n" {
			break
		}
	}
	if !quiet {
		fmt.Printf("101 响应 %d B\n", len(resp))
	}

	auth := make([]byte, 4)
	binary.LittleEndian.PutUint32(auth, 0xCA0FFFFF)
	if err := writeClientFrame(conn, 0x2, auth, nil); err != nil {
		return fmt.Errorf("auth 发送失败: %w", err)
	}

	conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	op, skeyPayload, err := readServerFrame(r)
	if err != nil {
		return fmt.Errorf("等 skey 超时/失败: %w", err)
	}
	if op != 0x2 || len(skeyPayload) < 4 {
		return fmt.Errorf("skey 帧异常: opcode=%d len=%d", op, len(skeyPayload))
	}
	if !quiet {
		fmt.Printf("收到 skey 帧 opcode=%d skey=%08x\n", op, binary.LittleEndian.Uint32(skeyPayload))
	}

	accountBytes := []byte(account)
	login := make([]byte, 0, 51+len(accountBytes))
	login = append(login, 1, 0, 0, 0)
	login = append(login, 0xff, 0x01, 0x01, 0, 0, 0)
	login = binary.LittleEndian.AppendUint16(login, uint16(len(accountBytes)))
	login = append(login, accountBytes...)
	login = append(login, 0, 0x20, 0x00)
	login = append(login, []byte("e10adc3949ba59abbe56e057f20f883e")...)
	login = append(login, 0)
	if err := writeClientFrame(conn, 0x2, login, skeyPayload); err != nil {
		return fmt.Errorf("登录发送失败: %w", err)
	}
	if !quiet {
		fmt.Printf("登录帧已发（%d B）\n", len(login))
	}

	conn.SetReadDeadline(time.Now().Add(observe))
	frames := 0
	for {
		op, payload, err := readServerFrame(r)
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				return nil
			}
			return err
		}
		frames++
		if !quiet {
			fmt.Printf("响应帧 #%d opcode=%d len=%d: %s\n", frames, op, len(payload), hex.Dump(payload[:min(48, len(payload))]))
		}
	}
}

func main() {
	addr := flag.String("addr", "127.0.0.1:9002", "gateway 地址")
	account := flag.String("account", "帅哥", "账号")
	count := flag.Int("count", 1, "总探测次数")
	concurrency := flag.Int("concurrency", 1, "并发数")
	observe := flag.Duration("observe", 5*time.Second, "每次登录后观察时长")
	quiet := flag.Bool("quiet", false, "批量时减少输出")
	flag.Parse()

	if *count <= 0 || *concurrency <= 0 || *observe <= 0 {
		fmt.Fprintln(os.Stderr, "count/concurrency/observe 必须为正数")
		os.Exit(2)
	}

	start := time.Now()
	jobs := make(chan int, *count)
	for i := 0; i < *count; i++ {
		jobs <- i
	}
	close(jobs)

	var wg sync.WaitGroup
	var mu sync.Mutex
	ok, fail := 0, 0
	for w := 0; w < *concurrency; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range jobs {
				if err := probe(*addr, *account, *observe, *quiet); err != nil {
					mu.Lock()
					fail++
					mu.Unlock()
					if !*quiet {
						fmt.Println("probe 失败:", err)
					}
					continue
				}
				mu.Lock()
				ok++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	fmt.Printf("wsprobe summary: total=%d ok=%d fail=%d concurrency=%d elapsed=%s\n",
		*count, ok, fail, *concurrency, time.Since(start).Round(time.Millisecond))
	if fail > 0 {
		os.Exit(1)
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
