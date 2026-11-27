package engineapi

import (
	"bytes"
	"encoding/binary"
	"math"
	"sync"
	"testing"
	"time"

	"github.com/xiaokentrl/ChuanQi/ltcq-go/internal/luahost"
	"github.com/xiaokentrl/ChuanQi/ltcq-go/internal/scene"
)

func TestCombatProtocolLayout(t *testing.T) {
	hp := buildHpChange(1<<30, 400)
	if len(hp) != 18 || hp[0] != cmdBase || hp[1] != sBaseCmdHpChange {
		t.Fatalf("HP包错误: %x", hp)
	}
	if math.Float64frombits(binary.LittleEndian.Uint64(hp[2:10])) != 1<<30 ||
		math.Float64frombits(binary.LittleEndian.Uint64(hp[10:18])) != 400 {
		t.Fatalf("HP包字段错误: %x", hp)
	}
	damage := buildNotifyFlyHp(9502731, 1<<30, 100)
	if len(damage) != 30 || damage[0] != cmdBase || damage[1] != sBaseCmdNotifyFlyHp {
		t.Fatalf("飘血包错误: %x", damage)
	}
	for i, want := range []float64{9502731, 1 << 30, 100} {
		offset := 2 + i*8
		if got := math.Float64frombits(binary.LittleEndian.Uint64(damage[offset : offset+8])); got != want {
			t.Fatalf("飘血字段%d=%v want=%v", i, got, want)
		}
	}
	if binary.LittleEndian.Uint32(damage[26:30]) != 0 {
		t.Fatalf("普通攻击伤害类型错误: %x", damage[26:30])
	}
}

func TestBuildWorldPacketCombatEvents(t *testing.T) {
	post := &postLoginManager{}
	monster := scene.Entity{ID: 1 << 30, Kind: scene.EntityMonster, HP: 0}
	attacker := scene.Entity{ID: 9502731, Kind: scene.EntityPlayer}
	leave := []byte{cmdBase, sBaseCmdRemoveEntity}
	leave = binary.LittleEndian.AppendUint64(leave, math.Float64bits(float64(monster.ID)))
	for _, tt := range []struct {
		event scene.Event
		want  []byte
	}{
		{scene.Event{Kind: scene.EventHP, Entity: monster}, buildHpChange(monster.ID, 0)},
		{scene.Event{Kind: scene.EventDamage, Entity: monster, Attacker: attacker, Damage: 50}, buildNotifyFlyHp(attacker.ID, monster.ID, 50)},
		{scene.Event{Kind: scene.EventLeave, Entity: monster}, leave},
	} {
		got, err := post.buildWorldPacket(tt.event)
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != string(tt.want) {
			t.Fatalf("事件%d包=%x want=%x", tt.event.Kind, got, tt.want)
		}
	}
}

