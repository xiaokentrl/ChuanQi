package engineapi

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math"
	"path/filepath"
	"testing"
	"time"

	"github.com/xiaokentrl/ChuanQi/ltcq-go/internal/luahost"
	"github.com/xiaokentrl/ChuanQi/ltcq-go/internal/netframe"
	"github.com/xiaokentrl/ChuanQi/ltcq-go/internal/scene"
)

type skillTestStore struct {
	*persistMockStore
	failUpgrade  bool
	upgradeCalls int
	gold         int64
}

func newSkillTestStore() *skillTestStore {
	return &skillTestStore{persistMockStore: newPersistMockStore()}
}

func (s *skillTestStore) SaveSkillUpgrade(actorID int, gold int64, data *actorPersistData) error {
	s.upgradeCalls++
	if s.failUpgrade {
		return fmt.Errorf("模拟事务失败")
	}
	if err := s.SaveActorPersist(actorID, data); err != nil {
		return err
	}
	s.gold = gold
	if actor := s.actors[actorID]; actor != nil {
		actor.Gold = gold
	}
	return nil
}

func testSkillCatalog(t *testing.T) *skillCatalog {
	t.Helper()
	root := filepath.Join("..", "..", "..", "data", "server", "s1", "Debug", "gameworld", "data")
	catalog, err := LoadSkillCatalog(root, "WIN")
	if err != nil {
		t.Fatal(err)
	}
	return catalog
}

func newSkillTestRouter(sid uint16, actorID int) (*NetRouter, *[][]byte) {
	router := NewNetRouter(nil)
	packets := &[][]byte{}
	router.SetSender(func(frame []byte) error {
		parsed, err := netframe.ReadGateFrame(bytes.NewReader(frame))
		if err != nil {
			return err
		}
		if len(parsed.Body) < netframe.NetIDLen {
			return fmt.Errorf("下行帧缺少 NetId")
		}
		*packets = append(*packets, append([]byte(nil), parsed.Body[netframe.NetIDLen:]...))
		return nil
	})
	router.OpenSession(sid, "")
	router.SetAccount(sid, "tester", 1, 1)
	router.BindActor(sid, actorID)
	return router, packets
}

func skillUpgradeBody(roleID, slot int) []byte {
	body := binary.LittleEndian.AppendUint16(nil, uint16(roleID))
	return binary.LittleEndian.AppendUint16(body, uint16(slot))
}

func skillUpgradeAllBody(mode, roleID, index int) []byte {
	body := binary.LittleEndian.AppendUint16(nil, uint16(mode))
	body = binary.LittleEndian.AppendUint16(body, uint16(roleID))
	return binary.LittleEndian.AppendUint16(body, uint16(index))
}

func skillCastBody(attackerID, targetID, skillID int) []byte {
	body := binary.LittleEndian.AppendUint64(nil, math.Float64bits(float64(attackerID)))
	body = binary.LittleEndian.AppendUint64(body, math.Float64bits(float64(targetID)))
	return binary.LittleEndian.AppendUint32(body, uint32(skillID))
}

func TestActor_GetRoleSkillLevel(t *testing.T) {
	DefaultActorManager = NewActorManager()
	g := luahost.NewGopherLua(luahost.Config{OSType: "WIN"})
	defer g.Close()
	if err := InstallAll(g); err != nil {
		t.Fatal(err)
	}
	actor := DefaultActorManager.GetOrCreate(7)
	actor.mu.Lock()
	actor.Job = 2
	actor.Skills = map[int]int{
		skillBaseID(2, 1): 1,
		skillBaseID(2, 2): 3,
		skillBaseID(2, 4): 12,
		skillBaseID(2, 5): 260,
	}
	actor.mu.Unlock()

	assertValue := func(want luahost.Value, args ...luahost.Value) {
		t.Helper()
		got, err := g.Call("LActor.getRoleSkillLevel", args...)
		if err != nil || got != want {
			t.Fatalf("getRoleSkillLevel args=%v 得到 %#v err=%v want %#v", args, got, err, want)
		}
	}
	for index, want := range []int{1, 3, 0, 12, 260} {
		assertValue(want, 7, 0, index)
		assertValue(want, 7.0, 0.0, float64(index))
	}
	for _, roleID := range []luahost.Value{nil, -1, 1, 0.5, "0", math.Exp2(63)} {
		assertValue(0, 7, roleID, 0)
	}
	for _, index := range []luahost.Value{nil, -1, 5, 1.5, "0", math.Exp2(63)} {
		assertValue(0, 7, 0, index)
	}
	for _, handle := range []luahost.Value{nil, 0, -1, 1.5, "7", math.Exp2(63), 999} {
		assertValue(0, handle, 0, 0)
	}
	assertValue(0)
	assertValue(0, 7)
	assertValue(0, 7, 0)

	invalidJob := DefaultActorManager.GetOrCreate(8)
	invalidJob.mu.Lock()
	invalidJob.Job = 4
	invalidJob.Skills = map[int]int{skillBaseID(2, 1): 9}
	invalidJob.mu.Unlock()
	assertValue(0, 8, 0, 0)
	if DefaultActorManager.Get(999) != nil {
		t.Fatal("技能查询不应创建 Actor")
	}
}

