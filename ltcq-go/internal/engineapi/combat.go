package engineapi

import (
	"fmt"
	"log/slog"
	"sort"
	"sync"
	"time"

	"github.com/xiaokentrl/ChuanQi/ltcq-go/internal/scene"
)

type CombatConfig struct {
	Damage      int
	AttackRange int
	Interval    time.Duration
	ReviveAfter time.Duration
}

// Ticker callbacks may precede the exact cooldown deadline by scheduler jitter.
const combatCooldownSlack = 50 * time.Millisecond

func DefaultCombatConfig() CombatConfig {
	return CombatConfig{Damage: 100, AttackRange: 2 * scene.CellSize, Interval: 500 * time.Millisecond, ReviveAfter: 3 * time.Second}
}

type CombatManager struct {
	world         *scene.World
	config        CombatConfig
	monsters      *MonsterManager
	progression   *ProgressionManager
	stop          chan struct{}
	once          sync.Once
	wg            sync.WaitGroup
	mu            sync.Mutex
	attackAt      map[int]time.Time
	aggro         map[int]int
	playerTargets map[int]int
	stopped       map[int]bool
	returning     map[int]bool
	reviveAt      map[int]time.Time
}

func NewCombatManager(world *scene.World, configs ...CombatConfig) *CombatManager {
	config := DefaultCombatConfig()
	if len(configs) > 0 {
		config = configs[0]
		if config.ReviveAfter <= 0 {
			config.ReviveAfter = DefaultCombatConfig().ReviveAfter
		}
	}
	manager := &CombatManager{world: world, config: config, stop: make(chan struct{}), attackAt: map[int]time.Time{}, aggro: map[int]int{}, playerTargets: map[int]int{}, stopped: map[int]bool{}, returning: map[int]bool{}, reviveAt: map[int]time.Time{}}
	manager.wg.Add(1)
	return manager
}

func (m *CombatManager) SetMonsterManager(monsters *MonsterManager) {
	m.monsters = monsters
}

func (m *CombatManager) SetProgression(progression *ProgressionManager) {
	m.progression = progression
}

func (m *CombatManager) Run() {
	defer m.wg.Done()
	ticker := time.NewTicker(m.config.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			m.Tick()
		case <-m.stop:
			return
		}
	}
}

func (m *CombatManager) Tick() {
	m.TickAt(time.Now())
}

func (m *CombatManager) TickAt(now time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.tickAt(now)
}

func (m *CombatManager) StopPlayerAI(actorID int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	entity, ok := m.world.Entity(actorID)
	if !ok || entity.Kind != scene.EntityPlayer || entity.HP <= 0 {
		return fmt.Errorf("停止 AI 的玩家不在场景中或已死亡: %d", actorID)
	}
	m.stopped[actorID] = true
	return nil
}

func (m *CombatManager) ResumePlayerAutoAttack(actorID int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	entity, ok := m.world.Entity(actorID)
	if !ok || entity.Kind != scene.EntityPlayer || entity.HP <= 0 {
		return fmt.Errorf("恢复 AI 的玩家不在场景中或已死亡: %d", actorID)
	}
	delete(m.stopped, actorID)
	delete(m.playerTargets, actorID)
	return nil
}

func (m *CombatManager) ClearAITarget(entityID int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	entity, ok := m.world.Entity(entityID)
	if !ok || entity.HP <= 0 || (entity.Kind != scene.EntityPlayer && entity.Kind != scene.EntityMonster) {
		return fmt.Errorf("清 AI 目标的实体不在场景中或已死亡: %d", entityID)
	}
	m.clearTargetLocked(entityID)
	return nil
}

func (m *CombatManager) SetAITarget(sourceID, targetID int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	source, ok := m.world.Entity(sourceID)
	if !ok || source.HP <= 0 {
		return fmt.Errorf("AI 来源实体不在场景中或已死亡: %d", sourceID)
	}
	target, ok := m.world.Entity(targetID)
	if !ok || target.HP <= 0 || target.SceneID != source.SceneID {
		return fmt.Errorf("AI 目标实体无效或不在同一场景: %d", targetID)
	}
	if source.Kind == scene.EntityPlayer && target.Kind == scene.EntityMonster {
		m.playerTargets[sourceID] = targetID
		delete(m.stopped, sourceID)
		return nil
	}
	if source.Kind == scene.EntityMonster && target.Kind == scene.EntityPlayer {
		m.aggro[sourceID] = targetID
		delete(m.returning, sourceID)
		return nil
	}
	return fmt.Errorf("当前 AI 仅支持玩家攻击怪物或怪物攻击玩家: %d -> %d", sourceID, targetID)
}

