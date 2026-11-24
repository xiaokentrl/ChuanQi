// gateway 的 Go 版实现（里程碑 M2）。
// 协议权威来源：Client::OnRecv 反编译（gateway_ws_decomp.c）+ golden 双向抓包。
// 客户端鉴权：首帧 4B==0xCA0FFFFF → 随机 skey 回发 → 每帧首 4B==skey。
// 转发格式：cmd=5 payload = NetId(8B) + 00 00 + cd cd + 数据（去 skey）。
package main

import (
	"bufio"
	"encoding/binary"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"sync"
	"time"
)

var (
	gateUpFile   *os.File
	gateDownFile *os.File
)

// sendToGate 发送 gate 帧并录制上行（诊断）。
func sendToGate(game net.Conn, frame []byte) {
	if gateUpFile != nil {
		gateUpFile.Write(frame)
	}
	if _, err := game.Write(frame); err != nil {
		slog.Error("gate 上行失败", "err", err)
	}
}

// readRawGateFrame 读一帧原始字节并解析，返回 (原始字节, cmd, body, err)。
// 原始字节用于 g2c 录制（ADR-007 对拍基准），body 为去掉 14B 头与 2B cmd 的负载。
func readRawGateFrame(r *bufio.Reader) (raw []byte, cmd uint16, body []byte, err error) {
	head := make([]byte, 14)
	if _, err = io.ReadFull(r, head); err != nil {
		return
	}
	c, bodyLen, ok := parseGateFrame(head)
	if !ok {
		err = fmt.Errorf("gate 帧魔数不符: %s", hex.Dump(head[:4]))
		return
	}
	// parseGateFrame 返回的 bodyLen 已是 payloadLen-2（即 cmd 之后的 body 实际长度）
	raw = make([]byte, 14+int(bodyLen))
	copy(raw, head)
	body = make([]byte, bodyLen)
	if _, err = io.ReadFull(r, body); err != nil {
		return
	}
	copy(raw[14:], body)
	return raw, c, body, nil
}

type sessionMgr struct {
	mu     sync.Mutex
	nextID uint16
	byID   map[uint16]net.Conn
	skeys  map[uint16]uint32
}

func newSessionMgr() *sessionMgr {
	return &sessionMgr{nextID: 552, byID: map[uint16]net.Conn{}, skeys: map[uint16]uint32{}}
}

func (m *sessionMgr) alloc() uint16 {
	m.mu.Lock()
	defer m.mu.Unlock()
	id := m.nextID
	m.nextID++
	return id
}

func (m *sessionMgr) bind(id uint16, skey uint32, c net.Conn) {
	m.mu.Lock()
	m.byID[id] = c
	m.skeys[id] = skey
	m.mu.Unlock()
}

func (m *sessionMgr) get(id uint16) (net.Conn, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	c, ok := m.byID[id]
	return c, ok
}

// getSkey 取会话 skey（回向登录帧需回显，对齐 C++ gateway 行为）。
func (m *sessionMgr) getSkey(id uint16) (uint32, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.skeys[id]
	return s, ok
}

func (m *sessionMgr) remove(id uint16) {
	m.mu.Lock()
	delete(m.byID, id)
	delete(m.skeys, id)
	m.mu.Unlock()
}

// knownIDs 当前活跃 sessionId（诊断用）。
func (m *sessionMgr) knownIDs() []uint16 {
	m.mu.Lock()
	defer m.mu.Unlock()
	ids := make([]uint16, 0, len(m.byID))
	for k := range m.byID {
		ids = append(ids, k)
	}
	return ids
}

// min 返回较小值。
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func main() {
	mode := flag.String("mode", "serve", "运行模式: serve | selftest")
	listen := flag.String("listen", "0.0.0.0:9002", "客户端 WS 监听地址（s1=9002，对齐 GateWay.txt）")
	backend := flag.String("backend", "127.0.0.1:6001", "gameworld 地址（s1=6001，对齐 GameWorld.txt）")
	gatename := flag.String("gatename", "gateway_s1", "注册服务名")
	recdir := flag.String("recdir", "", "字节录制目录")
	flag.Parse()

	if *mode == "selftest" {
		selftest()
		return
	}
	if *mode == "serve" {
		serve(*listen, *backend, *gatename, *recdir)
		return
	}
	slog.Error("未知模式", "mode", *mode)
	os.Exit(1)
}

