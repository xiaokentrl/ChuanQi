// Package engineapi 实现 Lua↔Go 的 L* API 桥（registry 表驱动）。
//
// 契约单一事实源：docs/engine-api.md + docs/engine-api-matrix.tsv。
// 所有绑定走本文件的注册表，不手写 Register；CI 校验注册表 ↔ 文档不漂移。
// 分层依赖：cmd→domain→engineapi→luahost→netframe（depguard 只许向下）。
package engineapi

import (
	"fmt"
	"sort"

	"github.com/xiaokentrl/ChuanQi/ltcq-go/internal/luahost"
)

// Entry 是一条 API 注册项。
type Entry struct {
	Name    string // 全名，如 "LDataPack.writeInt"
	Fn      func(...luahost.Value) (luahost.Value, error)
	FnMulti func(...luahost.Value) ([]luahost.Value, error)
	// 语义注释格式（ONBOARDING.md:37）：
	// 原版: <C++位置> | Lua调用: N处 | 规格: engine-api.md#节名
	Meta string
}

// registry 是全局 API 注册表（声明式单一事实源）。
// T0 优先：前50个 API 占 81% 调用；T1/T2 长尾机械注册。
var registry []Entry

// Register 声明一条 API（包 init 时调用）。重名会 panic，防止漂移。
func Register(e Entry) {
	for _, x := range registry {
		if x.Name == e.Name {
			panic("engineapi: 重复注册 " + e.Name)
		}
	}
	registry = append(registry, e)
}

// Registry 返回只读快照（按名称排序，便于 CI 漂移校验）。
func Registry() []Entry {
	out := make([]Entry, len(registry))
	copy(out, registry)
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Count 返回已注册 API 数。
func Count() int { return len(registry) }

// currentInterp 保存当前解释器（单 VM 场景），供需要创建 Lua 值的 API（如 getStaticVar）使用。
// 由 InstallAll/Install 设置。
var currentInterp luahost.Interpreter

// CurrentInterpreter 返回当前安装的解释器。
func CurrentInterpreter() luahost.Interpreter { return currentInterp }

// InstallAll 把注册表全部 API 安装到解释器。
func InstallAll(L luahost.Interpreter) error {
	currentInterp = L
	for _, e := range registry {
		entry := e
		if entry.FnMulti != nil {
			L.RegisterMulti(entry.Name, entry.FnMulti)
		} else {
			L.Register(entry.Name, entry.Fn)
		}
	}
	return nil
}

// Install 只安装指定命名空间前缀的 API（便于分批加载/测试）。
func Install(L luahost.Interpreter, prefix string) (int, error) {
	n := 0
	for _, e := range registry {
		if len(prefix) > 0 && len(e.Name) >= len(prefix) && e.Name[:len(prefix)] == prefix {
			if e.FnMulti != nil {
				L.RegisterMulti(e.Name, e.FnMulti)
			} else {
				L.Register(e.Name, e.Fn)
			}
			n++
		}
	}
	if n == 0 && prefix != "" {
		return 0, fmt.Errorf("engineapi: 前缀 %s 无注册项", prefix)
	}
	return n, nil
}
