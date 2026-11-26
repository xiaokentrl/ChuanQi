package engineapi

import (
	"encoding/binary"
	"fmt"
	"math"
	"path/filepath"
	"sync"
	"time"

	"github.com/xiaokentrl/ChuanQi/ltcq-go/internal/luahost"
	"github.com/xiaokentrl/ChuanQi/ltcq-go/internal/scene"
	lua "github.com/yuin/gopher-lua"
)

const (
	cmdSkill              = 2
	cSkillCmdCastSkill    = 1
	cSkillCmdUpgradeSkill = 4
	cSkillCmdUpgradeAll   = 5
	sSkillCmdCastSkill    = 1
	sSkillCmdUpdateSkill  = 4
	sSkillCmdUpdateAll    = 5
)

type skillConfig struct {
	ID          int
	Description int
	DamageRatio float64
	FlatDamage  int
	Cooldown    time.Duration
	CastRange   float64
	TargetType  int
	CalcType    int
	CastType    int
	AffectRange int
	AffectCount int
	Teleport    int
}

type skillCatalog struct {
	skills      map[int]skillConfig
	openLevels  [5]int
	upgradeCost map[int]int
}

func LoadSkillCatalog(dataRoot, ostype string) (*skillCatalog, error) {
	g := luahost.NewGopherLua(luahost.Config{DataRoot: dataRoot, OSType: ostype})
	defer g.Close()
	catalog := &skillCatalog{skills: map[int]skillConfig{}, upgradeCost: map[int]int{}}
	modules := []struct {
		path   string
		global string
	}{
		{"config/skill/skills.config", "SkillsConfig"},
		{"config/skill/skillsdesc.config", "SkillsDescConfig"},
		{"config/skill/skillopen.config", "SkillsOpenConfig"},
		{"config/skill/skillsupgrade.config", "SkillsUpgradeConfig"},
	}
	for _, module := range modules {
		if err := g.LoadFile(filepath.Join(dataRoot, filepath.FromSlash(module.path))); err != nil {
			return nil, fmt.Errorf("加载技能配置 %s 失败: %w", module.path, err)
		}
	}
	descriptions, ok := g.L.GetGlobal("SkillsDescConfig").(*lua.LTable)
	if !ok {
		return nil, fmt.Errorf("技能配置缺少 SkillsDescConfig")
	}
	skills, ok := g.L.GetGlobal("SkillsConfig").(*lua.LTable)
	if !ok {
		return nil, fmt.Errorf("技能配置缺少 SkillsConfig")
	}
	var parseErr error
	skills.ForEach(func(key, value lua.LValue) {
		if parseErr != nil {
			return
		}
		entry, ok := value.(*lua.LTable)
		if !ok {
			parseErr = fmt.Errorf("技能配置含非法条目")
			return
		}
		cfg := skillConfig{
			ID:          int(lua.LVAsNumber(key)),
			Description: int(lua.LVAsNumber(entry.RawGetString("desc"))),
			TargetType:  int(lua.LVAsNumber(entry.RawGetString("targetType"))),
			CalcType:    int(lua.LVAsNumber(entry.RawGetString("calcType"))),
			CastType:    int(lua.LVAsNumber(entry.RawGetString("castType"))),
			AffectRange: int(lua.LVAsNumber(entry.RawGetString("affectRange"))),
			AffectCount: int(lua.LVAsNumber(entry.RawGetString("affectCount"))),
			Teleport:    int(lua.LVAsNumber(entry.RawGetString("teleport"))),
		}
		if cfg.TargetType == 0 {
			cfg.TargetType = 2
		}
		if cfg.CalcType == 0 {
			cfg.CalcType = 1
		}
		if args, ok := entry.RawGetString("args").(*lua.LTable); ok {
			cfg.DamageRatio = float64(lua.LVAsNumber(args.RawGetString("a")))
			cfg.FlatDamage = int(lua.LVAsNumber(args.RawGetString("b")))
		}
		if desc, ok := descriptions.RawGetInt(cfg.Description).(*lua.LTable); ok {
			cfg.Cooldown = time.Duration(lua.LVAsNumber(desc.RawGetString("cd"))) * time.Millisecond
			cfg.CastRange = float64(lua.LVAsNumber(desc.RawGetString("castRange")))
		}
		if cfg.ID <= 0 {
			parseErr = fmt.Errorf("技能配置条目非法: id=%d desc=%d", cfg.ID, cfg.Description)
			return
		}
		catalog.skills[cfg.ID] = cfg
	})
	if parseErr != nil {
		return nil, parseErr
	}
	open, ok := g.L.GetGlobal("SkillsOpenConfig").(*lua.LTable)
	if !ok {
		return nil, fmt.Errorf("技能配置缺少 SkillsOpenConfig")
	}
	open.ForEach(func(key, value lua.LValue) {
		index := int(lua.LVAsNumber(key)) - 1
		entry, ok := value.(*lua.LTable)
		if ok && index >= 0 && index < len(catalog.openLevels) {
			catalog.openLevels[index] = int(lua.LVAsNumber(entry.RawGetString("level")))
		}
	})
	upgrades, ok := g.L.GetGlobal("SkillsUpgradeConfig").(*lua.LTable)
	if !ok {
		return nil, fmt.Errorf("技能配置缺少 SkillsUpgradeConfig")
	}
	upgrades.ForEach(func(key, value lua.LValue) {
		level := int(lua.LVAsNumber(key))
		if entry, ok := value.(*lua.LTable); ok {
			catalog.upgradeCost[level] = int(lua.LVAsNumber(entry.RawGetString("cost")))
		}
	})
	for job := 1; job <= 3; job++ {
		for slot := 1; slot <= 5; slot++ {
			for level := 1; level <= 260; level++ {
				cfg, ok := catalog.skills[skillConfigID(job, slot, level)]
				if !ok || cfg.Cooldown <= 0 {
					return nil, fmt.Errorf("角色技能配置缺失或冷却非法: job=%d slot=%d level=%d", job, slot, level)
				}
			}
		}
	}
	for slot, level := range catalog.openLevels {
		if level <= 0 {
			return nil, fmt.Errorf("技能槽 %d 开启等级非法: %d", slot, level)
		}
	}
	return catalog, nil
}

