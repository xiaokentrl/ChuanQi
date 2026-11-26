package scene

import (
	"errors"
	"fmt"
	"sort"
	"sync"
)

const (
	CellSize         = 64
	DefaultViewRange = 9 * CellSize
)

type SceneConfig struct {
	Width  int
	Height int
	SpawnX int
	SpawnY int
}

type EntityKind uint8

const (
	EntityPlayer EntityKind = iota + 1
	EntityMonster
)

type Entity struct {
	ID       int
	Kind     EntityKind
	SceneID  int
	X        int
	Y        int
	ConfigID int
	HP       int
	MaxHP    int
}

type EventKind uint8

const (
	EventEnter EventKind = iota + 1
	EventLeave
	EventMove
	EventHP
	EventDamage
	EventMoney
	EventExp
)

type Event struct {
	Kind      EventKind
	Entity    Entity
	From      Entity
	Attacker  Entity
	Damage    int
	Level     int
	Exp       int64
	ExpAdded  int64
	Gold      int64
	GoldAdded int64
}

type AttackResult struct {
	Deliveries []Delivery
	Killed     bool
	Attacker   Entity
	Target     Entity
	Damage     int
}

type PlayerAttackResult struct {
	Deliveries []Delivery
	Killed     bool
	Attacker   Entity
	Target     Entity
	Damage     int
}

type Delivery struct {
	ObserverID int
	Event      Event
}

type Observer func(Delivery) error

type World struct {
	mu        sync.Mutex
	scenes    map[int]SceneConfig
	entities  map[int]Entity
	observers map[int]Observer
	viewRange int
}

func NewWorld() *World {
	return &World{
		scenes:    map[int]SceneConfig{},
		entities:  map[int]Entity{},
		observers: map[int]Observer{},
		viewRange: DefaultViewRange,
	}
}

func (w *World) SetScenes(configs map[int]SceneConfig) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	for id, cfg := range configs {
		if id <= 0 || cfg.Width <= 0 || cfg.Height <= 0 || cfg.SpawnX < 0 || cfg.SpawnY < 0 || cfg.SpawnX >= cfg.Width || cfg.SpawnY >= cfg.Height {
			return fmt.Errorf("场景配置非法: id=%d config=%+v", id, cfg)
		}
	}
	w.scenes = make(map[int]SceneConfig, len(configs))
	for id, cfg := range configs {
		w.scenes[id] = cfg
	}
	return nil
}

func (w *World) Spawn(sceneID int) (SceneConfig, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	cfg, ok := w.scenes[sceneID]
	if !ok {
		return SceneConfig{}, fmt.Errorf("场景未配置: %d", sceneID)
	}
	return cfg, nil
}

func (w *World) AddPlayer(id, sceneID, x, y int, observer Observer) ([]Delivery, error) {
	return w.AddPlayerWithHealth(id, sceneID, x, y, 100, 100, observer)
}

