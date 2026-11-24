package netframe

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// golden 文件位置：ltcq-go/reference/captures/（ADR-007 对拍基准）。
func goldenPath(name string) string {
	_, thisFile, _, _ := runtime.Caller(0)
	// thisFile = <root>/internal/netframe/logframe_test.go
	root := filepath.Dir(filepath.Dir(filepath.Dir(thisFile)))
	return filepath.Join(root, "reference", "captures", name)
}

func readGolden(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(goldenPath(name))
	if err != nil {
		t.Fatalf("读取 golden %s 失败: %v", name, err)
	}
	return b
}

func TestParseHandshake_Golden(t *testing.T) {
	tests := []struct {
		name     string
		file     string
		serverID int32
		unknown  int32
		wantName string
	}{
		{"gameworld_s1", "loggerserver-7001.raw", 1, 6, "gameworld_s1"},
		{"gateway_s1", "golden_c_stream.raw", 0, 1, "gateway_s1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := readGolden(t, tt.file)
			got, err := ParseHandshake(b[:HandshakeSize])
			if err != nil {
				t.Fatalf("ParseHandshake 失败: %v", err)
			}
			if got.ServerID != tt.serverID {
				t.Errorf("ServerID = %d, want %d", got.ServerID, tt.serverID)
			}
			if got.Unknown != tt.unknown {
				t.Errorf("Unknown = %d, want %d", got.Unknown, tt.unknown)
			}
			if got.Name != tt.wantName {
				t.Errorf("Name = %q, want %q", got.Name, tt.wantName)
			}
		})
	}
}

func TestParseHandshake_Errors(t *testing.T) {
	short := make([]byte, HandshakeSize-1)
	if _, err := ParseHandshake(short); err == nil {
		t.Error("不足 80B 应报错")
	}
	bad := make([]byte, HandshakeSize)
	bad[0] = 0xFF
	if _, err := ParseHandshake(bad); err != ErrBadHandshake {
		t.Errorf("错误魔数应返回 ErrBadHandshake, got %v", err)
	}
}

func TestFrameReader_GoldenFullStream(t *testing.T) {
	b := readGolden(t, "loggerserver-7001.raw")
	if len(b) < HandshakeSize {
		t.Fatal("golden 太短")
	}
	fr := NewFrameReader(bytes.NewReader(b[HandshakeSize:]))

	// Node 独立解析基准：664 帧，精确消费全部字节。
	wantCmd := map[uint16]int{0: 2, 1: 205, 2: 2, 4: 169, 9: 3, 15: 283}
	gotCmd := map[uint16]int{}
	total := 0
	for {
		f, err := fr.ReadFrame()
		if err != nil {
			break // io.EOF = 正常结束
		}
		gotCmd[f.Cmd]++
		total++
	}
	if total != 664 {
		t.Errorf("帧总数 = %d, want 664", total)
	}
	for cmd, n := range wantCmd {
		if gotCmd[cmd] != n {
			t.Errorf("cmd=%d 数量 = %d, want %d", cmd, gotCmd[cmd], n)
		}
	}
}

func TestFrameReader_Errors(t *testing.T) {
	tests := []struct {
		name string
		head []byte
		want error
	}{
		{"bad magic", mustFrame(0xDEAD, 2, Separator, 1), ErrBadFrame},
		{"bad separator", mustFrame(MagicFrame, 2, 0x11223344, 1), ErrBadFrame},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fr := NewFrameReader(bytes.NewReader(tt.head))
			if _, err := fr.ReadFrame(); err != tt.want {
				t.Errorf("got %v, want %v", err, tt.want)
			}
		})
	}

	// payloadLen 越界（> 1<<20）
	over := mustFrame(MagicFrame, (1<<20)+1, Separator, 1)
	fr := NewFrameReader(bytes.NewReader(over))
	if _, err := fr.ReadFrame(); err == nil {
		t.Error("payloadLen 越界应报错")
	}
}

// mustFrame 构造 14B 帧头用于错误注入测试。
func mustFrame(magic uint32, plen uint32, sep uint32, cmd uint16) []byte {
	b := make([]byte, 14)
	binary.LittleEndian.PutUint32(b[0:4], magic)
	binary.LittleEndian.PutUint32(b[4:8], plen)
	binary.LittleEndian.PutUint32(b[8:12], sep)
	binary.LittleEndian.PutUint16(b[12:14], cmd)
	return b
}