func skillBaseID(job, slot int) int {
	return job*10000 + slot*1000 + 1
}

func skillConfigID(job, slot, level int) int {
	return skillBaseID(job, slot) + level - 1
}

func realGetRoleSkillLevel(a ...luahost.Value) (luahost.Value, error) {
	if len(a) < 3 {
		return 0, nil
	}
	actorID, ok := actorHandleArgument(a[0])
	if !ok {
		return 0, nil
	}
	roleID, err := integerArgument(a[1])
	if err != nil || roleID != 0 {
		return 0, nil
	}
	index, err := integerArgument(a[2])
	if err != nil || index < 0 || index > 4 {
		return 0, nil
	}
	actor := DefaultActorManager.Get(actorID)
	if actor == nil {
		return 0, nil
	}
	actor.mu.RLock()
	defer actor.mu.RUnlock()
	if actor.Job < 1 || actor.Job > 3 {
		return 0, nil
	}
	return actor.Skills[skillBaseID(actor.Job, index+1)], nil
}

type SkillManager struct {
	catalog *skillCatalog
	combat  *CombatManager
	mu      sync.Mutex
	readyAt map[[2]int]time.Time
	now     func() time.Time
}

func NewSkillManager(catalog *skillCatalog, combat *CombatManager) *SkillManager {
	return &SkillManager{
		catalog: catalog,
		combat:  combat,
		readyAt: map[[2]int]time.Time{},
		now:     time.Now,
	}
}

