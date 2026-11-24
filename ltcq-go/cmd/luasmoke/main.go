// Command luasmoke 验证原版 Lua 资产能否被 gopher-lua 加载（M4 风险前置预研）。
//
// 两级验证：
//   - compile：仅语法编译（Parse），不执行。验证 Lua 5.1 语法兼容性。
//   - load：语法编译 + 实际执行，注入 C++ 全局 API 的 stub 与自定义 require loader。
//
// 用法：
//
//	luasmoke -dir <gameworld/data 目录> [-mode compile|load] [-verbose]
package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	lua "github.com/yuin/gopher-lua"
)

type result struct {
	path string
	err  error
}

func main() {
	dir := flag.String("dir", "", "gameworld/data 目录路径")
	mode := flag.String("mode", "compile", "compile=仅语法编译, load=编译+执行")
	verbose := flag.Bool("verbose", false, "打印每个失败详情")
	exts := flag.String("exts", ".lua,.config,.confg", "要扫描的扩展名（逗号分隔）")
	limit := flag.Int("limit", 0, "限制文件数量（0=全部）")
	flag.Parse()

	if *dir == "" {
		fmt.Fprintln(os.Stderr, "必须指定 -dir")
		os.Exit(2)
	}

	extSet := map[string]bool{}
	for _, e := range strings.Split(*exts, ",") {
		extSet[strings.TrimSpace(e)] = true
	}

	var files []string
	filepath.WalkDir(*dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if extSet[strings.ToLower(filepath.Ext(path))] {
			files = append(files, path)
		}
		return nil
	})
	sort.Strings(files)
	if *limit > 0 && *limit < len(files) {
		files = files[:*limit]
	}

	fmt.Printf("luasmoke mode=%s dir=%s files=%d\n", *mode, *dir, len(files))

	var results []result
	switch *mode {
	case "compile":
		results = compileAll(files)
	case "load":
		results = loadAll(files, *dir)
	default:
		fmt.Fprintf(os.Stderr, "未知 mode: %s\n", *mode)
		os.Exit(2)
	}

	ok, fail := 0, 0
	byErr := map[string]int{}
	for _, r := range results {
		if r.err == nil {
			ok++
			continue
		}
		fail++
		key := classify(r.err)
		byErr[key]++
		if *verbose {
			fmt.Printf("FAIL %s: %v\n", r.path, r.err)
		}
	}

	fmt.Printf("\n=== RESULT: ok=%d fail=%d total=%d (%.1f%%) ===\n",
		ok, fail, len(results), float64(ok)*100/float64(len(results)))
	keys := make([]string, 0, len(byErr))
	for k := range byErr {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Printf("  %-40s %d\n", k, byErr[k])
	}
	if fail > 0 {
		os.Exit(1)
	}
}

// compileAll 仅做语法编译，不执行。用独立 LState 避免状态污染。
// 原版 .config 多为裸表表达式 "{...}"（引擎当表达式 eval），LoadFile 按语句解析会误报，
// 故失败时回退尝试 "return <content>" 包装，确认其作为表达式合法。
func compileAll(files []string) []result {
	results := make([]result, 0, len(files))
	for _, f := range files {
		L := lua.NewState(lua.Options{SkipOpenLibs: true})
		_, err := L.LoadFile(f)
		L.Close()
		if err != nil {
			// 回退：当表达式编译
			content, rerr := os.ReadFile(f)
			if rerr == nil {
				L2 := lua.NewState(lua.Options{SkipOpenLibs: true})
				_, exprErr := L2.LoadString("return " + string(content))
				L2.Close()
				if exprErr == nil {
					err = nil // 作为表达式合法
				}
			}
		}
		results = append(results, result{f, err})
	}
	return results
}

// loadAll 实际执行每个文件，注入 stub 全局与自定义 require loader。
func loadAll(files []string, dataDir string) []result {
	results := make([]result, 0, len(files))
	for _, f := range files {
		L := newStubState(dataDir)
		err := L.DoFile(f)
		L.Close()
		results = append(results, result{f, err})
	}
	return results
}

// newStubState 创建一个注入了最小 stub 的 Lua 状态：
//   - 全部标准库（原版依赖 io/os/debug 等）
//   - 自定义 package.loader，按 dataDir 解析 require 路径
//   - 关键 C++ 全局 API 的空 stub（避免执行期 nil 调用 panic）
func newStubState(dataDir string) *lua.LState {
	L := lua.NewState()

	// 自定义 searcher：require 时从 dataDir 下按点分路径找 .lua
	searcher := func(L *lua.LState) int {
		modName := L.ToString(1)
		rel := filepath.Join(strings.Split(modName, ".")...)
		candidates := []string{
			filepath.Join(dataDir, rel+".lua"),
			filepath.Join(dataDir, "functions", rel+".lua"),
			filepath.Join(dataDir, rel, "init.lua"),
		}
		for _, c := range candidates {
			if _, err := os.Stat(c); err == nil {
				fn, err := L.LoadFile(c)
				if err == nil {
					L.Push(fn)
					L.Push(lua.LString(c))
					return 2
				}
			}
		}
		L.Push(lua.LString("\n\tno file for module '" + modName + "'"))
		return 1
	}
	// 替换 package.searchers（gopher-lua 中为 package.loaders 的等价表）
	pkg := L.GetField(L.GetField(L.Get(lua.EnvironIndex), "package"), "loaders")
	if tbl, ok := pkg.(*lua.LTable); ok {
		tbl.Insert(1, L.NewFunction(searcher))
	}

	// C++ 注入的全局 API stub（空函数或占位表）
	stubFuncs := []string{
		"System", "engineevent", "ActorEvent", "NetMsgDispatcher",
		"CSMsgDispatcher", "DBRetDispatcher", "MoneyEvent", "G",
		"print", "Log", "LOG", "Debug",
	}
	for _, name := range stubFuncs {
		L.SetGlobal(name, L.NewFunction(func(L *lua.LState) int { return 0 }))
	}
	// DevelopList = {}，luaex.tabContains 等会被调用
	L.SetGlobal("DevelopList", L.NewTable())

	// 加载原版 _G.lua 枚举（tt*/mt*/dt*/ae* 等），消除加载期 nil 比较错误。
	gPath := filepath.Join(dataDir, "functions", "_G.lua")
	if _, err := os.Stat(gPath); err == nil {
		if fn, err := L.LoadFile(gPath); err == nil {
			L.CallByParam(lua.P{Fn: fn, NRet: 0, Protect: true})
		}
	}

	// 屏蔽 io.open 等副作用？保留标准库（冒烟仅执行一次）
	_ = bufio.NewReader // 避免未使用 import
	return L
}

func classify(err error) string {
	s := err.Error()
	switch {
	case strings.Contains(s, "syntax error"):
		return "语法错误"
	case strings.Contains(s, "no file for module") || strings.Contains(s, "module '"):
		return "require 模块未找到"
	case strings.Contains(s, "attempt to call a nil value"):
		return "全局 API 缺失(nil call)"
	case strings.Contains(s, "attempt to index a nil value"):
		return "全局 API 缺失(nil index)"
	case strings.Contains(s, "setfenv") || strings.Contains(s, "getfenv"):
		return "setfenv/getfenv 不支持"
	default:
		return "其他: " + truncate(s, 60)
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