func TestSkillCatalogLoadsClientRules(t *testing.T) {
	catalog := testSkillCatalog(t)
	if got := catalog.openLevels; got != [5]int{1, 5, 15, 25, 35} {
		t.Fatalf("技能开启等级=%v", got)
	}
	if catalog.upgradeCost[1] != 5000 || catalog.upgradeCost[80] != 44500 {
		t.Fatalf("技能升级费用错误: level1=%d level80=%d", catalog.upgradeCost[1], catalog.upgradeCost[80])
	}
	cfg := catalog.skills[11001]
	if cfg.DamageRatio != 0.95 || cfg.FlatDamage != 10 || cfg.Cooldown != 800*time.Millisecond || cfg.CastRange != 1.3 {
		t.Fatalf("基础技能配置错误: %+v", cfg)
	}
	if _, ok := catalog.skills[35260]; !ok {
		t.Fatal("缺少职业3第五槽260级技能")
	}
}

func TestSkillInitializationAndRoleDataLayout(t *testing.T) {
	previous := DefaultActorManager
	DefaultActorManager = NewActorManager()
	t.Cleanup(func() { DefaultActorManager = previous })
	actor := DefaultActorManager.GetOrCreate(1)
	actor.Job, actor.Level = 1, 35
	store := newPersistMockStore()
	manager := NewSkillManager(testSkillCatalog(t), nil)
	if err := manager.EnsureInitialized(actor, store, nil); err != nil {
		t.Fatal(err)
	}
	wantLevels := []int{1, 1, 1, 1, 1}
	for slot, want := range wantLevels {
		if got := actor.Skills[skillBaseID(1, slot+1)]; got != want {
			t.Fatalf("槽%d等级=%d want=%d", slot, got, want)
		}
	}
	actor.Skills[11001] = 3
	packet := buildRoleDataWithDemonSkills(&ActorData{Job: 1, TotalPower: 99}, demonCircleData{}, actor)
	wantWireLevels := []uint32{3, 1, 1, 1, 1}
	for i, want := range wantWireLevels {
		off := 28 + i*4
		if got := binary.LittleEndian.Uint32(packet[off : off+4]); got != want {
			t.Fatalf("RoleData技能%d等级=%d want=%d", i, got, want)
		}
	}
	for i := 0; i < 5; i++ {
		off := 48 + i*4
		if got := binary.LittleEndian.Uint32(packet[off : off+4]); got != 0 {
			t.Fatalf("RoleData技能后置字段%d=%d", i, got)
		}
	}
	stored, err := store.LoadActorPersist(1)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Skills[11001] != 1 {
		t.Fatalf("初始化快照被后续内存修改污染: %+v", stored.Skills)
	}
}

func TestSkillInitializationRespectsOpenLevelsAndSavedLevels(t *testing.T) {
	previous := DefaultActorManager
	DefaultActorManager = NewActorManager()
	t.Cleanup(func() { DefaultActorManager = previous })
	actor := DefaultActorManager.GetOrCreate(1)
	actor.Job, actor.Level = 1, 5
	actor.Skills[11001] = 4
	manager := NewSkillManager(testSkillCatalog(t), nil)
	if err := manager.EnsureInitialized(actor, newPersistMockStore(), nil); err != nil {
		t.Fatal(err)
	}
	if actor.Skills[11001] != 4 || actor.Skills[12001] != 1 || actor.Skills[13001] != 0 {
		t.Fatalf("技能初始化错误: %+v", actor.Skills)
	}
}

