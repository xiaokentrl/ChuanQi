package luahost

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	lua "github.com/yuin/gopher-lua"
)

// GopherLua 是 Interpreter 的 gopher-lua 实现（ADR-002 首选）。
type GopherLua struct {
	L       *lua.LState
	pre     *Preprocessor
	timeout time.Duration
	callMu  chan struct{} // 串行化 Call/PCall，避免并发改动 VM 栈
}

// NewGopherLua 创建基于 gopher-lua 的宿主。
func NewGopherLua(cfg Config) *GopherLua {
	ostype := cfg.OSType
	if ostype == "" {
		ostype = "WIN"
	}
	g := &GopherLua{
		L:       lua.NewState(),
		pre:     NewPreprocessor(cfg.DataRoot, ostype),
		timeout: cfg.DefaultTimeout,
		callMu:  make(chan struct{}, 1),
	}
	g.installRequireLoader(cfg.DataRoot)
	return g
}

// installRequireLoader 让 require("utils.common") 从 dataRoot 解析。
// 原版模块布局：data/functions/<点分路径>.lua，如 utils/common.lua。
func (g *GopherLua) installRequireLoader(dataRoot string) {
	if dataRoot == "" {
		return
	}
	searcher := func(L *lua.LState) int {
		modName := L.ToString(1)
		rel := strings.ReplaceAll(modName, ".", string(filepath.Separator))
		candidates := []string{
			filepath.Join(dataRoot, "functions", rel+".lua"),
			filepath.Join(dataRoot, "functions", rel, "init.lua"),
			filepath.Join(dataRoot, rel+".lua"),
		}
		for _, c := range candidates {
			if _, err := os.Stat(c); err == nil {
				fn, err := L.LoadFile(c) // 走 gopher-lua 原生编译（模块源码含 --# 指令时由 preprocess 处理）
				if err == nil {
					L.Push(fn)
					L.Push(lua.LString(c))
					return 2
				}
			}
		}
		L.Push(lua.LString("\n\tno field module '" + modName + "' in dataRoot"))
		return 1
	}
	pkg := g.L.GetField(g.L.GetField(g.L.Get(lua.EnvironIndex), "package"), "loaders")
	if tbl, ok := pkg.(*lua.LTable); ok {
		tbl.Insert(1, g.L.NewFunction(searcher))
	}
}

// Compile 只编译不执行（M4 冒烟用）。
func (g *GopherLua) Compile(chunkName, source string) error {
	fn, err := g.L.LoadString(source)
	if err != nil {
		return fmt.Errorf("%s: %w", chunkName, err)
	}
	if fn != nil {
		g.L.Pop(1)
	}
	return nil
}

func (g *GopherLua) Load(chunkName, source string) error {
	fn, err := g.L.LoadString(source)
	if err != nil {
		return fmt.Errorf("%s: %w", chunkName, err)
	}
	if err := g.L.CallByParam(lua.P{Fn: fn, NRet: 0, Protect: true}); err != nil {
		return fmt.Errorf("%s 执行: %w", chunkName, err)
	}
	return nil
}

func (g *GopherLua) LoadFile(path string) error {
	// path 是调用方给的文件路径（通常绝对），预处理器的 include 相对 data root 解析。
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	src, err := g.pre.Process(abs)
	if err != nil {
		return err
	}
	loadErr := g.Load(path, src)
	if loadErr == nil {
		return nil
	}
	// 裸表表达式回退：仅当首非注释字符是 '{' 时（原版 .config 的 {...} 形式，
	// 引擎按表达式 eval）。其他文件的错误原样返回，避免掩盖真实问题。
	trimmed := strings.TrimSpace(src)
	if !strings.HasPrefix(trimmed, "{") {
		return loadErr
	}
	return g.Load(path, "return "+src)
}

func (g *GopherLua) DoString(source string) (Value, error) {
	fn, err := g.L.LoadString("return " + source)
	if err != nil {
		return nil, err
	}
	if err := g.L.CallByParam(lua.P{Fn: fn, NRet: 1, Protect: true}); err != nil {
		return nil, err
	}
	return fromLValue(g.L.Get(-1)), nil
}

func (g *GopherLua) Call(global string, args ...Value) (Value, error) {
	g.callMu <- struct{}{}
	defer func() { <-g.callMu }()

	fn := g.getGlobalPath(global)
	if fn == lua.LNil {
		return nil, fmt.Errorf("全局函数未定义: %s", global)
	}
	largs := make([]lua.LValue, 0, len(args))
	for _, a := range args {
		largs = append(largs, toLValue(g.L, a))
	}
	if err := g.L.CallByParam(lua.P{Fn: fn, NRet: 1, Protect: true}, largs...); err != nil {
		return nil, err
	}
	return fromLValue(g.L.Get(-1)), nil
}