func (w *World) AddPlayerWithHealth(id, sceneID, x, y, hp, maxHP int, observer Observer) ([]Delivery, error) {
	if id <= 0 || observer == nil {
		return nil, fmt.Errorf("玩家或观察者非法: id=%d", id)
	}
	if hp <= 0 || maxHP < hp {
		return nil, fmt.Errorf("玩家属性非法: id=%d hp=%d maxHP=%d", id, hp, maxHP)
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.validateEntityLocked(Entity{ID: id, Kind: EntityPlayer, SceneID: sceneID, X: x, Y: y}); err != nil {
		return nil, err
	}
	if old, ok := w.entities[id]; ok && old.Kind != EntityPlayer {
		return nil, fmt.Errorf("实体 ID 已被怪物占用: %d", id)
	}
	deliveries := w.removeLocked(id)
	w.entities[id] = Entity{ID: id, Kind: EntityPlayer, SceneID: sceneID, X: x, Y: y, HP: hp, MaxHP: maxHP}
	w.observers[id] = observer
	entity := w.entities[id]
	for _, visible := range w.visibleLocked(entity, id) {
		deliveries = append(deliveries,
			Delivery{ObserverID: id, Event: Event{Kind: EventEnter, Entity: visible}})
		if _, ok := w.observers[visible.ID]; ok {
			deliveries = append(deliveries, Delivery{ObserverID: visible.ID, Event: Event{Kind: EventEnter, Entity: entity}})
		}
	}
	return deliveries, nil
}

func (w *World) AddMonster(id, sceneID, configID, x, y, hp, maxHP int) ([]Delivery, error) {
	if id <= 0 || configID <= 0 || hp <= 0 || maxHP < hp {
		return nil, fmt.Errorf("怪物属性非法: id=%d config=%d hp=%d maxHP=%d", id, configID, hp, maxHP)
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	entity := Entity{ID: id, Kind: EntityMonster, SceneID: sceneID, X: x, Y: y, ConfigID: configID, HP: hp, MaxHP: maxHP}
	if err := w.validateEntityLocked(entity); err != nil {
		return nil, err
	}
	if old, ok := w.entities[id]; ok && old.Kind != EntityMonster {
		return nil, fmt.Errorf("实体 ID 已被玩家占用: %d", id)
	}
	deliveries := w.removeLocked(id)
	w.entities[id] = entity
	for _, player := range w.visibleLocked(entity, id) {
		if w.observers[player.ID] != nil {
			deliveries = append(deliveries, Delivery{ObserverID: player.ID, Event: Event{Kind: EventEnter, Entity: entity}})
		}
	}
	return deliveries, nil
}

func (w *World) MovePlayer(id, x, y int) ([]Delivery, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	old, ok := w.entities[id]
	if !ok {
		return nil, fmt.Errorf("玩家不在场景中: %d", id)
	}
	if old.HP <= 0 {
		return nil, fmt.Errorf("玩家已死亡，不能移动: %d", id)
	}
	updated := old
	updated.X, updated.Y = x, y
	if err := w.validateEntityLocked(updated); err != nil {
		return nil, err
	}
	oldVisible := w.visibleIDsLocked(old, id)
	newVisible := w.visibleIDsLocked(updated, id)
	w.entities[id] = updated
	deliveries := make([]Delivery, 0)
	for watcherID := range oldVisible {
		watcher := w.entities[watcherID]
		if watcher.Kind == EntityMonster {
			if !newVisible[watcherID] {
				deliveries = append(deliveries, Delivery{ObserverID: id, Event: Event{Kind: EventLeave, Entity: watcher}})
			}
			continue
		}
		if w.observers[watcherID] == nil {
			continue
		}
		if !newVisible[watcherID] {
			deliveries = append(deliveries,
				Delivery{ObserverID: watcherID, Event: Event{Kind: EventLeave, Entity: old}},
				Delivery{ObserverID: id, Event: Event{Kind: EventLeave, Entity: watcher}},
			)
			continue
		}
		deliveries = append(deliveries, Delivery{ObserverID: watcherID, Event: Event{Kind: EventMove, Entity: updated, From: old}})
	}
	for watcherID := range newVisible {
		if oldVisible[watcherID] {
			continue
		}
		watcher := w.entities[watcherID]
		if watcher.Kind == EntityMonster {
			deliveries = append(deliveries, Delivery{ObserverID: id, Event: Event{Kind: EventEnter, Entity: watcher}})
			continue
		}
		if w.observers[watcherID] == nil {
			continue
		}
		deliveries = append(deliveries,
			Delivery{ObserverID: watcherID, Event: Event{Kind: EventEnter, Entity: updated}},
			Delivery{ObserverID: id, Event: Event{Kind: EventEnter, Entity: watcher}},
		)
	}
	return deliveries, nil
}

func (w *World) MoveMonster(id, x, y int) ([]Delivery, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	old, ok := w.entities[id]
	if !ok || old.Kind != EntityMonster || old.HP <= 0 {
		return nil, fmt.Errorf("怪物不在场景中或已死亡: %d", id)
	}
	if old.X == x && old.Y == y {
		return nil, nil
	}
	updated := old
	updated.X, updated.Y = x, y
	if err := w.validateEntityLocked(updated); err != nil {
		return nil, err
	}
	oldVisible := w.visibleIDsLocked(old, id)
	newVisible := w.visibleIDsLocked(updated, id)
	w.entities[id] = updated
	watcherSet := make(map[int]bool, len(oldVisible)+len(newVisible))
	for watcherID := range oldVisible {
		watcherSet[watcherID] = true
	}
	for watcherID := range newVisible {
		watcherSet[watcherID] = true
	}
	watcherIDs := make([]int, 0, len(watcherSet))
	for watcherID := range watcherSet {
		watcher := w.entities[watcherID]
		if watcher.Kind == EntityPlayer && w.observers[watcherID] != nil {
			watcherIDs = append(watcherIDs, watcherID)
		}
	}
	sort.Ints(watcherIDs)
	deliveries := make([]Delivery, 0, len(watcherIDs))
	for _, watcherID := range watcherIDs {
		switch {
		case oldVisible[watcherID] && !newVisible[watcherID]:
			deliveries = append(deliveries, Delivery{ObserverID: watcherID, Event: Event{Kind: EventLeave, Entity: old}})
		case !oldVisible[watcherID] && newVisible[watcherID]:
			deliveries = append(deliveries, Delivery{ObserverID: watcherID, Event: Event{Kind: EventEnter, Entity: updated}})
		case oldVisible[watcherID] && newVisible[watcherID]:
			deliveries = append(deliveries, Delivery{ObserverID: watcherID, Event: Event{Kind: EventMove, Entity: updated, From: old}})
		}
	}
	return deliveries, nil
}

func (w *World) RemovePlayer(id int) []Delivery {
	w.mu.Lock()
	defer w.mu.Unlock()
	entity, ok := w.entities[id]
	if !ok || entity.Kind != EntityPlayer {
		return nil
	}
	return w.removeLocked(id)
}

func (w *World) RevivePlayer(id int) ([]Delivery, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	entity, ok := w.entities[id]
	if !ok || entity.Kind != EntityPlayer {
		return nil, fmt.Errorf("复活玩家不在场景中: %d", id)
	}
	if entity.HP > 0 {
		return nil, fmt.Errorf("玩家尚未死亡: %d", id)
	}
	if entity.MaxHP <= 0 {
		return nil, fmt.Errorf("玩家最大 HP 非法: %d", id)
	}
	entity.HP = entity.MaxHP
	w.entities[id] = entity
	deliveries := make([]Delivery, 0)
	if w.observers[id] != nil {
		deliveries = append(deliveries, Delivery{ObserverID: id, Event: Event{Kind: EventEnter, Entity: entity}})
	}
	watchers := w.visibleIDsLocked(entity, id)
	watcherIDs := make([]int, 0, len(watchers))
	for watcherID := range watchers {
		if w.observers[watcherID] != nil {
			watcherIDs = append(watcherIDs, watcherID)
		}
	}
	sort.Ints(watcherIDs)
	for _, watcherID := range watcherIDs {
		deliveries = append(deliveries, Delivery{ObserverID: watcherID, Event: Event{Kind: EventEnter, Entity: entity}})
	}
	return deliveries, nil
}

func (w *World) RemoveMonster(id int) []Delivery {
	w.mu.Lock()
	defer w.mu.Unlock()
	entity, ok := w.entities[id]
	if !ok || entity.Kind != EntityMonster {
		return nil
	}
	return w.removeLocked(id)
}

func (w *World) Dispatch(deliveries []Delivery) error {
	var errs []error
	for _, delivery := range deliveries {
		w.mu.Lock()
		observer := w.observers[delivery.ObserverID]
		w.mu.Unlock()
		if observer == nil {
			continue
		}
		if err := observer(delivery); err != nil {
			errs = append(errs, fmt.Errorf("observer %d: %w", delivery.ObserverID, err))
		}
	}
	return errors.Join(errs...)
}

func (w *World) Entity(id int) (Entity, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	entity, ok := w.entities[id]
	return entity, ok
}

func (w *World) Entities() []Entity {
	w.mu.Lock()
	defer w.mu.Unlock()
	entities := make([]Entity, 0, len(w.entities))
	for _, entity := range w.entities {
		entities = append(entities, entity)
	}
	sort.Slice(entities, func(i, j int) bool { return entities[i].ID < entities[j].ID })
	return entities
}

func (w *World) AttackMonster(attackerID, targetID, damage, attackRange int) ([]Delivery, error) {
	result, err := w.AttackMonsterResult(attackerID, targetID, damage, attackRange)
	return result.Deliveries, err
}

func (w *World) AttackMonsterResult(attackerID, targetID, damage, attackRange int) (AttackResult, error) {
	if damage <= 0 || attackRange <= 0 {
		return AttackResult{}, fmt.Errorf("伤害或攻击范围非法: damage=%d range=%d", damage, attackRange)
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	attacker, ok := w.entities[attackerID]
	if !ok || attacker.Kind != EntityPlayer || attacker.HP <= 0 {
		return AttackResult{}, fmt.Errorf("攻击者不是存活玩家: %d", attackerID)
	}
	target, ok := w.entities[targetID]
	if !ok || target.Kind != EntityMonster || target.HP <= 0 {
		return AttackResult{}, fmt.Errorf("目标不是存活怪物: %d", targetID)
	}
	if attacker.SceneID != target.SceneID {
		return AttackResult{}, fmt.Errorf("攻击者与怪物不同场景: attackerScene=%d targetScene=%d", attacker.SceneID, target.SceneID)
	}
	if absInt(attacker.X-target.X)+absInt(attacker.Y-target.Y) > attackRange {
		return AttackResult{}, fmt.Errorf("怪物超出攻击范围: attacker=%d target=%d range=%d", attackerID, targetID, attackRange)
	}
	applied := min(damage, target.HP)
	target.HP -= applied
	w.entities[targetID] = target
	deliveries := make([]Delivery, 0)
	watchers := w.visibleIDsLocked(target, targetID)
	watcherIDs := make([]int, 0, len(watchers))
	for watcherID := range watchers {
		if w.observers[watcherID] != nil {
			watcherIDs = append(watcherIDs, watcherID)
		}
	}
	sort.Ints(watcherIDs)
	for _, watcherID := range watcherIDs {
		deliveries = append(deliveries,
			Delivery{ObserverID: watcherID, Event: Event{Kind: EventHP, Entity: target}},
			Delivery{ObserverID: watcherID, Event: Event{Kind: EventDamage, Entity: target, Attacker: attacker, Damage: applied}},
		)
	}
	killed := target.HP == 0
	if killed {
		delete(w.entities, targetID)
		for _, watcherID := range watcherIDs {
			deliveries = append(deliveries, Delivery{ObserverID: watcherID, Event: Event{Kind: EventLeave, Entity: target}})
		}
	}
	return AttackResult{Deliveries: deliveries, Killed: killed, Attacker: attacker, Target: target, Damage: applied}, nil
}

func (w *World) AttackPlayer(attackerID, targetID, damage, attackRange int) ([]Delivery, error) {
	result, err := w.AttackPlayerResult(attackerID, targetID, damage, attackRange)
	return result.Deliveries, err
}

func (w *World) AttackPlayerResult(attackerID, targetID, damage, attackRange int) (PlayerAttackResult, error) {
	if damage <= 0 || attackRange <= 0 {
		return PlayerAttackResult{}, fmt.Errorf("伤害或攻击范围非法: damage=%d range=%d", damage, attackRange)
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	attacker, ok := w.entities[attackerID]
	if !ok || attacker.Kind != EntityMonster || attacker.HP <= 0 {
		return PlayerAttackResult{}, fmt.Errorf("攻击者不是存活怪物: %d", attackerID)
	}
	target, ok := w.entities[targetID]
	if !ok || target.Kind != EntityPlayer || target.HP <= 0 {
		return PlayerAttackResult{}, fmt.Errorf("目标不是存活玩家: %d", targetID)
	}
	if attacker.SceneID != target.SceneID {
		return PlayerAttackResult{}, fmt.Errorf("怪物与玩家不同场景: attackerScene=%d targetScene=%d", attacker.SceneID, target.SceneID)
	}
	if absInt(attacker.X-target.X)+absInt(attacker.Y-target.Y) > attackRange {
		return PlayerAttackResult{}, fmt.Errorf("玩家超出攻击范围: attacker=%d target=%d range=%d", attackerID, targetID, attackRange)
	}
	applied := min(damage, target.HP)
	target.HP -= applied
	w.entities[targetID] = target
	deliveries := make([]Delivery, 0)
	watchers := w.visibleIDsLocked(target, targetID)
	watcherIDs := make([]int, 0, len(watchers)+1)
	if w.observers[targetID] != nil {
		watcherIDs = append(watcherIDs, targetID)
	}
	for watcherID := range watchers {
		if w.observers[watcherID] != nil {
			watcherIDs = append(watcherIDs, watcherID)
		}
	}
	sort.Ints(watcherIDs)
	for _, watcherID := range watcherIDs {
		deliveries = append(deliveries,
			Delivery{ObserverID: watcherID, Event: Event{Kind: EventHP, Entity: target}},
			Delivery{ObserverID: watcherID, Event: Event{Kind: EventDamage, Entity: target, Attacker: attacker, Damage: applied}},
		)
	}
	killed := target.HP == 0
	if killed {
		for _, watcherID := range watcherIDs {
			deliveries = append(deliveries, Delivery{ObserverID: watcherID, Event: Event{Kind: EventLeave, Entity: target}})
		}
	}
	return PlayerAttackResult{Deliveries: deliveries, Killed: killed, Attacker: attacker, Target: target, Damage: applied}, nil
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func (w *World) removeLocked(id int) []Delivery {
	old, ok := w.entities[id]
	delete(w.entities, id)
	delete(w.observers, id)
	if !ok {
		return nil
	}
	deliveries := make([]Delivery, 0)
	if old.Kind == EntityPlayer && old.HP <= 0 {
		return deliveries
	}
	for watcherID := range w.visibleIDsLocked(old, id) {
		if w.observers[watcherID] != nil {
			deliveries = append(deliveries, Delivery{ObserverID: watcherID, Event: Event{Kind: EventLeave, Entity: old}})
		}
	}
	return deliveries
}

func (w *World) validateEntityLocked(entity Entity) error {
	cfg, ok := w.scenes[entity.SceneID]
	if !ok {
		return fmt.Errorf("场景未配置: %d", entity.SceneID)
	}
	if entity.X < 0 || entity.Y < 0 || entity.X >= cfg.Width || entity.Y >= cfg.Height {
		return fmt.Errorf("坐标越界: scene=%d x=%d y=%d", entity.SceneID, entity.X, entity.Y)
	}
	return nil
}

func (w *World) visibleLocked(entity Entity, excludeID int) []Entity {
	visible := make([]Entity, 0)
	for id, candidate := range w.entities {
		if id != excludeID && candidate.SceneID == entity.SceneID && w.inView(entity, candidate) && (candidate.Kind != EntityPlayer || candidate.HP > 0) {
			visible = append(visible, candidate)
		}
	}
	return visible
}

func (w *World) visibleIDsLocked(entity Entity, excludeID int) map[int]bool {
	ids := map[int]bool{}
	for id, candidate := range w.entities {
		if id != excludeID && candidate.SceneID == entity.SceneID && w.inView(entity, candidate) && (candidate.Kind != EntityPlayer || candidate.HP > 0) {
			ids[id] = true
		}
	}
	return ids
}

func (w *World) inView(a, b Entity) bool {
	dx, dy := a.X-b.X, a.Y-b.Y
	if dx < 0 {
		dx = -dx
	}
	if dy < 0 {
		dy = -dy
	}
	return dx <= w.viewRange && dy <= w.viewRange
}
