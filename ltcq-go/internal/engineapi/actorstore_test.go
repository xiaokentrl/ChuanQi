package engineapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"sync"
	"testing"

	"github.com/xiaokentrl/ChuanQi/ltcq-go/internal/luahost"
	"github.com/xiaokentrl/ChuanQi/ltcq-go/internal/scene"
)

func TestActor_CurrencyAndItems(t *testing.T) {
	DefaultActorManager = NewActorManager() // 重置
	g := luahost.NewGopherLua(luahost.Config{OSType: "WIN"})
	defer g.Close()
	if err := InstallAll(g); err != nil {
		t.Fatal(err)
	}

	store := newMockStore()
	store.actors[1] = &ActorData{ActorID: 1}
	actor := DefaultActorManager.GetOrCreate(1)
	actor.SetCurrencyStore(store)

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

func TestActor_QueryZhuanShengAndVIP(t *testing.T) {
	DefaultActorManager = NewActorManager()
	g := luahost.NewGopherLua(luahost.Config{OSType: "WIN"})
	defer g.Close()
	if err := InstallAll(g); err != nil {
		t.Fatal(err)
	}

	actor := DefaultActorManager.GetOrCreate(7)
	actor.ZhuanSheng = 3
	actor.VIPLevel = 5
	for _, tc := range []struct {
		name string
		want int
	}{
		{"LActor.getZhuanShengLevel", 3},
		{"LActor.getVipLevel", 5},
	} {
		v, err := g.Call(tc.name, 7)
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if v != tc.want {
			t.Errorf("%s=%v want %d", tc.name, v, tc.want)
		}
		v, err = g.Call(tc.name)
		if err != nil || v != 0 {
			t.Errorf("%s 无参数=%v err=%v want 0", tc.name, v, err)
		}
		v, err = g.Call(tc.name, 999)
		if err != nil || v != 0 {
			t.Errorf("%s 离线=%v err=%v want 0", tc.name, v, err)
		}
	}
	if DefaultActorManager.Get(999) != nil {
		t.Error("离线查询不应创建 Actor")
	}
}

func TestActor_QueryJobSexSceneAndExp(t *testing.T) {
	DefaultActorManager = NewActorManager()
	g := luahost.NewGopherLua(luahost.Config{OSType: "WIN"})
	defer g.Close()
	if err := InstallAll(g); err != nil {
		t.Fatal(err)
	}

	actor := DefaultActorManager.GetOrCreate(7)
	actor.Job = 2
	actor.Sex = 1
	actor.SceneID = 41
	actor.Exp = 123456
	for _, tc := range []struct {
		name string
		want int
	}{
		{"LActor.getJob", 2},
		{"LActor.getActorJob", 2},
		{"LActor.getSex", 1},
		{"LActor.getActorSex", 1},
		{"LActor.getSceneId", 41},
		{"LActor.getExp", 123456},
	} {
		v, err := g.Call(tc.name, 7)
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if v != tc.want {
			t.Errorf("%s=%v want %d", tc.name, v, tc.want)
		}
		v, err = g.Call(tc.name)
		if err != nil || v != 0 {
			t.Errorf("%s 无参数=%v err=%v want 0", tc.name, v, err)
		}
		v, err = g.Call(tc.name, 999)
		if err != nil || v != 0 {
			t.Errorf("%s 离线=%v err=%v want 0", tc.name, v, err)
		}
	}
}

func TestActor_QueryAccountServerAndPowerFromLogin(t *testing.T) {
	DefaultActorManager = NewActorManager()
	g := luahost.NewGopherLua(luahost.Config{OSType: "WIN"})
	defer g.Close()
	if err := InstallAll(g); err != nil {
		t.Fatal(err)
	}

	manager := &postLoginManager{}
	manager.applyActorDataAt(&ActorData{
		ActorID: 7, Account: "query_test", AccountID: 121102, ServerID: 9,
		Level: 55, TotalPower: 67890, ZhuanSheng: 3,
	}, sceneSpawnResult{})
	for _, tc := range []struct {
		name string
		want any
	}{
		{"LActor.getServerId", 9},
		{"LActor.getAccountName", "query_test"},
		{"LActor.getActorPower", 67890},
		{"LActor.getAccountId", 121102},
		{"LActor.getActorLevel", 55},
		{"LActor.getActorZhuanShengLevel", 3},
	} {
		v, err := g.Call(tc.name, 7)
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if v != tc.want {
			t.Errorf("%s=%v want %v", tc.name, v, tc.want)
		}
	}
	for _, name := range []string{
		"LActor.getServerId", "LActor.getActorPower", "LActor.getAccountId",
		"LActor.getActorLevel", "LActor.getActorZhuanShengLevel",
	} {
		for _, args := range [][]luahost.Value{nil, {999}} {
			v, err := g.Call(name, args...)
			if err != nil || v != 0 {
				t.Errorf("%s args=%v 得到 %v err=%v want 0", name, args, v, err)
			}
		}
	}
	for _, args := range [][]luahost.Value{nil, {999}} {
		v, err := g.Call("LActor.getAccountName", args...)
		if err != nil || v != "" {
			t.Errorf("getAccountName args=%v 得到 %v err=%v want empty", args, v, err)
		}
	}
	if DefaultActorManager.Get(999) != nil {
		t.Error("查询不应创建离线 Actor")
	}
}

func TestActor_QueryLoginMetadata(t *testing.T) {
	DefaultActorManager = NewActorManager()
	g := luahost.NewGopherLua(luahost.Config{OSType: "WIN"})
	defer g.Close()
	if err := InstallAll(g); err != nil {
		t.Fatal(err)
	}

	(&postLoginManager{}).applyActorDataAt(&ActorData{
		ActorID: 7, Recharge: 12345, CreateTime: 1700000000,
		AppID: "app_test", PfID: "pf_test",
	}, sceneSpawnResult{})
	for _, tc := range []struct {
		name string
		want any
	}{
		{"LActor.getRecharge", 12345},
		{"LActor.getCreateTime", 1700000000},
		{"LActor.getAppid", "app_test"},
		{"LActor.getPf", "pf_test"},
		{"LActor.getPfId", "pf_test"},
	} {
		v, err := g.Call(tc.name, 7)
		if err != nil || v != tc.want {
			t.Errorf("%s=%v err=%v want %v", tc.name, v, err, tc.want)
		}
		for _, args := range [][]luahost.Value{nil, {999}} {
			v, err := g.Call(tc.name, args...)
			want := any(0)
			if _, ok := tc.want.(string); ok {
				want = ""
			}
			if err != nil || v != want {
				t.Errorf("%s args=%v 得到 %v err=%v want %v", tc.name, args, v, err, want)
			}
		}
	}
	if DefaultActorManager.Get(999) != nil {
		t.Error("离线查询不应创建 Actor")
	}
}

func TestActor_SetVipLevelPersistsBeforeMemoryUpdate(t *testing.T) {
	DefaultActorManager = NewActorManager()
	g := luahost.NewGopherLua(luahost.Config{OSType: "WIN"})
	defer g.Close()
	if err := InstallAll(g); err != nil {
		t.Fatal(err)
	}

	store := newMockStore()
	store.actors[7] = &ActorData{ActorID: 7, VIPLevel: 2}
	(&postLoginManager{}).applyActorDataAt(store.actors[7], sceneSpawnResult{})
	actor := DefaultActorManager.Get(7)
	actor.SetVIPStore(store)

	if _, err := g.Call("LActor.setVipLevel", 7, 7); err != nil {
		t.Fatalf("setVipLevel: %v", err)
	}
	if actor.VIPLevel != 7 || store.actors[7].VIPLevel != 7 {
		t.Fatalf("VIP 写入未同步: memory=%d store=%d", actor.VIPLevel, store.actors[7].VIPLevel)
	}

	store.vipSaveErr = errors.New("disk full")
	if _, err := g.Call("LActor.setVipLevel", 7, 9); err == nil {
		t.Fatal("存档失败应返回错误")
	}
	if actor.VIPLevel != 7 || store.actors[7].VIPLevel != 7 {
		t.Fatalf("存档失败不应改内存: memory=%d store=%d", actor.VIPLevel, store.actors[7].VIPLevel)
	}
	if _, err := g.Call("LActor.setVipLevel", 7, 7); err != nil {
		t.Fatalf("相同等级应幂等成功: %v", err)
	}

	DefaultActorManager.GetOrCreate(8)
	if _, err := g.Call("LActor.setVipLevel", 8, 3); err == nil {
		t.Fatal("缺少存档接口应返回错误")
	}
	if DefaultActorManager.Get(8).VIPLevel != 0 {
		t.Fatal("缺少存档接口不应改内存")
	}
	for _, args := range [][]luahost.Value{nil, {7}, {7, -1}, {7, 1.5}, {999, 3}} {
		if _, err := g.Call("LActor.setVipLevel", args...); err == nil {
			t.Errorf("setVipLevel args=%v 应拒绝", args)
		}
	}
	if DefaultActorManager.Get(999) != nil {
		t.Fatal("离线写入不应创建 Actor")
	}
}

func TestActor_GetActorByIdOnlyOnline(t *testing.T) {
	DefaultActorManager = NewActorManager()
	g := luahost.NewGopherLua(luahost.Config{OSType: "WIN"})
	defer g.Close()
	if err := InstallAll(g); err != nil {
		t.Fatal(err)
	}

	v, err := g.Call("LActor.getActorById", 99)
	if err != nil || v != nil {
		t.Fatalf("离线 getActorById=%v err=%v want nil", v, err)
	}
	DefaultActorManager.GetOrCreate(99)
	v, err = g.Call("LActor.getActorById", 99)
	if err != nil || v != 99 {
		t.Fatalf("在线 getActorById=%v err=%v want 99", v, err)
	}
	v, err = g.Call("LActor.getActorById")
	if err != nil || v != nil {
		t.Errorf("无参数 getActorById=%v err=%v want nil", v, err)
	}
	v, err = g.Call("LActor.getActorId")
	if err != nil || v != 0 {
		t.Errorf("无参数 getActorId=%v err=%v want 0", v, err)
	}
}

func TestActor_QueryBagItemCountAndAliases(t *testing.T) {
	DefaultActorManager = NewActorManager()
	g := luahost.NewGopherLua(luahost.Config{OSType: "WIN"})
	defer g.Close()
	if err := InstallAll(g); err != nil {
		t.Fatal(err)
	}

	actor := DefaultActorManager.GetOrCreate(7)
	actor.Items = map[int]int{1001: 10, 1002: 3, 1003: 0}
	actor.Exp = 123456
	actor.VIPLevel = 5
	for _, tc := range []struct {
		name string
		want int
	}{
		{"LActor.getBagItemCount", 13},
		{"LActor.getActorCurExp", 123456},
		{"LActor.getVIPLevel", 5},
	} {
		v, err := g.Call(tc.name, 7)
		if err != nil || v != tc.want {
			t.Errorf("%s=%v err=%v want %d", tc.name, v, err, tc.want)
		}
		for _, args := range [][]luahost.Value{nil, {999}} {
			v, err := g.Call(tc.name, args...)
			if err != nil || v != 0 {
				t.Errorf("%s args=%v 得到 %v err=%v want 0", tc.name, args, v, err)
			}
		}
	}
	actor.Items = nil
	v, err := g.Call("LActor.getBagItemCount", 7)
	if err != nil || v != 0 {
		t.Errorf("空背包=%v err=%v want 0", v, err)
	}
	if DefaultActorManager.Get(999) != nil {
		t.Error("离线查询不应创建 Actor")
	}
}

func TestActor_QueryMoneyCount(t *testing.T) {
	DefaultActorManager = NewActorManager()
	g := luahost.NewGopherLua(luahost.Config{OSType: "WIN"})
	defer g.Close()
	if err := InstallAll(g); err != nil {
		t.Fatal(err)
	}

	(&postLoginManager{}).applyActorDataAt(&ActorData{
		ActorID: 7, Gold: 12345, YuanBao: 67890,
	}, sceneSpawnResult{})
	store := newMockStore()
	store.actors[7] = &ActorData{ActorID: 7, Gold: 12345, YuanBao: 67890}
	DefaultActorManager.Get(7).SetCurrencyStore(store)
	for _, tc := range []struct {
		name string
		args []luahost.Value
		want int
	}{
		{"LActor.getMoneyCount", []luahost.Value{7, 1}, 12345},
		{"LActor.getMoneyCount", []luahost.Value{7, 3}, 67890},
		{"LActor.getMoneyCount", []luahost.Value{7, 0}, 0},
		{"LActor.getMoneyCount", []luahost.Value{7, 2}, 0},
		{"LActor.getMoneyCount", []luahost.Value{7, 99}, 0},
		{"LActor.getMoneyCount", nil, 0},
		{"LActor.getMoneyCount", []luahost.Value{7}, 0},
		{"LActor.getMoneyCount", []luahost.Value{999, 1}, 0},
	} {
		v, err := g.Call(tc.name, tc.args...)
		if err != nil || v != tc.want {
			t.Errorf("%s args=%v 得到 %v err=%v want %d", tc.name, tc.args, v, err, tc.want)
		}
	}
	if _, err := g.Call("LActor.changeCurrency", 7, 1, 55); err != nil {
		t.Fatal(err)
	}
	v, err := g.Call("LActor.getMoneyCount", 7, 1)
	if err != nil || v != 12400 {
		t.Fatalf("货币变更后查询=%v err=%v want 12400", v, err)
	}
	if DefaultActorManager.Get(999) != nil {
		t.Error("离线货币查询不应创建 Actor")
	}
}

func TestActor_QueryExistingActorMetadata(t *testing.T) {
	DefaultActorManager = NewActorManager()
	g := luahost.NewGopherLua(luahost.Config{OSType: "WIN"})
	defer g.Close()
	if err := InstallAll(g); err != nil {
		t.Fatal(err)
	}

	(&postLoginManager{}).applyActorDataAt(&ActorData{
		ActorID: 7, GuildID: 4321, GuildName: "测试公会", ChapterLevel: 18,
		MonthCard: 3, Reincarnate: 6, ZhuanSheng: 4,
	}, sceneSpawnResult{})
	for _, tc := range []struct {
		name string
		want any
	}{
		{"LActor.getGuildId", 4321},
		{"LActor.getGuildName", "测试公会"},
		{"LActor.getChapterLevel", 18},
		{"LActor.getMonthCard", 3},
		{"LActor.getReincarnateLv", 6},
		{"LActor.getZhuanShengLevel", 4},
	} {
		v, err := g.Call(tc.name, 7)
		if err != nil || v != tc.want {
			t.Errorf("%s=%v err=%v want %v", tc.name, v, err, tc.want)
		}
		for _, args := range [][]luahost.Value{nil, {999}} {
			v, err := g.Call(tc.name, args...)
			want := any(0)
			if _, ok := tc.want.(string); ok {
				want = ""
			}
			if err != nil || v != want {
				t.Errorf("%s args=%v 得到 %v err=%v want %v", tc.name, args, v, err, want)
			}
		}
	}
	if DefaultActorManager.Get(999) != nil {
		t.Error("离线元信息查询不应创建 Actor")
	}
}

func TestActor_GetActorAndRoleQueries(t *testing.T) {
	DefaultActorManager = NewActorManager()
	g := luahost.NewGopherLua(luahost.Config{OSType: "WIN"})
	defer g.Close()
	if err := InstallAll(g); err != nil {
		t.Fatal(err)
	}
	DefaultActorManager.GetOrCreate(7)

	assertValue := func(name string, want luahost.Value, args ...luahost.Value) {
		t.Helper()
		got, err := g.Call(name, args...)
		if err != nil || got != want {
			t.Fatalf("%s args=%v 得到 %#v err=%v want %#v", name, args, got, err, want)
		}
	}
	assertValue("LActor.getActor", 7, 7)
	assertValue("LActor.getActor", 7, 7.0)
	assertValue("LActor.getRoleCount", 1, 7)
	assertValue("LActor.getRole", 7, 7, 0)
	assertValue("LActor.getRole", 7, 7, 0.0)

	invalidHandles := []luahost.Value{nil, 0, -1, 1.5, "7", math.Exp2(63), 999}
	for _, handle := range invalidHandles {
		assertValue("LActor.getActor", nil, handle)
		assertValue("LActor.getRoleCount", 0, handle)
		assertValue("LActor.getRole", nil, handle, 0)
	}
	assertValue("LActor.getActor", nil)
	assertValue("LActor.getRoleCount", 0)
	assertValue("LActor.getRole", nil)
	assertValue("LActor.getRole", nil, 7)
	for _, index := range []luahost.Value{nil, -1, 1, 1.5, "0", math.Exp2(63)} {
		assertValue("LActor.getRole", nil, 7, index)
	}
	if DefaultActorManager.Get(999) != nil {
		t.Fatal("离线角色查询不应创建 Actor")
	}
}

func TestActor_GetRoleByJob(t *testing.T) {
	DefaultActorManager = NewActorManager()
	g := luahost.NewGopherLua(luahost.Config{OSType: "WIN"})
	defer g.Close()
	if err := InstallAll(g); err != nil {
		t.Fatal(err)
	}
	actor := DefaultActorManager.GetOrCreate(7)
	actor.mu.Lock()
	actor.Job = 2
	actor.mu.Unlock()

	assertValue := func(want luahost.Value, args ...luahost.Value) {
		t.Helper()
		got, err := g.Call("LActor.GetRoleByJob", args...)
		if err != nil || got != want {
			t.Fatalf("GetRoleByJob args=%v 得到 %#v err=%v want %#v", args, got, err, want)
		}
	}
	assertValue(7, 7, 2)
	assertValue(7, 7, 2.0)
	for _, job := range []luahost.Value{nil, 0, 1, 3, 4, -1, 1.5, "2", math.Exp2(63)} {
		assertValue(nil, 7, job)
	}
	for _, handle := range []luahost.Value{nil, 0, -1, 1.5, "7", math.Exp2(63), 999} {
		assertValue(nil, handle, 2)
	}
	assertValue(nil)
	assertValue(nil, 7)
	if DefaultActorManager.Get(999) != nil {
		t.Fatal("职业查询不应创建 Actor")
	}
}

func TestActor_EntityResolutionQueries(t *testing.T) {
	DefaultActorManager = NewActorManager()
	world := scene.NewWorld()
	setDefaultWorld(world)
	t.Cleanup(func() { setDefaultWorld(nil) })
	if err := world.SetScenes(map[int]scene.SceneConfig{1: {Width: 128, Height: 128}}); err != nil {
		t.Fatal(err)
	}
	observer := func(scene.Delivery) error { return nil }
	if _, err := world.AddPlayer(7, 1, 0, 0, observer); err != nil {
		t.Fatal(err)
	}
	if _, err := world.AddMonster(8, 1, 1, 64, 64, 100, 100); err != nil {
		t.Fatal(err)
	}
	DefaultActorManager.GetOrCreate(7)
	DefaultActorManager.GetOrCreate(9)

	g := luahost.NewGopherLua(luahost.Config{OSType: "WIN"})
	defer g.Close()
	if err := InstallAll(g); err != nil {
		t.Fatal(err)
	}
	assertValue := func(name string, want luahost.Value, args ...luahost.Value) {
		t.Helper()
		got, err := g.Call(name, args...)
		if err != nil || got != want {
			t.Fatalf("%s args=%v 得到 %#v err=%v want %#v", name, args, got, err, want)
		}
	}
	assertValue("LActor.getEntity", 7, 7)
	assertValue("LActor.getEntity", 7, 7.0)
	assertValue("LActor.getEntity", 8, 8)
	assertValue("LActor.getActorByEt", 7, 7)
	assertValue("LActor.getActorByEt", 7, 7.0)
	assertValue("LActor.getActorByEt", nil, 8)
	assertValue("LActor.getEntity", nil, 9)
	assertValue("LActor.getActorByEt", nil, 9)
	assertValue("LActor.getEntity", nil, 999)
	assertValue("LActor.getActorByEt", nil, 999)
	for _, handle := range []luahost.Value{nil, 0, -1, 1.5, "7", math.Exp2(63)} {
		assertValue("LActor.getEntity", nil, handle)
		assertValue("LActor.getActorByEt", nil, handle)
	}
	assertValue("LActor.getEntity", nil)
	assertValue("LActor.getActorByEt", nil)
	if DefaultActorManager.Get(8) != nil || DefaultActorManager.Get(999) != nil {
		t.Fatal("实体查询不应创建 Actor")
	}
	setDefaultWorld(nil)
	assertValue("LActor.getEntity", nil, 7)
	assertValue("LActor.getActorByEt", nil, 7)
}

func TestActor_EntityHandlePositionAndProperties(t *testing.T) {
	DefaultActorManager = NewActorManager()
	world := scene.NewWorld()
	setDefaultWorld(world)
	t.Cleanup(func() { setDefaultWorld(nil) })
	if err := world.SetScenes(map[int]scene.SceneConfig{1: {Width: 128, Height: 128}}); err != nil {
		t.Fatal(err)
	}
	observer := func(scene.Delivery) error { return nil }
	if _, err := world.AddPlayerWithHealth(7, 1, 0, 0, 500, 600, observer); err != nil {
		t.Fatal(err)
	}
	if _, err := world.AddMonster(8, 1, 1, 64, 64, 100, 100); err != nil {
		t.Fatal(err)
	}
	actor := DefaultActorManager.GetOrCreate(7)
	actor.Level = 55
	actor.Sex = 1
	actor.Job = 2
	actor.Exp = 123456
	actor.GuildID = 321
	actor.TotalPower = 67890
	actor.Recharge = 9876
	actor.VIPLevel = 5
	actor.Currency = map[int]int{0: 11, 1: 22, 2: 33, 3: 44}
	offlineActor := DefaultActorManager.GetOrCreate(9)
	offlineActor.Level = 12

	g := luahost.NewGopherLua(luahost.Config{OSType: "WIN"})
	defer g.Close()
	if err := InstallAll(g); err != nil {
		t.Fatal(err)
	}
	assertValue := func(name string, want luahost.Value, args ...luahost.Value) {
		t.Helper()
		got, err := g.Call(name, args...)
		if err != nil || got != want {
			t.Fatalf("%s args=%v 得到 %#v err=%v want %#v", name, args, got, err, want)
		}
	}
	for _, handle := range []luahost.Value{7, 7.0, 8} {
		want, _ := actorHandleArgument(handle)
		assertValue("LActor.getHandle", want, handle)
	}
	for _, handle := range []luahost.Value{nil, 0, -1, 1.5, "7", math.Exp2(63)} {
		assertValue("LActor.getHandle", 0, handle)
	}
	assertValue("LActor.getHandle", 0)
	for _, tc := range []struct {
		handle luahost.Value
		x, y   luahost.Value
	}{
		{7, 0, 0},
		{7.0, 0, 0},
		{8, 64, 64},
		{9, 0, 0},
		{999, 0, 0},
		{nil, 0, 0},
		{0, 0, 0},
		{-1, 0, 0},
		{1.5, 0, 0},
		{"7", 0, 0},
	} {
		v, err := g.DoString("(function() local x, y = LActor.getEntityPosition(" + luaLiteral(tc.handle) + "); return x * 10000 + y end)()")
		if err != nil {
			t.Fatal(err)
		}
		want := tc.x.(int)*10000 + tc.y.(int)
		if v != want {
			t.Fatalf("getEntityPosition(%v)=%v want (%v,%v)", tc.handle, v, tc.x, tc.y)
		}
	}
	if _, err := world.MovePlayer(7, 10, 20); err != nil {
		t.Fatal(err)
	}
	v, err := g.DoString("(function() local x, y = LActor.getEntityPosition(7); return x * 10000 + y end)()")
	if err != nil || v != 100020 {
		t.Fatalf("移动后坐标=%v err=%v want (10,20)", v, err)
	}
	for _, tc := range []struct {
		handle, property luahost.Value
		want             luahost.Value
	}{
		{7, propertyID, 7},
		{7, propertyPosX, 10},
		{7, propertyPosY, 20},
		{7, propertyHP, 500},
		{7, propertyMaxHP, 600},
		{7, propertyLevel, 55},
		{7, propertySex, 1},
		{7, propertyVocation, 2},
		{7, propertyExp, 123456},
		{7, propertyGuildID, 321},
		{7, propertyBindCoin, 11},
		{7, propertyCoin, 22},
		{7, propertyBindYB, 33},
		{7, propertyYB, 44},
		{7, propertyFight, 67890},
		{7, propertyRecharge, 9876},
		{7, propertyVIPLevel, 5},
		{8, propertyID, 1},
		{8, propertyPosX, 64},
		{8, propertyPosY, 64},
		{8, propertyHP, 100},
		{8, propertyMaxHP, 100},
		{8, propertyLevel, 0},
		{9, propertyID, 9},
		{9, propertyLevel, 12},
		{9, propertyPosX, 0},
		{9, propertyHP, 0},
		{7, 86, 0},
		{7, 9999, 0},
		{999, propertyID, 0},
	} {
		assertValue("LActor.getIntProperty", tc.want, tc.handle, tc.property)
	}
	for _, args := range [][]luahost.Value{
		nil, {7}, {7, nil}, {7, 1.5}, {7, "6"}, {7, math.Exp2(63)},
		{nil, propertyLevel}, {0, propertyLevel}, {-1, propertyLevel},
		{1.5, propertyLevel}, {"7", propertyLevel}, {math.Exp2(63), propertyLevel},
	} {
		assertValue("LActor.getIntProperty", 0, args...)
	}
	if _, err := world.AttackMonster(7, 8, 25, 128); err != nil {
		t.Fatal(err)
	}
	assertValue("LActor.getIntProperty", 75, 8, propertyHP)
	if DefaultActorManager.Get(8) != nil || DefaultActorManager.Get(999) != nil {
		t.Fatal("实体属性查询不应创建 Actor")
	}
	setDefaultWorld(nil)
	assertValue("LActor.getIntProperty", 9, 9, propertyID)
	assertValue("LActor.getIntProperty", 12, 9, propertyLevel)
	assertValue("LActor.getIntProperty", 0, 7, propertyPosX)
	assertValue("LActor.getIntProperty", 0, 7, propertyHP)
}

func TestActor_HealthPositionDeathAndEntityID(t *testing.T) {
	DefaultActorManager = NewActorManager()
	world := scene.NewWorld()
	setDefaultWorld(world)
	t.Cleanup(func() { setDefaultWorld(nil) })
	if err := world.SetScenes(map[int]scene.SceneConfig{1: {Width: 128, Height: 128}}); err != nil {
		t.Fatal(err)
	}
	observer := func(scene.Delivery) error { return nil }
	if _, err := world.AddPlayerWithHealth(7, 1, 10, 20, 500, 600, observer); err != nil {
		t.Fatal(err)
	}
	if _, err := world.AddMonster(8, 1, 1001, 64, 64, 100, 120); err != nil {
		t.Fatal(err)
	}
	actor := DefaultActorManager.GetOrCreate(7)
	actor.X, actor.Y = 11, 22
	actor.HP, actor.MaxHP = 400, 450
	offlineActor := DefaultActorManager.GetOrCreate(9)
	offlineActor.X, offlineActor.Y = 3, 4
	offlineActor.HP, offlineActor.MaxHP = 0, 300

	g := luahost.NewGopherLua(luahost.Config{OSType: "WIN"})
	defer g.Close()
	if err := InstallAll(g); err != nil {
		t.Fatal(err)
	}
	assertValue := func(name string, want luahost.Value, args ...luahost.Value) {
		t.Helper()
		got, err := g.Call(name, args...)
		if err != nil || got != want {
			t.Fatalf("%s args=%v 得到 %#v err=%v want %#v", name, args, got, err, want)
		}
	}
	assertPosition := func(name string, handle luahost.Value, x, y int) {
		t.Helper()
		v, err := g.DoString("(function() local x, y = " + name + "(" + luaLiteral(handle) + "); return x * 10000 + y end)()")
		if err != nil || v != x*10000+y {
			t.Fatalf("%s(%v)=%v err=%v want (%d,%d)", name, handle, v, err, x, y)
		}
	}
	assertValue("LActor.getHp", 500, 7)
	assertValue("LActor.getHp", 500, 7.0)
	assertValue("LActor.getHp", 100, 8)
	assertValue("LActor.getHpMax", 600, 7)
	assertValue("LActor.getHpMax", 120, 8)
	assertValue("LActor.isDeath", false, 7)
	assertValue("LActor.isDeath", false, 8)
	assertValue("LActor.getId", 7, 7)
	assertValue("LActor.getId", 1001, 8)
	assertPosition("LActor.getPosition", 7, 10, 20)
	assertPosition("LActor.getPosition", 8, 64, 64)
	assertPosition("LActor.getPosition", 9, 3, 4)
	assertValue("LActor.getHp", 0, 9)
	assertValue("LActor.getHpMax", 300, 9)
	assertValue("LActor.isDeath", true, 9)
	assertValue("LActor.getId", 9, 9)
	for _, handle := range []luahost.Value{nil, 0, -1, 1.5, "7", math.Exp2(63), 999} {
		assertValue("LActor.getHp", 0, handle)
		assertValue("LActor.getHpMax", 0, handle)
		assertValue("LActor.isDeath", false, handle)
		assertValue("LActor.getId", 0, handle)
		assertPosition("LActor.getPosition", handle, 0, 0)
	}
	assertValue("LActor.getHp", 0)
	assertValue("LActor.getHpMax", 0)
	assertValue("LActor.isDeath", false)
	assertValue("LActor.getId", 0)
	assertPosition("LActor.getPosition", nil, 0, 0)
	if _, err := world.MovePlayer(7, 30, 40); err != nil {
		t.Fatal(err)
	}
	assertPosition("LActor.getPosition", 7, 30, 40)
	if _, err := world.AttackMonster(7, 8, 25, 128); err != nil {
		t.Fatal(err)
	}
	assertValue("LActor.getHp", 75, 8)
	assertValue("LActor.isDeath", false, 8)
	if _, err := world.AttackPlayer(8, 7, 500, 128); err != nil {
		t.Fatal(err)
	}
	assertValue("LActor.getHp", 0, 7)
	assertValue("LActor.getHpMax", 600, 7)
	assertValue("LActor.isDeath", true, 7)
	if DefaultActorManager.Get(8) != nil || DefaultActorManager.Get(999) != nil {
		t.Fatal("实体状态查询不应创建 Actor")
	}
	setDefaultWorld(nil)
	assertValue("LActor.getHp", 400, 7)
	assertValue("LActor.getHpMax", 450, 7)
	assertValue("LActor.isDeath", false, 7)
	assertValue("LActor.getId", 7, 7)
	assertPosition("LActor.getPosition", 7, 11, 22)
	assertValue("LActor.getHp", 0, 8)
	assertValue("LActor.getId", 0, 8)
}

func TestActor_GetGamePf(t *testing.T) {
	DefaultActorManager = NewActorManager()
	actor := DefaultActorManager.GetOrCreate(7)
	g := luahost.NewGopherLua(luahost.Config{OSType: "WIN"})
	defer g.Close()
	if err := InstallAll(g); err != nil {
		t.Fatal(err)
	}
	assertValue := func(want luahost.Value, args ...luahost.Value) {
		t.Helper()
		got, err := g.Call("LActor.getGamePf", args...)
		if err != nil || got != want {
			t.Fatalf("LActor.getGamePf args=%v 得到 %#v err=%v want %#v", args, got, err, want)
		}
	}
	cases := []struct {
		pf   string
		want string
	}{
		{pf: "normal", want: "normal"},
		{pf: "union*channel", want: "channel"},
		{pf: "unionchannel*channel", want: "channel"},
		{pf: "union**nested", want: "*nested"},
		{pf: "union*", want: ""},
		{pf: "union", want: "union"},
		{pf: "Union*channel", want: "Union*channel"},
		{pf: "other*channel", want: "other*channel"},
		{pf: "", want: ""},
	}
	for _, tc := range cases {
		actor.mu.Lock()
		actor.PfID = tc.pf
		actor.mu.Unlock()
		assertValue(tc.want, 7)
	}
	actor.mu.Lock()
	actor.PfID = "base"
	actor.mu.Unlock()
	assertValue("base", 7.0)
	for _, handle := range []luahost.Value{nil, 0, -1, 1.5, "7", math.NaN(), math.Inf(1), math.Inf(-1), math.Exp2(63), 999} {
		assertValue("", handle)
	}
	assertValue("")
	if DefaultActorManager.Get(999) != nil {
		t.Fatal("平台查询不应创建 Actor")
	}

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				got, err := realGetGamePf(7)
				if err != nil || got != "base" {
					t.Errorf("并发 LActor.getGamePf 得到 %#v err=%v", got, err)
					return
				}
			}
		}()
	}
	wg.Wait()
}

