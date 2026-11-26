package engineapi

import (
	"encoding/binary"
	"fmt"
	"sync"

	"github.com/xiaokentrl/ChuanQi/ltcq-go/internal/luahost"
)

// 原版类型常量（data/functions/_G.lua:407-419），writeData/readData 用。
const (
	dtByte   = 0
	dtChar   = 1
	dtWord   = 2
	dtShort  = 3
	dtInt    = 4
	dtUint   = 5
	dtInt64  = 6
	dtUint64 = 7
	dtString = 8
)

// DataPack 是 LDataPack API 操作的不透明包。Lua 侧只见整数句柄，
// 保持与宿主无关（不依赖具体 Lua 实现的 userdata）。
type DataPack struct {
	buf     []byte
	pos     int
	actorID int  // allocPacket 绑定的玩家（回包 SendHook 用）
	sysid   byte // allocPacket 绑定的系统号（回包头）
	cmdid   byte // allocPacket 绑定的命令号（回包头）
}

var (
	packMu      sync.Mutex
	packs       = map[int]*DataPack{}
	nextPackID  = 1
	lastFlushed = map[int][]byte{} // handle → 最近一次 flush 的字节（测试/发送用）
	// SendHook 由网络路由层注入：flush 时回调 (actorID, 完整包字节含sysid/cmd头)。
	// 返回 nil 表示未安装（flush 只记录不发送）。
	sendHook func(actorID int, packet []byte)
)

// SetSendHook 安装回包发送钩子（网络路由层调用）。
func SetSendHook(fn func(actorID int, packet []byte)) {
	packMu.Lock()
	defer packMu.Unlock()
	sendHook = fn
}

// NewPackFromBytes 从已有字节构造只读 pack（读游标在0），供 OnNetMsg 上行路由使用。
// 返回句柄，交给 Lua 回调作为 pack 参数。
func NewPackFromBytes(data []byte) int {
	packMu.Lock()
	defer packMu.Unlock()
	id := nextPackID
	nextPackID++
	packs[id] = &DataPack{buf: append([]byte(nil), data...), pos: 0}
	return id
}

func newPack() int {
	packMu.Lock()
	defer packMu.Unlock()
	id := nextPackID
	nextPackID++
	packs[id] = &DataPack{}
	return id
}

func getPack(v luahost.Value) (*DataPack, error) {
	id, ok := v.(int)
	if !ok {
		// gopher-lua fromLValue 把 LNumber 转 int；其他宿主可能是 int64/float64
		switch x := v.(type) {
		case int64:
			id = int(x)
		case float64:
			id = int(x)
		default:
			return nil, fmt.Errorf("无效 pack 句柄: %v", v)
		}
	}
	packMu.Lock()
	defer packMu.Unlock()
	p, ok := packs[id]
	if !ok {
		return nil, fmt.Errorf("pack 句柄不存在: %d", id)
	}
	return p, nil
}

// LastFlushed 返回某句柄最近一次 flush 的字节副本（测试/对拍用）。
func LastFlushed(handle int) []byte {
	packMu.Lock()
	defer packMu.Unlock()
	b := lastFlushed[handle]
	out := make([]byte, len(b))
	copy(out, b)
	return out
}

func writeLE(p *DataPack, bytes []byte) {
	p.buf = append(p.buf, bytes...)
}

func readN(p *DataPack, n int) ([]byte, error) {
	if p.pos+n > len(p.buf) {
		return nil, fmt.Errorf("read 越界: pos=%d n=%d len=%d", p.pos, n, len(p.buf))
	}
	b := p.buf[p.pos : p.pos+n]
	p.pos += n
	return b, nil
}

func asInt(v luahost.Value) int {
	switch x := v.(type) {
	case int:
		return x
	case int64:
		return int(x)
	case float64:
		return int(x)
	default:
		return 0
	}
}

func asString(v luahost.Value) string {
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprint(v)
}

