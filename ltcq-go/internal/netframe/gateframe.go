// Package netframe：网关↔gameworld 帧编解码（gateway 与 gameworld 共用）。
// 常量与 logframe.go 一致（0xCCEE / 0xCDCDCDCD / 80B 握手）。
package netframe

import (
	"encoding/binary"
	"fmt"
	"io"
)

// 网关帧命令字（实测，与 cmd/gateway 一致）。
const (
	GateCmdHeartbeat uint16 = 0
	GateCmdUserOpen  uint16 = 1
	GateCmdUserClose uint16 = 2
	GateCmdUserData  uint16 = 5
	GateCmdBroadcast uint16 = 6
)

const (
	// NetIDLen 是网关帧 payload 里的 NetId 长度（低2B LE=sid，其余6B零）。
	NetIDLen = 8
	// GateFrameHeaderLen 是帧头长度：[magic4][len4][sep4][cmd2]。
	GateFrameHeaderLen = 14
)

// GateFrame 是一帧网关协议数据。
type GateFrame struct {
	Cmd  uint16
	Body []byte // 去掉14B头与2Bcmd后的负载
}

// BuildGateFrame 构造帧：[0xCCEE][payloadLen=2+len(body)][0xCDCDCDCD][cmd][body]。
func BuildGateFrame(cmd uint16, body []byte) []byte {
	plen := 2 + len(body)
	out := make([]byte, GateFrameHeaderLen+len(body))
	binary.LittleEndian.PutUint32(out[0:4], MagicFrame)
	binary.LittleEndian.PutUint32(out[4:8], uint32(plen))
	binary.LittleEndian.PutUint32(out[8:12], Separator)
	binary.LittleEndian.PutUint16(out[12:14], cmd)
	copy(out[GateFrameHeaderLen:], body)
	return out
}

// ParseGateFrame 解析14B帧头，返回 cmd 与 body 长度（bodyLen=payloadLen-2）。
func ParseGateFrame(head []byte) (cmd uint16, bodyLen int, err error) {
	if len(head) < GateFrameHeaderLen {
		return 0, 0, fmt.Errorf("gate 帧头不足14B")
	}
	if binary.LittleEndian.Uint32(head[0:4]) != MagicFrame {
		return 0, 0, fmt.Errorf("gate 帧魔数不符: %08x", binary.LittleEndian.Uint32(head[0:4]))
	}
	if binary.LittleEndian.Uint32(head[8:12]) != Separator {
		return 0, 0, ErrBadFrame
	}
	plen := int(binary.LittleEndian.Uint32(head[4:8]))
	if plen < 2 {
		return 0, 0, fmt.Errorf("gate payloadLen<2: %d", plen)
	}
	return binary.LittleEndian.Uint16(head[12:14]), plen - 2, nil
}

// ReadGateFrame 从 reader 读一帧（校验魔数与分隔符）。
func ReadGateFrame(r io.Reader) (GateFrame, error) {
	head := make([]byte, GateFrameHeaderLen)
	if _, err := io.ReadFull(r, head); err != nil {
		return GateFrame{}, err
	}
	cmd, bodyLen, err := ParseGateFrame(head)
	if err != nil {
		return GateFrame{}, err
	}
	body := make([]byte, bodyLen)
	if _, err := io.ReadFull(r, body); err != nil {
		return GateFrame{}, err
	}
	return GateFrame{Cmd: cmd, Body: body}, nil
}

// BuildNetID 构造8B NetId（sid 低2B LE，其余零）。
func BuildNetID(sid uint16) []byte {
	b := make([]byte, NetIDLen)
	b[0] = byte(sid)
	b[1] = byte(sid >> 8)
	return b
}

// NetIDSid 从8B NetId 提取 sid。
func NetIDSid(netID []byte) uint16 {
	if len(netID) < 2 {
		return 0
	}
	return uint16(netID[0]) | uint16(netID[1])<<8
}
