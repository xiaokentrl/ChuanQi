package scene

import (
	"fmt"
	"testing"
)

func testWorld(t *testing.T) *World {
	t.Helper()
	w := NewWorld()
	if err := w.SetScenes(map[int]SceneConfig{
		1: {Width: 128 * CellSize, Height: 128 * CellSize, SpawnX: 10 * CellSize, SpawnY: 10 * CellSize},
		2: {Width: 64 * CellSize, Height: 64 * CellSize, SpawnX: 5 * CellSize, SpawnY: 5 * CellSize},
	}); err != nil {
		t.Fatal(err)
	}
	return w
}

func addTestPlayer(t *testing.T, w *World, id, sceneID, x, y int) {
	t.Helper()
	if _, err := w.AddPlayer(id, sceneID, x, y, func(Delivery) error { return nil }); err != nil {
		t.Fatal(err)
	}
}

func TestWorldEnterAndLeave(t *testing.T) {
	w := testWorld(t)
	var first, second []Delivery
	if _, err := w.AddPlayer(1, 1, 640, 640, func(d Delivery) error { first = append(first, d); return nil }); err != nil {
		t.Fatal(err)
	}
	deliveries, err := w.AddPlayer(2, 1, 700, 640, func(d Delivery) error { second = append(second, d); return nil })
	if err != nil {
		t.Fatal(err)
	}
	if len(deliveries) != 2 || deliveries[0].ObserverID != 2 || deliveries[0].Event.Entity.ID != 1 || deliveries[1].ObserverID != 1 {
		t.Fatalf("进入视野事件错误: %+v", deliveries)
	}
	if err := w.Dispatch(deliveries); err != nil {
		t.Fatal(err)
	}
	if len(first) != 1 || first[0].ObserverID != 1 || first[0].Event.Kind != EventEnter || first[0].Event.Entity.ID != 2 {
		t.Fatalf("首个观察者未收到进入事件: %+v", first)
	}
	if len(deliveries) != 2 || deliveries[0].ObserverID != 2 || deliveries[0].Event.Entity.ID != 1 || deliveries[1].ObserverID != 1 {
		t.Fatalf("进入视野事件错误: %+v", deliveries)
	}
	leaves := w.RemovePlayer(2)
	if len(leaves) != 1 || leaves[0].ObserverID != 1 || leaves[0].Event.Kind != EventLeave || leaves[0].Event.Entity.ID != 2 {
		t.Fatalf("离场事件错误: %+v", leaves)
	}
	if _, ok := w.Entity(2); ok {
		t.Fatal("离场玩家仍存在")
	}
}

func TestWorldMoveBroadcastAndViewTransitions(t *testing.T) {
	w := testWorld(t)
	var received []Delivery
	addTestPlayer(t, w, 1, 1, 10*CellSize, 10*CellSize)
	if _, err := w.AddPlayer(2, 1, 19*CellSize, 10*CellSize, func(d Delivery) error { received = append(received, d); return nil }); err != nil {
		t.Fatal(err)
	}
	deliveries, err := w.MovePlayer(1, 11*CellSize, 10*CellSize)
	if err != nil {
		t.Fatal(err)
	}
	if len(deliveries) != 1 || deliveries[0].ObserverID != 2 || deliveries[0].Event.Kind != EventMove || deliveries[0].Event.Entity.X != 11*CellSize {
		t.Fatalf("视野内移动广播错误: %+v", deliveries)
	}
	deliveries, err = w.MovePlayer(1, 30*CellSize, 10*CellSize)
	if err != nil {
		t.Fatal(err)
	}
	if len(deliveries) != 2 {
		t.Fatalf("离开视野应通知双方: %+v", deliveries)
	}
	for _, d := range deliveries {
		if d.Event.Kind != EventLeave {
			t.Fatalf("离开视野事件类型错误: %+v", d)
		}
	}
	received = nil
	deliveries, err = w.MovePlayer(1, 20*CellSize, 10*CellSize)
	if err != nil {
		t.Fatal(err)
	}
	if len(deliveries) != 2 {
		t.Fatalf("进入视野应通知双方: %+v", deliveries)
	}
	for _, d := range deliveries {
		if d.Event.Kind != EventEnter {
			t.Fatalf("进入视野事件类型错误: %+v", d)
		}
	}
}

