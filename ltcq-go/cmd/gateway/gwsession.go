// gateway↔gameworld 网关会话协议 + 客户端鉴权协议。
// 权威来源：Client::OnRecv 反编译（gateway_ws_decomp.c）+ gw6001 golden 实测。
package main

import (
	"bufio"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"io"
	"math/rand"
	"strings"
)

const wsGUID = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"

const (
	gwMagicFrame  = 0xCCEE
	gwSeparator   = 0xCDCDCDCD
	handshakeCode = 0xCA0FFFFF // 客户端首帧校验值（-0x35f00001）
)

// gate 会话命令字（实证）
const (
	gwCmdHeartbeat = 0 // gateway→game 每 10s
	gwCmdUserOpen  = 1 // 客户端会话建立
	gwCmdUserClose = 2 // 客户端断开
	gwCmdUserData  = 5 // 数据转发（双向）
	gwCmdBroadcast = 6 // 广播下行（S方向观测131帧）
)

// NetId 构造：sessionId 低 2B LE + 6B 零（抓包观测 28 02 00 00 00 00 00 00）。
func buildNetID(sessionID uint16) []byte {
	b := make([]byte, 8)
	b[0] = byte(sessionID)
	b[1] = byte(sessionID >> 8)
	return b
}

// buildGateRegistration 80B gate 注册包（golden 逐字节复刻）。
func buildGateRegistration(name string) []byte {
	b := make([]byte, 80)
	binary.LittleEndian.PutUint32(b[0:4], 0x12AA66CA)
	binary.LittleEndian.PutUint32(b[4:8], 1)
	copy(b[16:80], name)
	return b
}

// buildGateFrame [CCEE][payloadLen][CDCDCDCD][2B cmd][payload]
func buildGateFrame(cmd uint16, payload []byte) []byte {
	plen := 2 + len(payload)
	out := make([]byte, 14+len(payload))
	binary.LittleEndian.PutUint32(out[0:4], gwMagicFrame)
	binary.LittleEndian.PutUint32(out[4:8], uint32(plen))
	binary.LittleEndian.PutUint32(out[8:12], gwSeparator)
	binary.LittleEndian.PutUint16(out[12:14], cmd)
	copy(out[14:], payload)
	return out
}

func buildHeartbeat() []byte { return buildGateFrame(gwCmdHeartbeat, []byte{0, 0}) }

// buildUserOpen 会话建立（cmd=1）。golden payload 46B：
// NetId(8B) + 00 00 + cd cd + 客户端IP\0 + 78 00 + 零填充。
func buildUserOpen(sessionID uint16, clientIP string) []byte {
	p := make([]byte, 0, 46)
	p = append(p, buildNetID(sessionID)...)
	p = append(p, 0, 0, 0xcd, 0xcd)
	p = append(p, []byte(clientIP)...)
	p = append(p, 0)
	p = append(p, 0x78, 0)
	for len(p) < 44 {
		p = append(p, 0)
	}
	return buildGateFrame(gwCmdUserOpen, p[:44])
}

// buildUserClose 会话断开（cmd=2）。golden payload 12B：NetId(8B) + 00 00 + cd cd。
func buildUserClose(sessionID uint16) []byte {
	p := make([]byte, 0, 12)
	p = append(p, buildNetID(sessionID)...)
	p = append(p, 0, 0, 0xcd, 0xcd)
	return buildGateFrame(gwCmdUserClose, p)
}

// buildUserData 数据转发（cmd=5）。golden payload 68B =
// NetId(8B) + 00 00 + cd cd + 客户端数据(54B) —— 对齐 Client::OnRecv：
// allocProtoPacket(cmd=5) → operator<<<NetId(net_id_) → writeBuf(buf+4, size-4)，前置另有 4B。
func buildUserData(sessionID uint16, data []byte) []byte {
	p := make([]byte, 0, 12+len(data))
	p = append(p, buildNetID(sessionID)...)
	p = append(p, 0, 0, 0xcd, 0xcd)
	p = append(p, data...)
	return buildGateFrame(gwCmdUserData, p)
}

// parseGateFrame 解析 gate 帧头（14B）：返回 cmd 与 body（payload 去 2B cmd）。
func parseGateFrame(head14 []byte) (cmd uint16, bodyLen uint32, ok bool) {
	if binary.LittleEndian.Uint32(head14[0:4]) != gwMagicFrame {
		return 0, 0, false
	}
	bodyLen = binary.LittleEndian.Uint32(head14[4:8])
	if bodyLen < 2 {
		return 0, 0, false
	}
	cmd = binary.LittleEndian.Uint16(head14[12:14])
	return cmd, bodyLen - 2, true
}

// clientHandshakeCode 校验客户端首帧 4B。
func clientHandshakeCode(b []byte) bool {
	return len(b) >= 4 && binary.LittleEndian.Uint32(b[0:4]) == handshakeCode
}

// genSkey 生成会话鉴权密钥（对齐 wrand(0x7fffffff)）。
func genSkey() uint32 { return uint32(rand.Int31n(0x7fffffff)) }

// 回向 cmd=5 body = NetId(8B) + 客户端协议数据 —— 路由按 NetId 低 2B。
const userDataNetIDLen = 8

// ---- WS 基础（RFC6455 服务端子集，手写零依赖，ADR-001） ----

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
	return key, nil
}

func wsAccept(key string) string {
	h := sha1.Sum([]byte(key + wsGUID))
	return base64.StdEncoding.EncodeToString(h[:])
}

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
	if l > 1<<24 {
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

func writeWSFrame(w io.Writer, opcode byte, payload []byte) error {
	var head [10]byte
	head[0] = 0x80 | opcode
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

func readWSHandshakeLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return strings.TrimSpace(s[:i])
	}
	return strings.TrimSpace(s)
}
