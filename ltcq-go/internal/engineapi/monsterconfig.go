package engineapi

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/xiaokentrl/ChuanQi/ltcq-go/internal/luahost"
	"github.com/xiaokentrl/ChuanQi/ltcq-go/internal/scene"
	lua "github.com/yuin/gopher-lua"
)

type MonsterConfig struct {
	ID    int
	Level int
	HP    int
}

type MonsterSpawn struct {
	SceneID               int   `json:"sceneID"`
	ConfigID              int   `json:"configID"`
	X                     int   `json:"x"`
	Y                     int   `json:"y"`
	Count                 int   `json:"count"`
	HP                    int   `json:"hp,omitempty"`
	Exp                   int64 `json:"exp,omitempty"`
	Gold                  int64 `json:"gold,omitempty"`
	ItemID                int   `json:"itemID,omitempty"`
	ItemCount             int   `json:"itemCount,omitempty"`
	RespawnSeconds        int   `json:"respawnSeconds,omitempty"`
	AttackDamage          int   `json:"attackDamage,omitempty"`
	AttackRange           int   `json:"attackRange,omitempty"`
	AttackIntervalSeconds int   `json:"attackIntervalSeconds,omitempty"`
	AggroRange            int   `json:"aggroRange,omitempty"`
	ChaseStep             int   `json:"chaseStep,omitempty"`
	LeashRange            int   `json:"leashRange,omitempty"`
}

func LoadMonsterCatalog(dataRoot, ostype string) (map[int]MonsterConfig, error) {
	g := luahost.NewGopherLua(luahost.Config{DataRoot: dataRoot, OSType: ostype})
	defer g.Close()
	if err := g.Load("monster_lan_stub", `
LAN=setmetatable({}, {__index=function() return setmetatable({}, {__index=function(_, key) return tostring(key) end}) end})
`); err != nil {
		return nil, err
	}
	catalog := map[int]MonsterConfig{}
	for _, module := range []struct{ name, global string }{
		{"monsters.config", "MonstersConfig"},
		{"chaptermonster.config", "ChapterMonstersConfig"},
	} {
		path := filepath.Join(dataRoot, "config", "monster", module.name)
		if err := g.LoadFile(path); err != nil {
			return nil, fmt.Errorf("加载怪物配置 %s 失败: %w", module.name, err)
		}
		table, ok := g.L.GetGlobal(module.global).(*lua.LTable)
		if !ok {
			return nil, fmt.Errorf("怪物配置 %s 缺少 %s 表", module.name, module.global)
		}
		var configErr error
		table.ForEach(func(key, value lua.LValue) {
			if configErr != nil {
				return
			}
			entry, ok := value.(*lua.LTable)
			if !ok {
				configErr = fmt.Errorf("怪物配置 %s 含非法条目", module.name)
				return
			}
			cfg := MonsterConfig{
				ID:    int(lua.LVAsNumber(key)),
				Level: int(lua.LVAsNumber(entry.RawGetString("level"))),
				HP:    int(lua.LVAsNumber(entry.RawGetString("hp"))),
			}
			if cfg.ID <= 0 {
				configErr = fmt.Errorf("怪物配置 %s 含非法 ID: %d", module.name, cfg.ID)
				return
			}
			catalog[cfg.ID] = cfg
		})
		if configErr != nil {
			return nil, configErr
		}
	}
	if len(catalog) == 0 {
		return nil, fmt.Errorf("怪物配置为空")
	}
	return catalog, nil
}

func LoadMonsterSpawns(path string) ([]MonsterSpawn, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取怪物刷新配置失败: %w", err)
	}
	var spawns []MonsterSpawn
	if err := json.Unmarshal(data, &spawns); err != nil {
		return nil, fmt.Errorf("解析怪物刷新配置失败: %w", err)
	}
	if len(spawns) == 0 {
		return nil, fmt.Errorf("怪物刷新配置为空")
	}
	return spawns, nil
}

type monsterRecord struct {
	spawn MonsterSpawn
	due   time.Time
}

type MonsterManager struct {
	world   *scene.World
	catalog map[int]MonsterConfig
	nextID  int
	mu      sync.Mutex
	records map[int]monsterRecord
}

func NewMonsterManager(world *scene.World, catalog map[int]MonsterConfig) *MonsterManager {
	return &MonsterManager{world: world, catalog: catalog, nextID: 1 << 30, records: map[int]monsterRecord{}}
}

func (m *MonsterManager) SpawnAll(spawns []MonsterSpawn) (int, error) {
	for _, spawn := range spawns {
		if err := m.validateSpawn(spawn); err != nil {
			return 0, err
		}
	}
	spawned := 0
	for _, spawn := range spawns {
		for range spawn.Count {
			id := m.nextID
			m.nextID++
			monster := m.catalog[spawn.ConfigID]
			hp := monster.HP
			if spawn.HP > 0 {
				hp = spawn.HP
			}
			deliveries, err := m.world.AddMonster(id, spawn.SceneID, spawn.ConfigID, spawn.X*scene.CellSize, spawn.Y*scene.CellSize, hp, hp)
			if err != nil {
				return spawned, err
			}
			m.mu.Lock()
			m.records[id] = monsterRecord{spawn: spawn}
			m.mu.Unlock()
			if err := m.world.Dispatch(deliveries); err != nil {
				return spawned, err
			}
			spawned++
		}
	}
	return spawned, nil
}