// PCall 带超时看门狗。Lua VM 非线程安全，故用 channel 在独立 goroutine 内执行，
// 主 goroutine select 超时——注意超时后 VM 状态可能不一致，调用方应视为致命错误。
func (g *GopherLua) PCall(timeout time.Duration, global string, args ...Value) (Value, error) {
	if timeout <= 0 {
		timeout = g.timeout
	}
	if timeout <= 0 {
		return g.Call(global, args...)
	}
	type res struct {
		v   Value
		err error
	}
	ch := make(chan res, 1)
	go func() {
		v, err := g.Call(global, args...)
		ch <- res{v, err}
	}()
	t := time.NewTimer(timeout)
	defer t.Stop()
	select {
	case r := <-ch:
		return r.v, r.err
	case <-t.C:
		return nil, fmt.Errorf("%w: %s 超过 %v", ErrTimeout, global, timeout)
	}
}

func (g *GopherLua) Register(global string, fn func(...Value) (Value, error)) {
	g.registerValues(global, func(args ...Value) ([]Value, error) {
		ret, err := fn(args...)
		if err != nil || ret == nil {
			return nil, err
		}
		return []Value{ret}, nil
	})
}

func (g *GopherLua) RegisterMulti(global string, fn func(...Value) ([]Value, error)) {
	g.registerValues(global, fn)
}

func (g *GopherLua) registerValues(global string, fn func(...Value) ([]Value, error)) {
	wrapper := func(L *lua.LState) int {
		args := make([]Value, 0, L.GetTop())
		for i := 1; i <= L.GetTop(); i++ {
			args = append(args, fromLValue(L.Get(i)))
		}
		rets, err := fn(args...)
		if err != nil {
			L.RaiseError("%v", err)
			return 0
		}
		for _, ret := range rets {
			L.Push(toLValue(L, ret))
		}
		return len(rets)
	}
	g.setGlobalPath(global, g.L.NewFunction(wrapper))
}

// setGlobalPath 支持点号路径（如 LActor.getActorId）：逐级创建表。
func (g *GopherLua) setGlobalPath(path string, val lua.LValue) {
	parts := strings.Split(path, ".")
	if len(parts) == 1 {
		g.L.SetGlobal(path, val)
		return
	}
	tbl, ok := g.L.GetGlobal(parts[0]).(*lua.LTable)
	if !ok {
		tbl = g.L.NewTable()
		g.L.SetGlobal(parts[0], tbl)
	}
	for _, p := range parts[1 : len(parts)-1] {
		next, ok := g.L.GetField(tbl, p).(*lua.LTable)
		if !ok {
			next = g.L.NewTable()
			g.L.SetField(tbl, p, next)
		}
		tbl = next
	}
	g.L.SetField(tbl, parts[len(parts)-1], val)
}

// getGlobalPath 支持点号路径读取（如 "LActor.getActorId"）。
func (g *GopherLua) getGlobalPath(path string) lua.LValue {
	parts := strings.Split(path, ".")
	if len(parts) == 1 {
		return g.L.GetGlobal(path)
	}
	cur := g.L.GetGlobal(parts[0])
	for _, p := range parts[1:] {
		tbl, ok := cur.(*lua.LTable)
		if !ok {
			return lua.LNil
		}
		cur = g.L.GetField(tbl, p)
	}
	return cur
}

func (g *GopherLua) SetTimeout(timeout time.Duration) { g.timeout = timeout }
func (g *GopherLua) Close()                           { g.L.Close() }

// Value 转换：gopher-lua LValue ↔ 宿主无关 Value。
func toLValue(L *lua.LState, v Value) lua.LValue {
	switch x := v.(type) {
	case nil:
		return lua.LNil
	case bool:
		return lua.LBool(x)
	case int:
		return lua.LNumber(x)
	case int64:
		return lua.LNumber(x)
	case float64:
		return lua.LNumber(x)
	case string:
		return lua.LString(x)
	case []byte:
		return lua.LString(string(x))
	case *lua.LTable:
		return x
	case lua.LValue:
		return x
	default:
		return lua.LString(fmt.Sprint(v))
	}
}

func fromLValue(v lua.LValue) Value {
	switch x := v.(type) {
	case *lua.LNilType:
		return nil
	case lua.LBool:
		return bool(x)
	case lua.LNumber:
		f := float64(x)
		if math.IsNaN(f) || math.IsInf(f, 0) || math.Trunc(f) != f {
			return f
		}
		if strconv.IntSize == 32 {
			if f >= math.MinInt32 && f <= math.MaxInt32 {
				return int(f)
			}
		} else if f >= -math.Exp2(63) && f < math.Exp2(63) {
			return int(f)
		}
		return f
	case lua.LString:
		return string(x)
	case *lua.LTable:
		return x
	default:
		return v.String()
	}
}

var _ Interpreter = (*GopherLua)(nil)
