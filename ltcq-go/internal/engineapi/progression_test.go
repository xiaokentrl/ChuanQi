package engineapi

import (
	"bytes"
	"encoding/binary"
	"errors"
	"math"
	"testing"
)

type progressionTestStore struct {
	level, exp, gold int64
	calls            int
	failures         int
}

func (s *progressionTestStore) SaveProgression(_ int, level int, exp, gold int64) error {
	s.calls++
	if s.failures > 0 {
		s.failures--
		return errors.New("temporary storage failure")
	}
	s.level, s.exp, s.gold = int64(level), exp, gold
	return nil
}

func TestRewardPacketLayouts(t *testing.T) {
	gold := buildGoldChange(currencyGold, 5063000)
	if len(gold) != 12 || !bytes.Equal(gold[:2], []byte{cmdBase, sBaseCmdUpdateMoney}) || binary.LittleEndian.Uint16(gold[2:4]) != 1 || binary.LittleEndian.Uint64(gold[4:12]) != 5063000 {
		t.Fatalf("金币奖励包错误: %x", gold)
	}
	exp := buildExpChange(52, 321, 100)
	if len(exp) != 14 || !bytes.Equal(exp[:2], []byte{cmdBase, sBaseCmdUpdateExp}) ||
		binary.LittleEndian.Uint32(exp[2:6]) != 52 || binary.LittleEndian.Uint32(exp[6:10]) != 321 || binary.LittleEndian.Uint32(exp[10:14]) != 100 {
		t.Fatalf("经验奖励包错误: %x", exp)
	}
}

func TestProgressionGrantKillPersistsAbsoluteValues(t *testing.T) {
	DefaultActorManager = NewActorManager()
	actor := DefaultActorManager.GetOrCreate(1)
	actor.Level, actor.Exp = 51, 500
	actor.Currency[currencyGold] = 1000
	store := &progressionTestStore{}
	manager := NewProgressionManager(store, map[int]int64{51: 1000, 52: 1200})

	result, err := manager.GrantKill(1, 100, 250)
	if err != nil {
		t.Fatal(err)
	}
	if result.Level != 51 || result.Exp != 600 || result.Gold != 1250 || actor.Exp != 600 || actor.Currency[currencyGold] != 1250 {
		t.Fatalf("奖励结果错误: result=%+v actor=%+v", result, actor)
	}
	if store.calls != 1 || store.level != 51 || store.exp != 600 || store.gold != 1250 {
		t.Fatalf("存档结果错误: %+v", store)
	}
}

func TestProgressionLevelsUpAndCapsAtMax(t *testing.T) {
	DefaultActorManager = NewActorManager()
	actor := DefaultActorManager.GetOrCreate(1)
	actor.Level, actor.Exp = 1, 90
	actor.Currency[currencyGold] = 0
	manager := NewProgressionManager(nil, map[int]int64{1: 100, 2: 200})

	result, err := manager.GrantKill(1, 20, 0)
	if err != nil || result.Level != 2 || result.Exp != 10 {
		t.Fatalf("升级结果=%+v err=%v", result, err)
	}
	result, err = manager.GrantKill(1, 500, 0)
	if err != nil || result.Level != 2 || result.Exp != 200 {
		t.Fatalf("满级封顶结果=%+v err=%v", result, err)
	}
}

func TestProgressionRetriesLatestAbsoluteSnapshot(t *testing.T) {
	DefaultActorManager = NewActorManager()
	actor := DefaultActorManager.GetOrCreate(1)
	actor.Level, actor.Exp = 1, 0
	actor.Currency[currencyGold] = 0
	store := &progressionTestStore{failures: 2}
	manager := NewProgressionManager(store, nil)

	if _, err := manager.GrantKill(1, 10, 20); err == nil {
		t.Fatal("首次存档失败应返回错误")
	}
	if _, err := manager.GrantKill(1, 10, 20); err == nil {
		t.Fatal("第二次存档失败应返回错误")
	}
	store.failures = 0
	if err := manager.Flush(); err != nil {
		t.Fatal(err)
	}
	if store.exp != 20 || store.gold != 40 || store.calls != 3 {
		t.Fatalf("重试保存了错误快照: %+v", store)
	}
	if err := manager.Flush(); err != nil {
		t.Fatalf("无脏数据时 Flush 应成功: %v", err)
	}
}

func TestProgressionRejectsInvalidRewards(t *testing.T) {
	DefaultActorManager = NewActorManager()
	actor := DefaultActorManager.GetOrCreate(1)
	actor.Level, actor.Exp = 1, math.MaxInt64
	actor.Currency[currencyGold] = 0
	manager := NewProgressionManager(nil, nil)
	if _, err := manager.GrantKill(1, -1, 0); err == nil {
		t.Fatal("负经验应拒绝")
	}
	if _, err := manager.GrantKill(1, math.MaxInt32+1, 0); err == nil {
		t.Fatal("超出协议 int32 的经验应拒绝")
	}
	if _, err := manager.GrantKill(1, 1, 0); err == nil {
		t.Fatal("经验溢出应拒绝")
	}
}