func (m *MonsterManager) Rule(monsterID int) (MonsterSpawn, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	record, ok := m.records[monsterID]
	return record.spawn, ok
}

func (m *MonsterManager) MarkDead(monsterID int, now time.Time) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	record, ok := m.records[monsterID]
	if !ok || record.spawn.RespawnSeconds <= 0 || !record.due.IsZero() {
		return false
	}
	record.due = now.Add(time.Duration(record.spawn.RespawnSeconds) * time.Second)
	m.records[monsterID] = record
	return true
}

func (m *MonsterManager) RespawnDue(now time.Time) []int {
	m.mu.Lock()
	defer m.mu.Unlock()
	ids := make([]int, 0)
	for id, record := range m.records {
		if !record.due.IsZero() && !now.Before(record.due) {
			ids = append(ids, id)
		}
	}
	sort.Ints(ids)
	return ids
}

func (m *MonsterManager) Respawn(monsterID int) error {
	m.mu.Lock()
	record, ok := m.records[monsterID]
	if !ok || record.due.IsZero() {
		m.mu.Unlock()
		return fmt.Errorf("怪物未等待重生: %d", monsterID)
	}
	spawn := record.spawn
	monster := m.catalog[spawn.ConfigID]
	hp := monster.HP
	if spawn.HP > 0 {
		hp = spawn.HP
	}
	deliveries, err := m.world.AddMonster(monsterID, spawn.SceneID, spawn.ConfigID, spawn.X*scene.CellSize, spawn.Y*scene.CellSize, hp, hp)
	if err != nil {
		m.mu.Unlock()
		return err
	}
	record.due = time.Time{}
	m.records[monsterID] = record
	m.mu.Unlock()
	return m.world.Dispatch(deliveries)
}

func (m *MonsterManager) validateSpawn(spawn MonsterSpawn) error {
	if spawn.SceneID <= 0 || spawn.ConfigID <= 0 || spawn.Count < 1 || spawn.Count > 256 || spawn.X < 0 || spawn.Y < 0 {
		return fmt.Errorf("怪物刷新参数非法: %+v", spawn)
	}
	cfg, err := m.world.Spawn(spawn.SceneID)
	if err != nil {
		return err
	}
	if spawn.X >= cfg.Width/scene.CellSize || spawn.Y >= cfg.Height/scene.CellSize {
		return fmt.Errorf("怪物刷新坐标越界: %+v", spawn)
	}
	monster, ok := m.catalog[spawn.ConfigID]
	if !ok {
		return fmt.Errorf("怪物配置不存在: %d", spawn.ConfigID)
	}
	if spawn.HP < 0 {
		return fmt.Errorf("怪物刷新 HP 覆盖非法: %+v", spawn)
	}
	if spawn.Exp < 0 || spawn.Gold < 0 || spawn.RespawnSeconds < 0 {
		return fmt.Errorf("怪物奖励或重生配置非法: %+v", spawn)
	}
	if spawn.ItemID < 0 || spawn.ItemCount < 0 {
		return fmt.Errorf("怪物物品奖励配置非法: %+v", spawn)
	}
	if spawn.ItemID > 0 && spawn.ItemCount == 0 || spawn.ItemCount > 0 && spawn.ItemID == 0 {
		return fmt.Errorf("怪物物品奖励缺少物品或数量: %+v", spawn)
	}
	if spawn.AttackDamage < 0 || spawn.AttackRange < 0 || spawn.AttackIntervalSeconds < 0 {
		return fmt.Errorf("怪物反击配置非法: %+v", spawn)
	}
	if spawn.AttackDamage > 0 && (spawn.AttackRange == 0 || spawn.AttackIntervalSeconds == 0) {
		return fmt.Errorf("怪物反击缺少范围或间隔: %+v", spawn)
	}
	if spawn.AttackDamage == 0 && (spawn.AttackRange != 0 || spawn.AttackIntervalSeconds != 0) {
		return fmt.Errorf("怪物反击伤害为零但配置了范围或间隔: %+v", spawn)
	}
	if spawn.AggroRange < 0 || spawn.ChaseStep < 0 || spawn.LeashRange < 0 {
		return fmt.Errorf("怪物 AI 配置非法: %+v", spawn)
	}
	if spawn.AggroRange > 0 || spawn.ChaseStep > 0 || spawn.LeashRange > 0 {
		if spawn.AggroRange == 0 || spawn.ChaseStep == 0 || spawn.LeashRange == 0 || spawn.AttackDamage == 0 {
			return fmt.Errorf("怪物主动 AI 缺少索敌、追击、归位或攻击配置: %+v", spawn)
		}
		if spawn.LeashRange < spawn.AggroRange {
			return fmt.Errorf("怪物归位范围小于索敌范围: %+v", spawn)
		}
	}
	if monster.HP <= 0 {
		return fmt.Errorf("怪物 HP 配置不完整: id=%d hp=%d", spawn.ConfigID, monster.HP)
	}
	return nil
}