func TestWorldRejectsInvalidState(t *testing.T) {
	w := testWorld(t)
	if _, err := w.AddPlayer(1, 99, 0, 0, func(Delivery) error { return nil }); err == nil {
		t.Fatal("未配置场景应拒绝")
	}
	addTestPlayer(t, w, 1, 1, 0, 0)
	if _, err := w.MovePlayer(1, -1, 0); err == nil {
		t.Fatal("越界移动应拒绝")
	}
	if _, err := w.MovePlayer(2, 0, 0); err == nil {
		t.Fatal("不存在角色应拒绝移动")
	}
	if entity, _ := w.Entity(1); entity.X != 0 || entity.Y != 0 {
		t.Fatalf("非法移动改变了坐标: %+v", entity)
	}
}

func TestWorldSeparatesScenes(t *testing.T) {
	w := testWorld(t)
	addTestPlayer(t, w, 1, 1, 0, 0)
	deliveries, err := w.AddPlayer(2, 2, 0, 0, func(Delivery) error { return nil })
	if err != nil || len(deliveries) != 0 {
		t.Fatalf("不同场景不应互相可见: deliveries=%+v err=%v", deliveries, err)
	}
}

func TestWorldReplacePlayerReturnsOldObserverLeave(t *testing.T) {
	w := testWorld(t)
	addTestPlayer(t, w, 1, 1, 0, 0)
	addTestPlayer(t, w, 2, 1, 64, 0)
	deliveries, err := w.AddPlayer(2, 1, 64, 0, func(Delivery) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if len(deliveries) != 3 {
		t.Fatalf("替换角色应返回旧观察者离场和双向进入事件: %+v", deliveries)
	}
	if deliveries[0].ObserverID != 1 || deliveries[0].Event.Kind != EventLeave || deliveries[0].Event.Entity.ID != 2 {
		t.Fatalf("旧观察者离场事件错误: %+v", deliveries[0])
	}
}

func TestWorldMonsterVisibilityLifecycle(t *testing.T) {
	w := testWorld(t)
	if deliveries, err := w.AddMonster(1001, 1, 10001, 10*CellSize, 10*CellSize, 315, 315); err != nil || len(deliveries) != 0 {
		t.Fatalf("无观察者时刷怪不应广播: deliveries=%+v err=%v", deliveries, err)
	}
	var received []Delivery
	deliveries, err := w.AddPlayer(1, 1, 11*CellSize, 10*CellSize, func(d Delivery) error {
		received = append(received, d)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(deliveries) != 1 || deliveries[0].Event.Kind != EventEnter || deliveries[0].Event.Entity.Kind != EntityMonster {
		t.Fatalf("登录时应收到怪物进入事件: %+v", deliveries)
	}
	if err := w.Dispatch(deliveries); err != nil {
		t.Fatal(err)
	}
	if len(received) != 1 {
		t.Fatalf("怪物进入事件未投递: %+v", received)
	}

	deliveries, err = w.MovePlayer(1, 12*CellSize, 10*CellSize)
	if err != nil || len(deliveries) != 0 {
		t.Fatalf("视野内移动不应重复发送静态怪物: deliveries=%+v err=%v", deliveries, err)
	}
	deliveries, err = w.MovePlayer(1, 30*CellSize, 10*CellSize)
	if err != nil || len(deliveries) != 1 || deliveries[0].ObserverID != 1 || deliveries[0].Event.Kind != EventLeave || deliveries[0].Event.Entity.ID != 1001 {
		t.Fatalf("离开怪物视野事件错误: deliveries=%+v err=%v", deliveries, err)
	}
	deliveries, err = w.MovePlayer(1, 18*CellSize, 10*CellSize)
	if err != nil || len(deliveries) != 1 || deliveries[0].ObserverID != 1 || deliveries[0].Event.Kind != EventEnter || deliveries[0].Event.Entity.ID != 1001 {
		t.Fatalf("重新进入怪物视野事件错误: deliveries=%+v err=%v", deliveries, err)
	}

	leaves := w.RemoveMonster(1001)
	if len(leaves) != 1 || leaves[0].ObserverID != 1 || leaves[0].Event.Kind != EventLeave || leaves[0].Event.Entity.ID != 1001 {
		t.Fatalf("移除怪物离场事件错误: %+v", leaves)
	}
	if _, ok := w.Entity(1001); ok {
		t.Fatal("已移除怪物仍存在")
	}
}

func TestWorldMonsterValidationAndSceneIsolation(t *testing.T) {
	w := testWorld(t)
	addTestPlayer(t, w, 1, 1, 0, 0)
	if deliveries, err := w.AddMonster(1001, 2, 10001, 0, 0, 315, 315); err != nil || len(deliveries) != 0 {
		t.Fatalf("不同场景怪物不应可见: deliveries=%+v err=%v", deliveries, err)
	}
	if _, err := w.AddMonster(1002, 1, 0, 0, 0, 315, 315); err == nil {
		t.Fatal("缺少 configID 的怪物应拒绝")
	}
	if _, err := w.AddMonster(1002, 1, 10001, 0, 0, 0, 315); err == nil {
		t.Fatal("HP 非法的怪物应拒绝")
	}
	if _, err := w.AddMonster(1, 1, 10001, 0, 0, 315, 315); err == nil {
		t.Fatal("怪物 ID 不应覆盖玩家")
	}
	if leaves := w.RemovePlayer(1001); len(leaves) != 0 {
		t.Fatalf("玩家清理不应移除怪物: %+v", leaves)
	}
	if _, ok := w.Entity(1001); !ok {
		t.Fatal("玩家清理误删了怪物")
	}
}

func TestWorldDispatchReportsObserverError(t *testing.T) {
	w := testWorld(t)
	addTestPlayer(t, w, 1, 1, 0, 0)
	boom := fmt.Errorf("send failed")
	deliveries, err := w.AddPlayer(2, 1, 0, 0, func(Delivery) error { return boom })
	if err != nil || len(deliveries) != 2 {
		t.Fatalf("AddPlayer deliveries=%+v err=%v", deliveries, err)
	}
	if err := w.Dispatch(deliveries); err == nil || err.Error() != "observer 2: send failed" {
		t.Fatalf("广播错误=%v", err)
	}
}

func TestWorldMoveMonsterBroadcastsViewTransitions(t *testing.T) {
	w := testWorld(t)
	addTestPlayer(t, w, 1, 1, 10*CellSize, 10*CellSize)
	if _, err := w.AddMonster(1001, 1, 10001, 11*CellSize, 10*CellSize, 315, 315); err != nil {
		t.Fatal(err)
	}
	deliveries, err := w.MoveMonster(1001, 12*CellSize, 10*CellSize)
	if err != nil || len(deliveries) != 1 {
		t.Fatalf("视野内怪物移动=%+v err=%v", deliveries, err)
	}
	move := deliveries[0]
	if move.ObserverID != 1 || move.Event.Kind != EventMove || move.Event.From.X != 11*CellSize || move.Event.Entity.X != 12*CellSize {
		t.Fatalf("怪物移动事件错误: %+v", move)
	}
	deliveries, err = w.MoveMonster(1001, 40*CellSize, 10*CellSize)
	if err != nil || len(deliveries) != 1 || deliveries[0].Event.Kind != EventLeave {
		t.Fatalf("怪物离开视野事件=%+v err=%v", deliveries, err)
	}
	deliveries, err = w.MoveMonster(1001, 20*CellSize, 10*CellSize)
	if err != nil || len(deliveries) != 0 {
		t.Fatalf("怪物仍在视野外不应广播: deliveries=%+v err=%v", deliveries, err)
	}
	deliveries, err = w.MoveMonster(1001, 19*CellSize, 10*CellSize)
	if err != nil || len(deliveries) != 1 || deliveries[0].Event.Kind != EventEnter || deliveries[0].Event.Entity.X != 19*CellSize {
		t.Fatalf("怪物进入视野事件=%+v err=%v", deliveries, err)
	}
}

func TestWorldMoveMonsterRejectsInvalidState(t *testing.T) {
	w := testWorld(t)
	addTestPlayer(t, w, 1, 1, 0, 0)
	if _, err := w.AddMonster(1001, 1, 10001, CellSize, 0, 315, 315); err != nil {
		t.Fatal(err)
	}
	for _, target := range []struct {
		id int
		x  int
		y  int
	}{
		{id: 1, x: 0, y: 0},
		{id: 999, x: 0, y: 0},
		{id: 1001, x: -1, y: 0},
	} {
		if _, err := w.MoveMonster(target.id, target.x, target.y); err == nil {
			t.Fatalf("非法怪物移动应拒绝: %+v", target)
		}
	}
	monster, _ := w.Entity(1001)
	if monster.X != CellSize || monster.Y != 0 {
		t.Fatalf("非法移动改变怪物坐标: %+v", monster)
	}
}
