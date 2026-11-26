package scene

import (
	"errors"
	"sync"
	"testing"
)

func newCombatTestWorld(t *testing.T) *World {
	t.Helper()
	world := NewWorld()
	if err := world.SetScenes(map[int]SceneConfig{
		1: {Width: 4096, Height: 4096, SpawnX: 1000, SpawnY: 1000},
		2: {Width: 4096, Height: 4096, SpawnX: 1000, SpawnY: 1000},
	}); err != nil {
		t.Fatal(err)
	}
	return world
}

func addCombatPlayer(t *testing.T, world *World, id, sceneID, x, y int, deliveries *[]Delivery) {
	t.Helper()
	entered, err := world.AddPlayer(id, sceneID, x, y, func(delivery Delivery) error {
		if deliveries != nil {
			*deliveries = append(*deliveries, delivery)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := world.Dispatch(entered); err != nil {
		t.Fatal(err)
	}
}

func TestAttackMonsterDamageAndDeathOrder(t *testing.T) {
	world := newCombatTestWorld(t)
	var first, second []Delivery
	addCombatPlayer(t, world, 1, 1, 1000, 1000, &first)
	addCombatPlayer(t, world, 2, 1, 1000, 1000, &second)
	first, second = nil, nil
	if _, err := world.AddMonster(1<<30, 1, 31001, 1064, 1000, 250, 250); err != nil {
		t.Fatal(err)
	}

	for i, remaining := range []int{150, 50, 0} {
		applied := 100
		if i == 2 {
			applied = 50
		}
		deliveries, err := world.AttackMonster(1, 1<<30, 100, 128)
		if err != nil {
			t.Fatal(err)
		}
		wantDeliveries, wantObserved := 4, 2
		if remaining == 0 {
			wantDeliveries, wantObserved = 6, 3
		}
		if len(deliveries) != wantDeliveries {
			t.Fatalf("第%d次攻击观察者事件数=%d want=%d", i+1, len(deliveries), wantDeliveries)
		}
		first, second = nil, nil
		if err := world.Dispatch(deliveries); err != nil {
			t.Fatal(err)
		}
		if len(first) != wantObserved || len(second) != wantObserved {
			t.Fatalf("第%d次攻击广播=%d/%d want=%d/%d", i+1, len(first), len(second), wantObserved, wantObserved)
		}
		for _, observed := range [][]Delivery{first, second} {
			if observed[0].Event.Kind != EventHP || observed[0].Event.Entity.HP != remaining {
				t.Fatalf("HP事件错误: %+v", observed[0])
			}
			if observed[1].Event.Kind != EventDamage || observed[1].Event.Attacker.ID != 1 || observed[1].Event.Entity.ID != 1<<30 || observed[1].Event.Damage != applied {
				t.Fatalf("飘血事件错误: %+v", observed[1])
			}
			if remaining == 0 {
				if observed[2].Event.Kind != EventLeave {
					t.Fatalf("死亡最后事件=%+v", observed[2])
				}
			}
		}
		if remaining > 0 {
			entity, ok := world.Entity(1 << 30)
			if !ok || entity.HP != remaining {
				t.Fatalf("怪物HP=%+v ok=%v want=%d", entity, ok, remaining)
			}
		}
	}
	if _, ok := world.Entity(1 << 30); ok {
		t.Fatal("死亡怪物仍在世界中")
	}
}

func TestAttackMonsterRejectsInvalidTargets(t *testing.T) {
	tests := []struct {
		name   string
		player int
		target int
		damage int
		range_ int
	}{
		{name: "攻击者不存在", player: 99, target: 1 << 30, damage: 10, range_: 128},
		{name: "攻击者是怪物", player: 1 << 30, target: 1 << 30, damage: 10, range_: 128},
		{name: "目标是玩家", player: 1, target: 2, damage: 10, range_: 128},
		{name: "目标不存在", player: 1, target: 99, damage: 10, range_: 128},
		{name: "跨场景", player: 1, target: 1 << 30, damage: 10, range_: 128},
		{name: "超距离", player: 1, target: 1 << 30, damage: 10, range_: 63},
		{name: "伤害非法", player: 1, target: 1 << 30, damage: 0, range_: 128},
		{name: "范围非法", player: 1, target: 1 << 30, damage: 10, range_: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			world := newCombatTestWorld(t)
			addCombatPlayer(t, world, 1, 1, 1000, 1000, nil)
			addCombatPlayer(t, world, 2, 1, 1000, 1000, nil)
			sceneID, x := 1, 1064
			if tt.name == "跨场景" {
				sceneID, x = 2, 1064
			}
			if _, err := world.AddMonster(1<<30, sceneID, 31001, x, 1000, 100, 100); err != nil {
				t.Fatal(err)
			}
			if _, err := world.AttackMonster(tt.player, tt.target, tt.damage, tt.range_); err == nil {
				t.Fatal("非法攻击应拒绝")
			}
			entity, ok := world.Entity(1 << 30)
			if !ok || entity.HP != 100 {
				t.Fatalf("非法攻击修改了怪物: %+v ok=%v", entity, ok)
			}
		})
	}
}

func TestAttackMonsterConcurrentDeathAppliesOnce(t *testing.T) {
	world := newCombatTestWorld(t)
	addCombatPlayer(t, world, 1, 1, 1000, 1000, nil)
	if _, err := world.AddMonster(1<<30, 1, 31001, 1000, 1000, 100, 100); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	var mu sync.Mutex
	succeeded, failed, killed := 0, 0, 0
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			result, err := world.AttackMonsterResult(1, 1<<30, 10, 128)
			mu.Lock()
			defer mu.Unlock()
			if err == nil {
				succeeded++
				if result.Killed {
					killed++
				}
			} else {
				failed++
			}
		}()
	}
	wg.Wait()
	if succeeded != 10 || failed != 10 || killed != 1 {
		t.Fatalf("并发攻击成功/失败/致死=%d/%d/%d want=10/10/1", succeeded, failed, killed)
	}
	if _, ok := world.Entity(1 << 30); ok {
		t.Fatal("并发死亡后怪物仍存在")
	}
}