func (m *CombatManager) GetAITarget(sourceID int) (int, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	source, ok := m.world.Entity(sourceID)
	if !ok || source.HP <= 0 {
		m.clearTargetLocked(sourceID)
		return 0, false
	}
	target, ok := m.targetLocked(source)
	if !ok {
		return 0, false
	}
	return target.ID, true
}

func (m *CombatManager) RemovePlayerAI(actorID int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.playerTargets, actorID)
	delete(m.stopped, actorID)
	delete(m.reviveAt, actorID)
	for monsterID, targetID := range m.aggro {
		if targetID == actorID {
			delete(m.aggro, monsterID)
			m.returning[monsterID] = true
		}
	}
}

func (m *CombatManager) targetLocked(source scene.Entity) (scene.Entity, bool) {
	var targetID int
	var ok bool
	switch source.Kind {
	case scene.EntityPlayer:
		targetID, ok = m.playerTargets[source.ID]
	case scene.EntityMonster:
		targetID, ok = m.aggro[source.ID]
	default:
		return scene.Entity{}, false
	}
	if !ok {
		return scene.Entity{}, false
	}
	target, ok := m.world.Entity(targetID)
	if !ok || target.HP <= 0 || target.SceneID != source.SceneID ||
		source.Kind == scene.EntityPlayer && target.Kind != scene.EntityMonster ||
		source.Kind == scene.EntityMonster && target.Kind != scene.EntityPlayer {
		m.clearTargetLocked(source.ID)
		return scene.Entity{}, false
	}
	return target, true
}

func (m *CombatManager) clearTargetLocked(sourceID int) {
	delete(m.playerTargets, sourceID)
	delete(m.aggro, sourceID)
}

func (m *CombatManager) tickAt(now time.Time) {
	if m.progression != nil {
		if err := m.progression.Flush(); err != nil {
			slog.Warn("奖励存档重试失败", "err", err)
		}
	}
	entities := m.world.Entities()
	monsters := make(map[int][]scene.Entity)
	for _, entity := range entities {
		if entity.Kind == scene.EntityMonster && entity.HP > 0 {
			monsters[entity.SceneID] = append(monsters[entity.SceneID], entity)
		}
	}
	for _, attacker := range entities {
		if attacker.Kind != scene.EntityPlayer || attacker.HP <= 0 || m.stopped[attacker.ID] {
			continue
		}
		target, explicit := m.targetLocked(attacker)
		if !explicit {
			nearest, found := 0, false
			for _, monster := range monsters[attacker.SceneID] {
				current, ok := m.world.Entity(monster.ID)
				if !ok || current.Kind != scene.EntityMonster || current.HP <= 0 || current.SceneID != attacker.SceneID {
					continue
				}
				distance := sceneDistance(attacker, current)
				if distance > m.config.AttackRange || found && distance >= nearest {
					continue
				}
				target, nearest, found = current, distance, true
			}
			if !found {
				continue
			}
		}
		if sceneDistance(attacker, target) > m.config.AttackRange {
			continue
		}
		if _, err := m.resolvePlayerAttackLocked(attacker.ID, target.ID, m.config.Damage, m.config.AttackRange, now); err != nil {
			slog.Warn("自动攻击失败", "actor", attacker.ID, "target", target.ID, "err", err)
		}
	}
	m.runMonsterAI(entities, now)
	m.reviveDue(now)
	if m.monsters != nil {
		for _, id := range m.monsters.RespawnDue(now) {
			if err := m.monsters.Respawn(id); err != nil {
				slog.Warn("怪物重生失败", "monster", id, "err", err)
			}
		}
	}
}

func (m *CombatManager) ResolvePlayerAttack(attackerID, targetID, damage, attackRange int, now time.Time) (scene.AttackResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.resolvePlayerAttackLocked(attackerID, targetID, damage, attackRange, now)
}

func (m *CombatManager) resolvePlayerAttackLocked(attackerID, targetID, damage, attackRange int, now time.Time) (scene.AttackResult, error) {
	result, err := m.world.AttackMonsterResult(attackerID, targetID, damage, attackRange)
	if err != nil {
		return scene.AttackResult{}, err
	}
	if err := m.world.Dispatch(result.Deliveries); err != nil {
		slog.Warn("战斗广播失败", "actor", attackerID, "target", targetID, "err", err)
	}
	m.aggro[targetID] = attackerID
	if result.Killed {
		delete(m.attackAt, targetID)
		delete(m.aggro, targetID)
		delete(m.returning, targetID)
		delete(m.playerTargets, attackerID)
		m.handleKill(result, now)
	}
	return result, nil
}

