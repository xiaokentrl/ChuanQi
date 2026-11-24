package main

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func goldenPathGw(name string) string {
	_, thisFile, _, _ := runtime.Caller(0)
	// thisFile = <root>/cmd/gateway/gwsession_test.go
	root := filepath.Dir(filepath.Dir(filepath.Dir(thisFile)))
	return filepath.Join(root, "reference", "captures", name)
}

func readGoldenGw(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(goldenPathGw(name))
	if err != nil {
		t.Fatalf("读取 golden %s 失败: %v", name, err)
	}
	return b
}

func TestBuildUserOpen_Golden(t *testing.T) {
	// gate_upstream.raw 的 cmd=1 帧 80B 处：真实注册包后的首帧。
	b := readGoldenGw(t, "gate_upstream.raw")
	// 握手 80B + 帧头 14B = 94；cmd=1 payload 从 94 起共 44B。
	wantPayload := b[94 : 94+44]
	frame := buildUserOpen(0x228, "127.0.0.1")
	// buildUserOpen 返回完整帧（14B 头 + 44B payload），取 payload 部分比对。
	gotPayload := frame[14:]
	if !bytes.Equal(gotPayload, wantPayload) {
		t.Errorf("buildUserOpen payload 与 golden 不符\n got: %x\nwant: %x", gotPayload, wantPayload)
	}
	// 同时验证帧头与 golden 一致。
	if !bytes.Equal(frame[:14], b[80:94]) {
		t.Errorf("buildUserOpen 帧头与 golden 不符\n got: %x\nwant: %x", frame[:14], b[80:94])
	}
}

func TestBuildGateRegistration_Golden(t *testing.T) {
	b := readGoldenGw(t, "gate_upstream.raw")
	want := b[:80]
	got := buildGateRegistration("gateway_s1")
	if !bytes.Equal(got, want) {
		t.Errorf("buildGateRegistration 与 golden 不符\n got: %x\nwant: %x", got, want)
	}
}

func TestBuildNetID(t *testing.T) {
	got := buildNetID(0x228)
	want := []byte{0x28, 0x02, 0, 0, 0, 0, 0, 0}
	if !bytes.Equal(got, want) {
		t.Errorf("buildNetID = %x, want %x", got, want)
	}
}

func TestBuildGateFrame_RoundTrip(t *testing.T) {
	payload := []byte{0xDE, 0xAD, 0xBE, 0xEF}
	frame := buildGateFrame(gwCmdUserData, payload)
	// 校验帧头
	if binary.LittleEndian.Uint32(frame[0:4]) != gwMagicFrame {
		t.Error("帧魔数错误")
	}
	if binary.LittleEndian.Uint32(frame[8:12]) != gwSeparator {
		t.Error("分隔符错误")
	}
	if got := binary.LittleEndian.Uint32(frame[4:8]); got != uint32(2+len(payload)) {
		t.Errorf("payloadLen = %d", got)
	}
	cmd, bodyLen, ok := parseGateFrame(frame[:14])
	if !ok || cmd != gwCmdUserData || int(bodyLen) != len(payload) {
		t.Errorf("parseGateFrame: cmd=%d bodyLen=%d ok=%v", cmd, bodyLen, ok)
	}
	if !bytes.Equal(frame[14:], payload) {
		t.Error("payload 不一致")
	}
}

func TestParseGateFrame_Golden(t *testing.T) {
	b := readGoldenGw(t, "gate_upstream.raw")
	// 第一帧：80B 注册后，偏移 80 起 14B 头。
	cmd, bodyLen, ok := parseGateFrame(b[80:94])
	if !ok {
		t.Fatal("golden 首帧解析失败")
	}
	if cmd != gwCmdUserOpen {
		t.Errorf("cmd = %d, want %d", cmd, gwCmdUserOpen)
	}
	if int(bodyLen) != 44 {
		t.Errorf("bodyLen = %d, want 44", bodyLen)
	}
}

func TestParseGateFrame_Errors(t *testing.T) {
	bad := make([]byte, 14)
	binary.LittleEndian.PutUint32(bad[4:8], 2) // plen>=2
	if _, _, ok := parseGateFrame(bad); ok {
		t.Error("错误魔数应失败")
	}
	binary.LittleEndian.PutUint32(bad[0:4], gwMagicFrame)
	binary.LittleEndian.PutUint32(bad[4:8], 1) // plen<2
	if _, _, ok := parseGateFrame(bad); ok {
		t.Error("plen<2 应失败")
	}
}

func TestClientHandshakeCode(t *testing.T) {
	b := make([]byte, 4)
	binary.LittleEndian.PutUint32(b, handshakeCode)
	if !clientHandshakeCode(b) {
		t.Error("正确握手码应通过")
	}
	b[0] ^= 0xFF
	if clientHandshakeCode(b) {
		t.Error("错误握手码应拒绝")
	}
	if clientHandshakeCode([]byte{1, 2}) {
		t.Error("过短应拒绝")
	}
}

