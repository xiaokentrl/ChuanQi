package engineapi

import (
	"errors"
	"testing"

	"github.com/xiaokentrl/ChuanQi/ltcq-go/internal/luahost"
)

func TestSetIntPropertyMemoryFields(t *testing.T) {
	DefaultActorManager = NewActorManager()
	g := luahost.NewGopherLua(luahost.Config{OSType: "WIN"})
	defer g.Close()
	if err := InstallAll(g); err != nil {
		t.Fatal(err)
	}
	actor := DefaultActorManager.GetOrCreate(101)

	cases := []struct {
		name     string
		property int
		value    int
	}{
		{"level", propertyLevel, 30},
		{"sex", propertySex, 1},
		{"vocation", propertyVocation, 2},
		{"guild", propertyGuildID, 77},
		{"exp", propertyExp, 12345},
		{"fight", propertyFight, 8888},
		{"recharge", propertyRecharge, 66},
	}
	for _, tc := range cases {
		if _, err := g.Call("LActor.setIntProperty", 101, tc.property, tc.value); err != nil {
			t.Fatalf("%s 写入失败: %v", tc.name, err)
		}
		got, err := g.Call("LActor.getIntProperty", 101, tc.property)
		if err != nil || got.(int) != tc.value {
			t.Errorf("%s 回读=%v err=%v want %d", tc.name, got, err, tc.value)
		}
	}
	if !actor.Dirty {
		t.Error("内存字段写入后应标脏")
	}
}

func TestSetIntPropertyWriteThrough(t *testing.T) {
	DefaultActorManager = NewActorManager()
	g := luahost.NewGopherLua(luahost.Config{OSType: "WIN"})
	defer g.Close()
	if err := InstallAll(g); err != nil {
		t.Fatal(err)
	}
	store := newMockStore()
	store.actors[101] = &ActorData{ActorID: 101}
	actor := DefaultActorManager.GetOrCreate(101)
	actor.SetCurrencyStore(store)
	actor.SetVIPStore(store)

	if _, err := g.Call("LActor.setIntProperty", 101, propertyCoin, 500); err != nil {
		t.Fatal(err)
	}
	if store.actors[101].Gold != 500 || actor.Currency[currencyGold] != 500 {
		t.Fatalf("金币写入结果错误: store=%d memory=%d", store.actors[101].Gold, actor.Currency[currencyGold])
	}
	if store.currencyCalls != 1 || store.currencyType != currencyGold || store.currencyAmount != 500 {
		t.Fatalf("金币存档调用错误: calls=%d type=%d amount=%d", store.currencyCalls, store.currencyType, store.currencyAmount)
	}

	if _, err := g.Call("LActor.setIntProperty", 101, propertyYB, 30); err != nil {
		t.Fatal(err)
	}
	if store.actors[101].YuanBao != 30 || actor.Currency[currencyYuanBao] != 30 {
		t.Fatalf("元宝写入结果错误: store=%d memory=%d", store.actors[101].YuanBao, actor.Currency[currencyYuanBao])
	}

	if _, err := g.Call("LActor.setIntProperty", 101, propertyBindCoin, 7); err != nil {
		t.Fatal(err)
	}
	if actor.Currency[0] != 7 {
		t.Fatalf("绑定金币内存值=%d want 7", actor.Currency[0])
	}
	if store.currencyCalls != 2 {
		t.Fatalf("绑定币种不应落 actors 表: calls=%d want 2", store.currencyCalls)
	}

	if _, err := g.Call("LActor.setIntProperty", 101, propertyVIPLevel, 6); err != nil {
		t.Fatal(err)
	}
	if store.actors[101].VIPLevel != 6 || actor.VIPLevel != 6 {
		t.Fatalf("VIP 写入结果错误: store=%d memory=%d", store.actors[101].VIPLevel, actor.VIPLevel)
	}
}

func TestSetIntPropertyFailureClosed(t *testing.T) {
	DefaultActorManager = NewActorManager()
	g := luahost.NewGopherLua(luahost.Config{OSType: "WIN"})
	defer g.Close()
	if err := InstallAll(g); err != nil {
		t.Fatal(err)
	}
	store := newMockStore()
	store.actors[101] = &ActorData{ActorID: 101, Gold: 100, VIPLevel: 2}
	actor := DefaultActorManager.GetOrCreate(101)
	actor.Level = 5
	actor.Currency[currencyGold] = 100
	actor.VIPLevel = 2
	actor.SetCurrencyStore(store)
	actor.SetVIPStore(store)

	cases := [][]luahost.Value{
		{},
		{101, propertyLevel},
		{101, 999, 10},
		{0, propertyLevel, 10},
		{-1, propertyLevel, 10},
		{101, "6", 10},
		{101, propertyLevel, "10"},
		{101, propertyLevel, 6.5},
		{101, propertyLevel, 0},
		{101, propertyID, 999},
		{101, propertyPosX, 999},
		{101, propertyPosY, 999},
		{101, propertyHP, 999},
		{101, propertyMaxHP, 999},
		{101, propertyCoin, -1},
		{101, propertyVIPLevel, -1},
		{9999, propertyLevel, 10},
	}
	for _, args := range cases {
		if _, err := g.Call("LActor.setIntProperty", args...); err != nil {
			t.Errorf("非法参数不应抛错: args=%v err=%v", args, err)
		}
	}
	if actor.Level != 5 || actor.Currency[currencyGold] != 100 || actor.VIPLevel != 2 || store.currencyCalls != 0 {
		t.Fatalf("失败关闭后状态被修改: actor=%+v calls=%d", actor, store.currencyCalls)
	}

	store.currencySaveErr = errors.New("存档失败")
	if _, err := g.Call("LActor.setIntProperty", 101, propertyCoin, 200); err != nil {
		t.Fatal(err)
	}
	if actor.Currency[currencyGold] != 100 || store.actors[101].Gold != 100 {
		t.Fatalf("存档失败不应改内存: memory=%d store=%d", actor.Currency[currencyGold], store.actors[101].Gold)
	}
}