func (m *CombatManager) runMonsterAI(entities []scene.Entity, now time.Time) {
	if m.monsters == nil {
		return
	}
	for _, monster := range entities {
		if monster.Kind != scene.EntityMonster || monster.HP <= 0 {
			continue
		}
		current, ok := m.world.Entity(monster.ID)
		if !ok || current.HP <= 0 {
			delete(m.aggro, monster.ID)
			continue
		}
		rule, ok := m.monsters.Rule(monster.ID)
		if !ok || rule.AttackDamage <= 0 {
			delete(m.aggro, monster.ID)
			continue
		}
		home := scene.Entity{SceneID: current.SceneID, X: rule.X * scene.CellSize, Y: rule.Y * scene.CellSize}
		if m.returning[current.ID] {
			if current.X == home.X && current.Y == home.Y {
				delete(m.returning, current.ID)
			} else {
				m.moveMonster(current, home, rule.ChaseStep)
				continue
			}
		}
		target, engaged := m.monsterTarget(current)
		if engaged && rule.LeashRange > 0 && sceneDistance(current, home) > rule.LeashRange {
			delete(m.aggro, current.ID)
			m.returning[current.ID] = true
			m.moveMonster(current, home, rule.ChaseStep)
			continue
		}
		if !engaged && rule.AggroRange > 0 {
			if nearest, found := m.nearestPlayer(entities, current, rule.AggroRange); found {
				m.aggro[current.ID] = nearest.ID
				target, engaged = nearest, true
			}
		}
		if !engaged {
			if rule.ChaseStep > 0 {
				if current.X != home.X || current.Y != home.Y {
					m.returning[current.ID] = true
				}
				m.moveMonster(current, home, rule.ChaseStep)
			}
			continue
		}
		if sceneDistance(current, target) > rule.AttackRange {
			if rule.ChaseStep > 0 {
				m.moveMonster(current, target, rule.ChaseStep)
			} else {
				delete(m.aggro, current.ID)
			}
			continue
		}
		if next := m.attackAt[current.ID]; !next.IsZero() && now.Add(combatCooldownSlack).Before(next) {
			continue
		}
		result, err := m.world.AttackPlayerResult(current.ID, target.ID, rule.AttackDamage, rule.AttackRange)
		if err != nil {
			slog.Warn("怪物攻击失败", "monster", current.ID, "actor", target.ID, "err", err)
			continue
		}
		m.attackAt[current.ID] = now.Add(time.Duration(rule.AttackIntervalSeconds) * time.Second)
		if err := m.world.Dispatch(result.Deliveries); err != nil {
			slog.Warn("怪物攻击广播失败", "monster", current.ID, "actor", target.ID, "err", err)
		}
		if actor := DefaultActorManager.Get(target.ID); actor != nil {
			actor.mu.Lock()
			actor.HP, actor.MaxHP = result.Target.HP, result.Target.MaxHP
			actor.mu.Unlock()
		}
		if result.Killed {
			m.handlePlayerDeath(result, now)
		}
	}
}

func (m *CombatManager) monsterTarget(monster scene.Entity) (scene.Entity, bool) {
	return m.targetLocked(monster)
}

func (m *CombatManager) nearestPlayer(entities []scene.Entity, monster scene.Entity, maxDistance int) (scene.Entity, bool) {
	var nearest scene.Entity
	distance, found := 0, false
	for _, player := range entities {
		if player.Kind != scene.EntityPlayer || player.SceneID != monster.SceneID {
			continue
		}
		current, ok := m.world.Entity(player.ID)
		if !ok || current.Kind != scene.EntityPlayer || current.HP <= 0 || current.SceneID != monster.SceneID {
			continue
		}
		currentDistance := sceneDistance(monster, current)
		if currentDistance > maxDistance || found && currentDistance >= distance {
			continue
		}
		nearest, distance, found = current, currentDistance, true
	}
	return nearest, found
}

func (m *CombatManager) moveMonster(monster, target scene.Entity, step int) {
	x, y := monster.X, monster.Y
	if dx, dy := absInt(target.X-monster.X), absInt(target.Y-monster.Y); dx >= dy {
		x = moveToward(monster.X, target.X, step)
	} else {
		y = moveToward(monster.Y, target.Y, step)
	}
	deliveries, err := m.world.MoveMonster(monster.ID, x, y)
	if err != nil {
		slog.Warn("怪物移动失败", "monster", monster.ID, "err", err)
		return
	}
	if err := m.world.Dispatch(deliveries); err != nil {
		slog.Warn("怪物移动广播失败", "monster", monster.ID, "err", err)
	}
}