func init() {
	// allocPacket([actor, cmd, subcmd]) → pack
	// 原版：3 参时自动写 cmd/subcmd 头（dtByte）；0 参变体（crossquery.lua:13）。
	Register(Entry{Name: "LDataPack.allocPacket", Fn: func(args ...luahost.Value) (luahost.Value, error) {
		id := newPack()
		p, _ := getPack(id)
		if len(args) >= 1 {
			p.actorID = asActorID(args[0])
		}
		if len(args) >= 3 {
			p.sysid, p.cmdid = byte(asInt(args[1])), byte(asInt(args[2]))
			writeLE(p, []byte{p.sysid, p.cmdid})
		}
		return id, nil
	}, Meta: "原版: gameworld LDataPack::allocPacket | Lua调用: 804处 | 规格: engine-api.md#LDataPack"})

	// allocBroadcastPacket(systemId, protocolId) → pack（Lua 侧会覆盖，见 datapack.lua:18）
	Register(Entry{Name: "LDataPack.allocBroadcastPacket", Fn: func(args ...luahost.Value) (luahost.Value, error) {
		return newPack(), nil
	}, Meta: "原版: LDataPack | Lua调用: 44处 | 注: Lua datapack.lua:18 覆盖此实现"})

	// 基础写函数（小端，对齐 golden 字节）
	Register(Entry{Name: "LDataPack.writeByte", Fn: mkWrite(1), Meta: "Lua调用: 677处"})
	Register(Entry{Name: "LDataPack.writeChar", Fn: mkWrite(1), Meta: "Lua调用: 200处"})
	Register(Entry{Name: "LDataPack.writeShort", Fn: mkWrite(2), Meta: "Lua调用: 706处"})
	Register(Entry{Name: "LDataPack.writeWord", Fn: mkWrite(2), Meta: "包组"})
	Register(Entry{Name: "LDataPack.writeInt", Fn: mkWrite(4), Meta: "Lua调用: 1405处"})
	Register(Entry{Name: "LDataPack.writeUInt", Fn: mkWrite(4), Meta: "Lua调用: 48处"})
	Register(Entry{Name: "LDataPack.writeDouble", Fn: mkWrite(8), Meta: "Lua调用: 61处"})
	Register(Entry{Name: "LDataPack.writeInt64", Fn: mkWrite(8), Meta: "包组"})
	Register(Entry{Name: "LDataPack.writeString", Fn: func(args ...luahost.Value) (luahost.Value, error) {
		p, err := getPack(args[0])
		if err != nil {
			return nil, err
		}
		s := asString(args[1])
		writeLE(p, []byte(s))
		writeLE(p, []byte{0}) // C 风格 NUL 结尾（golden: 账号后 00）
		return nil, nil
	}, Meta: "Lua调用: 224处"})

	// writeData(pack, n, type, value, ...) —— 变参按 type/value 交替写
	Register(Entry{Name: "LDataPack.writeData", Fn: func(args ...luahost.Value) (luahost.Value, error) {
		p, err := getPack(args[0])
		if err != nil {
			return nil, err
		}
		n := asInt(args[1])
		for i := 0; i < n; i++ {
			idx := 2 + i*2
			if idx+1 >= len(args) {
				return nil, fmt.Errorf("writeData 参数不足")
			}
			typ := asInt(args[idx])
			val := args[idx+1]
			if err := writeTyped(p, typ, val); err != nil {
				return nil, err
			}
		}
		return nil, nil
	}, Meta: "Lua调用: 111处"})

	// 基础读函数
	Register(Entry{Name: "LDataPack.readByte", Fn: mkRead(1), Meta: "Lua调用: 73处"})
	Register(Entry{Name: "LDataPack.readChar", Fn: mkRead(1), Meta: "Lua调用: 49处"})
	Register(Entry{Name: "LDataPack.readShort", Fn: mkRead(2), Meta: "Lua调用: 214处"})
	Register(Entry{Name: "LDataPack.readWord", Fn: mkRead(2), Meta: "包组"})
	Register(Entry{Name: "LDataPack.readInt", Fn: mkRead(4), Meta: "Lua调用: 361处"})
	Register(Entry{Name: "LDataPack.readUInt", Fn: mkRead(4), Meta: "包组"})
	Register(Entry{Name: "LDataPack.readInt64", Fn: func(args ...luahost.Value) (luahost.Value, error) {
		p, err := getPack(args[0])
		if err != nil {
			return nil, err
		}
		b, err := readN(p, 8)
		if err != nil {
			return nil, err
		}
		return int64(binary.LittleEndian.Uint64(b)), nil
	}, Meta: "Lua调用: 12处 | 与 writeInt64 对称的小端 64 位读取"})
	Register(Entry{Name: "LDataPack.readString", Fn: func(args ...luahost.Value) (luahost.Value, error) {
		p, err := getPack(args[0])
		if err != nil {
			return nil, err
		}
		var b []byte
		for {
			c, err := readN(p, 1)
			if err != nil {
				return nil, err
			}
			if c[0] == 0 {
				break
			}
			b = append(b, c[0])
		}
		return string(b), nil
	}, Meta: "Lua调用: 106处"})

	// readData(pack, n, dt1, dt2, ...) —— 按 dt 常量返回 n 个值
	Register(Entry{Name: "LDataPack.readData", FnMulti: func(args ...luahost.Value) ([]luahost.Value, error) {
		p, err := getPack(args[0])
		if err != nil {
			return nil, err
		}
		if len(args) < 2 {
			return nil, fmt.Errorf("readData 缺少字段数")
		}
		n := asInt(args[1])
		if n < 0 || n > len(args)-2 {
			return nil, fmt.Errorf("readData 字段数非法: n=%d args=%d", n, len(args))
		}
		out := make([]luahost.Value, 0, n)
		for i := 0; i < n; i++ {
			v, err := readTyped(p, asInt(args[2+i]))
			if err != nil {
				return nil, err
			}
			out = append(out, v)
		}
		return out, nil
	}, Meta: "Lua调用: 26处 | 按 _G.lua dt* 常量读取变参"})

	// 游标定位
	Register(Entry{Name: "LDataPack.setPosition", Fn: func(args ...luahost.Value) (luahost.Value, error) {
		p, err := getPack(args[0])
		if err != nil {
			return nil, err
		}
		p.pos = asInt(args[1])
		return nil, nil
	}, Meta: "Lua调用: 185处"})
	Register(Entry{Name: "LDataPack.getPosition", Fn: func(args ...luahost.Value) (luahost.Value, error) {
		p, err := getPack(args[0])
		if err != nil {
			return nil, err
		}
		return p.pos, nil
	}, Meta: "Lua调用: 98处"})

	// flush：记录字节 + 触发 SendHook 回包（网络路由层注入）
	Register(Entry{Name: "LDataPack.flush", Fn: func(args ...luahost.Value) (luahost.Value, error) {
		p, err := getPack(args[0])
		if err != nil {
			return nil, err
		}
		packMu.Lock()
		for id, x := range packs {
			if x == p {
				lastFlushed[id] = append([]byte(nil), p.buf...)
				break
			}
		}
		hook := sendHook
		packet := append([]byte(nil), p.buf...) // 完整包（含 sysid/cmdid 头）
		actorID := p.actorID
		packMu.Unlock()
		if hook != nil {
			hook(actorID, packet)
		}
		return nil, nil
	}, Meta: "Lua调用: 657处"})

	// rollBack：放弃未 flush 写（重置 buf）
	Register(Entry{Name: "LDataPack.rollBack", Fn: func(args ...luahost.Value) (luahost.Value, error) {
		p, err := getPack(args[0])
		if err != nil {
			return nil, err
		}
		p.buf = nil
		p.pos = 0
		return nil, nil
	}, Meta: "包组"})

	// test_allocPack（test_wyyj.lua:9）——待确认语义，先用标准分配
	Register(Entry{Name: "LDataPack.test_allocPack", Fn: func(args ...luahost.Value) (luahost.Value, error) {
		return newPack(), nil
	}, Meta: "Lua调用: 72处 | 待确认"})
}

