package engineapi

import (
	"bytes"
	"encoding/binary"
	"math"
	"testing"

	"github.com/xiaokentrl/ChuanQi/ltcq-go/internal/luahost"
	"github.com/xiaokentrl/ChuanQi/ltcq-go/internal/netframe"
	"github.com/xiaokentrl/ChuanQi/ltcq-go/internal/scene"
)

func testActorData() *ActorData {
	return &ActorData{
		ActorID: 98765, AccountID: 121102, Account: "帅哥", Name: "帅哥", ServerID: 1,
		Job: 1, Sex: 0, Level: 55, Exp: 123456, SceneID: 1, Gold: 100, YuanBao: 200,
		TotalPower: 12345, BagGridCount: 80, VIPLevel: 3, ZhuanSheng: 2,
		Soul: 11, Feats: 12, RuneShatter: 13, RuneExchange: 14, Together1: 15, Together2: 16, WeiWang: 17,
	}
}

func TestBuildActorBaseData_ClientParserLayout(t *testing.T) {
	d := testActorData()
	p := buildActorBaseData(d)
	if len(p) != 143 || !bytes.Equal(p[:2], []byte{0, 1}) {
		t.Fatalf("ActorBaseData 长度/包头错误: len=%d head=%x", len(p), p[:2])
	}
	off := 2
	read32 := func() uint32 {
		v := binary.LittleEndian.Uint32(p[off : off+4])
		off += 4
		return v
	}
	read64 := func() uint64 {
		v := binary.LittleEndian.Uint64(p[off : off+8])
		off += 8
		return v
	}
	if math.Float64frombits(read64()) != 98765 || read32() != 98765 || read32() != 1 {
		t.Fatal("handle/actorid/serverid 错误")
	}
	if string(bytes.TrimRight(p[off:off+33], "\x00")) != "帅哥" {
		t.Fatalf("固定长度角色名=%q", p[off:off+33])
	}
	off += 33
	if read32() != 55 || read32() != 123456 || read64() != 0 {
		t.Fatal("等级/经验字段错误")
	}
	if read64() != 100 || read64() != 200 || read32() != 3 || read64() != 11 || read32() != 80 {
		t.Fatal("货币/VIP/灵魂/背包字段错误")
	}
	if read64() != 12 || read64() != 13 || read64() != 14 || read64() != 15 || read64() != 16 || read32() != 17 {
		t.Fatal("扩展货币字段错误")
	}
	if off != len(p) {
		t.Fatalf("ActorBaseData 解析到 %d，总长 %d", off, len(p))
	}
}

func TestBuildRoleData_MinimalClientLayout(t *testing.T) {
	p := buildRoleData(testActorData())
	// 原骨架 3677，末尾 2×uint16（4B）替换为法阵段 436B（4B 头 + 6×72B 子阵）→ 4109。
	if len(p) != 3677-4+436 || !bytes.Equal(p[:2], []byte{0, 2}) {
		t.Fatalf("RoleData 长度/包头错误: len=%d head=%x", len(p), p[:2])
	}
	if binary.LittleEndian.Uint16(p[2:4]) != 1 {
		t.Fatal("RoleData 角色数错误")
	}
	off := 4
	if !bytes.Equal(p[off:off+8], make([]byte, 8)) || binary.LittleEndian.Uint32(p[off+8:off+12]) != 1 || binary.LittleEndian.Uint32(p[off+12:off+16]) != 0 {
		t.Fatalf("Role 前缀字段非零: %x", p[off:off+16])
	}
	off += 16
	if got := binary.LittleEndian.Uint64(p[off : off+8]); got != 12345 {
		t.Fatalf("Role 战力=%d", got)
	}
	off += 8 + 40
	if got := binary.LittleEndian.Uint32(p[off : off+4]); got != 2 {
		t.Fatalf("元神等级=%d", got)
	}
	off += 4 + 6 + 13*(7*4+144) + 4*4 + 20 + 8 + 13 + 8*144 + 8*4 + 4
	if off+2+2+2+3*4+8*2+8*5*2+436 != len(p) {
		t.Fatalf("RoleData 尾部偏移错误: %d", off)
	}
}

