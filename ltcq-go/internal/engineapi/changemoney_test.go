package engineapi

import (
	"testing"

	"github.com/xiaokentrl/ChuanQi/ltcq-go/internal/luahost"
)

func TestChangeMoneyAlias(t *testing.T) {
	DefaultActorManager = NewActorManager()
	g := luahost.NewGopherLua(luahost.Config{OSType: "WIN"})
	defer g.Close()
	if err := InstallAll(g); err != nil {
		t.Fatal(err)
	}
	store := newMockStore()
	store.actors[101] = &ActorData{ActorID: 101, Gold: 50, YuanBao: 100}
	actor := DefaultActorManager.GetOrCreate(101)
	actor.Currency[currencyGold] = 50
	actor.Currency[currencyYuanBao] = 100
	actor.SetCurrencyStore(store)

	if _, err := g.Call("LActor.changeMoney", 101, currencyYuanBao, -30, 1, true, "limitbuy", "limitbuy", "itemid:1"); err != nil {
		t.Fatal(err)
	}
	if store.actors[101].YuanBao != 70 || actor.Currency[currencyYuanBao] != 70 {
		t.Fatalf("元宝变更错误: store=%d memory=%d", store.actors[101].YuanBao, actor.Currency[currencyYuanBao])
	}
	if store.currencyCalls != 1 || store.currencyType != currencyYuanBao || store.currencyAmount != 70 {
		t.Fatalf("存档调用错误: calls=%d type=%d amount=%d", store.currencyCalls, store.currencyType, store.currencyAmount)
	}

	if _, err := g.Call("LActor.changeMoney", 101, currencyGold, 10); err != nil {
		t.Fatal(err)
	}
	if store.actors[101].Gold != 60 || actor.Currency[currencyGold] != 60 {
		t.Fatalf("金币三参变更错误: store=%d memory=%d", store.actors[101].Gold, actor.Currency[currencyGold])
	}
}

func TestChangeMoneyFailureClosed(t *testing.T) {
	DefaultActorManager = NewActorManager()
	g := luahost.NewGopherLua(luahost.Config{OSType: "WIN"})
	defer g.Close()
	if err := InstallAll(g); err != nil {
		t.Fatal(err)
	}
	store := newMockStore()
	store.actors[101] = &ActorData{ActorID: 101, YuanBao: 100}
	actor := DefaultActorManager.GetOrCreate(101)
	actor.Currency[currencyYuanBao] = 100
	actor.SetCurrencyStore(store)

	cases := [][]luahost.Value{
		{101, currencyYuanBao},
		{9999, currencyYuanBao, -1},
		{101, currencyYuanBao, "10"},
		{101, currencyYuanBao, -200, 1, true, "limitbuy"},
	}
	for _, args := range cases {
		if _, err := g.Call("LActor.changeMoney", args...); err == nil {
			t.Errorf("非法 changeMoney 应返回错误: args=%v", args)
		}
	}
	if actor.Currency[currencyYuanBao] != 100 || store.actors[101].YuanBao != 100 {
		t.Fatalf("失败后状态被修改: memory=%d store=%d", actor.Currency[currencyYuanBao], store.actors[101].YuanBao)
	}
}