func luaLiteral(v luahost.Value) string {
	switch x := v.(type) {
	case nil:
		return "nil"
	case string:
		return fmt.Sprintf("%q", x)
	default:
		return fmt.Sprint(x)
	}
}

func TestActor_FailClosedStatusQueries(t *testing.T) {
	DefaultActorManager = NewActorManager()
	DefaultActorManager.GetOrCreate(7)
	g := luahost.NewGopherLua(luahost.Config{OSType: "WIN"})
	defer g.Close()
	if err := InstallAll(g); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name string
		want luahost.Value
	}{
		{"LActor.getGmLevel", 0},
		{"LActor.isImage", false},
		{"LActor.getFubenHandle", 0},
		{"LActor.getFubenId", 0},
		{"LActor.isInFuben", false},
	}
	for _, tc := range cases {
		for _, args := range [][]luahost.Value{nil, {7}, {999}, {nil}, {0}, {-1}, {1.5}, {"7"}, {math.Exp2(63)}} {
			got, err := g.Call(tc.name, args...)
			if err != nil || got != tc.want {
				t.Fatalf("%s args=%v 得到 %#v err=%v want %#v", tc.name, args, got, err, tc.want)
			}
		}
	}
}

func TestActor_OnlineNameLookupAndIsActor(t *testing.T) {
	DefaultActorManager = NewActorManager()
	g := luahost.NewGopherLua(luahost.Config{OSType: "WIN"})
	defer g.Close()
	if err := InstallAll(g); err != nil {
		t.Fatal(err)
	}

	first := DefaultActorManager.GetOrCreate(7)
	first.Name = "在线甲"
	second := DefaultActorManager.GetOrCreate(8)
	second.Name = "在线乙"

	v, err := g.Call("LActor.getActorByName", "在线甲")
	if err != nil || v != 7 {
		t.Fatalf("getActorByName=%v err=%v want 7", v, err)
	}
	v, err = g.Call("LActor.getActorIdByName", "在线乙")
	if err != nil || v != 8 {
		t.Fatalf("getActorIdByName=%v err=%v want 8", v, err)
	}
	for _, args := range [][]luahost.Value{nil, {""}, {"不存在"}, {7}} {
		v, err := g.Call("LActor.getActorByName", args...)
		if err != nil || v != nil {
			t.Errorf("getActorByName args=%v 得到 %v err=%v want nil", args, v, err)
		}
		v, err = g.Call("LActor.getActorIdByName", args...)
		if err != nil || v != 0 {
			t.Errorf("getActorIdByName args=%v 得到 %v err=%v want 0", args, v, err)
		}
	}
	for _, tc := range []struct {
		arg  luahost.Value
		want bool
	}{
		{7, true},
		{8, true},
		{999, false},
		{nil, false},
		{1 << 30, false},
	} {
		v, err := g.Call("LActor.isActor", tc.arg)
		if err != nil || v != tc.want {
			t.Errorf("isActor(%v)=%v err=%v want %v", tc.arg, v, err, tc.want)
		}
	}
	v, err = g.Call("LActor.isActor")
	if err != nil || v != false {
		t.Errorf("isActor 无参数=%v err=%v want false", v, err)
	}

	duplicate := DefaultActorManager.GetOrCreate(9)
	duplicate.Name = "在线甲"
	v, err = g.Call("LActor.getActorByName", "在线甲")
	if err != nil || v != nil {
		t.Errorf("重名查询应失败关闭: actor=%v err=%v", v, err)
	}
	v, err = g.Call("LActor.getActorIdByName", "在线甲")
	if err != nil || v != 0 {
		t.Errorf("重名 ID 查询应失败关闭: id=%v err=%v", v, err)
	}
	if DefaultActorManager.Get(999) != nil {
		t.Error("在线查询不应创建离线 Actor")
	}
}