// TestBuildRoleData_DemonCircleSegment 校验法阵数据段——
// 修复"跑进度条弹框 Cannot read properties of undefined (reading 'Lv')"：
// 客户端 SubRoles.parser 依据 [2B 子阵数] 构造 matrixInfo，数量必须为 6，
// 否则 fzRedShow→getMatrixInfo 读到 undefined.Lv 抛异常。
func TestBuildRoleData_DemonCircleSegment(t *testing.T) {
	p := buildRoleData(testActorData())
	// 法阵段起点 = RoleData 总长 - 436。
	off := len(p) - 436

	read16 := func() uint16 {
		v := binary.LittleEndian.Uint16(p[off : off+2])
		off += 2
		return v
	}
	if got := read16(); got != 0 {
		t.Fatalf("huanhuaId=%d want 0", got)
	}
	if got := read16(); got != 6 {
		t.Fatalf("子法阵数量=%d want 6（否则客户端 matrixInfo 为空导致弹框）", got)
	}
	for sub := 0; sub < 6; sub++ {
		if got := read16(); got != uint16(sub+1) {
			t.Fatalf("子阵%d index=%d want %d", sub, got, sub+1)
		}
		if got := read16(); got != 0 {
			t.Fatalf("子阵%d Lv=%d want 0", sub, got)
		}
		if got := read16(); got != 0 {
			t.Fatalf("子阵%d WYLv=%d want 0", sub, got)
		}
		off += 4 // WYEXP
		if got := read16(); got != 4 {
			t.Fatalf("子阵%d 特质数=%d want 4", sub, got)
		}
		for tezhi := 1; tezhi <= 4; tezhi++ {
			if got := read16(); got != uint16(tezhi) {
				t.Fatalf("子阵%d 特质id=%d want %d", sub, got, tezhi)
			}
			if got := read16(); got != 0 {
				t.Fatalf("子阵%d 特质等级=%d want 0", sub, got)
			}
		}
		if got := read16(); got != 0 {
			t.Fatalf("子阵%d 领悟次数=%d want 0", sub, got)
		}
		if got := read16(); got != 5 {
			t.Fatalf("子阵%d 潜能数=%d want 5", sub, got)
		}
		for _, want := range demonQiannengIDs[sub] {
			if got := read16(); got != want {
				t.Fatalf("子阵%d 潜能id=%d want %d", sub, got, want)
			}
			if got := read16(); got != 0 {
				t.Fatalf("子阵%d 潜能激活=%d want 0", sub, got)
			}
			off += 4 // 额外属性
		}
	}
	if off != len(p) {
		t.Fatalf("法阵段解析到 %d，总长 %d", off, len(p))
	}
}

