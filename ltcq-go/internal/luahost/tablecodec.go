package luahost

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"

	lua "github.com/yuin/gopher-lua"
)

// TableNode 是 Lua 值的宿主无关、可 JSON 序列化表示。
// 用于 Go 自有存档（不兼容原版 C++ 私有 blob），仅保存数据值；
// function/userdata 等不可持久值按 nil 处理，循环引用直接报错。
type TableNode struct {
	Kind  string      `json:"k"`
	Bool  bool        `json:"b,omitempty"`
	Int   int64       `json:"i,omitempty"`
	Float float64     `json:"f,omitempty"`
	Str   string      `json:"s,omitempty"`
	Pairs []TablePair `json:"p,omitempty"`
}

// TablePair 保留 Lua table 的键类型（字符串/数字/表），避免数组与字典混用后语义丢失。
type TablePair struct {
	Key   TableNode `json:"k"`
	Value TableNode `json:"v"`
}

// TableCodec 把宿主无关 Value 编码为可落库节点，或从节点还原为 Lua Value。
type TableCodec interface {
	EncodeValue(Value) (TableNode, error)
	DecodeValue(TableNode) (Value, error)
}

const (
	tableKindNil   = "nil"
	tableKindBool  = "bool"
	tableKindInt   = "int"
	tableKindFloat = "float"
	tableKindStr   = "str"
	tableKindTable = "table"

	tableMaxDepth = 64
)

func (g *GopherLua) EncodeValue(v Value) (TableNode, error) {
	g.callMu <- struct{}{}
	defer func() { <-g.callMu }()
	return encodeTableValue(v, map[*lua.LTable]bool{}, 0)
}

func (g *GopherLua) DecodeValue(n TableNode) (Value, error) {
	g.callMu <- struct{}{}
	defer func() { <-g.callMu }()
	return g.decodeTableValue(n, 0)
}

func encodeTableValue(v Value, visiting map[*lua.LTable]bool, depth int) (TableNode, error) {
	if depth > tableMaxDepth {
		return TableNode{}, fmt.Errorf("Lua 值嵌套超过 %d 层", tableMaxDepth)
	}
	switch x := v.(type) {
	case nil:
		return TableNode{Kind: tableKindNil}, nil
	case *lua.LNilType:
		return TableNode{Kind: tableKindNil}, nil
	case lua.LBool:
		return TableNode{Kind: tableKindBool, Bool: bool(x)}, nil
	case bool:
		return TableNode{Kind: tableKindBool, Bool: x}, nil
	case lua.LNumber:
		return encodeTableNumber(float64(x)), nil
	case float64:
		return encodeTableNumber(x), nil
	case int:
		return TableNode{Kind: tableKindInt, Int: int64(x)}, nil
	case int64:
		return TableNode{Kind: tableKindInt, Int: x}, nil
	case lua.LString:
		return TableNode{Kind: tableKindStr, Str: string(x)}, nil
	case string:
		return TableNode{Kind: tableKindStr, Str: x}, nil
	case *lua.LTable:
		if visiting[x] {
			return TableNode{}, fmt.Errorf("Lua table 存在循环引用，无法持久化")
		}
		visiting[x] = true
		defer delete(visiting, x)
		pairs := make([]TablePair, 0, x.Len())
		var encodeErr error
		x.ForEach(func(k, val lua.LValue) {
			if encodeErr != nil {
				return
			}
			keyNode, err := encodeTableValue(k, visiting, depth+1)
			if err != nil {
				encodeErr = fmt.Errorf("编码 table key: %w", err)
				return
			}
			valNode, err := encodeTableValue(val, visiting, depth+1)
			if err != nil {
				encodeErr = fmt.Errorf("编码 table value: %w", err)
				return
			}
			pairs = append(pairs, TablePair{Key: keyNode, Value: valNode})
		})
		if encodeErr != nil {
			return TableNode{}, encodeErr
		}
		sort.Slice(pairs, func(i, j int) bool {
			left, _ := json.Marshal(pairs[i].Key)
			right, _ := json.Marshal(pairs[j].Key)
			return string(left) < string(right)
		})
		return TableNode{Kind: tableKindTable, Pairs: pairs}, nil
	default:
		// function/userdata/lua.LChannel 等不是玩家数据，按 nil 落库。
		return TableNode{Kind: tableKindNil}, nil
	}
}

func encodeTableNumber(f float64) TableNode {
	if !math.IsNaN(f) && !math.IsInf(f, 0) && f == math.Trunc(f) &&
		f >= -9.223372036854776e18 && f <= 9.223372036854776e18 {
		return TableNode{Kind: tableKindInt, Int: int64(f)}
	}
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return TableNode{Kind: tableKindNil}
	}
	return TableNode{Kind: tableKindFloat, Float: f}
}

func (g *GopherLua) decodeTableValue(n TableNode, depth int) (lua.LValue, error) {
	if depth > tableMaxDepth {
		return lua.LNil, fmt.Errorf("Lua 值嵌套超过 %d 层", tableMaxDepth)
	}
	switch n.Kind {
	case tableKindNil, "":
		return lua.LNil, nil
	case tableKindBool:
		return lua.LBool(n.Bool), nil
	case tableKindInt:
		return lua.LNumber(n.Int), nil
	case tableKindFloat:
		return lua.LNumber(n.Float), nil
	case tableKindStr:
		return lua.LString(n.Str), nil
	case tableKindTable:
		tbl := g.L.NewTable()
		for _, pair := range n.Pairs {
			key, err := g.decodeTableValue(pair.Key, depth+1)
			if err != nil {
				return lua.LNil, fmt.Errorf("还原 table key: %w", err)
			}
			val, err := g.decodeTableValue(pair.Value, depth+1)
			if err != nil {
				return lua.LNil, fmt.Errorf("还原 table value: %w", err)
			}
			tbl.RawSet(key, val)
		}
		return tbl, nil
	default:
		return lua.LNil, fmt.Errorf("未知 Lua 值类型: %s", n.Kind)
	}
}

// Field 按字符串键读取 table 字段；非 table 或未命中返回 ok=false。
func (n TableNode) Field(key string) (TableNode, bool) {
	if n.Kind != tableKindTable {
		return TableNode{}, false
	}
	for _, pair := range n.Pairs {
		if pair.Key.Kind == tableKindStr && pair.Key.Str == key {
			return pair.Value, true
		}
	}
	return TableNode{}, false
}

// Index 按数字键读取 table 项（Lua 数组从 1 开始）。
func (n TableNode) Index(index int) (TableNode, bool) {
	if n.Kind != tableKindTable {
		return TableNode{}, false
	}
	for _, pair := range n.Pairs {
		if pair.Key.Kind == tableKindInt && pair.Key.Int == int64(index) {
			return pair.Value, true
		}
	}
	return TableNode{}, false
}

// AsInt 把整数/浮点/字符串数字节点转为整数；不可转换返回 0。
func (n TableNode) AsInt() int {
	switch n.Kind {
	case tableKindInt:
		return int(n.Int)
	case tableKindFloat:
		return int(n.Float)
	case tableKindStr:
		var v int64
		if _, err := fmt.Sscanf(n.Str, "%d", &v); err == nil {
			return int(v)
		}
	}
	return 0
}

var _ TableCodec = (*GopherLua)(nil)
