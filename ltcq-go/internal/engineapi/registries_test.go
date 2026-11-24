package engineapi

import (
	"path/filepath"
	"testing"

	"github.com/xiaokentrl/ChuanQi/ltcq-go/internal/luahost"
)

func testDataRoot(t *testing.T) string {
	t.Helper()
	root := filepath.Join("..", "..", "..", "data", "server", "s1", "Debug", "gameworld", "data")
	if _, err := filepath.Abs(root); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestLoadRegistries(t *testing.T) {
	root := testDataRoot(t)
	g := luahost.NewGopherLua(luahost.Config{DataRoot: root, OSType: "WIN"})
	defer g.Close()

	if err := g.InjectEnums(); err != nil {
		t.Skipf("枚举注入失败（资产不可达）: %v", err)
	}
	InstallSystemStubs(g)
	loaded, err := LoadRegistries(g, root)
	if err != nil {
		t.Fatalf("加载注册表失败: %v", err)
	}
	if len(loaded) != 5 {
		t.Errorf("加载了 %d 个注册表，want 5: %v", len(loaded), loaded)
	}
	// 校验 On* 全局回调已挂载
	for _, m := range RegistryModules {
		for _, gname := range m.Globals {
			v, err := g.DoString("type(" + gname + ")")
			if err != nil {
				t.Errorf("%s.%s 校验失败: %v", m.Name, gname, err)
				continue
			}
			if v.(string) != "function" {
				t.Errorf("%s.%s type=%v want function", m.Name, gname, v)
			}
		}
	}
}

func TestFireActorEvent(t *testing.T) {
	root := testDataRoot(t)
	g := luahost.NewGopherLua(luahost.Config{DataRoot: root, OSType: "WIN"})
	defer g.Close()
	if err := g.InjectEnums(); err != nil {
		t.Skipf("资产不可达: %v", err)
	}
	InstallSystemStubs(g)
	if _, err := LoadRegistries(g, root); err != nil {
		t.Fatal(err)
	}

	// 注册一个 actorevent 回调（通过 Lua 直接调 actorevent.reg）
	err := g.Load("regtest", `
		received = nil
		actorevent.reg(aeUserLogin, function(actor, arg)
			received = actor
		end)
	`)
	if err != nil {
		t.Fatalf("注册回调失败: %v", err)
	}

	// Go 触发 OnActorEvent(actor, eid, ...)
	// 注册时 aeUserLogin 应为在 _G.lua 注入的数字
	eid, err := g.DoString("aeUserLogin")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Fire(g, "OnActorEvent", 999, eid); err != nil {
		t.Fatalf("触发 OnActorEvent 失败: %v", err)
	}
	got, err := g.DoString("received")
	if err != nil {
		t.Fatal(err)
	}
	if got.(int) != 999 {
		t.Errorf("回调收到 actor=%v want 999", got)
	}
}

func TestFireNetMsg(t *testing.T) {
	root := testDataRoot(t)
	g := luahost.NewGopherLua(luahost.Config{DataRoot: root, OSType: "WIN"})
	defer g.Close()
	if err := g.InjectEnums(); err != nil {
		t.Skipf("资产不可达: %v", err)
	}
	InstallSystemStubs(g)
	if _, err := LoadRegistries(g, root); err != nil {
		t.Fatal(err)
	}

	// 注册 netmsg 回调并触发
	err := g.Load("nettest", `
		netReceived = nil
		netmsgdispatcher.reg(10, 20, function(actor, pack)
			netReceived = pack
		end)
	`)
	if err != nil {
		t.Fatalf("注册 netmsg 失败: %v", err)
	}
	if _, err := Fire(g, "OnNetMsg", 10, 20, 1, 888); err != nil {
		t.Fatalf("触发 OnNetMsg 失败: %v", err)
	}
	got, err := g.DoString("netReceived")
	if err != nil {
		t.Fatal(err)
	}
	if got.(int) != 888 {
		t.Errorf("netmsg 回调收到 pack=%v want 888", got)
	}
}

func TestFirePCallTimeout(t *testing.T) {
	root := testDataRoot(t)
	g := luahost.NewGopherLua(luahost.Config{DataRoot: root, OSType: "WIN"})
	defer g.Close()
	if err := g.InjectEnums(); err != nil {
		t.Skipf("资产不可达: %v", err)
	}
	InstallSystemStubs(g)
	if _, err := LoadRegistries(g, root); err != nil {
		t.Fatal(err)
	}
	// 注册一个死循环回调，验证超时保护
	g.Load("spintest", `
		actorevent.reg(aeUserLogout, function(actor) while true do end end)
	`)
	eid, _ := g.DoString("aeUserLogout")
	_, err := FirePCall(g, 100, "OnActorEvent", 1, eid)
	if err == nil {
		t.Error("死循环回调应触发超时")
	}
}