func TestBuildRoleDataUsesStaticDemonCircleData(t *testing.T) {
	DefaultActorManager = NewActorManager()
	g := luahost.NewGopherLua(luahost.Config{OSType: "WIN"})
	defer g.Close()
	if err := InstallAll(g); err != nil {
		t.Fatal(err)
	}
	DefaultActorManager.GetOrCreate(98765)
	_, err := g.DoString(`(function()
		local st = LActor.getStaticVar(98765)
		st.demoncircledata = {
			[0] = {
				huanhuaid = 2,
				subdemoncircleinfo = {
					[1] = {
						arrayeyelv = 3, nutritionlv = 4, nutritionexp = 42, studycount = 6,
						tezhiinfo = { [1] = 5 },
						potentialpoint = { [1] = { extravalue = 99 } },
					},
				},
			},
		}
	end)()`)
	if err != nil {
		t.Fatal(err)
	}
	data, err := loadDemonCircleData(DefaultActorManager.Get(98765), g)
	if err != nil {
		t.Fatal(err)
	}
	p := buildRoleDataWithDemon(testActorData(), data)
	off := len(p) - 436
	read16 := func() uint16 {
		v := binary.LittleEndian.Uint16(p[off : off+2])
		off += 2
		return v
	}
	read32 := func() uint32 {
		v := binary.LittleEndian.Uint32(p[off : off+4])
		off += 4
		return v
	}
	if got := read16(); got != 2 {
		t.Fatalf("huanhuaId=%d want 2", got)
	}
	if got := read16(); got != demonCircleCount {
		t.Fatalf("子阵数=%d want %d", got, demonCircleCount)
	}
	if got := read16(); got != 1 {
		t.Fatalf("subId=%d want 1", got)
	}
	if got := read16(); got != 3 {
		t.Fatalf("阵眼等级=%d want 3", got)
	}
	if got := read16(); got != 4 {
		t.Fatalf("蕴养等级=%d want 4", got)
	}
	if got := read32(); got != 42 {
		t.Fatalf("蕴养经验=%d want 42", got)
	}
	if got := read16(); got != demonTezhiCount {
		t.Fatalf("特质数=%d want %d", got, demonTezhiCount)
	}
	if got := read16(); got != 1 {
		t.Fatalf("特质id=%d want 1", got)
	}
	if got := read16(); got != 5 {
		t.Fatalf("特质等级=%d want 5", got)
	}
	off += 4 * (demonTezhiCount - 1)
	if got := read16(); got != 6 {
		t.Fatalf("领悟次数=%d want 6", got)
	}
	if got := read16(); got != demonQiannengCount {
		t.Fatalf("潜能数=%d want %d", got, demonQiannengCount)
	}
	if got := read16(); got != 1 {
		t.Fatalf("潜能id=%d want 1", got)
	}
	if got := read16(); got != 1 {
		t.Fatalf("潜能激活=%d want 1", got)
	}
	if got := read32(); got != 99 {
		t.Fatalf("潜能额外值=%d want 99", got)
	}
}

func TestBuildEnterScene_ClientParserLayout(t *testing.T) {
	p := buildEnterScene(testActorData())
	if !bytes.Equal(p[:2], []byte{0, 3}) || binary.LittleEndian.Uint32(p[2:6]) != 0 || binary.LittleEndian.Uint32(p[6:10]) != 1 {
		t.Fatalf("EnterScene 包头错误: %x", p[:10])
	}
	if binary.LittleEndian.Uint16(p[10:12]) != 56*64 || binary.LittleEndian.Uint16(p[12:14]) != 37*64 || p[14] != 0 {
		t.Fatalf("EnterScene 坐标/类型错误: %x", p[10:15])
	}
	name, off, err := parseEgretString(p, 15)
	if err != nil || name != "比奇省" {
		t.Fatalf("场景名=%q err=%v", name, err)
	}
	desc, off, err := parseEgretString(p, off)
	if err != nil || desc != "主城" || off != len(p) {
		t.Fatalf("场景描述=%q off=%d err=%v", desc, off, err)
	}
}

func TestBuildEnterScene_MineEntryCoordinates(t *testing.T) {
	d := testActorData()
	d.SceneID = 41
	p := buildEnterScene(d)
	if binary.LittleEndian.Uint32(p[2:6]) != 0 || binary.LittleEndian.Uint32(p[6:10]) != 41 {
		t.Fatalf("矿洞应为普通地图 41: %x", p[:10])
	}
	if binary.LittleEndian.Uint16(p[10:12]) != 77*64 || binary.LittleEndian.Uint16(p[12:14]) != 22*64 {
		t.Fatalf("矿洞出生坐标错误: %x", p[10:14])
	}
	name, _, err := parseEgretString(p, 15)
	if err != nil || name != "比奇矿洞" {
		t.Fatalf("矿洞名=%q err=%v", name, err)
	}
}

