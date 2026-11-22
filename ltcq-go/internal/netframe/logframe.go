// Package netframe：日志服务 7001 协议帧层。
// 格式经 golden 实证（docs/loggerserver-spec.md §四a，ADR-007）：
//
//	握手(80B): [4B 0x12AA66CA][4B 未知][4B serverId][64B 服务名][4B 0]
//	帧: [4B 0xCCEE][4B payloadLen][4B 0xCDCDCDCD][payload: 2B cmd + 结构体裸拷贝]
package netframe

import (
	"bufio"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

const (
	MagicHandshake = 0x12AA66CA
	MagicFrame     = 0xCCEE
	Separator      = 0xCDCDCDCD
	HandshakeSize  = 80
)

var (
	ErrBadHandshake = errors.New("netframe: 握手魔数不符")
	ErrBadFrame     = errors.New("netframe: 帧魔数/分隔符不符（流错位）")
)

// LogHandshake 是 gameworld 连上 7001 后发送的注册包。
type LogHandshake struct {
	ServerID int32
	Name     string // 定长 64B，去尾 \0
	Unknown  int32  // 观测值 6，含义待定
}

// ParseHandshake 解析 80 字节握手。
func ParseHandshake(b []byte) (LogHandshake, error) {
	var h LogHandshake
	if len(b) < HandshakeSize {
		return h, fmt.Errorf("netframe: 握手不足 %d 字节: %d", HandshakeSize, len(b))
	}
	if binary.LittleEndian.Uint32(b[0:4]) != MagicHandshake {
		return h, ErrBadHandshake
	}
	h.Unknown = int32(binary.LittleEndian.Uint32(b[4:8]))
	h.ServerID = int32(binary.LittleEndian.Uint32(b[8:12]))
	h.Name = cstr(b[16:80])
	return h, nil
}

// LogFrame 是一条日志记录帧：Cmd + 结构体裸拷贝 body。
type LogFrame struct {
	Cmd   uint16
	Body  []byte // payload 去掉 2B cmd 后的部分（即结构体本体）
}

// FrameReader 从流式连接持续读帧。调用方需先读完握手。
type FrameReader struct {
	r *bufio.Reader
}

func NewFrameReader(r io.Reader) *FrameReader {
	return &FrameReader{r: bufio.NewReaderSize(r, 1<<20)}
}

// ReadFrame 阻塞读取下一帧。流错位时返回 ErrBadFrame。
func (fr *FrameReader) ReadFrame() (LogFrame, error) {
	var f LogFrame
	head := make([]byte, 14) // [CCEE 4][len 4][CDCDCDCD 4][cmd 2]
	if _, err := io.ReadFull(fr.r, head); err != nil {
		return f, err // io.EOF = 正常结束
	}
	if binary.LittleEndian.Uint32(head[0:4]) != MagicFrame {
		return f, ErrBadFrame
	}
	if binary.LittleEndian.Uint32(head[8:12]) != Separator {
		return f, ErrBadFrame
	}
	plen := binary.LittleEndian.Uint32(head[4:8])
	if plen < 2 || plen > 1<<20 {
		return f, fmt.Errorf("netframe: payloadLen 越界 %d", plen)
	}
	f.Cmd = binary.LittleEndian.Uint16(head[12:14])
	body := make([]byte, plen-2)
	if _, err := io.ReadFull(fr.r, body); err != nil {
		return f, err
	}
	f.Body = body
	return f, nil
}

// cstr 去除定宽 char 数组字段的尾部 \0 与空白。
func cstr(b []byte) string {
	for i, c := range b {
		if c == 0 {
			return string(b[:i])
		}
	}
	// 无 \0：去掉尾部空白
	end := len(b)
	for end > 0 && (b[end-1] == ' ' || b[end-1] == '\t' || b[end-1] == '\r' || b[end-1] == '\n') {
		end--
	}
	return string(b[:end])
}