func TestSkillUpgradePersistsCostAndNotifiesClient(t *testing.T) {
	previous := DefaultActorManager
	DefaultActorManager = NewActorManager()
	t.Cleanup(func() { DefaultActorManager = previous })
	actor := DefaultActorManager.GetOrCreate(1)
	actor.Job, actor.Level = 1, 2
	actor.Skills[11001] = 1
	actor.Currency[currencyGold] = 10000
	store := newSkillTestStore()
	store.actors[1] = &ActorData{ActorID: 1, Gold: 10000}
	manager := NewSkillManager(testSkillCatalog(t), nil)
	router, packets := newSkillTestRouter(7, 1)
	if err := manager.Handle(7, 1, cSkillCmdUpgradeSkill, skillUpgradeBody(0, 0), router, store, nil); err != nil {
		t.Fatal(err)
	}
	if actor.Skills[11001] != 2 || actor.Currency[currencyGold] != 5000 || store.gold != 5000 || store.upgradeCalls != 1 {
		t.Fatalf("升级结果错误: skills=%v gold=%d stored=%d calls=%d", actor.Skills, actor.Currency[currencyGold], store.gold, store.upgradeCalls)
	}
	if len(*packets) != 2 || !bytes.Equal((*packets)[0], buildSkillUpdate(0, 0, 2)) || !bytes.Equal((*packets)[1], buildGoldChange(currencyGold, 5000)) {
		t.Fatalf("升级下行错误: %x", *packets)
	}
}

func TestSkillUpgradeRejectsInsufficientGoldAndFailedTransaction(t *testing.T) {
	previous := DefaultActorManager
	DefaultActorManager = NewActorManager()
	t.Cleanup(func() { DefaultActorManager = previous })
	catalog := testSkillCatalog(t)
	for _, tt := range []struct {
		name string
		gold int
		fail bool
	}{
		{name: "金币不足", gold: 4999},
		{name: "事务失败", gold: 5000, fail: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			actor := DefaultActorManager.GetOrCreate(1)
			actor.Job, actor.Level = 1, 2
			actor.Skills = map[int]int{11001: 1}
			actor.Currency[currencyGold] = tt.gold
			store := newSkillTestStore()
			store.failUpgrade = tt.fail
			manager := NewSkillManager(catalog, nil)
			router, packets := newSkillTestRouter(7, 1)
			if err := manager.Handle(7, 1, cSkillCmdUpgradeSkill, skillUpgradeBody(0, 0), router, store, nil); err == nil {
				t.Fatal("应拒绝升级")
			}
			if actor.Skills[11001] != 1 || actor.Currency[currencyGold] != tt.gold || len(*packets) != 0 {
				t.Fatalf("失败后状态被修改: skills=%v gold=%d packets=%x", actor.Skills, actor.Currency[currencyGold], *packets)
			}
			if tt.fail && store.upgradeCalls != 1 || !tt.fail && store.upgradeCalls != 0 {
				t.Fatalf("存档调用次数错误: %d", store.upgradeCalls)
			}
		})
	}
}

func TestSkillUpgradeUsesEffectiveLevelAndFreeActivation(t *testing.T) {
	previous := DefaultActorManager
	DefaultActorManager = NewActorManager()
	t.Cleanup(func() { DefaultActorManager = previous })
	actor := DefaultActorManager.GetOrCreate(1)
	actor.Job, actor.Level, actor.ZhuanSheng = 1, 81, 1
	actor.Skills[11001] = 90
	actor.Currency[currencyGold] = 0
	manager := NewSkillManager(testSkillCatalog(t), nil)
	router, packets := newSkillTestRouter(7, 1)
	if err := manager.Handle(7, 1, cSkillCmdUpgradeSkill, skillUpgradeBody(0, 0), router, newSkillTestStore(), nil); err == nil {
		t.Fatal("超过有效等级上限应拒绝")
	}
	actor.Level, actor.ZhuanSheng = 5, 0
	actor.Skills = map[int]int{11001: 1}
	if err := manager.Handle(7, 1, cSkillCmdUpgradeSkill, skillUpgradeBody(0, 1), router, newSkillTestStore(), nil); err != nil {
		t.Fatal(err)
	}
	if actor.Skills[12001] != 1 || actor.Currency[currencyGold] != 0 || len(*packets) != 1 || !bytes.Equal((*packets)[0], buildSkillUpdate(0, 1, 1)) {
		t.Fatalf("免费激活错误: skills=%v gold=%d packets=%x", actor.Skills, actor.Currency[currencyGold], *packets)
	}
}