func TestBuildOtherEntity_ClientParserLayout(t *testing.T) {
	d := testActorData()
	p := buildOtherEntity(d)
	if !bytes.Equal(p[:2], []byte{0, 4}) || binary.LittleEndian.Uint16(p[2:4]) != 1 {
		t.Fatalf("OtherEntity 类型错误: %x", p[:4])
	}
	if math.Float64frombits(binary.LittleEndian.Uint64(p[4:12])) != float64(d.ActorID) || binary.LittleEndian.Uint32(p[12:16]) != 0 || math.Float64frombits(binary.LittleEndian.Uint64(p[16:24])) != float64(d.ActorID) {
		t.Fatalf("OtherEntity 句柄错误: %x", p[2:24])
	}
	if binary.LittleEndian.Uint32(p[24:28]) != 56*64 || binary.LittleEndian.Uint32(p[28:32]) != 37*64 {
		t.Fatalf("OtherEntity 坐标错误: %x", p[24:32])
	}
	if binary.LittleEndian.Uint16(p[32:34]) != 3 ||
		math.Float64frombits(binary.LittleEndian.Uint64(p[34:42])) != 100 ||
		math.Float64frombits(binary.LittleEndian.Uint64(p[42:50])) != 100 ||
		math.Float64frombits(binary.LittleEndian.Uint64(p[50:58])) != 100 ||
		binary.LittleEndian.Uint16(p[58:60]) != 0 {
		t.Fatalf("OtherEntity 属性错误: %x", p[32:60])
	}
	name, off, err := parseEgretString(p, 60)
	if err != nil || name != d.Name {
		t.Fatalf("实体名=%q off=%d err=%v", name, off, err)
	}
	if binary.LittleEndian.Uint32(p[off:off+4]) != d.ServerID || p[off+4] != 1 || p[off+5] != 0 || binary.LittleEndian.Uint32(p[off+6:off+10]) != uint32(d.Level) {
		t.Fatalf("实体职业/等级错误: %x", p[off:off+10])
	}
	off += 10 + 24
	guild, off, err := parseEgretString(p, off)
	if err != nil || guild != "" || off+54 != len(p) || !bytes.Equal(p[off:], make([]byte, 54)) {
		t.Fatalf("实体外观尾部错误: guild=%q off=%d len=%d err=%v", guild, off, len(p), err)
	}
}

func TestPostLoginRollbackClearsAI(t *testing.T) {
	previousActors := DefaultActorManager
	DefaultActorManager = NewActorManager()
	t.Cleanup(func() { DefaultActorManager = previousActors; SetSendHook(nil) })
	world := scene.NewWorld()
	if err := world.SetScenes(map[int]scene.SceneConfig{
		1: {Width: 4096, Height: 4096, SpawnX: 0, SpawnY: 0},
	}); err != nil {
		t.Fatal(err)
	}
	const monsterID = 1 << 30
	if _, err := world.AddMonster(monsterID, 1, 31001, 0, 0, 500, 500); err != nil {
		t.Fatal(err)
	}
	store := newMockStore()
	store.actors[98765] = testActorData()
	g := luahost.NewGopherLua(luahost.Config{OSType: "WIN"})
	defer g.Close()
	if err := InstallAll(g); err != nil {
		t.Fatal(err)
	}
	combat := NewCombatManager(world)
	installCombatAPI(g, combat)
	if err := g.Load("login_rollback_ai", `
function OnActorEvent(actor, eid)
    LActor.setAITarget(actor, 1073741824)
    error("rollback")
end
`); err != nil {
		t.Fatal(err)
	}
	router := NewNetRouter(g)
	router.SetSender(func([]byte) error { return nil })
	const sid = uint16(554)
	router.OpenSession(sid, "127.0.0.1")
	router.SetAccount(sid, "帅哥", 121102, 1)
	post := newPostLoginManager(store, router, g, nil, world).(*postLoginManager)
	post.Combat = combat
	if err := post.HandlePostLogin(sid, 98765, "帅哥", nil); err == nil {
		t.Fatal("登录事件失败未返回错误")
	}
	if _, ok := world.Entity(98765); ok {
		t.Fatal("登录回滚后玩家仍在场景中")
	}
	if _, ok := combat.GetAITarget(98765); ok {
		t.Fatal("登录回滚后 AI 目标仍残留")
	}
	if len(combat.stopped) != 0 || len(combat.reviveAt) != 0 {
		t.Fatal("登录回滚后 AI 停战或复活状态仍残留")
	}
}