func TestMonsterAttackPlayerDamageAndDeathClamp(t *testing.T) {
	world := newCombatTestWorld(t)
	var first, second []Delivery
	addCombatPlayer(t, world, 1, 1, 1000, 1000, &first)
	addCombatPlayer(t, world, 2, 1, 1000, 1000, &second)
	first, second = nil, nil
	if _, err := world.AddMonster(1<<30, 1, 31001, 1064, 1000, 250, 250); err != nil {
		t.Fatal(err)
	}
	result, err := world.AttackPlayerResult(1<<30, 1, 25, 128)
	if err != nil {
		t.Fatal(err)
	}
	if err := world.Dispatch(result.Deliveries); err != nil {
		t.Fatal(err)
	}
	for _, observed := range [][]Delivery{first, second} {
		if len(observed) != 2 || observed[0].Event.Kind != EventHP || observed[0].Event.Entity.HP != 75 {
			t.Fatalf("玩家受击广播错误: %+v", observed)
		}
		if observed[1].Event.Kind != EventDamage || observed[1].Event.Attacker.ID != 1<<30 || observed[1].Event.Entity.ID != 1 || observed[1].Event.Damage != 25 {
			t.Fatalf("怪物飘血事件错误: %+v", observed[1])
		}
	}
	if _, err := world.MovePlayer(1, 1001, 1000); err != nil {
		t.Fatal(err)
	}
	player, _ := world.Entity(1)
	if player.HP != 75 || player.MaxHP != 100 {
		t.Fatalf("移动后玩家HP=%+v", player)
	}
	result, err = world.AttackPlayerResult(1<<30, 1, 100, 128)
	if err != nil {
		t.Fatal(err)
	}
	if result.Damage != 75 || result.Target.HP != 0 || !result.Killed {
		t.Fatalf("致死伤害未钳制或未标记死亡: %+v", result)
	}
	first, second = nil, nil
	if err := world.Dispatch(result.Deliveries); err != nil {
		t.Fatal(err)
	}
	for _, observed := range [][]Delivery{first, second} {
		if len(observed) != 3 || observed[2].Event.Kind != EventLeave || observed[2].Event.Entity.HP != 0 {
			t.Fatalf("玩家致死包序错误: %+v", observed)
		}
	}
	if _, err := world.AttackPlayerResult(1<<30, 1, 1, 128); err == nil {
		t.Fatal("已倒下玩家不应继续受击")
	}
	if _, err := world.MovePlayer(1, 1001, 1000); err == nil {
		t.Fatal("已倒下玩家不应移动")
	}
	if _, err := world.AttackMonsterResult(1, 1<<30, 1, 128); err == nil {
		t.Fatal("已倒下玩家不应攻击")
	}
	player, ok := world.Entity(1)
	if !ok || player.HP != 0 || player.X != 1001 || player.Y != 1000 {
		t.Fatalf("死亡玩家状态错误: %+v ok=%v", player, ok)
	}
}