func (m *SkillManager) EnsureInitialized(actor *Actor, store AccountStore, codec luahost.TableCodec) error {
	if m == nil || actor == nil {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	actor.mu.Lock()
	job, level := actor.Job, actor.Level
	if actor.Skills == nil {
		actor.Skills = map[int]int{}
	}
	changed := false
	if job >= 1 && job <= 3 {
		for slot := 1; slot <= 5; slot++ {
			if level < m.catalog.openLevels[slot-1] {
				continue
			}
			baseID := skillBaseID(job, slot)
			if actor.Skills[baseID] == 0 {
				actor.Skills[baseID] = 1
				changed = true
			}
		}
	}
	if changed {
		actor.SkillsDirty = true
		actor.Dirty = true
	}
	actor.mu.Unlock()
	if changed {
		return flushDirty(actor, store, codec)
	}
	return nil
}

func (m *SkillManager) Handle(sid uint16, actorID int, cmdid uint16, body []byte, router *NetRouter, store AccountStore, codec luahost.TableCodec) error {
	sess := router.AuthenticatedSession(sid)
	if sess == nil || sess.ActorID != actorID || actorID == 0 {
		return fmt.Errorf("技能会话未绑定: sid=%d actor=%d", sid, actorID)
	}
	switch cmdid {
	case cSkillCmdCastSkill:
		return m.cast(sid, actorID, body, router)
	case cSkillCmdUpgradeSkill:
		return m.upgradeOne(sid, actorID, body, router, store, codec)
	case cSkillCmdUpgradeAll:
		return m.upgradeAll(sid, actorID, body, router, store, codec)
	default:
		return fmt.Errorf("未支持的技能命令: %d", cmdid)
	}
}

func (m *SkillManager) upgradeOne(sid uint16, actorID int, body []byte, router *NetRouter, store AccountStore, codec luahost.TableCodec) error {
	if len(body) != 4 {
		return fmt.Errorf("技能升级包长度非法: %d", len(body))
	}
	roleID := int(int16(binary.LittleEndian.Uint16(body[:2])))
	slot := int(int16(binary.LittleEndian.Uint16(body[2:4]))) + 1
	if roleID != 0 || slot < 1 || slot > 5 {
		return fmt.Errorf("技能升级目标非法: role=%d slot=%d", roleID, slot)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	actor := DefaultActorManager.Get(actorID)
	if actor == nil {
		return fmt.Errorf("角色不存在: %d", actorID)
	}
	actor.mu.Lock()
	job, level, maxLevel := actor.Job, actor.Level, skillMaxLevel(actor)
	current := actor.Skills[skillBaseID(job, slot)]
	activating := current == 0 && level >= m.catalog.openLevels[slot-1]
	newLevel := current + 1
	if activating {
		newLevel = 1
	}
	if current <= 0 && !activating || current >= maxLevel {
		actor.mu.Unlock()
		return fmt.Errorf("技能不可升级: job=%d slot=%d level=%d max=%d", job, slot, current, maxLevel)
	}
	cost := 0
	if !activating {
		cost = m.catalog.upgradeCost[current]
		if cost <= 0 {
			actor.mu.Unlock()
			return fmt.Errorf("技能升级费用缺失: level=%d", current)
		}
	}
	gold := actor.Currency[currencyGold]
	if gold < cost {
		actor.mu.Unlock()
		return fmt.Errorf("技能升级金币不足: gold=%d cost=%d", gold, cost)
	}
	actor.mu.Unlock()
	if err := m.persistUpgrade(actor, map[int]int{skillBaseID(job, slot): newLevel}, gold-cost, gold, store, codec); err != nil {
		return err
	}
	if err := router.SendToSession(sid, buildSkillUpdate(0, slot-1, newLevel)); err != nil {
		return err
	}
	if cost > 0 {
		return router.SendToSession(sid, buildGoldChange(currencyGold, int64(gold-cost)))
	}
	return nil
}

func (m *SkillManager) upgradeAll(sid uint16, actorID int, body []byte, router *NetRouter, store AccountStore, codec luahost.TableCodec) error {
	if len(body) != 6 {
		return fmt.Errorf("一键技能升级包长度非法: %d", len(body))
	}
	mode := int(binary.LittleEndian.Uint16(body[:2]))
	roleID := int(int16(binary.LittleEndian.Uint16(body[2:4])))
	selected := int(int16(binary.LittleEndian.Uint16(body[4:6])))
	if mode < 0 || mode > 1 || roleID != 0 || selected < 0 {
		return fmt.Errorf("一键技能升级参数非法: mode=%d role=%d index=%d", mode, roleID, selected)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	actor := DefaultActorManager.Get(actorID)
	if actor == nil {
		return fmt.Errorf("角色不存在: %d", actorID)
	}
	actor.mu.Lock()
	job, maxLevel := actor.Job, skillMaxLevel(actor)
	levels := [5]int{}
	beforeLevels := [5]int{}
	bases := [5]int{}
	activeSlots := make([]int, 0, 5)
	for slot := 1; slot <= 5; slot++ {
		bases[slot-1] = skillBaseID(job, slot)
		levels[slot-1] = actor.Skills[bases[slot-1]]
		beforeLevels[slot-1] = levels[slot-1]
		if levels[slot-1] > 0 {
			activeSlots = append(activeSlots, slot)
		}
	}
	gold := actor.Currency[currencyGold]
	oldGold := gold
	actor.mu.Unlock()
	if selected >= len(activeSlots) {
		return fmt.Errorf("一键技能升级索引越界: index=%d active=%d", selected, len(activeSlots))
	}
	if mode == 0 {
		m.upgradeSingleTrack(levels[:], activeSlots[selected], maxLevel, &gold)
	} else {
		m.upgradeWholeTrack(levels[:], maxLevel, &gold)
	}
	changed := map[int]int{}
	for i := range levels {
		if levels[i] > 0 && levels[i] != beforeLevels[i] {
			changed[bases[i]] = levels[i]
		}
	}
	if len(changed) == 0 {
		return fmt.Errorf("没有可升级技能")
	}
	if err := m.persistUpgrade(actor, changed, gold, oldGold, store, codec); err != nil {
		return err
	}
	packet := []byte{cmdSkill, sSkillCmdUpdateAll}
	packet = binary.LittleEndian.AppendUint16(packet, 0)
	for _, level := range levels {
		packet = binary.LittleEndian.AppendUint16(packet, uint16(level))
	}
	if err := router.SendToSession(sid, packet); err != nil {
		return err
	}
	if oldGold != gold {
		return router.SendToSession(sid, buildGoldChange(currencyGold, int64(gold)))
	}
	return nil
}

func (m *SkillManager) upgradeSingleTrack(levels []int, selected, maxLevel int, gold *int) {
	if selected < 1 || selected > len(levels) || levels[selected-1] == 0 {
		return
	}
	for level := levels[selected-1]; level < maxLevel; level++ {
		cost := m.catalog.upgradeCost[level]
		if cost <= 0 || *gold < cost {
			return
		}
		*gold -= cost
		levels[selected-1] = level + 1
	}
}

func (m *SkillManager) upgradeWholeTrack(levels []int, maxLevel int, gold *int) {
	for level := 1; level < maxLevel; level++ {
		cost := m.catalog.upgradeCost[level]
		if cost <= 0 {
			return
		}
		for i := range levels {
			if levels[i] != level {
				continue
			}
			if *gold < cost {
				return
			}
			*gold -= cost
			levels[i] = level + 1
		}
	}
}

func (m *SkillManager) persistUpgrade(actor *Actor, changed map[int]int, gold, expectedGold int, store AccountStore, codec luahost.TableCodec) error {
	actor.mu.Lock()
	defer actor.mu.Unlock()
	if actor.Currency[currencyGold] != expectedGold {
		return fmt.Errorf("角色金币在技能升级期间发生变化: actor=%d", actor.ID)
	}
	levels := make(map[int]int, len(actor.Skills))
	for id, level := range actor.Skills {
		levels[id] = level
	}
	for id, level := range changed {
		levels[id] = level
	}
	data := actorPersistData{Version: actorPersistVersion, ClientConfig: map[int]int{}, Skills: levels}
	for key, val := range actor.ClientConfig {
		data.ClientConfig[key] = val
	}
	if actor.Static != nil {
		if codec == nil {
			return fmt.Errorf("角色 %d Static 未落库：缺少 Lua 表编码器", actor.ID)
		}
		node, err := codec.EncodeValue(actor.Static)
		if err != nil {
			return fmt.Errorf("角色 %d Static 编码失败: %w", actor.ID, err)
		}
		data.Static = &node
	}
	atomicStore, ok := store.(interface {
		SaveSkillUpgrade(actorID int, gold int64, data *actorPersistData) error
	})
	if !ok {
		return fmt.Errorf("技能升级需要原子存档支持")
	}
	if err := atomicStore.SaveSkillUpgrade(actor.ID, int64(gold), &data); err != nil {
		return fmt.Errorf("技能升级存档失败: actor=%d %w", actor.ID, err)
	}
	for id, level := range changed {
		actor.Skills[id] = level
	}
	actor.Currency[currencyGold] = gold
	actor.SkillsDirty = false
	actor.ClientConfigDirty = false
	actor.StaticDirty = false
	actor.Dirty = false
	return nil
}

func (m *SkillManager) cast(sid uint16, actorID int, body []byte, router *NetRouter) error {
	if len(body) != 20 {
		return fmt.Errorf("技能释放包长度非法: %d", len(body))
	}
	attacker := math.Float64frombits(binary.LittleEndian.Uint64(body[:8]))
	target := math.Float64frombits(binary.LittleEndian.Uint64(body[8:16]))
	configID := int(int32(binary.LittleEndian.Uint32(body[16:20])))
	if attacker != float64(actorID) || target != float64(int64(target)) || target <= 0 {
		return fmt.Errorf("技能释放句柄非法: attacker=%g target=%g", attacker, target)
	}
	targetID := int(target)
	m.mu.Lock()
	defer m.mu.Unlock()
	actor := DefaultActorManager.Get(actorID)
	if actor == nil {
		return fmt.Errorf("角色不存在: %d", actorID)
	}
	actor.mu.RLock()
	job, hp := actor.Job, actor.HP
	actor.mu.RUnlock()
	if hp <= 0 {
		return fmt.Errorf("玩家已死亡，不能释放技能: %d", actorID)
	}
	slot := (configID/1000)%10 - 1
	if job < 1 || job > 3 || slot < 0 || slot > 4 || configID != skillConfigID(job, slot+1, configID%1000) {
		return fmt.Errorf("技能 ID 非法: id=%d job=%d", configID, job)
	}
	baseID := skillBaseID(job, slot+1)
	actor.mu.RLock()
	level := actor.Skills[baseID]
	actor.mu.RUnlock()
	if level <= 0 || configID != skillConfigID(job, slot+1, level) {
		return fmt.Errorf("技能未学习或等级不匹配: id=%d level=%d", configID, level)
	}
	cfg, ok := m.catalog.skills[configID]
	if !ok || cfg.DamageRatio <= 0 || cfg.FlatDamage <= 0 || cfg.TargetType != 2 || cfg.CalcType != 1 || cfg.CastType != 0 || cfg.AffectCount > 1 || cfg.Teleport != 0 {
		return fmt.Errorf("技能暂不支持单目标伤害释放: id=%d", configID)
	}
	cooldownKey := [2]int{actorID, baseID}
	now := m.now()
	if now.Before(m.readyAt[cooldownKey]) {
		return fmt.Errorf("技能冷却中: id=%d", configID)
	}
	if m.combat == nil || m.combat.world == nil {
		return fmt.Errorf("战斗系统未就绪")
	}
	attackerEntity, ok := m.combat.world.Entity(actorID)
	if !ok || attackerEntity.Kind != scene.EntityPlayer || attackerEntity.HP <= 0 {
		return fmt.Errorf("玩家不在有效战斗场景: %d", actorID)
	}
	targetEntity, ok := m.combat.world.Entity(targetID)
	if !ok || targetEntity.Kind != scene.EntityMonster || targetEntity.HP <= 0 || targetEntity.SceneID != attackerEntity.SceneID {
		return fmt.Errorf("目标不是同场景存活怪物: %d", targetID)
	}
	attackRange := int(math.Ceil(cfg.CastRange * scene.CellSize))
	if attackRange <= 0 {
		attackRange = scene.CellSize
	}
	if absInt(attackerEntity.X-targetEntity.X)+absInt(attackerEntity.Y-targetEntity.Y) > attackRange {
		return fmt.Errorf("目标超出技能范围: id=%d range=%d", configID, attackRange)
	}
	damageFloat := float64(m.combat.config.Damage)*cfg.DamageRatio + float64(cfg.FlatDamage)
	if damageFloat > math.MaxInt32 {
		damageFloat = math.MaxInt32
	}
	damage := max(1, int(math.Round(damageFloat)))
	result, err := m.combat.ResolvePlayerAttack(actorID, targetID, damage, attackRange, now)
	if err != nil {
		return err
	}
	m.readyAt[cooldownKey] = now.Add(cfg.Cooldown)
	packet := buildSkillCast(actorID, configID, targetID, result.Damage)
	return router.SendToSession(sid, packet)
}

func skillMaxLevel(actor *Actor) int {
	level := min(actor.Level, 80) + max(0, actor.ZhuanSheng)*10
	return min(level, 260)
}

func buildSkillUpdate(roleID, slot, level int) []byte {
	packet := []byte{cmdSkill, sSkillCmdUpdateSkill}
	packet = binary.LittleEndian.AppendUint16(packet, uint16(roleID))
	packet = binary.LittleEndian.AppendUint16(packet, uint16(slot))
	return binary.LittleEndian.AppendUint16(packet, uint16(level))
}

func buildSkillCast(attackerID, skillID, targetID, damage int) []byte {
	packet := []byte{cmdSkill, sSkillCmdCastSkill}
	packet = binary.LittleEndian.AppendUint64(packet, math.Float64bits(float64(attackerID)))
	packet = binary.LittleEndian.AppendUint32(packet, uint32(skillID))
	packet = binary.LittleEndian.AppendUint64(packet, math.Float64bits(float64(targetID)))
	packet = binary.LittleEndian.AppendUint16(packet, 1)
	return binary.LittleEndian.AppendUint64(packet, math.Float64bits(float64(damage)))
}
