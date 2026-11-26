// Package luahost 定义 Lua 宿主抽象（ADR-002）。
//
// Interpreter 接口把"换宿主"从架构手术降级为新增实现类：
// 上层 engineapi 只依赖此接口，gopher-lua 为首个实现，性能不足时切换 LuaJIT 零改动。
package luahost

import (
	"errors"
	"time"
)

// ErrTimeout 表示 Lua 调用超过看门狗时限（原版缺失的健壮性升级）。
var ErrTimeout = errors.New("luahost: call timeout")

// Value 是宿主无关的值抽象（engineapi 与 luahost 之间的桥梁）。
// gopher-lua 实现把它映射为 LValue；LuaJIT 实现映射为 lua_State 栈索引。
type Value interface{}

// Interpreter 是 Lua 宿主的最小接口（ADR-002:9）。
// 所有方法都必须由具体实现保证并发安全（或用外层锁序列化）。
type Interpreter interface {
	// Load 加载并执行一段 Lua 源码（已预处理的源码）。
	Load(chunkName, source string) error
	// LoadFile 加载并执行磁盘上的 .lua/.config 文件（内部做预处理）。
	LoadFile(path string) error
	// Call 调用全局函数，返回第一个返回值。无超时保护（调用方可控时用）。
	Call(global string, args ...Value) (Value, error)
	// PCall 带超时看门狗的调用，超时返回 ErrTimeout。
	PCall(timeout time.Duration, global string, args ...Value) (Value, error)
	// Register 注册一个 Go 函数到 Lua 全局（global 可含点号表示命名空间，如 "LActor.getActorId"）。
	Register(global string, fn func(...Value) (Value, error))
	// RegisterMulti 注册具有多个返回值的 Go 函数到 Lua 全局。
	RegisterMulti(global string, fn func(...Value) ([]Value, error))
	// SetTimeout 设置默认调用超时（0=禁用）。
	SetTimeout(timeout time.Duration)
	// DoString 执行任意 Lua 表达式并返回第一个结果（用于表达式求值的 .config）。
	DoString(source string) (Value, error)
	// Close 释放宿主资源。
	Close()
}

// Config 宿主创建参数。
type Config struct {
	// DataRoot 是 gameworld/data 目录，用于 include 解析与 LoadFile。
	DataRoot string
	// OSType 用于 #if OSTYPE == WIN 条件编译（"WIN"/"LINUX"）。
	OSType string
	// DefaultTimeout 默认 PCall 超时（0=禁用）。
	DefaultTimeout time.Duration
}