func TestMonsterAttackPlayerRejectsInvalidState(t *testing.T) {
	tests := []struct {
		name     string
		attacker int
		target   int
		damage   int
		range_   int
	}{
		{name: "攻击者不存在", attacker: 99, target: 1, damage: 10, range_: 128},
		{name: "攻击者是玩家", attacker: 1, target: 2, damage: 10, range_: 128},
		{name: "目标是怪物", attacker: 1 << 30, target: 1 << 30, damage: 10, range_: 128},
		{name: "跨场景", attacker: 1 << 30, target: 2, damage: 10, range_: 128},
		{name: "超距离", attacker: 1 << 30, target: 1, damage: 10, range_: 63},
		{name: "伤害非法", attacker: 1 << 30, target: 1, damage: 0, range_: 128},
		{name: "范围非法", attacker: 1 << 30, target: 1, damage: 10, range_: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			world := newCombatTestWorld(t)
			addCombatPlayer(t, world, 1, 1, 1000, 1000, nil)
			addCombatPlayer(t, world, 2, 1, 1000, 1000, nil)
			sceneID, x := 1, 1064
			if tt.name == "跨场景" {
				sceneID, x = 2, 1064
			}
			if _, err := world.AddMonster(1<<30, sceneID, 31001, x, 1000, 100, 100); err != nil {
				t.Fatal(err)
			}
			if _, err := world.AttackPlayerResult(tt.attacker, tt.target, tt.damage, tt.range_); err == nil {
				t.Fatal("非法反击应拒绝")
			}
		})
	}
}

func TestMonsterAttackPlayerConcurrentDeathAppliesOnce(t *testing.T) {
	world := newCombatTestWorld(t)
	addCombatPlayer(t, world, 1, 1, 1000, 1000, nil)
	if _, err := world.AddMonster(1<<30, 1, 31001, 1000, 1000, 100, 100); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	var mu sync.Mutex
	succeeded, failed, killed := 0, 0, 0
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			result, err := world.AttackPlayerResult(1<<30, 1, 10, 128)
			mu.Lock()
			defer mu.Unlock()
			if err == nil {
				succeeded++
				if result.Killed {
					killed++
				}
			} else {
				failed++
			}
		}()
	}
	wg.Wait()
	if succeeded != 10 || failed != 10 || killed != 1 {
		t.Fatalf("并发反击成功/失败/致死=%d/%d/%d want=10/10/1", succeeded, failed, killed)
	}
	player, ok := world.Entity(1)
	if !ok || player.HP != 0 {
		t.Fatalf("并发致死后玩家状态=%+v ok=%v", player, ok)
	}
}