func TestCombatManagerAttacksNearestLivingMonsterInSameScene(t *testing.T) {
	world := scene.NewWorld()
	if err := world.SetScenes(map[int]scene.SceneConfig{
		1: {Width: 4096, Height: 4096, SpawnX: 1000, SpawnY: 1000},
		2: {Width: 4096, Height: 4096, SpawnX: 1000, SpawnY: 1000},
	}); err != nil {
		t.Fatal(err)
	}
	var packets [][]byte
	_, err := world.AddPlayer(9502731, 1, 1000, 1000, func(delivery scene.Delivery) error {
		packet, err := (&postLoginManager{}).buildWorldPacket(delivery.Event)
		if err == nil {
			packets = append(packets, packet)
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, monster := range []scene.Entity{
		{ID: 1 << 30, SceneID: 1, X: 1128, Y: 1000, HP: 500, MaxHP: 500},
		{ID: 1<<30 + 1, SceneID: 1, X: 1064, Y: 1000, HP: 500, MaxHP: 500},
		{ID: 1<<30 + 2, SceneID: 2, X: 1000, Y: 1000, HP: 500, MaxHP: 500},
	} {
		if _, err := world.AddMonster(monster.ID, monster.SceneID, 31001, monster.X, monster.Y, monster.HP, monster.MaxHP); err != nil {
			t.Fatal(err)
		}
	}
	combat := NewCombatManager(world, CombatConfig{Damage: 100, AttackRange: 128, Interval: 500 * time.Millisecond})
	combat.Tick()
	far, _ := world.Entity(1 << 30)
	near, _ := world.Entity(1<<30 + 1)
	otherScene, _ := world.Entity(1<<30 + 2)
	if far.HP != 500 || near.HP != 400 || otherScene.HP != 500 {
		t.Fatalf("自动战斗目标错误: far=%d near=%d other=%d", far.HP, near.HP, otherScene.HP)
	}
	if len(packets) != 2 || !bytes.Equal(packets[0], buildHpChange(near.ID, 400)) || !bytes.Equal(packets[1], buildNotifyFlyHp(9502731, near.ID, 100)) {
		t.Fatalf("自动战斗下行错误: %x", packets)
	}
}

func TestCombatManagerMonsterRetaliationUsesConfiguredInterval(t *testing.T) {
	DefaultActorManager = NewActorManager()
	actor := DefaultActorManager.GetOrCreate(1)
	actor.HP, actor.MaxHP = 100, 100
	world := scene.NewWorld()
	if err := world.SetScenes(map[int]scene.SceneConfig{1: {Width: 4096, Height: 4096, SpawnX: 0, SpawnY: 0}}); err != nil {
		t.Fatal(err)
	}
	var packets [][]byte
	post := &postLoginManager{}
	if _, err := world.AddPlayer(1, 1, 256, 384, func(delivery scene.Delivery) error {
		packet, err := post.buildWorldPacket(delivery.Event)
		if err == nil {
			packets = append(packets, packet)
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	monsters := NewMonsterManager(world, map[int]MonsterConfig{31001: {ID: 31001, HP: 127738}})
	if _, err := monsters.SpawnAll([]MonsterSpawn{{SceneID: 1, ConfigID: 31001, X: 6, Y: 6, Count: 1, HP: 500, AttackDamage: 10, AttackRange: 128, AttackIntervalSeconds: 1}}); err != nil {
		t.Fatal(err)
	}
	packets = nil
	combat := NewCombatManager(world, CombatConfig{Damage: 100, AttackRange: 128, Interval: 500 * time.Millisecond})
	combat.SetMonsterManager(monsters)
	now := time.Unix(1000, 0)
	combat.TickAt(now)
	want := [][]byte{
		buildHpChange(1<<30, 400),
		buildNotifyFlyHp(1, 1<<30, 100),
		buildHpChange(1, 90),
		buildNotifyFlyHp(1<<30, 1, 10),
	}
	if len(packets) != len(want) {
		t.Fatalf("反击包数量=%d want=%d", len(packets), len(want))
	}
	for i := range want {
		if !bytes.Equal(packets[i], want[i]) {
			t.Fatalf("反击包%d=%x want=%x", i, packets[i], want[i])
		}
	}
	combat.TickAt(now.Add(500 * time.Millisecond))
	if len(packets) != len(want)+2 || actor.HP != 90 {
		t.Fatalf("攻击间隔内发生反击: packets=%d hp=%d", len(packets), actor.HP)
	}
	combat.TickAt(now.Add(time.Second))
	if len(packets) != len(want)+6 || actor.HP != 80 {
		t.Fatalf("第二次反击未按间隔执行: packets=%d hp=%d", len(packets), actor.HP)
	}
	if !bytes.Equal(packets[len(packets)-2], buildHpChange(1, 80)) || !bytes.Equal(packets[len(packets)-1], buildNotifyFlyHp(1<<30, 1, 10)) {
		t.Fatalf("第二次反击下行错误: %x", packets[len(packets)-2:])
	}
	combat.TickAt(now.Add(2*time.Second - combatCooldownSlack/2))
	if actor.HP != 70 {
		t.Fatalf("tick 调度抖动导致反击延迟: hp=%d", actor.HP)
	}
}

func TestCombatManagerSkipsRetaliationWithoutTarget(t *testing.T) {
	world := scene.NewWorld()
	if err := world.SetScenes(map[int]scene.SceneConfig{1: {Width: 4096, Height: 4096, SpawnX: 0, SpawnY: 0}}); err != nil {
		t.Fatal(err)
	}
	if _, err := world.AddPlayer(1, 1, 448, 384, func(scene.Delivery) error { return nil }); err != nil {
		t.Fatal(err)
	}
	monsters := NewMonsterManager(world, map[int]MonsterConfig{31001: {ID: 31001, HP: 127738}})
	if _, err := monsters.SpawnAll([]MonsterSpawn{{SceneID: 1, ConfigID: 31001, X: 6, Y: 6, Count: 1, HP: 500, AttackDamage: 10, AttackRange: 128, AttackIntervalSeconds: 1}}); err != nil {
		t.Fatal(err)
	}
	combat := NewCombatManager(world, CombatConfig{Damage: 100, AttackRange: 1, Interval: 500 * time.Millisecond})
	combat.SetMonsterManager(monsters)
	combat.TickAt(time.Unix(1000, 0))
	player, _ := world.Entity(1)
	if player.HP != 100 {
		t.Fatalf("未被攻击的玩家遭到反击: %+v", player)
	}
}

func TestCombatManagerClearsAggroAfterTargetLeaves(t *testing.T) {
	for _, action := range []string{"disconnect", "outOfRange"} {
		t.Run(action, func(t *testing.T) {
			DefaultActorManager = NewActorManager()
			actor := DefaultActorManager.GetOrCreate(1)
			actor.HP, actor.MaxHP = 100, 100
			world := scene.NewWorld()
			if err := world.SetScenes(map[int]scene.SceneConfig{1: {Width: 4096, Height: 4096, SpawnX: 0, SpawnY: 0}}); err != nil {
				t.Fatal(err)
			}
			var packets [][]byte
			post := &postLoginManager{}
			observer := func(delivery scene.Delivery) error {
				packet, err := post.buildWorldPacket(delivery.Event)
				if err == nil {
					packets = append(packets, packet)
				}
				return err
			}
			if _, err := world.AddPlayer(1, 1, 383, 384, observer); err != nil {
				t.Fatal(err)
			}
			monsters := NewMonsterManager(world, map[int]MonsterConfig{31001: {ID: 31001, HP: 127738}})
			if _, err := monsters.SpawnAll([]MonsterSpawn{{SceneID: 1, ConfigID: 31001, X: 6, Y: 6, Count: 1, HP: 500, AttackDamage: 10, AttackRange: 128, AttackIntervalSeconds: 1}}); err != nil {
				t.Fatal(err)
			}
			combat := NewCombatManager(world, CombatConfig{Damage: 100, AttackRange: 1, Interval: 500 * time.Millisecond})
			combat.SetMonsterManager(monsters)
			now := time.Unix(1000, 0)
			combat.TickAt(now)
			if actor.HP != 90 {
				t.Fatalf("首次反击HP=%d", actor.HP)
			}
			if action == "disconnect" {
				world.RemovePlayer(1)
				combat.TickAt(now.Add(500 * time.Millisecond))
				if _, err := world.AddPlayer(1, 1, 600, 384, observer); err != nil {
					t.Fatal(err)
				}
			} else if _, err := world.MovePlayer(1, 600, 384); err != nil {
				t.Fatal(err)
			}
			packets = nil
			combat.TickAt(now.Add(time.Second))
			if actor.HP != 90 || len(packets) != 0 {
				t.Fatalf("目标离开后仍反击: hp=%d packets=%x", actor.HP, packets)
			}
		})
	}
}

func TestCombatManagerPlayerDeathAndRevival(t *testing.T) {
	previousActors := DefaultActorManager
	DefaultActorManager = NewActorManager()
	t.Cleanup(func() { DefaultActorManager = previousActors })
	actor := DefaultActorManager.GetOrCreate(1)
	actor.Name, actor.Level, actor.Exp = "测试角色", 51, 500
	actor.HP, actor.MaxHP = 10, 100
	actor.Currency[currencyGold] = 1000
	world := scene.NewWorld()
	if err := world.SetScenes(map[int]scene.SceneConfig{1: {Width: 4096, Height: 4096, SpawnX: 0, SpawnY: 0}}); err != nil {
		t.Fatal(err)
	}
	var packets [][]byte
	post := &postLoginManager{}
	if _, err := world.AddPlayerWithHealth(1, 1, 256, 384, 10, 100, func(delivery scene.Delivery) error {
		packet, err := post.buildWorldPacket(delivery.Event)
		if err == nil {
			packets = append(packets, packet)
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	monsters := NewMonsterManager(world, map[int]MonsterConfig{31001: {ID: 31001, HP: 127738}})
	if _, err := monsters.SpawnAll([]MonsterSpawn{{SceneID: 1, ConfigID: 31001, X: 6, Y: 6, Count: 1, HP: 500, AttackDamage: 10, AttackRange: 128, AttackIntervalSeconds: 2}}); err != nil {
		t.Fatal(err)
	}
	packets = nil
	combat := NewCombatManager(world, CombatConfig{Damage: 100, AttackRange: 128, Interval: 500 * time.Millisecond, ReviveAfter: 2 * time.Second})
	combat.SetMonsterManager(monsters)
	now := time.Unix(1000, 0)
	combat.TickAt(now)
	wantDeath := [][]byte{
		buildHpChange(1<<30, 400),
		buildNotifyFlyHp(1, 1<<30, 100),
		buildHpChange(1, 0),
		buildNotifyFlyHp(1<<30, 1, 10),
		{cmdBase, sBaseCmdRemoveEntity},
	}
	wantDeath[4] = binary.LittleEndian.AppendUint64(wantDeath[4], math.Float64bits(1))
	if len(packets) != len(wantDeath) {
		t.Fatalf("致死包数量=%d want=%d", len(packets), len(wantDeath))
	}
	for i := range wantDeath {
		if !bytes.Equal(packets[i], wantDeath[i]) {
			t.Fatalf("致死包%d=%x want=%x", i, packets[i], wantDeath[i])
		}
	}
	if actor.HP != 0 {
		t.Fatalf("Actor HP 未同步死亡: %d", actor.HP)
	}
	if _, err := world.MovePlayer(1, 300, 384); err == nil {
		t.Fatal("死亡玩家仍可移动")
	}
	if _, err := world.AttackMonsterResult(1, 1<<30, 100, 128); err == nil {
		t.Fatal("死亡玩家仍可攻击")
	}
	combat.TickAt(now.Add(time.Second))
	if len(packets) != len(wantDeath) || actor.HP != 0 {
		t.Fatalf("复活倒计时内状态错误: packets=%d hp=%d", len(packets), actor.HP)
	}
	combat.TickAt(now.Add(2 * time.Second))
	wantRevive := buildOtherEntityAt(&ActorData{ActorID: 1, Name: "测试角色", Job: 1, Level: 51, SceneID: 1, HP: 100, MaxHP: 100}, sceneSpawnResult{x: 256, y: 384})
	if len(packets) != len(wantDeath)+1 || !bytes.Equal(packets[len(packets)-1], wantRevive) {
		t.Fatalf("复活包错误: %x", packets[len(packets)-1])
	}
	player, ok := world.Entity(1)
	if !ok || player.HP != 100 || player.MaxHP != 100 || player.X != 256 || player.Y != 384 || actor.HP != 100 || actor.MaxHP != 100 {
		t.Fatalf("复活状态错误: world=%+v ok=%v actorHP=%d/%d", player, ok, actor.HP, actor.MaxHP)
	}
	combat.TickAt(now.Add(2500 * time.Millisecond))
	if len(packets) != len(wantDeath)+5 || packets[len(packets)-1][1] != sBaseCmdNotifyFlyHp {
		t.Fatalf("复活后未恢复自动战斗或重复复活: %x", packets[len(wantDeath)+1:])
	}
	monster, _ := world.Entity(1 << 30)
	if monster.HP != 300 || actor.HP != 90 || actor.Exp != 500 || actor.Currency[currencyGold] != 1000 {
		t.Fatalf("复活后战斗或奖励状态错误: monster=%+v exp=%d gold=%d", monster, actor.Exp, actor.Currency[currencyGold])
	}
}

func TestCombatManagerDropsRevivalAfterDisconnectAndRelogin(t *testing.T) {
	previousActors := DefaultActorManager
	DefaultActorManager = NewActorManager()
	t.Cleanup(func() { DefaultActorManager = previousActors })
	actor := DefaultActorManager.GetOrCreate(1)
	actor.HP, actor.MaxHP = 10, 100
	world := scene.NewWorld()
	if err := world.SetScenes(map[int]scene.SceneConfig{1: {Width: 4096, Height: 4096, SpawnX: 0, SpawnY: 0}}); err != nil {
		t.Fatal(err)
	}
	if _, err := world.AddPlayerWithHealth(1, 1, 256, 384, 10, 100, func(scene.Delivery) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if _, err := world.AddMonster(1<<30, 1, 31001, 384, 384, 500, 500); err != nil {
		t.Fatal(err)
	}
	combat := NewCombatManager(world, CombatConfig{Damage: 100, AttackRange: 1, Interval: 500 * time.Millisecond, ReviveAfter: 2 * time.Second})
	now := time.Unix(1000, 0)
	result, err := world.AttackPlayerResult(1<<30, 1, 10, 128)
	if err != nil || !result.Killed {
		t.Fatalf("致死攻击结果=%+v err=%v", result, err)
	}
	combat.handlePlayerDeath(result, now)
	world.RemovePlayer(1)
	if _, err := world.AddPlayer(1, 1, 600, 384, func(scene.Delivery) error { return nil }); err != nil {
		t.Fatal(err)
	}
	combat.TickAt(now.Add(2 * time.Second))
	player, ok := world.Entity(1)
	if !ok || player.HP != 100 || player.X != 600 || player.Y != 384 {
		t.Fatalf("重登玩家被旧复活计时器修改: %+v ok=%v", player, ok)
	}
	if len(combat.reviveAt) != 0 {
		t.Fatalf("断线后的复活计时器未清理: %+v", combat.reviveAt)
	}
}

func TestCombatKillGrantsRewardOnceAndRespawns(t *testing.T) {
	DefaultActorManager = NewActorManager()
	actor := DefaultActorManager.GetOrCreate(1)
	actor.Level, actor.Exp = 51, 500
	actor.Currency[currencyGold] = 1000
	bagStore := &memoryBagStore{}
	actor.ReplaceBag(nil, nil, bagStore)
	store := newMockStore()
	store.actors[1] = &ActorData{ActorID: 1, Level: 51, Exp: 500, Gold: 1000}
	world := scene.NewWorld()
	if err := world.SetScenes(map[int]scene.SceneConfig{1: {Width: 4096, Height: 4096, SpawnX: 0, SpawnY: 0}}); err != nil {
		t.Fatal(err)
	}
	var packets [][]byte
	post := &postLoginManager{}
	if _, err := world.AddPlayer(1, 1, 256, 384, func(delivery scene.Delivery) error {
		packet, err := post.buildWorldPacket(delivery.Event)
		if err == nil {
			packets = append(packets, packet)
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	monsters := NewMonsterManager(world, map[int]MonsterConfig{31001: {ID: 31001, HP: 127738}})
	if _, err := monsters.SpawnAll([]MonsterSpawn{{SceneID: 1, ConfigID: 31001, X: 6, Y: 6, Count: 1, HP: 100, Exp: 100, Gold: 1000, ItemID: 200013, ItemCount: 1, RespawnSeconds: 3}}); err != nil {
		t.Fatal(err)
	}
	actor.SetBagNotifier(func(packet []byte) error {
		packets = append(packets, append([]byte(nil), packet...))
		return nil
	})
	packets = nil
	combat := NewCombatManager(world, CombatConfig{Damage: 100, AttackRange: 128, Interval: 500 * time.Millisecond})
	combat.SetMonsterManager(monsters)
	combat.SetProgression(NewProgressionManager(store, map[int]int64{51: 1000, 52: 1200}))
	now := time.Unix(1000, 0)
	combat.TickAt(now)
	// A2：击杀物品奖励改走地面掉落——击杀包 = HP/飞血/怪物消失 + 掉落 OtherEntity + 金币/经验。
	want := [][]byte{
		buildHpChange(1<<30, 0),
		buildNotifyFlyHp(1, 1<<30, 100),
		{cmdBase, sBaseCmdRemoveEntity},
	}
	want[2] = binary.LittleEndian.AppendUint64(want[2], math.Float64bits(float64(1<<30)))
	if len(packets) != len(want)+3 { // want 3 帧 + 掉落 OtherEntity + 金币 + 经验
		t.Fatalf("击杀包数量=%d want=%d", len(packets), len(want)+3)
	}
	for i := range want {
		if !bytes.Equal(packets[i], want[i]) {
			t.Fatalf("击杀包%d=%x want=%x", i, packets[i], want[i])
		}
	}
	dropFrame := packets[3]
	if dropFrame[0] != cmdBase || dropFrame[1] != sBaseCmdOtherEntity || dropFrame[2] != 1 || binary.LittleEndian.Uint32(dropFrame[4:]) != 6 {
		t.Fatalf("击杀第4包非掉落 OtherEntity(type=6): %x", dropFrame)
	}
	pending := combat.Drops().NearDrops(1, 0, 0, 1<<30)
	if len(pending) != 1 || pending[0].ItemID != 200013 || pending[0].Count != 1 {
		t.Fatalf("击杀后应恰好 1 份待拾掉落(200013×1): %v", pending)
	}
	// 物品不再直接进包：经验/金币照常落盘，物品等拾取结算
	if store.actors[1].Exp != 600 || store.actors[1].Gold != 2000 || actor.Items[200013] != 0 || bagStore.saves != 0 {
		t.Fatalf("击杀奖励落盘异常: actor=%+v items=%v bagSaves=%d", store.actors[1], actor.Items, bagStore.saves)
	}
	combat.TickAt(now.Add(time.Second))
	if len(packets) != len(want)+3 || store.actors[1].Exp != 600 {
		t.Fatal("非致死 tick 重复发放了奖励")
	}
	// 拾取上行（[01][01] flag=0，玩家距击杀点 128=拾取半径内）：BagAdd + 掉落消失包广播
	if err := newDropPickupHandler(combat, world)(0, 1, []byte{0}); err != nil {
		t.Fatal(err)
	}
	if actor.Items[200013] != 1 || bagStore.saves != 1 || len(combat.Drops().NearDrops(1, 0, 0, 1<<30)) != 0 {
		t.Fatalf("拾取后物品未进包: items=%v bagSaves=%d", actor.Items, bagStore.saves)
	}
	if len(packets) != len(want)+5 || !bytes.Equal(packets[6], buildBagAddItem(1, 200013, 1, 0, nil, 0)) {
		t.Fatalf("拾取包序异常: %d 帧 / 包6=%x", len(packets), packets[6])
	}
	removeDrop := packets[7]
	if removeDrop[0] != cmdBase || removeDrop[1] != sBaseCmdRemoveEntity || binary.LittleEndian.Uint64(removeDrop[2:]) != pending[0].Handle {
		t.Fatalf("拾取后消失包异常: %x want handle=%x", removeDrop, pending[0].Handle)
	}
	basePackets := len(packets)
	combat.TickAt(now.Add(3 * time.Second))
	entity, ok := world.Entity(1 << 30)
	if !ok || entity.HP != 100 || len(packets) != basePackets+1 || packets[len(packets)-1][1] != sBaseCmdOtherEntity {
		t.Fatalf("重生结果=%+v ok=%v packets=%d", entity, ok, len(packets))
	}
	combat.TickAt(now.Add(3500 * time.Millisecond))
	if store.actors[1].Exp != 700 || store.actors[1].Gold != 3000 {
		t.Fatalf("第二次击杀奖励错误: actor=%+v", store.actors[1])
	}
	second := combat.Drops().NearDrops(1, 0, 0, 1<<30)
	if len(second) != 1 || second[0].ItemID != 200013 || actor.Items[200013] != 1 || bagStore.saves != 1 {
		t.Fatalf("第二次击杀应生成待拾掉落: %v items=%v saves=%d", second, actor.Items, bagStore.saves)
	}
	if err := newDropPickupHandler(combat, world)(0, 1, []byte{0}); err != nil {
		t.Fatal(err)
	}
	if actor.Items[200013] != 2 || bagStore.saves != 2 || len(combat.Drops().NearDrops(1, 0, 0, 1<<30)) != 0 {
		t.Fatalf("第二次拾取错误: items=%v bagSaves=%d", actor.Items, bagStore.saves)
	}
}

func setupMonsterAITest(t *testing.T, playerX int, spawn MonsterSpawn) (*scene.World, *CombatManager, *[][]byte) {
	t.Helper()
	previousActors := DefaultActorManager
	DefaultActorManager = NewActorManager()
	t.Cleanup(func() { DefaultActorManager = previousActors })
	actor := DefaultActorManager.GetOrCreate(1)
	actor.HP, actor.MaxHP = 100, 100
	world := scene.NewWorld()
	if err := world.SetScenes(map[int]scene.SceneConfig{1: {Width: 4096, Height: 4096, SpawnX: 0, SpawnY: 0}}); err != nil {
		t.Fatal(err)
	}
	var packets [][]byte
	post := &postLoginManager{}
	if _, err := world.AddPlayer(1, 1, playerX, 384, func(delivery scene.Delivery) error {
		packet, err := post.buildWorldPacket(delivery.Event)
		if err == nil {
			packets = append(packets, packet)
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	spawn.SceneID, spawn.ConfigID, spawn.X, spawn.Y, spawn.Count = 1, 31001, 7, 6, 1
	if spawn.HP == 0 {
		spawn.HP = 500
	}
	monsters := NewMonsterManager(world, map[int]MonsterConfig{31001: {ID: 31001, HP: 127738}})
	if _, err := monsters.SpawnAll([]MonsterSpawn{spawn}); err != nil {
		t.Fatal(err)
	}
	packets = nil
	combat := NewCombatManager(world, CombatConfig{Damage: 100, AttackRange: 1, Interval: 500 * time.Millisecond})
	combat.SetMonsterManager(monsters)
	return world, combat, &packets
}

func TestCombatManagerMonsterChasesAndAttacks(t *testing.T) {
	world, combat, packets := setupMonsterAITest(t, 256, MonsterSpawn{
		AttackDamage: 10, AttackRange: 128, AttackIntervalSeconds: 1,
		AggroRange: 256, ChaseStep: 64, LeashRange: 512,
	})
	now := time.Unix(1000, 0)
	combat.TickAt(now)
	monster, _ := world.Entity(1 << 30)
	if monster.X != 384 || monster.Y != 384 || combat.aggro[monster.ID] != 1 {
		t.Fatalf("索敌追击状态错误: monster=%+v aggro=%v", monster, combat.aggro)
	}
	wantMove := buildEntitySyncPosition(float64(monster.ID), 384, 384, 1, 16)
	if len(*packets) != 1 || !bytes.Equal((*packets)[0], wantMove) {
		t.Fatalf("追击位置同步包错误: %x", *packets)
	}
	combat.TickAt(now.Add(500 * time.Millisecond))
	want := [][]byte{wantMove, buildHpChange(1, 90), buildNotifyFlyHp(monster.ID, 1, 10)}
	if len(*packets) != len(want) {
		t.Fatalf("追击后攻击包数量=%d want=%d", len(*packets), len(want))
	}
	for i := range want {
		if !bytes.Equal((*packets)[i], want[i]) {
			t.Fatalf("追击后攻击包%d=%x want=%x", i, (*packets)[i], want[i])
		}
	}
}

func TestCombatManagerMonsterIgnoresFarAndDeadPlayers(t *testing.T) {
	t.Run("far", func(t *testing.T) {
		world, combat, packets := setupMonsterAITest(t, 1000, MonsterSpawn{
			AttackDamage: 10, AttackRange: 128, AttackIntervalSeconds: 1,
			AggroRange: 256, ChaseStep: 64, LeashRange: 512,
		})
		combat.TickAt(time.Unix(1000, 0))
		monster, _ := world.Entity(1 << 30)
		if monster.X != 448 || len(*packets) != 0 || len(combat.aggro) != 0 {
			t.Fatalf("范围外玩家触发 AI: monster=%+v packets=%x aggro=%v", monster, *packets, combat.aggro)
		}
	})
	t.Run("dead", func(t *testing.T) {
		world, combat, packets := setupMonsterAITest(t, 384, MonsterSpawn{
			AttackDamage: 10, AttackRange: 128, AttackIntervalSeconds: 1,
			AggroRange: 256, ChaseStep: 64, LeashRange: 512,
		})
		if _, err := world.AttackPlayerResult(1<<30, 1, 100, 128); err != nil {
			t.Fatal(err)
		}
		combat.TickAt(time.Unix(1000, 0))
		monster, _ := world.Entity(1 << 30)
		if monster.X != 448 || len(*packets) != 0 || len(combat.aggro) != 0 {
			t.Fatalf("死亡玩家触发 AI: monster=%+v packets=%x aggro=%v", monster, *packets, combat.aggro)
		}
	})
}

func TestCombatManagerMonsterChoosesNearestPlayer(t *testing.T) {
	world, combat, packets := setupMonsterAITest(t, 384, MonsterSpawn{
		AttackDamage: 10, AttackRange: 128, AttackIntervalSeconds: 1,
		AggroRange: 256, ChaseStep: 64, LeashRange: 512,
	})
	actor := DefaultActorManager.GetOrCreate(2)
	actor.HP, actor.MaxHP = 100, 100
	if _, err := world.AddPlayer(2, 1, 640, 384, func(scene.Delivery) error { return nil }); err != nil {
		t.Fatal(err)
	}
	combat.TickAt(time.Unix(1000, 0))
	if actor.HP != 100 || DefaultActorManager.Get(1).HP != 90 || combat.aggro[1<<30] != 1 {
		t.Fatalf("最近目标选择错误: hp1=%d hp2=%d aggro=%v", DefaultActorManager.Get(1).HP, actor.HP, combat.aggro)
	}
	if len(*packets) != 2 || (*packets)[0][1] != sBaseCmdHpChange || (*packets)[1][1] != sBaseCmdNotifyFlyHp {
		t.Fatalf("最近目标攻击包错误: %x", *packets)
	}
}

func TestCombatManagerMonsterReturnsHomeAfterLeash(t *testing.T) {
	world, combat, packets := setupMonsterAITest(t, 256, MonsterSpawn{
		AttackDamage: 10, AttackRange: 128, AttackIntervalSeconds: 1,
		AggroRange: 256, ChaseStep: 64, LeashRange: 512,
	})
	now := time.Unix(1000, 0)
	combat.TickAt(now)
	if _, err := world.MovePlayer(1, 1152, 384); err != nil {
		t.Fatal(err)
	}
	if _, err := world.MoveMonster(1<<30, 1048, 384); err != nil {
		t.Fatal(err)
	}
	*packets = nil
	combat.TickAt(now.Add(500 * time.Millisecond))
	monster, _ := world.Entity(1 << 30)
	if monster.X != 984 || len(combat.aggro) != 0 || !combat.returning[monster.ID] {
		t.Fatalf("超过归位范围后未脱战: monster=%+v aggro=%v returning=%v", monster, combat.aggro, combat.returning)
	}
	for step := 2; step <= 11; step++ {
		combat.TickAt(now.Add(time.Duration(step) * 500 * time.Millisecond))
		monster, _ = world.Entity(1 << 30)
		if len(combat.aggro) != 0 {
			t.Fatalf("归位期间重新索敌: step=%d monster=%+v aggro=%v", step, monster, combat.aggro)
		}
	}
	if monster.X != 448 || combat.returning[monster.ID] {
		t.Fatalf("归位状态未完成: monster=%+v returning=%v", monster, combat.returning)
	}
}

func TestCombatManagerMonsterReturnsHomeAfterDisconnect(t *testing.T) {
	world, combat, packets := setupMonsterAITest(t, 256, MonsterSpawn{
		AttackDamage: 10, AttackRange: 128, AttackIntervalSeconds: 1,
		AggroRange: 256, ChaseStep: 64, LeashRange: 512,
	})
	now := time.Unix(1000, 0)
	combat.TickAt(now)
	world.RemovePlayer(1)
	*packets = nil
	combat.TickAt(now.Add(500 * time.Millisecond))
	monster, _ := world.Entity(1 << 30)
	if monster.X != 448 || len(combat.aggro) != 0 || len(*packets) != 0 {
		t.Fatalf("断线后未脱战归位: monster=%+v aggro=%v packets=%x", monster, combat.aggro, *packets)
	}
}

func setupPlayerAITest(t *testing.T) (*scene.World, *CombatManager) {
	t.Helper()
	world := scene.NewWorld()
	if err := world.SetScenes(map[int]scene.SceneConfig{
		1: {Width: 4096, Height: 4096, SpawnX: 0, SpawnY: 0},
		2: {Width: 4096, Height: 4096, SpawnX: 0, SpawnY: 0},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := world.AddPlayer(1, 1, 256, 384, func(scene.Delivery) error { return nil }); err != nil {
		t.Fatal(err)
	}
	for _, monster := range []scene.Entity{
		{ID: 1 << 30, SceneID: 1, X: 640, Y: 384, HP: 500, MaxHP: 500},
		{ID: 1<<30 + 1, SceneID: 1, X: 320, Y: 384, HP: 500, MaxHP: 500},
		{ID: 1<<30 + 2, SceneID: 1, X: 384, Y: 384, HP: 500, MaxHP: 500},
		{ID: 1<<30 + 3, SceneID: 2, X: 256, Y: 384, HP: 500, MaxHP: 500},
	} {
		if _, err := world.AddMonster(monster.ID, monster.SceneID, 31001, monster.X, monster.Y, monster.HP, monster.MaxHP); err != nil {
			t.Fatal(err)
		}
	}
	return world, NewCombatManager(world, CombatConfig{Damage: 100, AttackRange: 128, Interval: time.Hour})
}

func TestCombatManagerPlayerAIStopTargetAndResume(t *testing.T) {
	world, combat := setupPlayerAITest(t)
	nearID, farID := 1<<30+1, 1<<30+2
	now := time.Unix(1000, 0)
	if err := combat.SetAITarget(1, farID); err != nil {
		t.Fatal(err)
	}
	combat.TickAt(now)
	near, _ := world.Entity(nearID)
	far, _ := world.Entity(farID)
	if near.HP != 500 || far.HP != 400 {
		t.Fatalf("显式目标未覆盖最近怪: near=%d far=%d", near.HP, far.HP)
	}
	if err := combat.StopPlayerAI(1); err != nil {
		t.Fatal(err)
	}
	combat.TickAt(now.Add(time.Second))
	near, _ = world.Entity(nearID)
	far, _ = world.Entity(farID)
	if near.HP != 500 || far.HP != 400 {
		t.Fatalf("停止 AI 后仍然攻击: near=%d far=%d", near.HP, far.HP)
	}
	if target, ok := combat.GetAITarget(1); !ok || target != farID {
		t.Fatalf("停止 AI 不应清掉目标: target=%d ok=%v", target, ok)
	}
	if err := combat.ResumePlayerAutoAttack(1); err != nil {
		t.Fatal(err)
	}
	combat.TickAt(now.Add(2 * time.Second))
	near, _ = world.Entity(nearID)
	if near.HP != 400 {
		t.Fatalf("恢复 AI 后未攻击最近怪: near=%d", near.HP)
	}
	if _, ok := combat.GetAITarget(1); ok {
		t.Fatal("恢复自动索敌后仍保留显式目标")
	}
	if err := combat.SetAITarget(1, 1<<30); err != nil {
		t.Fatal(err)
	}
	combat.TickAt(now.Add(3 * time.Second))
	near, _ = world.Entity(nearID)
	outOfRange, _ := world.Entity(1 << 30)
	if near.HP != 400 || outOfRange.HP != 500 {
		t.Fatalf("显式目标超出范围时错误回退最近怪: near=%d target=%d", near.HP, outOfRange.HP)
	}
	if err := combat.SetAITarget(1, farID); err != nil {
		t.Fatal(err)
	}
	if _, err := combat.ResolvePlayerAttack(1, farID, 500, 128, now.Add(4*time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, ok := combat.GetAITarget(1); ok {
		t.Fatal("目标死亡后仍保留 AI 目标")
	}
}

func TestCombatManagerSetAITargetRejectsUnsupportedPairs(t *testing.T) {
	world, combat := setupPlayerAITest(t)
	if _, err := world.AddPlayer(2, 1, 320, 384, func(scene.Delivery) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if _, err := world.AddPlayer(3, 2, 256, 384, func(scene.Delivery) error { return nil }); err != nil {
		t.Fatal(err)
	}
	deadID := 1<<30 + 4
	if _, err := world.AddMonster(deadID, 1, 31001, 256, 384, 1, 1); err != nil {
		t.Fatal(err)
	}
	if _, err := combat.ResolvePlayerAttack(1, deadID, 1, 128, time.Unix(1000, 0)); err != nil {
		t.Fatal(err)
	}
	for _, pair := range [][2]int{
		{1, 2},
		{1, 3},
		{1, 1<<30 + 3},
		{1, deadID},
		{1, 999999},
		{1<<30 + 3, 1},
		{1 << 30, 1<<30 + 1},
		{1 << 30, 3},
		{999999, 1},
	} {
		if err := combat.SetAITarget(pair[0], pair[1]); err == nil {
			t.Errorf("非法 AI 目标对应被接受: %v", pair)
		}
	}
	if len(combat.playerTargets) != 0 || len(combat.aggro) != 0 {
		t.Fatalf("非法调用污染 AI 状态: players=%v monsters=%v", combat.playerTargets, combat.aggro)
	}
}

func TestCombatManagerMonsterExplicitTargetAndCleanup(t *testing.T) {
	world, combat, _ := setupMonsterAITest(t, 256, MonsterSpawn{
		AttackDamage: 10, AttackRange: 256, AttackIntervalSeconds: 1,
		AggroRange: 64, ChaseStep: 64, LeashRange: 512,
	})
	actor2 := DefaultActorManager.GetOrCreate(2)
	actor2.HP, actor2.MaxHP = 100, 100
	if _, err := world.AddPlayer(2, 1, 384, 384, func(scene.Delivery) error { return nil }); err != nil {
		t.Fatal(err)
	}
	monsterID := 1 << 30
	if err := combat.SetAITarget(monsterID, 1); err != nil {
		t.Fatal(err)
	}
	combat.TickAt(time.Unix(1000, 0))
	if actor2.HP != 100 || DefaultActorManager.Get(1).HP != 90 {
		t.Fatalf("怪物未优先攻击显式目标: actor1=%d actor2=%d", DefaultActorManager.Get(1).HP, actor2.HP)
	}
	if target, ok := combat.GetAITarget(monsterID); !ok || target != 1 {
		t.Fatalf("怪物目标读取错误: target=%d ok=%v", target, ok)
	}
	world.RemovePlayer(1)
	if _, ok := combat.GetAITarget(monsterID); ok {
		t.Fatal("玩家离场后怪物目标未清理")
	}
	combat.RemovePlayerAI(2)
	if _, ok := world.Entity(2); !ok {
		t.Fatal("RemovePlayerAI 不应操作场景实体")
	}
}

func TestActorAIAPIUsesSharedCombatAndFailsClosed(t *testing.T) {
	previousActors := DefaultActorManager
	DefaultActorManager = NewActorManager()
	world, combat := setupPlayerAITest(t)
	t.Cleanup(func() {
		DefaultActorManager = previousActors
	})
	DefaultActorManager.GetOrCreate(1)
	g := luahost.NewGopherLua(luahost.Config{OSType: "WIN"})
	defer g.Close()
	if err := InstallAll(g); err != nil {
		t.Fatal(err)
	}
	installCombatAPI(g, combat)
	if _, err := g.DoString("LActor.stopAI(1)"); err != nil {
		t.Fatal(err)
	}
	combat.TickAt(time.Unix(1000, 0))
	near, _ := world.Entity(1<<30 + 1)
	if near.HP != 500 {
		t.Fatalf("Lua stopAI 未停止攻击: near=%d", near.HP)
	}
	if _, err := g.DoString("LActor.setAITarget(1, 1073741826)"); err != nil {
		t.Fatal(err)
	}
	value, err := g.DoString("(function() return LActor.getAITarget(1) end)()")
	if err != nil || value.(int) != 1<<30+2 {
		t.Fatalf("Lua AI 目标读取错误: value=%v err=%v", value, err)
	}
	combat.TickAt(time.Unix(1001, 0))
	far, _ := world.Entity(1<<30 + 2)
	if far.HP != 400 {
		t.Fatalf("setAITarget 应恢复停战并攻击显式目标: target=%d", far.HP)
	}
	if _, err := g.DoString("LActor.setAIAttackMonster(1)"); err != nil {
		t.Fatal(err)
	}
	combat.TickAt(time.Unix(1002, 0))
	near, _ = world.Entity(1<<30 + 1)
	if near.HP != 400 {
		t.Fatalf("setAIAttackMonster 未恢复最近怪索敌: near=%d", near.HP)
	}
	for _, statement := range []string{
		"LActor.stopAI()",
		"LActor.stopAI(0)",
		"LActor.stopAI(-1)",
		"LActor.stopAI(1.5)",
		"LActor.stopAI(\"1\")",
		"LActor.stopAI(0/0)",
		"LActor.stopAI(math.huge)",
		"LActor.stopAI(999)",
		"LActor.setAITarget()",
		"LActor.setAITarget(1)",
		"LActor.setAITarget(1, 2)",
		"LActor.setAITarget(1, \"1073741826\")",
		"LActor.getAITarget()",
		"LActor.getAITarget(0)",
		"LActor.getAITarget(0/0)",
		"LActor.getAITarget(999)",
		"LActor.setAITargetNull()",
		"LActor.setAITargetNull(0)",
		"LActor.setAITargetNull(1.5)",
		"LActor.setAITargetNull(\"1\")",
		"LActor.setAITargetNull(999)",
		"LActor.setAIAttackMonster()",
		"LActor.setAIAttackMonster(0)",
		"LActor.setAIAttackMonster(999)",
	} {
		if _, err := g.DoString(statement); err != nil {
			t.Fatalf("非法 Lua AI 参数应失败关闭而非报错: %s: %v", statement, err)
		}
	}
	if DefaultActorManager.Get(999) != nil {
		t.Fatal("非法或离线 AI 调用创建了 Actor")
	}
	unbound := luahost.NewGopherLua(luahost.Config{OSType: "WIN"})
	defer unbound.Close()
	if err := InstallAll(unbound); err != nil {
		t.Fatal(err)
	}
	installCombatAPI(unbound, nil)
	for _, statement := range []string{"LActor.stopAI(1)", "LActor.setAITarget(1, 1073741826)", "LActor.setAIAttackMonster(1)", "LActor.setAITargetNull(1)"} {
		if _, err := unbound.DoString(statement); err != nil {
			t.Fatalf("未绑定 Combat 时 AI API 应无操作: %s: %v", statement, err)
		}
	}
}

func TestCombatManagerClearAITargetPreservesStopped(t *testing.T) {
	world, combat := setupPlayerAITest(t)
	targetID := 1<<30 + 2
	if err := combat.SetAITarget(1, targetID); err != nil {
		t.Fatal(err)
	}
	if err := combat.StopPlayerAI(1); err != nil {
		t.Fatal(err)
	}
	if err := combat.ClearAITarget(1); err != nil {
		t.Fatal(err)
	}
	if _, ok := combat.GetAITarget(1); ok {
		t.Fatal("清目标后仍保留显式目标")
	}
	if !combat.stopped[1] {
		t.Fatal("清目标解除了停战")
	}
	combat.TickAt(time.Unix(1000, 0))
	near, _ := world.Entity(1<<30 + 1)
	if near.HP != 500 {
		t.Fatal("清目标后停战玩家仍然自动攻击")
	}
	if err := combat.SetAITarget(1<<30, 1); err != nil {
		t.Fatal(err)
	}
	if err := combat.ClearAITarget(1 << 30); err != nil {
		t.Fatal(err)
	}
	if _, ok := combat.GetAITarget(1 << 30); ok {
		t.Fatal("清目标后仍保留怪物仇恨")
	}
	world.RemovePlayer(1)
	if err := combat.ClearAITarget(1); err == nil {
		t.Fatal("离场实体允许清 AI 目标")
	}
}

func TestActorCombatAPIIsolatedPerVM(t *testing.T) {
	previousActors := DefaultActorManager
	DefaultActorManager = NewActorManager()
	t.Cleanup(func() { DefaultActorManager = previousActors })
	DefaultActorManager.GetOrCreate(1)
	world1, combat1 := setupPlayerAITest(t)
	world2, combat2 := setupPlayerAITest(t)
	g1 := luahost.NewGopherLua(luahost.Config{OSType: "WIN"})
	defer g1.Close()
	g2 := luahost.NewGopherLua(luahost.Config{OSType: "WIN"})
	defer g2.Close()
	for _, g := range []*luahost.GopherLua{g1, g2} {
		if err := InstallAll(g); err != nil {
			t.Fatal(err)
		}
	}
	installCombatAPI(g1, combat1)
	installCombatAPI(g2, combat2)
	if _, err := g1.DoString("LActor.stopAI(1)"); err != nil {
		t.Fatal(err)
	}
	if _, err := g2.DoString("LActor.setAITarget(1, 1073741826)"); err != nil {
		t.Fatal(err)
	}
	if !combat1.stopped[1] || combat2.stopped[1] {
		t.Fatal("停战状态跨 VM 串线")
	}
	if _, ok := combat1.GetAITarget(1); ok {
		t.Fatal("第二个 VM 的目标写入第一个 Combat")
	}
	if target, ok := combat2.GetAITarget(1); !ok || target != 1<<30+2 {
		t.Fatal("第二个 VM 未绑定自己的 Combat")
	}
	if _, err := g2.DoString("LActor.setAITargetNull(1)"); err != nil {
		t.Fatal(err)
	}
	if _, ok := combat2.GetAITarget(1); ok {
		t.Fatal("setAITargetNull 未清第二个 VM 的目标")
	}
	if entity, ok := world1.Entity(1); !ok || entity.HP != 100 {
		t.Fatal("隔离测试意外修改第一个 World")
	}
	if entity, ok := world2.Entity(1); !ok || entity.HP != 100 {
		t.Fatal("隔离测试意外修改第二个 World")
	}
}

func TestCombatManagerSkipsPlayerKilledEarlierInTick(t *testing.T) {
	previousActors := DefaultActorManager
	DefaultActorManager = NewActorManager()
	t.Cleanup(func() { DefaultActorManager = previousActors })
	actor := DefaultActorManager.GetOrCreate(1)
	actor.HP, actor.MaxHP = 10, 100
	world := scene.NewWorld()
	if err := world.SetScenes(map[int]scene.SceneConfig{
		1: {Width: 4096, Height: 4096, SpawnX: 0, SpawnY: 0},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := world.AddPlayerWithHealth(1, 1, 256, 384, 10, 100, func(scene.Delivery) error { return nil }); err != nil {
		t.Fatal(err)
	}
	monsters := NewMonsterManager(world, map[int]MonsterConfig{31001: {ID: 31001, HP: 500}})
	rule := MonsterSpawn{SceneID: 1, ConfigID: 31001, X: 6, Y: 6, Count: 1, HP: 500, AttackDamage: 10, AttackRange: 128, AttackIntervalSeconds: 1, AggroRange: 128, ChaseStep: 64, LeashRange: 512}
	if _, err := monsters.SpawnAll([]MonsterSpawn{rule, rule}); err != nil {
		t.Fatal(err)
	}
	combat := NewCombatManager(world, CombatConfig{Damage: 100, AttackRange: 1, Interval: time.Hour})
	combat.SetMonsterManager(monsters)
	combat.TickAt(time.Unix(1000, 0))
	if actor.HP != 0 {
		t.Fatalf("首个怪物未击杀玩家: hp=%d", actor.HP)
	}
	if _, ok := combat.aggro[1<<30+1]; ok {
		t.Fatal("后续怪物根据旧 HP 快照索敌已死亡玩家")
	}
}

func TestCombatManagerAIConcurrentAccess(t *testing.T) {
	world, combat := setupPlayerAITest(t)
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for worker := 0; worker < 8; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for iteration := 0; iteration < 100; iteration++ {
				if err := combat.SetAITarget(1, 1<<30+2); err != nil {
					errs <- err
					return
				}
				combat.GetAITarget(1)
				if err := combat.StopPlayerAI(1); err != nil {
					errs <- err
					return
				}
				if err := combat.ResumePlayerAutoAttack(1); err != nil {
					errs <- err
					return
				}
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
	if entity, ok := world.Entity(1); !ok || entity.HP != 100 {
		t.Fatalf("并发 AI 状态操作改变了玩家实体: entity=%+v ok=%v", entity, ok)
	}
}