func TestSkillUpgradeAllModesAndIndexMapping(t *testing.T) {
	previous := DefaultActorManager
	DefaultActorManager = NewActorManager()
	t.Cleanup(func() { DefaultActorManager = previous })
	catalog := testSkillCatalog(t)
	for _, tt := range []struct {
		name       string
		mode       int
		index      int
		gold       int
		wantFirst  int
		wantSecond int
		wantGold   int
	}{
		{name: "单技能列表索引", mode: 0, index: 1, gold: 11000, wantFirst: 1, wantSecond: 3, wantGold: 500},
		{name: "全部已学习技能", mode: 1, index: 0, gold: 11000, wantFirst: 2, wantSecond: 2, wantGold: 1000},
	} {
		t.Run(tt.name, func(t *testing.T) {
			actor := DefaultActorManager.GetOrCreate(1)
			actor.Job, actor.Level = 1, 3
			actor.Skills = map[int]int{11001: 1, 12001: 1}
			actor.Currency[currencyGold] = tt.gold
			store := newSkillTestStore()
			manager := NewSkillManager(catalog, nil)
			router, packets := newSkillTestRouter(7, 1)
			if err := manager.Handle(7, 1, cSkillCmdUpgradeAll, skillUpgradeAllBody(tt.mode, 0, tt.index), router, store, nil); err != nil {
				t.Fatal(err)
			}
			if actor.Skills[11001] != tt.wantFirst || actor.Skills[12001] != tt.wantSecond || actor.Currency[currencyGold] != tt.wantGold {
				t.Fatalf("一键升级错误: skills=%v gold=%d", actor.Skills, actor.Currency[currencyGold])
			}
			if len(*packets) != 2 || len((*packets)[0]) != 14 || (*packets)[0][0] != cmdSkill || (*packets)[0][1] != sSkillCmdUpdateAll {
				t.Fatalf("一键升级下行错误: %x", *packets)
			}
		})
	}
}

func TestSkillUpgradeAllRejectsNoOpAndInvalidIndex(t *testing.T) {
	previous := DefaultActorManager
	DefaultActorManager = NewActorManager()
	t.Cleanup(func() { DefaultActorManager = previous })
	actor := DefaultActorManager.GetOrCreate(1)
	actor.Job, actor.Level = 1, 2
	actor.Skills = map[int]int{11001: 1}
	actor.Currency[currencyGold] = 100
	manager := NewSkillManager(testSkillCatalog(t), nil)
	router, packets := newSkillTestRouter(7, 1)
	for _, body := range [][]byte{skillUpgradeAllBody(1, 0, 0), skillUpgradeAllBody(1, 0, 1)} {
		if err := manager.Handle(7, 1, cSkillCmdUpgradeAll, body, router, newSkillTestStore(), nil); err == nil {
			t.Fatalf("应拒绝一键升级: %x", body)
		}
	}
	if len(*packets) != 0 || actor.Skills[11001] != 1 {
		t.Fatalf("无效请求产生副作用: packets=%x skills=%v", *packets, actor.Skills)
	}
}

func newSkillCombatFixture(t *testing.T) (*scene.World, *CombatManager, *SkillManager, *Actor, *Actor) {
	t.Helper()
	previous := DefaultActorManager
	DefaultActorManager = NewActorManager()
	t.Cleanup(func() { DefaultActorManager = previous })
	world := scene.NewWorld()
	if err := world.SetScenes(map[int]scene.SceneConfig{
		1: {Width: 4096, Height: 4096},
		2: {Width: 4096, Height: 4096},
	}); err != nil {
		t.Fatal(err)
	}
	combat := NewCombatManager(world, CombatConfig{Damage: 100, AttackRange: 64, Interval: time.Hour})
	manager := NewSkillManager(testSkillCatalog(t), combat)
	now := time.Unix(1000, 0)
	manager.now = func() time.Time { return now }
	actors := make([]*Actor, 2)
	for i := range actors {
		id := i + 1
		actor := DefaultActorManager.GetOrCreate(id)
		actor.Job, actor.Level, actor.HP = 1, 1, 100
		actor.Skills[11001] = 1
		actors[i] = actor
		if _, err := world.AddPlayer(id, 1, 100, 100, func(scene.Delivery) error { return nil }); err != nil {
			t.Fatal(err)
		}
	}
	for _, monster := range []scene.Entity{
		{ID: 1 << 30, SceneID: 1, X: 150, Y: 100, HP: 1000, MaxHP: 1000},
		{ID: 1<<30 + 1, SceneID: 1, X: 1000, Y: 100, HP: 1000, MaxHP: 1000},
		{ID: 1<<30 + 2, SceneID: 2, X: 150, Y: 100, HP: 1000, MaxHP: 1000},
	} {
		if _, err := world.AddMonster(monster.ID, monster.SceneID, 31001, monster.X, monster.Y, monster.HP, monster.MaxHP); err != nil {
			t.Fatal(err)
		}
	}
	return world, combat, manager, actors[0], actors[1]
}