func TestActor_OnlineAccountLookup(t *testing.T) {
	DefaultActorManager = NewActorManager()
	g := luahost.NewGopherLua(luahost.Config{OSType: "WIN"})
	defer g.Close()
	if err := InstallAll(g); err != nil {
		t.Fatal(err)
	}

	first := DefaultActorManager.GetOrCreate(7)
	first.Account = "account_online"
	second := DefaultActorManager.GetOrCreate(8)
	second.Account = "account_other"

	v, err := g.Call("LActor.getActorByAccountName", "account_online")
	if err != nil || v != 7 {
		t.Fatalf("getActorByAccountName=%v err=%v want 7", v, err)
	}
	v, err = g.Call("LActor.getActorIdByAccountName", "account_other")
	if err != nil || v != 8 {
		t.Fatalf("getActorIdByAccountName=%v err=%v want 8", v, err)
	}
	for _, args := range [][]luahost.Value{nil, {""}, {"不存在"}, {7}} {
		v, err := g.Call("LActor.getActorByAccountName", args...)
		if err != nil || v != nil {
			t.Errorf("getActorByAccountName args=%v 得到 %v err=%v want nil", args, v, err)
		}
		v, err = g.Call("LActor.getActorIdByAccountName", args...)
		if err != nil || v != 0 {
			t.Errorf("getActorIdByAccountName args=%v 得到 %v err=%v want 0", args, v, err)
		}
	}

	duplicate := DefaultActorManager.GetOrCreate(9)
	duplicate.Account = "account_online"
	v, err = g.Call("LActor.getActorByAccountName", "account_online")
	if err != nil || v != nil {
		t.Errorf("账号重名查询应失败关闭: actor=%v err=%v", v, err)
	}
	v, err = g.Call("LActor.getActorIdByAccountName", "account_online")
	if err != nil || v != 0 {
		t.Errorf("账号重名 ID 查询应失败关闭: id=%v err=%v", v, err)
	}
	if DefaultActorManager.Get(999) != nil {
		t.Error("在线账号查询不应创建离线 Actor")
	}
}