func TestRevivePlayerRestoresHealthAndAOI(t *testing.T) {
	world := newCombatTestWorld(t)
	var first, second []Delivery
	addCombatPlayer(t, world, 1, 1, 1000, 1000, &first)
	addCombatPlayer(t, world, 2, 1, 1000, 1000, &second)
	if _, err := world.AddMonster(1<<30, 1, 31001, 1064, 1000, 250, 250); err != nil {
		t.Fatal(err)
	}
	result, err := world.AttackPlayerResult(1<<30, 1, 100, 128)
	if err != nil || !result.Killed {
		t.Fatalf("致死攻击结果=%+v err=%v", result, err)
	}
	first, second = nil, nil
	if err := world.Dispatch(result.Deliveries); err != nil {
		t.Fatal(err)
	}
	first, second = nil, nil
	entered, err := world.AddMonster(1<<30+1, 1, 31001, 1000, 1000, 100, 100)
	if err != nil {
		t.Fatal(err)
	}
	if err := world.Dispatch(entered); err != nil {
		t.Fatal(err)
	}
	if len(first) != 0 || len(second) != 1 {
		t.Fatalf("死亡玩家仍进入 AOI: first=%+v second=%+v", first, second)
	}
	first, second = nil, nil
	revived, err := world.RevivePlayer(1)
	if err != nil {
		t.Fatal(err)
	}
	if len(revived) != 2 || revived[0].ObserverID != 1 || revived[1].ObserverID != 2 {
		t.Fatalf("复活广播对象错误: %+v", revived)
	}
	for _, delivery := range revived {
		if delivery.Event.Kind != EventEnter || delivery.Event.Entity.HP != 100 || delivery.Event.Entity.MaxHP != 100 || delivery.Event.Entity.X != 1000 || delivery.Event.Entity.Y != 1000 {
			t.Fatalf("复活实体错误: %+v", delivery.Event)
		}
	}
	if err := world.Dispatch(revived); err != nil {
		t.Fatal(err)
	}
	if len(first) != 1 || len(second) != 1 {
		t.Fatalf("复活 AOI 广播错误: first=%+v second=%+v", first, second)
	}
	if _, err := world.RevivePlayer(1); err == nil {
		t.Fatal("重复复活应拒绝")
	}
	if _, err := world.AttackPlayerResult(1<<30, 1, 100, 128); err != nil {
		t.Fatal(err)
	}
	if leaves := world.RemovePlayer(1); len(leaves) != 0 {
		t.Fatalf("死亡玩家断线不应重复发送消失包: %+v", leaves)
	}
}

func TestRevivePlayerBroadcastFailureDoesNotRollback(t *testing.T) {
	world := newCombatTestWorld(t)
	if _, err := world.AddPlayer(1, 1, 1000, 1000, func(Delivery) error { return errors.New("send failed") }); err != nil {
		t.Fatal(err)
	}
	if _, err := world.AddMonster(1<<30, 1, 31001, 1000, 1000, 100, 100); err != nil {
		t.Fatal(err)
	}
	if _, err := world.AttackPlayerResult(1<<30, 1, 100, 128); err != nil {
		t.Fatal(err)
	}
	deliveries, err := world.RevivePlayer(1)
	if err != nil {
		t.Fatal(err)
	}
	if err := world.Dispatch(deliveries); err == nil {
		t.Fatal("复活广播失败应返回错误")
	}
	player, ok := world.Entity(1)
	if !ok || player.HP != 100 {
		t.Fatalf("复活广播失败后状态被回滚: %+v ok=%v", player, ok)
	}
}

func TestAttackMonsterObserverErrorDoesNotRollbackHP(t *testing.T) {
	world := newCombatTestWorld(t)
	_, err := world.AddPlayer(1, 1, 1000, 1000, func(Delivery) error { return errors.New("send failed") })
	if err != nil {
		t.Fatal(err)
	}
	if _, err := world.AddMonster(1<<30, 1, 31001, 1000, 1000, 100, 100); err != nil {
		t.Fatal(err)
	}
	deliveries, err := world.AttackMonster(1, 1<<30, 25, 128)
	if err != nil {
		t.Fatal(err)
	}
	if err := world.Dispatch(deliveries); err == nil {
		t.Fatal("观察者发送错误应返回")
	}
	entity, _ := world.Entity(1 << 30)
	if entity.HP != 75 {
		t.Fatalf("广播失败后HP=%d want=75", entity.HP)
	}
}
