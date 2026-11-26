package engineapi

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"sync"
)

type ProgressionStore interface {
	SaveProgression(actorID int, level int, exp, gold int64) error
}

type RewardResult struct {
	Level     int
	Exp       int64
	ExpAdded  int64
	Gold      int64
	GoldAdded int64
}

type ProgressionManager struct {
	store      ProgressionStore
	thresholds map[int]int64
	mu         sync.Mutex
	dirty      map[int]struct{}
}

func NewProgressionManager(store ProgressionStore, thresholds map[int]int64) *ProgressionManager {
	return &ProgressionManager{store: store, thresholds: thresholds, dirty: map[int]struct{}{}}
}

func (m *ProgressionManager) GrantKill(actorID int, expAdded, goldAdded int64) (RewardResult, error) {
	if expAdded < 0 || expAdded > math.MaxInt32 || goldAdded < 0 {
		return RewardResult{}, fmt.Errorf("击杀奖励非法: actor=%d exp=%d gold=%d", actorID, expAdded, goldAdded)
	}
	actor := DefaultActorManager.Get(actorID)
	if actor == nil {
		return RewardResult{}, fmt.Errorf("奖励角色不存在: %d", actorID)
	}
	actor.mu.Lock()
	defer actor.mu.Unlock()
	if actor.Level <= 0 || actor.Exp < 0 || expAdded > math.MaxInt64-actor.Exp {
		return RewardResult{}, fmt.Errorf("角色经验非法: actor=%d level=%d exp=%d", actorID, actor.Level, actor.Exp)
	}
	oldGold := int64(actor.Currency[currencyGold])
	if oldGold < 0 || goldAdded > math.MaxInt64-oldGold {
		return RewardResult{}, fmt.Errorf("角色金币非法: actor=%d gold=%d", actorID, oldGold)
	}
	level, exp := m.applyExp(actor.Level, actor.Exp+expAdded)
	exp = min(exp, int64(math.MaxInt32))
	gold := oldGold + goldAdded
	actor.Level, actor.Exp = level, exp
	actor.Currency[currencyGold] = int(gold)
	result := RewardResult{Level: level, Exp: exp, ExpAdded: expAdded, Gold: gold, GoldAdded: goldAdded}
	if m.store == nil || expAdded == 0 && goldAdded == 0 {
		return result, nil
	}
	m.mu.Lock()
	m.dirty[actorID] = struct{}{}
	m.mu.Unlock()
	if err := m.saveActorLocked(actor); err != nil {
		return result, err
	}
	return result, nil
}

func (m *ProgressionManager) applyExp(level int, exp int64) (int, int64) {
	for {
		threshold, ok := m.thresholds[level]
		if !ok || exp < threshold {
			return level, exp
		}
		if _, hasNext := m.thresholds[level+1]; !hasNext {
			return level, threshold
		}
		exp -= threshold
		level++
	}
}

func (m *ProgressionManager) saveActorLocked(actor *Actor) error {
	err := m.store.SaveProgression(actor.ID, actor.Level, actor.Exp, int64(actor.Currency[currencyGold]))
	if err == nil {
		m.mu.Lock()
		delete(m.dirty, actor.ID)
		m.mu.Unlock()
	}
	return err
}

func (m *ProgressionManager) Flush() error {
	m.mu.Lock()
	ids := make([]int, 0, len(m.dirty))
	for id := range m.dirty {
		ids = append(ids, id)
	}
	m.mu.Unlock()
	var errs []error
	for _, id := range ids {
		actor := DefaultActorManager.Get(id)
		if actor == nil {
			continue
		}
		actor.mu.Lock()
		err := m.saveActorLocked(actor)
		actor.mu.Unlock()
		if err != nil {
			errs = append(errs, fmt.Errorf("actor %d: %w", id, err))
		}
	}
	return errors.Join(errs...)
}

func buildGoldChange(moneyType int, count int64) []byte {
	p := []byte{cmdBase, sBaseCmdUpdateMoney}
	p = binary.LittleEndian.AppendUint16(p, uint16(moneyType))
	return binary.LittleEndian.AppendUint64(p, uint64(count))
}

func buildExpChange(level int, exp, added int64) []byte {
	p := []byte{cmdBase, sBaseCmdUpdateExp}
	p = binary.LittleEndian.AppendUint32(p, uint32(level))
	p = binary.LittleEndian.AppendUint32(p, uint32(exp))
	return binary.LittleEndian.AppendUint32(p, uint32(added))
}