func TestActor_ChangeCurrencyPersistsGoldAndYuanBao(t *testing.T) {
	DefaultActorManager = NewActorManager()
	g := luahost.NewGopherLua(luahost.Config{OSType: "WIN"})
	defer g.Close()
	if err := InstallAll(g); err != nil {
		t.Fatal(err)
	}

	store := newMockStore()
	store.actors[7] = &ActorData{ActorID: 7, Gold: 100, YuanBao: 200}
	actor := DefaultActorManager.GetOrCreate(7)
	actor.Currency = map[int]int{currencyGold: 100, currencyYuanBao: 200}
	actor.SetCurrencyStore(store)

	for _, tc := range []struct {
		typ   int
		delta int
		want  int
	}{
		{currencyGold, 50, 150},
		{currencyGold, -30, 120},
		{currencyYuanBao, 80, 280},
		{currencyYuanBao, -200, 80},
	} {
		if _, err := g.Call("LActor.changeCurrency", 7, tc.typ, tc.delta, "unit_test"); err != nil {
			t.Fatalf("changeCurrency type=%d delta=%d: %v", tc.typ, tc.delta, err)
		}
		if got := actor.Currency[tc.typ]; got != tc.want {
			t.Fatalf("type=%d 内存货币=%d want %d", tc.typ, got, tc.want)
		}
		stored := store.actors[7].Gold
		if tc.typ == currencyYuanBao {
			stored = store.actors[7].YuanBao
		}
		if stored != int64(tc.want) {
			t.Fatalf("type=%d 存档货币=%d want %d", tc.typ, stored, tc.want)
		}
	}
	if store.currencyCalls != 4 || store.currencyType != currencyYuanBao || store.currencyAmount != 80 {
		t.Fatalf("存档调用错误: calls=%d type=%d amount=%d", store.currencyCalls, store.currencyType, store.currencyAmount)
	}
	if _, err := g.Call("LActor.changeCurrency", 7, currencyGold, 0); err != nil {
		t.Fatalf("零变更应幂等成功: %v", err)
	}
	if store.currencyCalls != 4 {
		t.Fatalf("零变更不应写库: calls=%d", store.currencyCalls)
	}
}