func TestPostLoginSendsBaseRoleSceneAndBagPacketsBeforeLoginEvent(t *testing.T) {
	DefaultActorManager = NewActorManager()
	store := newMockStore()
	store.actors[98765].Recharge = 12345
	store.actors[98765].CreateTime = 1700000000
	store.actors[98765].AppID = "app_test"
	store.actors[98765].PfID = "pf_test"
	store.actors[98765].GuildID = 4321
	store.actors[98765].GuildName = "登录公会"
	store.actors[98765].ChapterLevel = 18
	store.actors[98765].MonthCard = 3
	store.actors[98765].Reincarnate = 6
	g := luahost.NewGopherLua(luahost.Config{OSType: "WIN"})
	defer g.Close()
	defer SetSendHook(nil)
	if err := InstallAll(g); err != nil {
		t.Fatal(err)
	}
	if err := g.Load("stub", `
		lastEvent = nil
		function OnActorEvent(actor, eid)
			lastEvent = {actor, eid}
			LActor.setVipLevel(actor, 4)
			LActor.changeCurrency(actor, 1, 5, "login_event_test")
		end
	`); err != nil {
		t.Fatal(err)
	}
	var packets [][]byte
	readyCalled := false
	router := NewNetRouter(g)
	router.SetSender(func(frame []byte) error {
		if !readyCalled {
			t.Fatal("场景包先于 ff05 成功回包发送")
		}
		packets = append(packets, append([]byte(nil), frame[netframe.GateFrameHeaderLen+netframe.NetIDLen:]...))
		return nil
	})
	const sid = uint16(553)
	router.OpenSession(sid, "127.0.0.1")
	router.SetAccount(sid, "帅哥", 121102, 1)
	post := NewPostLoginManager(store, router, g)
	if err := post.HandlePostLogin(sid, 98765, "帅哥", func() error {
		readyCalled = true
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if len(packets) != 8 {
		t.Fatalf("进场景回包数=%d", len(packets))
	}
	wantHeaders := [][2]byte{{0, 1}, {0, 2}, {0, 3}, {0, 4}, {0, 19}, {3, 1}, {3, 1}, {3, 1}}
	for i, want := range wantHeaders {
		if len(packets[i]) < 2 || packets[i][0] != want[0] || packets[i][1] != want[1] {
			t.Fatalf("回包 %d 头=%x", i, packets[i])
		}
	}
	for i, bagType := range []byte{0, 1, 2} {
		packet := packets[i+5]
		if len(packet) < 8 || packet[2] != 1 || packet[3] != bagType || binary.LittleEndian.Uint32(packet[4:8]) != 0 {
			t.Fatalf("背包初始化包 %d=%x", bagType, packet)
		}
	}
	actor := DefaultActorManager.Get(98765)
	if actor == nil || actor.Name != "帅哥" || actor.Level != 55 {
		t.Fatalf("Actor 数据未加载: %+v", actor)
	}
	if actor.Recharge != 12345 || actor.CreateTime != 1700000000 || actor.AppID != "app_test" || actor.PfID != "pf_test" {
		t.Fatalf("登录元信息未加载: %+v", actor)
	}
	if actor.GuildID != 4321 || actor.GuildName != "登录公会" || actor.ChapterLevel != 18 || actor.MonthCard != 3 || actor.Reincarnate != 6 {
		t.Fatalf("登录玩法元信息未加载: %+v", actor)
	}
	if actor.vipStore != store || actor.VIPLevel != 4 || store.actors[98765].VIPLevel != 4 {
		t.Fatalf("登录事件 VIP 存档未接通: memory=%d store=%d", actor.VIPLevel, store.actors[98765].VIPLevel)
	}
	if actor.currencyStore != store || actor.Currency[currencyGold] != 5 || store.actors[98765].Gold != 5 || store.currencyCalls != 1 {
		t.Fatalf("登录事件金币存档未接通: memory=%d store=%d calls=%d", actor.Currency[currencyGold], store.actors[98765].Gold, store.currencyCalls)
	}
	actorV, err := g.DoString("lastEvent[1]")
	if err != nil || actorV.(int) != 98765 {
		t.Fatalf("登录事件 actor=%v err=%v", actorV, err)
	}
	eidV, err := g.DoString("lastEvent[2]")
	if err != nil || eidV.(int) != aeUserLoginEventID {
		t.Fatalf("登录事件 eid=%v err=%v", eidV, err)
	}
}

// TestBuildClientConfig 校验 ClientConfig 包字节布局，对齐 clientconfig.lua:50-62 onLogin。
func TestBuildClientConfig(t *testing.T) {
	a := newActor(98765)
	a.SetClientConfig(1, 100)
	a.SetClientConfig(0, 42)

	p := buildClientConfig(a)
	if len(p) != 2+2+2*8 {
		t.Fatalf("ClientConfig 长度=%d want %d", len(p), 2+2+16)
	}
	if p[0] != cmdBase || p[1] != sBaseCmdClientConfig {
		t.Fatalf("包头=%x want 00 13", p[:2])
	}
	if n := binary.LittleEndian.Uint16(p[2:4]); n != 2 {
		t.Fatalf("配置项数量=%d want 2", n)
	}
	off := 4
	// 按 key 升序：key=0 val=42，key=1 val=100
	if k := binary.LittleEndian.Uint32(p[off : off+4]); k != 0 {
		t.Errorf("第1项 key=%d want 0", k)
	}
	if v := binary.LittleEndian.Uint32(p[off+4 : off+8]); v != 42 {
		t.Errorf("第1项 val=%d want 42", v)
	}
	off += 8
	if k := binary.LittleEndian.Uint32(p[off : off+4]); k != 1 {
		t.Errorf("第2项 key=%d want 1", k)
	}
	if v := binary.LittleEndian.Uint32(p[off+4 : off+8]); v != 100 {
		t.Errorf("第2项 val=%d want 100", v)
	}
}

// TestHandleClientConfig 校验 cBaseCmd_ClientConfig(19) 请求更新静态数据。
func TestHandleClientConfig(t *testing.T) {
	DefaultActorManager = NewActorManager()
	actor := DefaultActorManager.GetOrCreate(98765)

	body := make([]byte, 8)
	binary.LittleEndian.PutUint32(body[0:4], 5)
	binary.LittleEndian.PutUint32(body[4:8], 777)
	if err := handleClientConfig(1, 98765, body); err != nil {
		t.Fatal(err)
	}
	got := actor.ClientConfigSnapshot()
	if len(got) != 1 || got[0][0] != 5 || got[0][1] != 777 {
		t.Fatalf("配置未更新: %v", got)
	}
	if !actor.Dirty {
		t.Error("配置更新后未标脏（持久化会丢）")
	}

	if err := handleClientConfig(1, 98765, []byte{1, 2, 3}); err == nil {
		t.Error("过短请求体应报错")
	}
	if err := handleClientConfig(1, 999999, body); err == nil {
		t.Error("不存在的 actor 应报错")
	}
}