func serve(listen, backend, gatename, recdir string) {
	game, err := net.Dial("tcp", backend)
	if err != nil {
		slog.Error("gameworld 连接失败", "backend", backend, "err", err)
		os.Exit(1)
	}
	slog.Info("gameworld 已连接", "addr", backend)
	if recdir != "" {
		os.MkdirAll(recdir, 0755)
		gateUpFile, _ = os.OpenFile(recdir+"/gate_upstream.raw", os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
		gateDownFile, _ = os.OpenFile(recdir+"/gate_downstream.raw", os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	}
	sendToGate(game, buildGateRegistration(gatename))
	slog.Info("gate 注册已发送", "name", gatename)

	go func() { // 心跳
		t := time.NewTicker(10 * time.Second)
		defer t.Stop()
		for range t.C {
			sendToGate(game, buildHeartbeat())
		}
	}()

	mgr := newSessionMgr()
	go func() { // 回向帧读取 → 路由
		r := bufio.NewReaderSize(game, 1<<20)
		for {
			raw, cmd, body, err := readRawGateFrame(r)
			if err != nil {
				slog.Error("gate 回向结束（整体退出，守护重启）", "err", err)
				os.Exit(1)
			}
			if gateDownFile != nil {
				gateDownFile.Write(raw) // g2c 录制：完整下行帧（含14B头）
			}
			switch cmd {
			case gwCmdUserData:
				if len(body) < userDataNetIDLen+1 {
					slog.Error("回向 cmd=5 body 过短", "len", len(body), "hex", hex.Dump(body))
					continue
				}
				sid := uint16(body[0]) | uint16(body[1])<<8
				data := body[userDataNetIDLen:]
				c, ok := mgr.get(sid)
				if !ok {
					slog.Error("回向 sid 未匹配", "sid", sid, "known", fmt.Sprintf("%v", mgr.knownIDs()), "hex", hex.Dump(body[:min(32, len(body))]))
					continue
				}
				// 登录结果回包（[ff][01][result]）需回显 skey 4B，对齐 C++ 现网
				// 实测 0a bb 09 1c ff 01 00（skey = 登录帧前缀 1c09bb0a 的小端字节）。
				// 其他业务回包保持原样（golden 记录回向零样本，避免破坏已验证协议）。
				if len(data) >= 3 && data[0] == 0xff && data[1] == 0x01 {
					if sk, ok := mgr.getSkey(sid); ok {
						skBytes := make([]byte, 4)
						binary.LittleEndian.PutUint32(skBytes, sk)
						data = append(skBytes, data...)
					}
				}
				if err := writeWSFrame(c, 0x2, data); err != nil {
					slog.Error("WS 下发失败", "sid", sid, "err", err)
				}
			case gwCmdBroadcast:
				if len(body) > userDataNetIDLen {
					data := body[userDataNetIDLen:]
					mgr.mu.Lock()
					for _, c := range mgr.byID {
						if c != nil {
							writeWSFrame(c, 0x2, data)
						}
					}
					mgr.mu.Unlock()
				}
			default:
				slog.Info("gate 回向帧", "cmd", cmd, "len", len(body), "hex", hex.Dump(body[:min(32, len(body))]))
			}
		}
	}()

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
		go handleClient(conn, game, mgr, recdir, id)
	}
}

// handleClient 客户端会话：鉴权状态机 + 转发（对齐 Client::OnRecv）。
func handleClient(client net.Conn, game net.Conn, mgr *sessionMgr, recdir string, id int) {
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
		return
	}

	var skey uint32
	authed := false
	sid := mgr.alloc()
	var recC2G *os.File
	if recdir != "" {
		recC2G, _ = os.OpenFile(fmt.Sprintf("%s/session%03d_c2g_%s.raw", recdir, id, start.Format("150405")), os.O_CREATE|os.O_WRONLY, 0644)
		if recC2G != nil {
			defer recC2G.Close()
		}
		// g2c 改为回向 pump 统一写 gate_downstream.raw（原每会话 g2c 文件只 open 不写）。
	}

	for {
		opcode, payload, err := readWSFrame(r)
		if err != nil {
			slog.Info("客户端断开", "sid", sid, "err", err)
			if authed {
				sendToGate(game, buildUserClose(sid))
				mgr.remove(sid)
			}
			return
		}
		switch opcode {
		case 0x8:
			slog.Info("客户端 close", "sid", sid)
			if authed {
				sendToGate(game, buildUserClose(sid))
				mgr.remove(sid)
			}
			return
		case 0x9:
			writeWSFrame(client, 0xA, payload)
			continue
		case 0xA:
			continue
		}
		if !authed {
			if !clientHandshakeCode(payload) {
				slog.Error("客户端首帧校验失败", "payload", hex.Dump(payload))
				return
			}
			skey = genSkey()
			sk := make([]byte, 4)
			binary.LittleEndian.PutUint32(sk, skey)
			if err := writeWSFrame(client, 0x2, sk); err != nil {
				return
			}
			authed = true
			ip, _, _ := net.SplitHostPort(remote)
			sendToGate(game, buildUserOpen(sid, ip))
			mgr.bind(sid, skey, client)
			slog.Info("客户端鉴权通过", "sid", sid, "skey", fmt.Sprintf("%08x", skey))
			continue
		}
		if len(payload) < 4 || binary.LittleEndian.Uint32(payload[0:4]) != skey {
			slog.Error("skey 校验失败，断开", "sid", sid)
			return
		}
		if recC2G != nil {
			recC2G.Write(payload[4:])
		}
		sendToGate(game, buildUserData(sid, payload[4:]))
	}
}

// selftest 打印协议构造 hex，与 golden 逐字节对照。
func selftest() {
	fmt.Println("== 注册 80B ==")
	fmt.Println(hex.Dump(buildGateRegistration("gateway_s1")))
	fmt.Println("== 心跳 ==")
	fmt.Println(hex.Dump(buildHeartbeat()))
	fmt.Println("== cmd=1 会话建立 (golden payloadLen=0x2E) ==")
	fmt.Println(hex.Dump(buildUserOpen(552, "127.0.0.1")))
	data := make([]byte, 54)
	copy(data, []byte{0, 0, 0, 0, 0xff, 1, 1, 0, 0, 0, 6, 0})
	copy(data[12:], "\xe5\xb8\x85\xe5\x93\xa5\x00\x20\x00")
	fmt.Println("== cmd=5 数据转发 (golden payloadLen=0x44) ==")
	fmt.Println(hex.Dump(buildUserData(552, data)))
}