func moveToward(current, target, step int) int {
	if current < target {
		return min(current+step, target)
	}
	return max(current-step, target)
}

func (m *CombatManager) handlePlayerDeath(result scene.PlayerAttackResult, now time.Time) {
	m.reviveAt[result.Target.ID] = now.Add(m.config.ReviveAfter)
	for monsterID, targetID := range m.aggro {
		if targetID == result.Target.ID {
			delete(m.aggro, monsterID)
			m.returning[monsterID] = true
		}
	}
}

func (m *CombatManager) reviveDue(now time.Time) {
	ids := make([]int, 0, len(m.reviveAt))
	for id := range m.reviveAt {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	for _, id := range ids {
		deadline := m.reviveAt[id]
		entity, ok := m.world.Entity(id)
		if !ok || entity.Kind != scene.EntityPlayer || entity.HP > 0 {
			delete(m.reviveAt, id)
			continue
		}
		if now.Before(deadline) {
			continue
		}
		deliveries, err := m.world.RevivePlayer(id)
		delete(m.reviveAt, id)
		if err != nil {
			slog.Warn("玩家复活失败", "actor", id, "err", err)
			continue
		}
		if actor := DefaultActorManager.Get(id); actor != nil {
			actor.mu.Lock()
			actor.HP, actor.MaxHP = entity.MaxHP, entity.MaxHP
			actor.mu.Unlock()
		}
		if err := m.world.Dispatch(deliveries); err != nil {
			slog.Warn("玩家复活广播失败", "actor", id, "err", err)
		}
	}
}

func (m *CombatManager) handleKill(result scene.AttackResult, now time.Time) {
	var rule MonsterSpawn
	var hasRule bool
	if m.monsters != nil {
		rule, hasRule = m.monsters.Rule(result.Target.ID)
		m.monsters.MarkDead(result.Target.ID, now)
		delete(m.attackAt, result.Target.ID)
		delete(m.aggro, result.Target.ID)
		delete(m.returning, result.Target.ID)
	}
	if hasRule && rule.ItemID > 0 {
		actor := DefaultActorManager.Get(result.Attacker.ID)
		if actor == nil {
			slog.Warn("击杀物品奖励角色不存在", "actor", result.Attacker.ID, "monster", result.Target.ID)
		} else if ok, err := actor.AddItem(rule.ItemID, rule.ItemCount); err != nil {
			slog.Warn("击杀物品奖励结算失败", "actor", result.Attacker.ID, "monster", result.Target.ID, "err", err)
		} else if !ok {
			slog.Warn("击杀物品奖励参数非法", "actor", result.Attacker.ID, "monster", result.Target.ID, "item", rule.ItemID, "count", rule.ItemCount)
		}
	}
	if !hasRule || m.progression == nil {
		return
	}
	reward, err := m.progression.GrantKill(result.Attacker.ID, rule.Exp, rule.Gold)
	if err != nil {
		slog.Warn("击杀奖励结算失败", "actor", result.Attacker.ID, "monster", result.Target.ID, "err", err)
	}
	if reward.ExpAdded != rule.Exp || reward.GoldAdded != rule.Gold {
		return
	}
	deliveries := make([]scene.Delivery, 0, 2)
	if rule.Gold > 0 {
		deliveries = append(deliveries, scene.Delivery{ObserverID: result.Attacker.ID, Event: scene.Event{
			Kind: scene.EventMoney, Entity: result.Attacker, Gold: reward.Gold, GoldAdded: reward.GoldAdded,
		}})
	}
	if rule.Exp > 0 {
		deliveries = append(deliveries, scene.Delivery{ObserverID: result.Attacker.ID, Event: scene.Event{
			Kind: scene.EventExp, Entity: result.Attacker, Level: reward.Level, Exp: reward.Exp, ExpAdded: reward.ExpAdded,
		}})
	}
	if err := m.world.Dispatch(deliveries); err != nil {
		slog.Warn("奖励广播失败", "actor", result.Attacker.ID, "monster", result.Target.ID, "err", err)
	}
}

func (m *CombatManager) Close() {
	m.once.Do(func() {
		close(m.stop)
		m.wg.Wait()
		if m.progression != nil {
			if err := m.progression.Flush(); err != nil {
				slog.Warn("停机前奖励存档失败", "err", err)
			}
		}
	})
}

func sceneDistance(a, b scene.Entity) int {
	dx, dy := a.X-b.X, a.Y-b.Y
	if dx < 0 {
		dx = -dx
	}
	if dy < 0 {
		dy = -dy
	}
	return dx + dy
}