func TestActor_ChangeCurrencyRejectsInvalidAndInsufficient(t *testing.T) {
	DefaultActorManager = NewActorManager()
	g := luahost.NewGopherLua(luahost.Config{OSType: "WIN"})
	defer g.Close()
	if err := InstallAll(g); err != nil {
		t.Fatal(err)
	}

	store := newMockStore()
	store.actors[8] = &ActorData{ActorID: 8, Gold: 100}
	actor := DefaultActorManager.GetOrCreate(8)
	actor.Currency = map[int]int{currencyGold: 100}
	actor.SetCurrencyStore(store)

	for _, args := range [][]luahost.Value{
		nil,
		{8},
		{8, currencyGold},
		{8, currencyGold, 1.5},
		{8, 1.5, 1},
		{8, "1", 1},
		{8, currencyGold, "1"},
		{8, currencyGold, math.NaN()},
		{8, currencyGold, math.Inf(1)},
		{nil, currencyGold, 1},
		{0, currencyGold, 1},
		{-1, currencyGold, 1},
		{999, currencyGold, 1},
		{8, currencyGold, -101},
		{8, currencyYuanBao, -1},
	} {
		if _, err := g.Call("LActor.changeCurrency", args...); err == nil {
			t.Errorf("changeCurrency args=%v 应拒绝", args)
		}
		if actor.Currency[currencyGold] != 100 || actor.Currency[currencyYuanBao] != 0 {
			t.Fatalf("失败调用修改了内存: args=%v currency=%v", args, actor.Currency)
		}
	}
	if store.currencyCalls != 0 || store.actors[8].Gold != 100 {
		t.Fatalf("失败调用不应写库: calls=%d gold=%d", store.currencyCalls, store.actors[8].Gold)
	}
	if DefaultActorManager.Get(999) != nil {
		t.Fatal("离线货币写入不应创建 Actor")
	}
}