func mkWrite(width int) func(...luahost.Value) (luahost.Value, error) {
	return func(args ...luahost.Value) (luahost.Value, error) {
		p, err := getPack(args[0])
		if err != nil {
			return nil, err
		}
		v := asInt(args[1])
		b := make([]byte, width)
		switch width {
		case 1:
			b[0] = byte(v)
		case 2:
			binary.LittleEndian.PutUint16(b, uint16(v))
		case 4:
			binary.LittleEndian.PutUint32(b, uint32(v))
		case 8:
			binary.LittleEndian.PutUint64(b, uint64(v))
		}
		writeLE(p, b)
		return nil, nil
	}
}

func mkRead(width int) func(...luahost.Value) (luahost.Value, error) {
	return func(args ...luahost.Value) (luahost.Value, error) {
		p, err := getPack(args[0])
		if err != nil {
			return nil, err
		}
		b, err := readN(p, width)
		if err != nil {
			return nil, err
		}
		switch width {
		case 1:
			return int(b[0]), nil
		case 2:
			return int(binary.LittleEndian.Uint16(b)), nil
		case 4:
			return int(binary.LittleEndian.Uint32(b)), nil
		case 8:
			return int(binary.LittleEndian.Uint64(b)), nil
		}
		return 0, nil
	}
}

func writeTyped(p *DataPack, typ int, val luahost.Value) error {
	switch typ {
	case dtByte, dtChar:
		writeLE(p, []byte{byte(asInt(val))})
	case dtWord, dtShort:
		b := make([]byte, 2)
		binary.LittleEndian.PutUint16(b, uint16(asInt(val)))
		writeLE(p, b)
	case dtInt, dtUint:
		b := make([]byte, 4)
		binary.LittleEndian.PutUint32(b, uint32(asInt(val)))
		writeLE(p, b)
	case dtInt64, dtUint64:
		b := make([]byte, 8)
		binary.LittleEndian.PutUint64(b, uint64(asInt(val)))
		writeLE(p, b)
	case dtString:
		writeLE(p, []byte(asString(val)))
		writeLE(p, []byte{0})
	default:
		return fmt.Errorf("未知 dtType: %d", typ)
	}
	return nil
}

func readTyped(p *DataPack, typ int) (luahost.Value, error) {
	switch typ {
	case dtByte, dtChar:
		b, err := readN(p, 1)
		if err != nil {
			return nil, err
		}
		return int(b[0]), nil
	case dtWord, dtShort:
		b, err := readN(p, 2)
		if err != nil {
			return nil, err
		}
		return int(binary.LittleEndian.Uint16(b)), nil
	case dtInt, dtUint:
		b, err := readN(p, 4)
		if err != nil {
			return nil, err
		}
		return int(binary.LittleEndian.Uint32(b)), nil
	case dtInt64, dtUint64:
		b, err := readN(p, 8)
		if err != nil {
			return nil, err
		}
		return int64(binary.LittleEndian.Uint64(b)), nil
	case dtString:
		var b []byte
		for {
			c, err := readN(p, 1)
			if err != nil {
				return nil, err
			}
			if c[0] == 0 {
				break
			}
			b = append(b, c[0])
		}
		return string(b), nil
	default:
		return nil, fmt.Errorf("未知 dtType: %d", typ)
	}
}
