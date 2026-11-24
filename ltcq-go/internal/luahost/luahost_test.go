package luahost

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeTmp(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	os.MkdirAll(filepath.Dir(p), 0755)
	if err := os.WriteFile(p, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestPreprocessor_IncludeOnce(t *testing.T) {
	dir := t.TempDir()
	writeTmp(t, dir, "inc.lua", "INC = 1\n")
	main := writeTmp(t, dir, "main.lua", "--#include \"inc.lua\" once\n--#include \"inc.lua\" once\nX = INC\n")

	p := NewPreprocessor(dir, "WIN")
	src, err := p.Process(main)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(src, "INC = 1") != 1 {
		t.Errorf("include once 未生效，src=%q", src)
	}
}

func TestPreprocessor_PragmaOnce(t *testing.T) {
	dir := t.TempDir()
	writeTmp(t, dir, "guard.lua", "--#pragma once\nG = 1\n")
	main := writeTmp(t, dir, "main.lua", "--#include \"guard.lua\"\n--#include \"guard.lua\"\n")

	p := NewPreprocessor(dir, "WIN")
	src, err := p.Process(main)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(src, "G = 1") != 1 {
		t.Errorf("pragma once 未生效，src=%q", src)
	}
}

func TestPreprocessor_IfndefGuard(t *testing.T) {
	dir := t.TempDir()
	main := writeTmp(t, dir, "main.lua",
		"--#ifndef MY_GUARD\n"+
			"--#define MY_GUARD 1\n"+
			"SHOULD_APPEAR = 1\n"+
			"--#endif // end guard\n")

	p := NewPreprocessor(dir, "WIN")
	src, err := p.Process(main)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(src, "SHOULD_APPEAR = 1") {
		t.Errorf("ifndef 首次应通过，src=%q", src)
	}
}

func TestPreprocessor_OSTypeConditional(t *testing.T) {
	dir := t.TempDir()
	main := writeTmp(t, dir, "main.lua",
		"--#if OSTYPE == WIN\n"+
			"PLATFORM = \"win\"\n"+
			"--#endif\n")

	pWin := NewPreprocessor(dir, "WIN")
	srcWin, _ := pWin.Process(main)
	if !strings.Contains(srcWin, "PLATFORM") {
		t.Errorf("WIN 应启用分支，src=%q", srcWin)
	}
	pLin := NewPreprocessor(dir, "LINUX")
	srcLin, _ := pLin.Process(main)
	if strings.Contains(srcLin, "PLATFORM") {
		t.Errorf("LINUX 应跳过分支，src=%q", srcLin)
	}
}

func TestPreprocessor_DefineConstant(t *testing.T) {
	dir := t.TempDir()
	main := writeTmp(t, dir, "main.lua",
		"--#define MAX_COUNT 256\n"+
			"N = --#define MAX_COUNT 256\n"+
			"LIMIT = 256\n")

	p := NewPreprocessor(dir, "WIN")
	src, err := p.Process(main)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(src, "LIMIT = 256") {
		t.Errorf("define 分支输出异常，src=%q", src)
	}
}

func TestPreprocessor_CaseInsensitivePath(t *testing.T) {
	dir := t.TempDir()
	writeTmp(t, dir, "LangCode.lua", "LANG = 1\n")
	main := writeTmp(t, dir, "main.lua", "--#include \"langcode.lua\"\n")

	p := NewPreprocessor(dir, "WIN")
	src, err := p.Process(main)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(src, "LANG = 1") {
		t.Errorf("大小写不敏感 include 失败，src=%q", src)
	}
}

func TestGopherLua_LoadAndCall(t *testing.T) {
	g := NewGopherLua(Config{OSType: "WIN"})
	defer g.Close()

	if err := g.Load("test", "function add(a,b) return a+b end\n"); err != nil {
		t.Fatal(err)
	}
	v, err := g.Call("add", 2, 3)
	if err != nil {
		t.Fatal(err)
	}
	if v.(int) != 5 {
		t.Errorf("add(2,3)=%v want 5", v)
	}
}

func TestGopherLua_RegisterNamespace(t *testing.T) {
	g := NewGopherLua(Config{OSType: "WIN"})
	defer g.Close()

	g.Register("LActor.getActorId", func(args ...Value) (Value, error) {
		return 12345, nil
	})
	v, err := g.DoString("LActor.getActorId()")
	if err != nil {
		t.Fatal(err)
	}
	if v.(int) != 12345 {
		t.Errorf("getActorId=%v want 12345", v)
	}
}

func TestGopherLua_PCallTimeout(t *testing.T) {
	g := NewGopherLua(Config{OSType: "WIN"})
	defer g.Close()

	// 死循环函数：验证 500ms 看门狗生效
	if err := g.Load("loop", "function spin() while true do end end\n"); err != nil {
		t.Fatal(err)
	}
	_, err := g.PCall(100*time.Millisecond, "spin")
	if err == nil || !strings.Contains(err.Error(), "timeout") {
		t.Errorf("应返回超时错误，got %v", err)
	}
}

func TestGopherLua_InjectEnums(t *testing.T) {
	root := filepath.Join("..", "..", "..", "data", "server", "s1", "Debug", "gameworld", "data")
	gPath := filepath.Join(root, "functions", "_G.lua")
	if _, err := os.Stat(gPath); os.IsNotExist(err) {
		t.Skip("真实资产目录不可达")
	}
	g := NewGopherLua(Config{DataRoot: root, OSType: "WIN"})
	defer g.Close()
	if err := g.InjectEnums(); err != nil {
		t.Fatalf("注入枚举失败: %v", err)
	}
	tests := []struct {
		name string
		want int
	}{
		{"ttScreenCenter", 2},
		{"ttTipmsgWindow", 1},
		{"mtBindCoin", 0},
		{"mtYuanbao", 3},
		{"dtByte", 0},
		{"dtString", 8},
		{"qatYuanbao", 15},
	}
	for _, tt := range tests {
		v, err := g.EnumValue(tt.name)
		if err != nil {
			t.Errorf("%s 读取失败: %v", tt.name, err)
			continue
		}
		if v.(int) != tt.want {
			t.Errorf("%s=%v want %d", tt.name, v, tt.want)
		}
	}
	// C++ 全局表占位应非 nil
	v, err := g.DoString("SystemId.enCommonSystemID")
	if err != nil || v == nil || v.(int) == 0 {
		t.Errorf("SystemId.enCommonSystemID=%v err=%v", v, err)
	}
}

func TestGopherLua_LoadRealConfig(t *testing.T) {
	// 用真实 actorloginconf.lua 验证加载链（含中文注释与裸表赋值）
	root := filepath.Join("..", "..", "..", "data", "server", "s1", "Debug", "gameworld", "data")
	path := filepath.Join(root, "config", "actorloginconf", "actorloginconf.lua")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Skip("真实资产目录不可达")
	}
	g := NewGopherLua(Config{DataRoot: root, OSType: "WIN"})
	defer g.Close()
	if err := g.LoadFile(path); err != nil {
		t.Fatalf("加载真实配置失败: %v", err)
	}
	v, err := g.DoString("ActorLoginConf.defHpStone")
	if err != nil {
		t.Fatal(err)
	}
	if v.(int) != 5000 {
		t.Errorf("defHpStone=%v want 5000", v)
	}
}