func TestActor_ChangeCurrencyRejectsOverflow(t *testing.T) {
	DefaultActorManager = NewActorManager()
	actor := DefaultActorManager.GetOrCreate(9)
	actor.Currency = map[int]int{currencyGold: math.MaxInt}
	store := newMockStore()
	store.actors[9] = &ActorData{ActorID: 9, Gold: int64(math.MaxInt)}
	actor.SetCurrencyStore(store)

	if _, err := realChangeCurrency(9, currencyGold, 1); err == nil {
		t.Fatal("金币溢出应拒绝")
	}
	if actor.Currency[currencyGold] != math.MaxInt || store.currencyCalls != 0 {
		t.Fatalf("溢出后状态被修改: memory=%d calls=%d", actor.Currency[currencyGold], store.currencyCalls)
	}
}

func TestActor_ChangeCurrencyRequiresStoreAndRollsBackSaveFailure(t *testing.T) {
	DefaultActorManager = NewActorManager()
	withoutStore := DefaultActorManager.GetOrCreate(10)
	withoutStore.Currency = map[int]int{currencyGold: 10}
	if _, err := realChangeCurrency(10, currencyGold, 5); err == nil {
		t.Fatal("缺少金币存档器应返回错误")
	}
	if withoutStore.Currency[currencyGold] != 10 {
		t.Fatal("缺少存档器时不应改内存")
	}
	if _, err := realChangeCurrency(10, currencyGold, 0); err != nil {
		t.Fatalf("零变更无需存档器: %v", err)
	}

	store := newMockStore()
	store.actors[11] = &ActorData{ActorID: 11, Gold: 20}
	store.currencySaveErr = errors.New("disk full")
	actor := DefaultActorManager.GetOrCreate(11)
	actor.Currency = map[int]int{currencyGold: 20}
	actor.SetCurrencyStore(store)
	if _, err := realChangeCurrency(11, currencyGold, 5); err == nil {
		t.Fatal("存档失败应返回错误")
	}
	if actor.Currency[currencyGold] != 20 || store.actors[11].Gold != 20 || store.currencyCalls != 0 {
		t.Fatalf("存档失败后状态被修改: memory=%d stored=%d calls=%d", actor.Currency[currencyGold], store.actors[11].Gold, store.currencyCalls)
	}
}