func TestWsAccept_RFC6455(t *testing.T) {
	// RFC6455 官方测试向量：key "dGhlIHNhbXBsZSBub25jZQ==" → "s3pPLMBiTxaQ9kYGzzhZRbK+xOo="
	got := wsAccept("dGhlIHNhbXBsZSBub25jZQ==")
	if got != "s3pPLMBiTxaQ9kYGzzhZRbK+xOo=" {
		t.Errorf("wsAccept = %q", got)
	}
}

func TestReadWSHandshake(t *testing.T) {
	req := "GET / HTTP/1.1\r\nHost: 127.0.0.1:9002\r\n" +
		"Upgrade: websocket\r\nConnection: Upgrade\r\n" +
		"Sec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==\r\n" +
		"Sec-WebSocket-Version: 13\r\n\r\n"
	r := bufio.NewReader(strings.NewReader(req))
	key, err := readWSHandshake(r)
	if err != nil {
		t.Fatalf("readWSHandshake: %v", err)
	}
	if key != "dGhlIHNhbXBsZSBub25jZQ==" {
		t.Errorf("key = %q", key)
	}
	// 缺 key
	r2 := bufio.NewReader(strings.NewReader("GET / HTTP/1.1\r\n\r\n"))
	if _, err := readWSHandshake(r2); err == nil {
		t.Error("缺 Sec-WebSocket-Key 应报错")
	}
}

func TestWSFrame_RoundTrip(t *testing.T) {
	// 客户端掩码帧 → readWSFrame 解掩码
	payload := []byte{0xCA, 0x0F, 0xFF, 0xFF, 1, 2, 3}
	mask := []byte{0x11, 0x22, 0x33, 0x44}
	var raw bytes.Buffer
	raw.WriteByte(0x81) // FIN + text
	raw.WriteByte(0x80 | byte(len(payload)))
	raw.Write(mask)
	for i, b := range payload {
		raw.WriteByte(b ^ mask[i%4])
	}
	r := bufio.NewReader(bytes.NewReader(raw.Bytes()))
	opcode, got, err := readWSFrame(r)
	if err != nil {
		t.Fatalf("readWSFrame: %v", err)
	}
	if opcode != 0x1 || !bytes.Equal(got, payload) {
		t.Errorf("opcode=%d payload=%x", opcode, got)
	}
	// 服务端无掩码帧 → writeWSFrame
	var out bytes.Buffer
	if err := writeWSFrame(&out, 0x2, payload); err != nil {
		t.Fatalf("writeWSFrame: %v", err)
	}
	if out.Bytes()[0] != 0x82 || out.Bytes()[1] != byte(len(payload)) {
		t.Errorf("writeWSFrame 头错误: %x", out.Bytes()[:2])
	}
}

func TestBuildUserData_NoDoublePrefix(t *testing.T) {
	// 回归测试：曾因重复添加 00 00 cd cd 导致 gameworld 静默忽略登录。
	login := []byte{0, 0, 0xcd, 0xcd, 0xFF, 0x01, 0x01}
	frame := buildUserData(0x228, login)
	// payload = 8B NetId + 原样 data，不应出现两次 00 00 cd cd
	body := frame[14:]
	if !bytes.Equal(body[8:], login) {
		t.Errorf("data 部分被改动: %x", body[8:])
	}
	if !bytes.Equal(body[:8], buildNetID(0x228)) {
		t.Error("NetId 错误")
	}
	// 确认 data 部分首 4B 就是客户端自带的 00 00 cd cd（未重复添加）
	count := bytes.Count(body, []byte{0, 0, 0xcd, 0xcd})
	if count != 1 {
		t.Errorf("00 00 cd cd 出现 %d 次，应为 1", count)
	}
}

func TestReadRawGateFrame(t *testing.T) {
	// 构造一帧：cmd=5, payload=4B
	frame := buildGateFrame(gwCmdUserData, []byte{1, 2, 3, 4})
	r := bufio.NewReader(bytes.NewReader(frame))
	raw, cmd, body, err := readRawGateFrame(r)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(raw, frame) {
		t.Errorf("raw 与原帧不符: %x vs %x", raw, frame)
	}
	if cmd != gwCmdUserData {
		t.Errorf("cmd=%d", cmd)
	}
	// body = 14B头后的 payload(6B)去掉 cmd(2B) = 4B
	if !bytes.Equal(body, []byte{1, 2, 3, 4}) {
		t.Errorf("body=%x", body)
	}
}

// 确保 base64/binary import 被使用（wsAccept 已用 base64）。
var _ = base64.StdEncoding