func TestSkillCastDamageCooldownAndResultLayout(t *testing.T) {
	world, _, manager, _, _ := newSkillCombatFixture(t)
	router, packets := newSkillTestRouter(7, 1)
	body := skillCastBody(1, 1<<30, 11001)
	if err := manager.Handle(7, 1, cSkillCmdCastSkill, body, router, nil, nil); err != nil {
		t.Fatal(err)
	}
	target, _ := world.Entity(1 << 30)
	if target.HP != 895 {
		t.Fatalf("技能伤害错误: hp=%d want=895", target.HP)
	}
	if len(*packets) != 1 || len((*packets)[0]) != 32 {
		t.Fatalf("技能结果包错误: %x", *packets)
	}
	packet := (*packets)[0]
	if packet[0] != cmdSkill || packet[1] != sSkillCmdCastSkill || binary.LittleEndian.Uint32(packet[10:14]) != 11001 || binary.LittleEndian.Uint16(packet[22:24]) != 1 {
		t.Fatalf("技能结果包字段错误: %x", packet)
	}
	if got := math.Float64frombits(binary.LittleEndian.Uint64(packet[2:10])); got != 1 {
		t.Fatalf("技能结果攻击者=%v", got)
	}
	if got := math.Float64frombits(binary.LittleEndian.Uint64(packet[14:22])); got != 1<<30 {
		t.Fatalf("技能结果目标=%v", got)
	}
	if got := math.Float64frombits(binary.LittleEndian.Uint64(packet[24:32])); got != 105 {
		t.Fatalf("技能结果伤害=%v", got)
	}
	if err := manager.Handle(7, 1, cSkillCmdCastSkill, body, router, nil, nil); err == nil {
		t.Fatal("冷却中应拒绝释放")
	}
	manager.now = func() time.Time { return time.Unix(1000, 0).Add(800 * time.Millisecond) }
	if err := manager.Handle(7, 1, cSkillCmdCastSkill, body, router, nil, nil); err != nil {
		t.Fatal(err)
	}
	target, _ = world.Entity(1 << 30)
	if target.HP != 790 {
		t.Fatalf("冷却结束后未再次伤害: hp=%d", target.HP)
	}
}

func TestSkillCooldownIsPerPlayerAndRejectsInvalidCasts(t *testing.T) {
	world, _, manager, actor, second := newSkillCombatFixture(t)
	router1, _ := newSkillTestRouter(7, 1)
	router2, _ := newSkillTestRouter(8, 2)
	valid := skillCastBody(1, 1<<30, 11001)
	if err := manager.Handle(7, 1, cSkillCmdCastSkill, valid, router1, nil, nil); err != nil {
		t.Fatal(err)
	}
	if err := manager.Handle(8, 2, cSkillCmdCastSkill, skillCastBody(2, 1<<30, 11001), router2, nil, nil); err != nil {
		t.Fatalf("另一玩家被错误共享冷却: %v", err)
	}
	before, _ := world.Entity(1<<30 + 1)
	actor.Skills[12001] = 1
	for _, tt := range []struct {
		name   string
		body   []byte
		mutate func()
	}{
		{name: "包长错误", body: valid[:19]},
		{name: "攻击者句柄错误", body: skillCastBody(2, 1<<30, 11001)},
		{name: "未学习技能", body: skillCastBody(1, 1<<30, 13001)},
		{name: "技能等级不匹配", body: skillCastBody(1, 1<<30, 11002)},
		{name: "目标越界", body: skillCastBody(1, 1<<30+1, 11001)},
		{name: "跨场景目标", body: skillCastBody(1, 1<<30+2, 11001)},
		{name: "AOE暂不支持", body: skillCastBody(1, 1<<30, 13001), mutate: func() { actor.Skills[13001] = 1 }},
		{name: "玩家死亡", body: skillCastBody(1, 1<<30, 11001), mutate: func() { actor.HP = 0 }},
		{name: "目标不是玩家", body: skillCastBody(1, 2, 11001)},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if tt.mutate != nil {
				tt.mutate()
			}
			if err := manager.Handle(7, 1, cSkillCmdCastSkill, tt.body, router1, nil, nil); err == nil {
				t.Fatal("应拒绝释放")
			}
		})
	}
	after, _ := world.Entity(1<<30 + 1)
	if before.HP != after.HP {
		t.Fatalf("无效释放造成伤害: before=%d after=%d", before.HP, after.HP)
	}
	second.Skills[11001] = 1
}

