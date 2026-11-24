package engineapi

import (
	"testing"

	"github.com/xiaokentrl/ChuanQi/ltcq-go/internal/luahost"
)

func TestActor_CurrencyAndItems(t *testing.T) {
	DefaultActorManager = NewActorManager() // 重置
	g := luahost.NewGopherLua(luahost.Config{OSType: "WIN"})
	defer g.Close()
	if err := InstallAll(g); err != nil {
		t.Fatal(err)
	}

	// 变更货币 mtYuanbao=3，+100
	_, err := g.Call("LActor.changeCurrency", 1, 3, 100)
	if err != nil {
		t.Fatal(err)
	}
	v, err := g.Call("LActor.getCurrency", 1, 3)
	if err != nil || v.(int) != 100 {
		t.Errorf("currency=%v err=%v want 100", v, err)
	}

	// 物品：给 10 个（直接操作实体），costItem 扣 4 成功、再扣 7 失败
	a := DefaultActorManager.GetOrCreate(1)
	a.Items[1001] = 10
	ok, err := g.Call("LActor.costItem", 1, 1001, 4)
	if err != nil || ok.(bool) != true {
		t.Errorf("costItem(4)=%v err=%v want true", ok, err)
	}
	ok, err = g.Call("LActor.costItem", 1, 1001, 7)
	if err != nil || ok.(bool) != false {
		t.Errorf("costItem(7)=%v err=%v want false（不足不扣）", ok, err)
	}
	v, _ = g.Call("LActor.getItemCount", 1, 1001)
	if v.(int) != 6 {
		t.Errorf("剩余物品=%v want 6", v)
	}
}

func TestActor_StaticVarRoundTrip(t *testing.T) {
	DefaultActorManager = NewActorManager()
	root := testDataRoot(t)
	g := luahost.NewGopherLua(luahost.Config{DataRoot: root, OSType: "WIN"})
	defer g.Close()
	if err := InstallAll(g); err != nil {
		t.Fatal(err)
	}

	// Lua 写 staticVar（模拟业务脚本）
	err := g.Load("write", `
		local st = LActor.getStaticVar(42)
		st.kills = 7
		st.level = 30
	`)
	if err != nil {
		t.Fatal(err)
	}
	// 同一 id 再读（模拟"重登"）
	v, err := g.DoString(`(function() local st = LActor.getStaticVar(42); return st.kills end)()`)
	if err != nil {
		t.Fatal(err)
	}
	if v.(int) != 7 {
		t.Errorf("staticVar.kills=%v want 7", v)
	}
	actor := DefaultActorManager.Get(42)
	if actor == nil || !actor.Dirty {
		t.Errorf("staticVar 写后应标脏: %+v", actor)
	}
}

func TestActor_GetLevelAndName(t *testing.T) {
	DefaultActorManager = NewActorManager()
	g := luahost.NewGopherLua(luahost.Config{OSType: "WIN"})
	defer g.Close()
	InstallAll(g)

	a := DefaultActorManager.GetOrCreate(7)
	a.Level = 55
	a.Name = "测试玩家"

	v, _ := g.Call("LActor.getLevel", 7)
	if v.(int) != 55 {
		t.Errorf("level=%v want 55", v)
	}
	v, _ = g.Call("LActor.getName", 7)
	if v.(string) != "测试玩家" {
		t.Errorf("name=%v want 测试玩家", v)
	}
}
