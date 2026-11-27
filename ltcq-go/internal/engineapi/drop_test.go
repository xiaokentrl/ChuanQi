package engineapi

import (
	"encoding/binary"
	"testing"
	"time"
)

// A2 掉落拾取单元测试：半径判定、Take 幂等、过期清扫（combat_test.go 覆盖击杀→拾取全链路）。
func TestDropNearDropsRadiusAndTake(t *testing.T) {
	m := NewDropManager()
	_, frame := m.CreateDrop(1, 200013, 1, 384, 384, 1)
	if len(frame) == 0 || frame[0] != cmdBase || frame[1] != sBaseCmdOtherEntity || binary.LittleEndian.Uint32(frame[4:]) != 6 {
		t.Fatalf("CreateDrop 应返回 type=6 的 OtherEntity 帧: %x", frame)
	}
	// 半径外不返回；半径内按近先序返回
	if got := m.NearDrops(1, 384+dropPickupRadius+1, 384, dropPickupRadius); len(got) != 0 {
		t.Fatalf("拾取半径外不应返回掉落: %v", got)
	}
	got := m.NearDrops(1, 384, 384, dropPickupRadius)
	if len(got) != 1 || got[0].ItemID != 200013 || got[0].Count != 1 {
		t.Fatalf("拾取半径内应返回掉落(200013×1): %v", got)
	}
	// 跨场景不返回
	if got := m.NearDrops(2, 384, 384, dropPickupRadius); len(got) != 0 {
		t.Fatalf("跨场景不应返回掉落: %v", got)
	}
	// Take 幂等：首取成功，重取失败
	d := got[0]
	taken, ok := m.Take(d.Handle)
	if !ok || taken.ItemID != 200013 {
		t.Fatalf("Take 失败: %v ok=%v", taken, ok)
	}
	if _, ok := m.Take(d.Handle); ok {
		t.Fatal("重复 Take 应失败")
	}
	if got := m.NearDrops(1, 384, 384, dropPickupRadius); len(got) != 0 {
		t.Fatalf("Take 后不应再返回掉落: %v", got)
	}
}

func TestDropSweepExpired(t *testing.T) {
	m := NewDropManager()
	m.CreateDrop(1, 200013, 1, 0, 0, 0)
	if got := m.SweepExpired(time.Now()); len(got) != 0 {
		t.Fatalf("未过期掉落不应被清扫: %v", got)
	}
	expired := m.SweepExpired(time.Now().Add(2 * time.Minute))
	if len(expired) != 1 || expired[0].ItemID != 200013 {
		t.Fatalf("应清出 1 份过期掉落: %v", expired)
	}
	if got := m.SweepExpired(time.Now().Add(2 * time.Minute)); len(got) != 0 {
		t.Fatal("重复清扫应为空")
	}
}
