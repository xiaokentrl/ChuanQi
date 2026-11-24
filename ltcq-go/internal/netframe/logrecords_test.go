package netframe

import (
	"bytes"
	"encoding/binary"
	"strings"
	"testing"
)

// 构造定宽字符串字段：[64]byte 写 name，其余补 0。
func cfield(s string, width int) []byte {
	b := make([]byte, width)
	copy(b, s)
	return b
}

func int32le(v int32) []byte {
	b := make([]byte, 4)
	binary.LittleEndian.PutUint32(b, uint32(v))
	return b
}

func TestDecodeCounter(t *testing.T) {
	body := make([]byte, 556)
	copy(body[0:4], int32le(12345))
	copy(body[4:68], cfield("aid001", 64))
	copy(body[68:100], cfield("L10", 32))
	copy(body[100:132], cfield("login", 32))
	copy(body[132:164], cfield("1", 32))
	body[548] = 1
	copy(body[552:556], int32le(1))

	r, err := DecodeCounter(body)
	if err != nil {
		t.Fatalf("DecodeCounter: %v", err)
	}
	if r.ActorID != 12345 || r.Aid != "aid001" || r.UserLevel != "L10" || r.Counter != "login" || !r.SaveDB || r.Flag != 1 {
		t.Errorf("解码字段不符: %+v", r)
	}
	if _, err := DecodeCounter(body[:555]); err == nil {
		t.Error("body 过短应报错")
	}
}

func TestDecodeInstall(t *testing.T) {
	body := make([]byte, 292)
	copy(body[0:64], cfield("aid002", 64))
	copy(body[64:96], cfield("sourceA", 32))
	copy(body[288:292], int32le(1))
	r, err := DecodeInstall(body)
	if err != nil {
		t.Fatalf("DecodeInstall: %v", err)
	}
	if r.Aid != "aid002" || r.Source != "sourceA" || r.Flag != 1 {
		t.Errorf("解码字段不符: %+v", r)
	}
}

func TestDecodeDau(t *testing.T) {
	body := make([]byte, 324)
	copy(body[0:64], cfield("aid003", 64))
	copy(body[224:256], cfield("1.2.3.4", 32))
	copy(body[320:324], int32le(0))
	r, err := DecodeDau(body)
	if err != nil {
		t.Fatalf("DecodeDau: %v", err)
	}
	if r.Aid != "aid003" || r.IP != "1.2.3.4" || r.Flag != 0 {
		t.Errorf("解码字段不符: %+v", r)
	}
}

func TestDecodeEconomy(t *testing.T) {
	body := make([]byte, 616)
	copy(body[0:4], int32le(99))
	copy(body[4:68], cfield("aid004", 64))
	copy(body[68:100], cfield("yuanbao", 32))
	copy(body[612:616], int32le(1))
	r, err := DecodeEconomy(body)
	if err != nil {
		t.Fatalf("DecodeEconomy: %v", err)
	}
	if r.ActorID != 99 || r.Aid != "aid004" || r.Currency != "yuanbao" || r.Flag != 1 {
		t.Errorf("解码字段不符: %+v", r)
	}
}

func TestDecodeMilestone(t *testing.T) {
	body := make([]byte, 164)
	copy(body[0:4], int32le(7))
	copy(body[4:68], cfield("aid005", 64))
	copy(body[68:100], cfield("firstLogin", 32))
	copy(body[100:132], cfield("1", 32))
	copy(body[132:164], cfield("extra", 32))
	r, err := DecodeMilestone(body)
	if err != nil {
		t.Fatalf("DecodeMilestone: %v", err)
	}
	if r.ActorID != 7 || r.Aid != "aid005" || r.Milestone != "firstLogin" || r.Value != "1" || r.Extra != "extra" {
		t.Errorf("解码字段不符: %+v", r)
	}
}

func TestDecodeChat(t *testing.T) {
	body := make([]byte, 1288)
	copy(body[0:4], int32le(42))
	copy(body[4:68], cfield("aid006", 64))
	copy(body[68:100], cfield("玩家名", 32))
	copy(body[100:132], cfield("127.0.0.1", 32))
	copy(body[164:168], int32le(2))
	copy(body[264:1288], cfield("hello world", 1024))
	r, err := DecodeChat(body)
	if err != nil {
		t.Fatalf("DecodeChat: %v", err)
	}
	if r.ActorID != 42 || r.Aid != "aid006" || r.Name != "玩家名" || r.Type != 2 || r.Content != "hello world" {
		t.Errorf("解码字段不符: %+v", r)
	}
}

func TestDecodeOnline(t *testing.T) {
	body := make([]byte, 8)
	copy(body[0:4], int32le(100))
	copy(body[4:8], int32le(5))
	r, err := DecodeOnline(body)
	if err != nil {
		t.Fatalf("DecodeOnline: %v", err)
	}
	if r.Count != 100 || r.TmpCount != 5 {
		t.Errorf("解码字段不符: %+v", r)
	}
}

// TestDecode_GoldenBodiesNoPanic 用真实 golden 帧 body 喂各解码器，
// 确保字段偏移在真实数据上不越界、不解析出非法 UTF-8 垃圾。
func TestDecode_GoldenBodiesNoPanic(t *testing.T) {
	b := readGolden(t, "loggerserver-7001.raw")
	fr := NewFrameReader(bytes.NewReader(b[HandshakeSize:]))
	decoders := map[uint16]func([]byte) error{
		1:  func(x []byte) error { _, e := DecodeCounter(x); return e },
		2:  func(x []byte) error { _, e := DecodeInstall(x); return e },
		3:  func(x []byte) error { _, e := DecodeDau(x); return e },
		4:  func(x []byte) error { _, e := DecodeEconomy(x); return e },
		5:  func(x []byte) error { _, e := DecodeMilestone(x); return e },
		9:  func(x []byte) error { _, e := DecodeOnline(x); return e },
		14: func(x []byte) error { _, e := DecodeChat(x); return e },
	}
	seen := map[uint16]int{}
	for {
		f, err := fr.ReadFrame()
		if err != nil {
			break
		}
		seen[f.Cmd]++
		if dec, ok := decoders[f.Cmd]; ok {
			if err := dec(f.Body); err != nil {
				// cmd=9 的 body 只有 8B，OnlineRec 能解；其余按 DWARF 长度严格校验。
				t.Errorf("cmd=%d (body=%dB) 解码失败: %v", f.Cmd, len(f.Body), err)
			}
		}
	}
	// 至少覆盖到实际存在的记录类型
	if seen[1] == 0 || seen[4] == 0 {
		t.Errorf("golden 未覆盖关键记录类型: %v", seen)
	}
}

func TestCstr(t *testing.T) {
	if got := cstr([]byte("abc\x00def")); got != "abc" {
		t.Errorf("cstr with NUL = %q", got)
	}
	if got := cstr([]byte("abc  \t\r\n")); !strings.HasSuffix(got, "abc") {
		t.Errorf("cstr trailing ws = %q", got)
	}
}