func TestActor_ChangeCurrencyMemoryOnlyTypesSkipStore(t *testing.T) {
	DefaultActorManager = NewActorManager()
	store := newMockStore()
	store.actors[12] = &ActorData{ActorID: 12}
	actor := DefaultActorManager.GetOrCreate(12)
	actor.Currency = map[int]int{0: 5, 2: 7, 99: 3}
	actor.SetCurrencyStore(store)

	for _, tc := range []struct {
		typ   int
		delta int
		want  int
	}{
		{0, 2, 7},
		{2, -3, 4},
		{99, 1, 4},
	} {
		if _, err := realChangeCurrency(12, tc.typ, tc.delta); err != nil {
			t.Fatalf("内存货币 type=%d delta=%d: %v", tc.typ, tc.delta, err)
		}
		if got := actor.Currency[tc.typ]; got != tc.want {
			t.Fatalf("内存货币 type=%d=%d want %d", tc.typ, got, tc.want)
		}
	}
	if _, err := realChangeCurrency(12, 0, -8); err == nil {
		t.Fatal("绑定金币余额不足应拒绝")
	}
	if store.currencyCalls != 0 {
		t.Fatalf("非持久化类型不应写库: calls=%d", store.currencyCalls)
	}
}

func TestActor_LogStructuredAndStateless(t *testing.T) {
	DefaultActorManager = NewActorManager()
	g := luahost.NewGopherLua(luahost.Config{OSType: "WIN"})
	defer g.Close()
	if err := InstallAll(g); err != nil {
		t.Fatal(err)
	}

	var output bytes.Buffer
	previousLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&output, nil)))
	t.Cleanup(func() { slog.SetDefault(previousLogger) })

	if _, err := g.Call("LActor.log", 7, "friendsystem.sAddResp", "1", 42, true, nil); err != nil {
		t.Fatalf("LActor.log 不应中断业务: %v", err)
	}
	var record map[string]any
	if err := json.Unmarshal(output.Bytes(), &record); err != nil {
		t.Fatalf("日志不是有效 JSON: %v output=%q", err, output.String())
	}
	if record["msg"] != "LActor.log" || record["actorID"] != float64(7) || record["level"] != "INFO" {
		t.Fatalf("日志字段错误: %v", record)
	}
	wantArgs := []any{"friendsystem.sAddResp", "1", "42", "true", "nil"}
	gotArgs, ok := record["args"].([]any)
	if !ok || len(gotArgs) != len(wantArgs) {
		t.Fatalf("日志参数错误: %v", record["args"])
	}
	for i := range wantArgs {
		if gotArgs[i] != wantArgs[i] {
			t.Fatalf("日志参数[%d]=%v want %v", i, gotArgs[i], wantArgs[i])
		}
	}
	if DefaultActorManager.Get(7) != nil {
		t.Fatal("记录日志不应创建 Actor")
	}
}

func TestActor_LogHandlesInvalidAndOfflineActors(t *testing.T) {
	g := luahost.NewGopherLua(luahost.Config{OSType: "WIN"})
	defer g.Close()
	if err := InstallAll(g); err != nil {
		t.Fatal(err)
	}

	var output bytes.Buffer
	previousLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&output, nil)))
	t.Cleanup(func() { slog.SetDefault(previousLogger) })

	for _, tc := range []struct {
		name    string
		handle  luahost.Value
		actorID float64
	}{
		{"空参", nil, 0},
		{"零句柄", 0, 0},
		{"负句柄", -1, 0},
		{"小数", 1.5, 0},
		{"字符串", "7", 0},
		{"NaN", math.NaN(), 0},
		{"Inf", math.Inf(1), 0},
		{"离线正整数", 999, 999},
	} {
		t.Run(tc.name, func(t *testing.T) {
			output.Reset()
			if _, err := g.Call("LActor.log", tc.handle, "probe"); err != nil {
				t.Fatalf("日志边界输入不应报错: %v", err)
			}
			var record map[string]any
			if err := json.Unmarshal(output.Bytes(), &record); err != nil {
				t.Fatal(err)
			}
			if record["actorID"] != tc.actorID {
				t.Fatalf("actorID=%v want %v", record["actorID"], tc.actorID)
			}
		})
	}
	if DefaultActorManager.Get(999) != nil {
		t.Fatal("记录离线 Actor 日志不应创建 Actor")
	}
}