func TestSkillPostLoginInitializesAndRestoresRoleData(t *testing.T) {
	previous := DefaultActorManager
	t.Cleanup(func() { DefaultActorManager = previous })
	store := newSkillTestStore()
	store.actors[98765].Level = 55
	store.actors[98765].ZhuanSheng = 3
	store.persisted[98765] = actorPersistData{
		Version: actorPersistVersion,
		Skills:  map[int]int{11001: 4, 13001: 2},
	}
	manager := NewSkillManager(testSkillCatalog(t), nil)
	wantLevels := []uint32{4, 1, 2, 1, 1}

	for login := 0; login < 2; login++ {
		DefaultActorManager = NewActorManager()
		router := NewNetRouter(nil)
		var packets [][]byte
		router.SetSender(func(frame []byte) error {
			packets = append(packets, append([]byte(nil), frame[netframe.GateFrameHeaderLen+netframe.NetIDLen:]...))
			return nil
		})
		sid := uint16(20 + login)
		router.OpenSession(sid, "")
		router.SetAccount(sid, "帅哥", 121102, 1)
		post := newPostLoginManager(store, router, nil, manager)
		if err := post.HandlePostLogin(sid, 98765, "帅哥", nil); err != nil {
			t.Fatal(err)
		}
		var role []byte
		for _, packet := range packets {
			if len(packet) >= 2 && packet[0] == cmdBase && packet[1] == sBaseCmdRoleData {
				role = packet
				break
			}
		}
		if len(role) != 3677-4+436 {
			t.Fatalf("第%d次登录 RoleData 长度=%d", login+1, len(role))
		}
		for i, want := range wantLevels {
			off := 28 + i*4
			if got := binary.LittleEndian.Uint32(role[off : off+4]); got != want {
				t.Fatalf("第%d次登录技能%d等级=%d want=%d", login+1, i+1, got, want)
			}
		}
		actor := DefaultActorManager.Get(98765)
		if actor == nil || actor.ZhuanSheng != 3 || actor.Skills[11001] != 4 || actor.Skills[13001] != 2 {
			t.Fatalf("第%d次登录技能状态错误: %+v", login+1, actor)
		}
		if actor.SkillsDirty || actor.Dirty {
			t.Fatalf("第%d次登录还原后不应标脏", login+1)
		}
	}
}

func TestNetRouterDispatchesSkillWithoutLua(t *testing.T) {
	router := NewNetRouter(nil)
	router.OpenSession(7, "")
	router.SetAccount(7, "tester", 1, 1)
	router.BindActor(7, 1)
	called := false
	body := skillCastBody(1, 1<<30, 11001)
	router.SetSkillHandler(func(sid uint16, actorID int, cmdid uint16, got []byte) error {
		called = true
		if sid != 7 || actorID != 1 || cmdid != cSkillCmdCastSkill || !bytes.Equal(got, body) {
			return fmt.Errorf("技能路由参数错误: sid=%d actor=%d cmd=%d body=%x", sid, actorID, cmdid, got)
		}
		return nil
	})
	if err := router.RouteUpstream(clientBusinessBody(7, cmdSkill, cSkillCmdCastSkill, body)); err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Fatal("技能包未分流到 Go handler")
	}
}