func TestActor_ChangeYuanBaoPersistsAndPreservesOtherCurrency(t *testing.T) {
	DefaultActorManager = NewActorManager()
	g := luahost.NewGopherLua(luahost.Config{OSType: "WIN"})
	defer g.Close()
	if err := InstallAll(g); err != nil {
		t.Fatal(err)
	}

	store := newMockStore()
	store.actors[7] = &ActorData{ActorID: 7, Gold: 100, YuanBao: 200}
	actor := DefaultActorManager.GetOrCreate(7)
	actor.Currency = map[int]int{currencyGold: 100, currencyYuanBao: 200}
	actor.SetCurrencyStore(store)

	for _, tc := range []struct {
		delta int
		want  int
	}{
		{80, 280},
		{-200, 80},
	} {
		if _, err := g.Call("LActor.changeYuanBao", 7, tc.delta, "unit_test", true); err != nil {
			t.Fatalf("changeYuanBao delta=%d: %v", tc.delta, err)
		}
		if got := actor.Currency[currencyYuanBao]; got != tc.want {
			t.Fatalf("内存元宝=%d want %d", got, tc.want)
		}
		if got := store.actors[7].YuanBao; got != int64(tc.want) {
			t.Fatalf("存档元宝=%d want %d", got, tc.want)
		}
	}
	if actor.Currency[currencyGold] != 100 || store.actors[7].Gold != 100 {
		t.Fatal("changeYuanBao 不应修改金币")
	}
	if _, err := g.Call("LActor.changeYuanBao", 7, 0); err != nil {
		t.Fatalf("零变更应幂等成功: %v", err)
	}
	if store.currencyCalls != 2 || store.currencyType != currencyYuanBao || store.currencyAmount != 80 {
		t.Fatalf("存档调用错误: calls=%d type=%d amount=%d", store.currencyCalls, store.currencyType, store.currencyAmount)
	}
}

func TestActor_ChangeYuanBaoRejectsInvalidAndRollsBackSaveFailure(t *testing.T) {
	DefaultActorManager = NewActorManager()
	g := luahost.NewGopherLua(luahost.Config{OSType: "WIN"})
	defer g.Close()
	if err := InstallAll(g); err != nil {
		t.Fatal(err)
	}

	actor := DefaultActorManager.GetOrCreate(8)
	actor.Currency = map[int]int{currencyYuanBao: 100}
	store := newMockStore()
	store.actors[8] = &ActorData{ActorID: 8, YuanBao: 100}
	actor.SetCurrencyStore(store)

	for _, args := range [][]luahost.Value{
		nil,
		{8},
		{8, 1.5},
		{8, "1"},
		{8, math.NaN()},
		{8, math.Inf(1)},
		{nil, 1},
		{0, 1},
		{-1, 1},
		{999, 1},
		{8, -101},
	} {
		if _, err := g.Call("LActor.changeYuanBao", args...); err == nil {
			t.Errorf("changeYuanBao args=%v 应拒绝", args)
		}
		if actor.Currency[currencyYuanBao] != 100 || store.actors[8].YuanBao != 100 || store.currencyCalls != 0 {
			t.Fatalf("失败调用修改了状态: args=%v memory=%d stored=%d calls=%d", args, actor.Currency[currencyYuanBao], store.actors[8].YuanBao, store.currencyCalls)
		}
	}
	if DefaultActorManager.Get(999) != nil {
		t.Fatal("离线元宝写入不应创建 Actor")
	}

	store.currencySaveErr = errors.New("disk full")
	if _, err := g.Call("LActor.changeYuanBao", 8, 5); err == nil {
		t.Fatal("元宝存档失败应返回错误")
	}
	if actor.Currency[currencyYuanBao] != 100 || store.actors[8].YuanBao != 100 {
		t.Fatal("存档失败不应修改内存元宝")
	}

	overflow := DefaultActorManager.GetOrCreate(9)
	overflow.Currency = map[int]int{currencyYuanBao: math.MaxInt}
	overflowStore := newMockStore()
	overflowStore.actors[9] = &ActorData{ActorID: 9, YuanBao: int64(math.MaxInt)}
	overflow.SetCurrencyStore(overflowStore)
	if _, err := g.Call("LActor.changeYuanBao", 9, 1); err == nil {
		t.Fatal("元宝变更溢出应拒绝")
	}
	if overflow.Currency[currencyYuanBao] != math.MaxInt || overflowStore.currencyCalls != 0 {
		t.Fatal("溢出不应修改内存或写库")
	}

	withoutStore := DefaultActorManager.GetOrCreate(10)
	withoutStore.Currency = map[int]int{currencyYuanBao: 10}
	if _, err := g.Call("LActor.changeYuanBao", 10, 5); err == nil {
		t.Fatal("缺少元宝存档器应返回错误")
	}
	if withoutStore.Currency[currencyYuanBao] != 10 {
		t.Fatal("缺少存档器不应修改内存")
	}
}

func TestActor_ChangeYuanBaoConcurrentIncrements(t *testing.T) {
	DefaultActorManager = NewActorManager()
	g := luahost.NewGopherLua(luahost.Config{OSType: "WIN"})
	defer g.Close()
	if err := InstallAll(g); err != nil {
		t.Fatal(err)
	}

	store := newMockStore()
	store.actors[11] = &ActorData{ActorID: 11}
	actor := DefaultActorManager.GetOrCreate(11)
	actor.Currency = map[int]int{currencyYuanBao: 0}
	actor.SetCurrencyStore(store)

	const goroutines = 8
	errs := make(chan error, goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			_, err := g.Call("LActor.changeYuanBao", 11, 1)
			errs <- err
		}()
	}
	for i := 0; i < goroutines; i++ {
		if err := <-errs; err != nil {
			t.Fatalf("并发元宝变更失败: %v", err)
		}
	}
	if actor.Currency[currencyYuanBao] != goroutines || store.actors[11].YuanBao != goroutines || store.currencyCalls != goroutines {
		t.Fatalf("并发结果错误: memory=%d stored=%d calls=%d", actor.Currency[currencyYuanBao], store.actors[11].YuanBao, store.currencyCalls)
	}
}

func TestMySQLAccountStore_SaveCurrencyRejectsUnsupportedInput(t *testing.T) {
	store := &MySQLAccountStore{}
	for _, tc := range []struct {
		actorID int
		typ     int
		amount  int64
	}{
		{0, currencyGold, 1},
		{1, currencyGold, -1},
		{1, 0, 1},
		{1, currencyYuanBao + 1, 1},
	} {
		if err := store.SaveCurrency(tc.actorID, tc.typ, tc.amount); err == nil {
			t.Errorf("SaveCurrency actor=%d type=%d amount=%d 应拒绝", tc.actorID, tc.typ, tc.amount)
		}
	}
}

func TestActor_CurrencyConcurrentAccess(t *testing.T) {
	DefaultActorManager = NewActorManager()
	actor := DefaultActorManager.GetOrCreate(7)
	actor.Currency = map[int]int{1: 0}
	store := newMockStore()
	store.actors[7] = &ActorData{ActorID: 7}
	actor.SetCurrencyStore(store)

	const writers = 4
	const iterations = 250
	var wg sync.WaitGroup
	for range writers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range iterations {
				if _, err := realChangeCurrency(7, 1, 1); err != nil {
					t.Errorf("changeCurrency: %v", err)
					return
				}
			}
		}()
	}
	for range writers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range iterations {
				if _, err := realGetCurrency(7, 1); err != nil {
					t.Errorf("getCurrency: %v", err)
					return
				}
			}
		}()
	}
	wg.Wait()
	if got := actor.Currency[1]; got != writers*iterations {
		t.Fatalf("并发变更后的金币=%d want %d", got, writers*iterations)
	}
	if got := store.actors[7].Gold; got != int64(writers*iterations) {
		t.Fatalf("并发存档后的金币=%d want %d", got, writers*iterations)
	}
}
