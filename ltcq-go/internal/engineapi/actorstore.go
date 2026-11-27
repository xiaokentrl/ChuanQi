package engineapi

import (
	"fmt"
	"log/slog"
	"math"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/xiaokentrl/ChuanQi/ltcq-go/internal/luahost"
)

// actorstore.go：内存玩家实体（阶段4.6 骨架）。
// 完整领域逻辑（背包/属性/场景AOI）属后续工作；此处实现 T0 验证所需的核心：
// id/level/name/currency/items/staticVar + 脏标记，让 LActor API 从 stub 变真实读写。

// Actor 是内存中的玩家实体。
type Actor struct {
	ID                int
	Level             int
	Exp               int64
	Name              string
	Account           string // 登录账号（actors.accountname）
	AccountID         int    // 账号 ID（actors.accountid）
	Job               int    // 职业（actors.job）
	ZhuanSheng        int    // 转生等级（actors.zhuansheng_lv）
	Sex               int    // 性别（actors.sex）
	SceneID           int    // 进入副本前的场景 id（actors.sceneid）
	ServerID          uint32
	TotalPower        int64 // 登录时的战力快照（actors.totalpower）
	X                 int   // 场景像素坐标
	Y                 int
	HP                int
	MaxHP             int
	mu                sync.RWMutex
	VIPLevel          int                        // vip 等级（actors.vip_level）
	Recharge          int                        // 累计充值（actors.recharge）
	CreateTime        int64                      // 角色创建 Unix 秒（actors.createtime）
	bagCapacity       int                        // 普通背包容量（BagBaseConfig.baseSize=200；0=不限，待装备容量证据）
	itemResolver      func(int) (ItemInfo, bool) // 物品判定注入（BagTab 路由）
	AppID             string                     // 渠道 ID（actors.appid）
	PfID              string                     // 平台 ID（actors.pfid）
	GuildID           int                        // 公会 ID（actors.guildid）
	Camp              int                        // 进程内阵营标签，不落库、不发包、不参与战斗判定
	GuildName         string                     // 公会名快照（actors.guild_name）
	ChapterLevel      int                        // 章节等级（actors.chapterlevel）
	MonthCard         int                        // 月卡状态（actors.monthcard）
	Reincarnate       int                        // 轮回等级（actors.reincarnate_lv）
	Currency          map[int]int                // NumericType → 数量
	Items             map[int]int                // itemId → 数量
	ClientConfig      map[int]int                // 客户端配置 key→val（clientconfig.lua cliCfg；持久化）
	Skills            map[int]int                // skillId → 等级（持久化）
	Static            luahost.Value              // staticVar（Lua 表，Go 自有 TableNode 格式持久化）
	Dirty             bool                       // 任一持久化字段变更
	ClientConfigDirty bool                       // ClientConfig 是否已变更且未落库
	SkillsDirty       bool                       // Skills 是否已变更且未落库
	StaticDirty       bool                       // Static 是否已变更且未落库
	bagBase           []byte
	bagStore          BagStore
	bagNotify         func([]byte) error
	vipStore          VIPLevelStore
	currencyStore     CurrencyStore
}

type VIPLevelStore interface {
	SaveVIPLevel(actorID, level int) error
}

type CurrencyStore interface {
	SaveCurrency(actorID, currencyType int, amount int64) error
}

func newActor(id int) *Actor {
	return &Actor{
		ID:           id,
		Level:        1,
		Name:         "",
		Currency:     map[int]int{},
		Items:        map[int]int{},
		ClientConfig: map[int]int{},
		Skills:       map[int]int{},
		Dirty:        false,
	}
}

// ActorManager 管理所有在线 Actor。
type ActorManager struct {
	mu     sync.RWMutex
	actors map[int]*Actor
}

func NewActorManager() *ActorManager {
	return &ActorManager{actors: map[int]*Actor{}}
}

// GetOrCreate 按 id 取 Actor，不存在则创建（登录建号）。
func (m *ActorManager) GetOrCreate(id int) *Actor {
	m.mu.Lock()
	defer m.mu.Unlock()
	if a, ok := m.actors[id]; ok {
		return a
	}
	a := newActor(id)
	m.actors[id] = a
	return a
}

func (m *ActorManager) Get(id int) *Actor {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.actors[id]
}

func (m *ActorManager) GetByName(name string) *Actor {
	if name == "" {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	var found *Actor
	for _, actor := range m.actors {
		actor.mu.RLock()
		matches := actor.Name == name
		actor.mu.RUnlock()
		if !matches {
			continue
		}
		if found != nil {
			return nil
		}
		found = actor
	}
	return found
}

func (m *ActorManager) GetByAccount(account string) *Actor {
	if account == "" {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	var found *Actor
	for _, actor := range m.actors {
		actor.mu.RLock()
		matches := actor.Account == account
		actor.mu.RUnlock()
		if !matches {
			continue
		}
		if found != nil {
			return nil
		}
		found = actor
	}
	return found
}

// DefaultActorManager 全局默认管理器（engineapi 包级使用）。
var DefaultActorManager = NewActorManager()

func (a *Actor) SetVIPStore(store VIPLevelStore) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.vipStore = store
}

func (a *Actor) SetCurrencyStore(store CurrencyStore) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.currencyStore = store
}

// 以下为真实 LActor API 实现（替换 actor.go 中的 stub）。

func realGetActorId(a ...luahost.Value) (luahost.Value, error) {
	if len(a) == 0 {
		return 0, nil
	}
	// actor 句柄即 id（骨架）；未来实体指针时改为从句柄查
	return asActorID(a[0]), nil
}

func realActorLog(a ...luahost.Value) (luahost.Value, error) {
	actorID := 0
	if len(a) > 0 {
		if id, ok := actorHandleArgument(a[0]); ok {
			actorID = id
		}
	}
	args := make([]string, 0, len(a))
	if len(a) > 1 {
		args = make([]string, 0, len(a)-1)
		for _, value := range a[1:] {
			if value == nil {
				args = append(args, "nil")
			} else {
				args = append(args, fmt.Sprint(value))
			}
		}
	}
	slog.Info("LActor.log", "actorID", actorID, "args", args)
	return nil, nil
}

func installCombatAPI(L luahost.Interpreter, combat *CombatManager) {
	L.Register("LActor.stopAI", func(a ...luahost.Value) (luahost.Value, error) {
		return actorStopAI(combat, a...)
	})
	L.Register("LActor.setAITarget", func(a ...luahost.Value) (luahost.Value, error) {
		return actorSetAITarget(combat, a...)
	})
	L.Register("LActor.getAITarget", func(a ...luahost.Value) (luahost.Value, error) {
		return actorGetAITarget(combat, a...)
	})
	L.Register("LActor.setAIAttackMonster", func(a ...luahost.Value) (luahost.Value, error) {
		return actorSetAIAttackMonster(combat, a...)
	})
	L.Register("LActor.setAITargetNull", func(a ...luahost.Value) (luahost.Value, error) {
		return actorSetAITargetNull(combat, a...)
	})
}

func actorStopAI(combat *CombatManager, a ...luahost.Value) (luahost.Value, error) {
	if combat == nil || len(a) == 0 {
		return nil, nil
	}
	actorID, ok := actorHandleArgument(a[0])
	if !ok || DefaultActorManager.Get(actorID) == nil {
		return nil, nil
	}
	_ = combat.StopPlayerAI(actorID)
	return nil, nil
}

func actorSetAITarget(combat *CombatManager, a ...luahost.Value) (luahost.Value, error) {
	if combat == nil || len(a) < 2 {
		return nil, nil
	}
	sourceID, sourceOK := actorHandleArgument(a[0])
	targetID, targetOK := actorHandleArgument(a[1])
	if !sourceOK || !targetOK {
		return nil, nil
	}
	_ = combat.SetAITarget(sourceID, targetID)
	return nil, nil
}

func actorGetAITarget(combat *CombatManager, a ...luahost.Value) (luahost.Value, error) {
	if combat == nil || len(a) == 0 {
		return nil, nil
	}
	entityID, ok := actorHandleArgument(a[0])
	if !ok {
		return nil, nil
	}
	targetID, ok := combat.GetAITarget(entityID)
	if !ok {
		return nil, nil
	}
	return targetID, nil
}

func actorSetAIAttackMonster(combat *CombatManager, a ...luahost.Value) (luahost.Value, error) {
	if combat == nil || len(a) == 0 {
		return nil, nil
	}
	actorID, ok := actorHandleArgument(a[0])
	if !ok || DefaultActorManager.Get(actorID) == nil {
		return nil, nil
	}
	_ = combat.ResumePlayerAutoAttack(actorID)
	return nil, nil
}

func actorSetAITargetNull(combat *CombatManager, a ...luahost.Value) (luahost.Value, error) {
	if combat == nil || len(a) == 0 {
		return nil, nil
	}
	entityID, ok := actorHandleArgument(a[0])
	if !ok {
		return nil, nil
	}
	_ = combat.ClearAITarget(entityID)
	return nil, nil
}

func realGetActorById(a ...luahost.Value) (luahost.Value, error) {
	if len(a) == 0 {
		return nil, nil
	}
	actor := DefaultActorManager.Get(asActorID(a[0]))
	if actor == nil {
		return nil, nil
	}
	return actor.ID, nil
}

func actorHandleArgument(v luahost.Value) (int, bool) {
	id, err := integerArgument(v)
	return id, err == nil && id > 0
}

func realGetActor(a ...luahost.Value) (luahost.Value, error) {
	if len(a) == 0 {
		return nil, nil
	}
	actorID, ok := actorHandleArgument(a[0])
	if !ok {
		return nil, nil
	}
	actor := DefaultActorManager.Get(actorID)
	if actor == nil {
		return nil, nil
	}
	return actor.ID, nil
}

func realGetRoleCount(a ...luahost.Value) (luahost.Value, error) {
	if len(a) == 0 {
		return 0, nil
	}
	actorID, ok := actorHandleArgument(a[0])
	if !ok || DefaultActorManager.Get(actorID) == nil {
		return 0, nil
	}
	return 1, nil
}

func realGetRole(a ...luahost.Value) (luahost.Value, error) {
	if len(a) < 2 {
		return nil, nil
	}
	actorID, ok := actorHandleArgument(a[0])
	if !ok {
		return nil, nil
	}
	index, err := integerArgument(a[1])
	if err != nil || index != 0 {
		return nil, nil
	}
	actor := DefaultActorManager.Get(actorID)
	if actor == nil {
		return nil, nil
	}
	return actor.ID, nil
}

func realGetRoleByJob(a ...luahost.Value) (luahost.Value, error) {
	if len(a) < 2 {
		return nil, nil
	}
	actorID, ok := actorHandleArgument(a[0])
	if !ok {
		return nil, nil
	}
	job, err := integerArgument(a[1])
	if err != nil || job < 1 || job > 3 {
		return nil, nil
	}
	actor := DefaultActorManager.Get(actorID)
	if actor == nil {
		return nil, nil
	}
	actor.mu.RLock()
	defer actor.mu.RUnlock()
	if actor.Job != job {
		return nil, nil
	}
	return actor.ID, nil
}

func realGetServerId(a ...luahost.Value) (luahost.Value, error) {
	if len(a) == 0 {
		return 0, nil
	}
	actor := DefaultActorManager.Get(asActorID(a[0]))
	if actor == nil {
		return 0, nil
	}
	actor.mu.RLock()
	defer actor.mu.RUnlock()
	return int(actor.ServerID), nil
}

func realGetAccountName(a ...luahost.Value) (luahost.Value, error) {
	if len(a) == 0 {
		return "", nil
	}
	actor := DefaultActorManager.Get(asActorID(a[0]))
	if actor == nil {
		return "", nil
	}
	actor.mu.RLock()
	defer actor.mu.RUnlock()
	return actor.Account, nil
}

func realGetActorPower(a ...luahost.Value) (luahost.Value, error) {
	if len(a) == 0 {
		return 0, nil
	}
	actor := DefaultActorManager.Get(asActorID(a[0]))
	if actor == nil {
		return 0, nil
	}
	actor.mu.RLock()
	defer actor.mu.RUnlock()
	return actor.TotalPower, nil
}

func realGetAccountId(a ...luahost.Value) (luahost.Value, error) {
	if len(a) == 0 {
		return 0, nil
	}
	actor := DefaultActorManager.Get(asActorID(a[0]))
	if actor == nil {
		return 0, nil
	}
	actor.mu.RLock()
	defer actor.mu.RUnlock()
	return actor.AccountID, nil
}

func realGetRecharge(a ...luahost.Value) (luahost.Value, error) {
	if len(a) == 0 {
		return 0, nil
	}
	actor := DefaultActorManager.Get(asActorID(a[0]))
	if actor == nil {
		return 0, nil
	}
	actor.mu.RLock()
	defer actor.mu.RUnlock()
	return actor.Recharge, nil
}

func realGetCreateTime(a ...luahost.Value) (luahost.Value, error) {
	if len(a) == 0 {
		return 0, nil
	}
	actor := DefaultActorManager.Get(asActorID(a[0]))
	if actor == nil {
		return 0, nil
	}
	actor.mu.RLock()
	defer actor.mu.RUnlock()
	return actor.CreateTime, nil
}

func realGetAppID(a ...luahost.Value) (luahost.Value, error) {
	if len(a) == 0 {
		return "", nil
	}
	actor := DefaultActorManager.Get(asActorID(a[0]))
	if actor == nil {
		return "", nil
	}
	actor.mu.RLock()
	defer actor.mu.RUnlock()
	return actor.AppID, nil
}

func realGetPfID(a ...luahost.Value) (luahost.Value, error) {
	if len(a) == 0 {
		return "", nil
	}
	actor := DefaultActorManager.Get(asActorID(a[0]))
	if actor == nil {
		return "", nil
	}
	actor.mu.RLock()
	defer actor.mu.RUnlock()
	return actor.PfID, nil
}

func realGetGamePf(a ...luahost.Value) (luahost.Value, error) {
	if len(a) == 0 {
		return "", nil
	}
	actorID, ok := actorHandleArgument(a[0])
	if !ok {
		return "", nil
	}
	actor := DefaultActorManager.Get(actorID)
	if actor == nil {
		return "", nil
	}
	actor.mu.RLock()
	defer actor.mu.RUnlock()
	pf := actor.PfID
	if strings.HasPrefix(pf, "union") {
		if star := strings.IndexByte(pf, '*'); star >= 0 {
			return pf[star+1:], nil
		}
	}
	return pf, nil
}

func realGetGuildID(a ...luahost.Value) (luahost.Value, error) {
	if len(a) == 0 {
		return 0, nil
	}
	actor := DefaultActorManager.Get(asActorID(a[0]))
	if actor == nil {
		return 0, nil
	}
	actor.mu.RLock()
	defer actor.mu.RUnlock()
	return actor.GuildID, nil
}

func realGetGuildName(a ...luahost.Value) (luahost.Value, error) {
	if len(a) == 0 {
		return "", nil
	}
	actor := DefaultActorManager.Get(asActorID(a[0]))
	if actor == nil {
		return "", nil
	}
	actor.mu.RLock()
	defer actor.mu.RUnlock()
	return actor.GuildName, nil
}

func realGetChapterLevel(a ...luahost.Value) (luahost.Value, error) {
	if len(a) == 0 {
		return 0, nil
	}
	actor := DefaultActorManager.Get(asActorID(a[0]))
	if actor == nil {
		return 0, nil
	}
	actor.mu.RLock()
	defer actor.mu.RUnlock()
	return actor.ChapterLevel, nil
}

func realGetMonthCard(a ...luahost.Value) (luahost.Value, error) {
	if len(a) == 0 {
		return 0, nil
	}
	actor := DefaultActorManager.Get(asActorID(a[0]))
	if actor == nil {
		return 0, nil
	}
	actor.mu.RLock()
	defer actor.mu.RUnlock()
	return actor.MonthCard, nil
}

func realGetReincarnateLevel(a ...luahost.Value) (luahost.Value, error) {
	if len(a) == 0 {
		return 0, nil
	}
	actor := DefaultActorManager.Get(asActorID(a[0]))
	if actor == nil {
		return 0, nil
	}
	actor.mu.RLock()
	defer actor.mu.RUnlock()
	return actor.Reincarnate, nil
}

func realIsActor(a ...luahost.Value) (luahost.Value, error) {
	if len(a) == 0 {
		return false, nil
	}
	id := asActorID(a[0])
	return id != 0 && DefaultActorManager.Get(id) != nil, nil
}

func realGetActorByName(a ...luahost.Value) (luahost.Value, error) {
	if len(a) == 0 {
		return nil, nil
	}
	name, ok := a[0].(string)
	if !ok || name == "" {
		return nil, nil
	}
	actor := DefaultActorManager.GetByName(name)
	if actor == nil {
		return nil, nil
	}
	return actor.ID, nil
}

func realGetActorIdByName(a ...luahost.Value) (luahost.Value, error) {
	if len(a) == 0 {
		return 0, nil
	}
	name, ok := a[0].(string)
	if !ok || name == "" {
		return 0, nil
	}
	actor := DefaultActorManager.GetByName(name)
	if actor == nil {
		return 0, nil
	}
	return actor.ID, nil
}

func realGetActorByAccountName(a ...luahost.Value) (luahost.Value, error) {
	if len(a) == 0 {
		return nil, nil
	}
	account, ok := a[0].(string)
	if !ok || account == "" {
		return nil, nil
	}
	actor := DefaultActorManager.GetByAccount(account)
	if actor == nil {
		return nil, nil
	}
	return actor.ID, nil
}

func realGetActorIdByAccountName(a ...luahost.Value) (luahost.Value, error) {
	if len(a) == 0 {
		return 0, nil
	}
	account, ok := a[0].(string)
	if !ok || account == "" {
		return 0, nil
	}
	actor := DefaultActorManager.GetByAccount(account)
	if actor == nil {
		return 0, nil
	}
	return actor.ID, nil
}

func realSetVipLevel(a ...luahost.Value) (luahost.Value, error) {
	if len(a) < 2 {
		return nil, fmt.Errorf("setVipLevel 缺少角色或等级")
	}
	var level int
	switch value := a[1].(type) {
	case int:
		level = value
	case int64:
		level = int(value)
	default:
		return nil, fmt.Errorf("setVipLevel 等级非法")
	}
	if level < 0 {
		return nil, fmt.Errorf("setVipLevel 等级非法: %d", level)
	}
	actor := DefaultActorManager.Get(asActorID(a[0]))
	if actor == nil {
		return nil, fmt.Errorf("setVipLevel 角色不存在: %d", asActorID(a[0]))
	}
	actor.mu.Lock()
	defer actor.mu.Unlock()
	if actor.VIPLevel == level {
		return nil, nil
	}
	if actor.vipStore == nil {
		return nil, fmt.Errorf("setVipLevel 存档不可用: actor=%d", actor.ID)
	}
	if err := actor.vipStore.SaveVIPLevel(actor.ID, level); err != nil {
		return nil, fmt.Errorf("setVipLevel 存档失败: actor=%d %w", actor.ID, err)
	}
	actor.VIPLevel = level
	return nil, nil
}

func realGetJob(a ...luahost.Value) (luahost.Value, error) {
	if len(a) == 0 {
		return 0, nil
	}
	actor := DefaultActorManager.Get(asActorID(a[0]))
	if actor == nil {
		return 0, nil
	}
	actor.mu.RLock()
	defer actor.mu.RUnlock()
	return actor.Job, nil
}

func realGetSex(a ...luahost.Value) (luahost.Value, error) {
	if len(a) == 0 {
		return 0, nil
	}
	actor := DefaultActorManager.Get(asActorID(a[0]))
	if actor == nil {
		return 0, nil
	}
	actor.mu.RLock()
	defer actor.mu.RUnlock()
	return actor.Sex, nil
}

func realGetSceneId(a ...luahost.Value) (luahost.Value, error) {
	if len(a) == 0 {
		return 0, nil
	}
	actor := DefaultActorManager.Get(asActorID(a[0]))
	if actor == nil {
		return 0, nil
	}
	actor.mu.RLock()
	defer actor.mu.RUnlock()
	return actor.SceneID, nil
}

func realGetExp(a ...luahost.Value) (luahost.Value, error) {
	if len(a) == 0 {
		return 0, nil
	}
	actor := DefaultActorManager.Get(asActorID(a[0]))
	if actor == nil {
		return 0, nil
	}
	actor.mu.RLock()
	defer actor.mu.RUnlock()
	return actor.Exp, nil
}

func realGetBagItemCount(a ...luahost.Value) (luahost.Value, error) {
	if len(a) == 0 {
		return 0, nil
	}
	actor := DefaultActorManager.Get(asActorID(a[0]))
	if actor == nil {
		return 0, nil
	}
	actor.mu.RLock()
	defer actor.mu.RUnlock()
	total := 0
	for _, count := range actor.Items {
		total += count
	}
	return total, nil
}

func realGetStoreItemCount(a ...luahost.Value) (luahost.Value, error) {
	if len(a) < 2 {
		return 0, nil
	}
	storeType, err := integerArgument(a[1])
	if err != nil || storeType != 0 {
		return 0, nil
	}
	actorID, ok := actorHandleArgument(a[0])
	if !ok {
		return 0, nil
	}
	actor := DefaultActorManager.Get(actorID)
	if actor == nil {
		return 0, nil
	}
	actor.mu.RLock()
	defer actor.mu.RUnlock()
	total := 0
	for _, count := range actor.Items {
		total += count
	}
	return total, nil
}

func realAddItem(a ...luahost.Value) (luahost.Value, error) {
	if len(a) < 5 {
		return false, nil
	}
	actorID, ok := actorHandleArgument(a[0])
	if !ok {
		return false, nil
	}
	itemID, err := integerArgument(a[1])
	if err != nil || itemID <= 0 {
		return false, nil
	}
	count, err := integerArgument(a[4])
	if err != nil || count <= 0 {
		return false, nil
	}
	actor := DefaultActorManager.GetOrCreate(actorID)
	return actor.AddItem(itemID, count)
}

func realGetZhuanShengLevel(a ...luahost.Value) (luahost.Value, error) {
	if len(a) == 0 {
		return 0, nil
	}
	actor := DefaultActorManager.Get(asActorID(a[0]))
	if actor == nil {
		return 0, nil
	}
	actor.mu.RLock()
	defer actor.mu.RUnlock()
	return actor.ZhuanSheng, nil
}

func realGetVipLevel(a ...luahost.Value) (luahost.Value, error) {
	if len(a) == 0 {
		return 0, nil
	}
	actor := DefaultActorManager.Get(asActorID(a[0]))
	if actor == nil {
		return 0, nil
	}
	actor.mu.RLock()
	defer actor.mu.RUnlock()
	return actor.VIPLevel, nil
}

func realGetLevel(a ...luahost.Value) (luahost.Value, error) {
	if len(a) == 0 {
		return 0, nil
	}
	actor := DefaultActorManager.Get(asActorID(a[0]))
	if actor == nil {
		return 0, nil
	}
	return actor.Level, nil
}

func realGetName(a ...luahost.Value) (luahost.Value, error) {
	if len(a) == 0 {
		return "", nil
	}
	actor := DefaultActorManager.Get(asActorID(a[0]))
	if actor == nil {
		return "", nil
	}
	return actor.Name, nil
}

func realGetCurrency(a ...luahost.Value) (luahost.Value, error) {
	if len(a) < 2 {
		return 0, nil
	}
	actor := DefaultActorManager.Get(asActorID(a[0]))
	if actor == nil {
		return 0, nil
	}
	actor.mu.RLock()
	defer actor.mu.RUnlock()
	return actor.Currency[asInt(a[1])], nil
}

func integerArgument(v luahost.Value) (int, error) {
	switch x := v.(type) {
	case int:
		return x, nil
	case int64:
		if int64(int(x)) != x {
			return 0, fmt.Errorf("参数超出整数范围: %v", v)
		}
		return int(x), nil
	case float64:
		if math.IsNaN(x) || math.IsInf(x, 0) || math.Trunc(x) != x {
			return 0, fmt.Errorf("参数必须是整数: %v", v)
		}
		if strconv.IntSize == 32 {
			if x < -float64(math.MaxInt32)-1 || x > float64(math.MaxInt32) {
				return 0, fmt.Errorf("参数超出整数范围: %v", v)
			}
		} else if x < -float64(math.MaxInt64) || x >= float64(math.MaxInt64) {
			return 0, fmt.Errorf("参数超出整数范围: %v", v)
		}
		return int(x), nil
	default:
		return 0, fmt.Errorf("参数必须是整数: %v", v)
	}
}

func realChangeCurrency(a ...luahost.Value) (luahost.Value, error) {
	if len(a) < 3 {
		return nil, fmt.Errorf("changeCurrency 缺少角色、类型或数量")
	}
	actorID, err := integerArgument(a[0])
	if err != nil || actorID <= 0 {
		return nil, fmt.Errorf("changeCurrency 角色参数非法: %v", a[0])
	}
	typ, err := integerArgument(a[1])
	if err != nil {
		return nil, fmt.Errorf("changeCurrency 类型参数非法: %v", a[1])
	}
	delta, err := integerArgument(a[2])
	if err != nil {
		return nil, fmt.Errorf("changeCurrency 数量参数非法: %v", a[2])
	}
	actor := DefaultActorManager.Get(actorID)
	if actor == nil {
		return nil, fmt.Errorf("changeCurrency 角色不存在: %d", actorID)
	}
	reason := ""
	if len(a) > 3 {
		reason = asString(a[3])
	}

	actor.mu.Lock()
	defer actor.mu.Unlock()
	current := actor.Currency[typ]
	if current < 0 {
		return nil, fmt.Errorf("角色货币余额非法: actor=%d type=%d amount=%d", actorID, typ, current)
	}
	if delta < -current {
		return nil, fmt.Errorf("角色货币余额不足: actor=%d type=%d amount=%d delta=%d", actorID, typ, current, delta)
	}
	if delta > 0 && current > math.MaxInt-delta {
		return nil, fmt.Errorf("角色货币变更溢出: actor=%d type=%d amount=%d delta=%d", actorID, typ, current, delta)
	}
	if delta == 0 {
		return nil, nil
	}
	next := current + delta
	if typ == currencyGold || typ == currencyYuanBao {
		if actor.currencyStore == nil {
			return nil, fmt.Errorf("金币/元宝存档器未绑定: actor=%d type=%d", actorID, typ)
		}
		if err := actor.currencyStore.SaveCurrency(actorID, typ, int64(next)); err != nil {
			return nil, fmt.Errorf("货币存档失败: actor=%d type=%d amount=%d: %w", actorID, typ, next, err)
		}
	}
	if actor.Currency == nil {
		actor.Currency = map[int]int{}
	}
	actor.Currency[typ] = next
	slog.Debug("LActor.changeCurrency", "actorID", actorID, "type", typ, "delta", delta, "amount", next, "reason", reason)
	return nil, nil
}

func setCurrencyAmount(actor *Actor, currencyType, amount int) error {
	if actor == nil || currencyType < 0 || amount < 0 {
		return fmt.Errorf("货币写入参数非法: actor=%v type=%d amount=%d", actor, currencyType, amount)
	}
	actor.mu.Lock()
	defer actor.mu.Unlock()
	if actor.Currency == nil {
		actor.Currency = map[int]int{}
	}
	if actor.Currency[currencyType] == amount {
		return nil
	}
	if currencyType == currencyGold || currencyType == currencyYuanBao {
		if actor.currencyStore == nil {
			return fmt.Errorf("金币/元宝存档器未绑定: actor=%d type=%d", actor.ID, currencyType)
		}
		if err := actor.currencyStore.SaveCurrency(actor.ID, currencyType, int64(amount)); err != nil {
			return fmt.Errorf("货币存档失败: actor=%d type=%d amount=%d: %w", actor.ID, currencyType, amount, err)
		}
	}
	actor.Currency[currencyType] = amount
	return nil
}

func realChangeYuanBao(a ...luahost.Value) (luahost.Value, error) {
	if len(a) < 2 {
		return nil, fmt.Errorf("changeYuanBao 缺少角色或数量")
	}
	currencyArgs := []luahost.Value{a[0], currencyYuanBao, a[1]}
	currencyArgs = append(currencyArgs, a[2:]...)
	return realChangeCurrency(currencyArgs...)
}

func realChangeMoney(a ...luahost.Value) (luahost.Value, error) {
	if len(a) < 3 {
		return nil, fmt.Errorf("changeMoney 缺少角色、类型或数量")
	}
	args := []luahost.Value{a[0], a[1], a[2]}
	// 原版调用：changeMoney(actor, mtType, delta, 1, true, tips, ...)。
	// 前三个参数与 changeCurrency 同构；第 6 参是日志 tip，其余业务扩展参数
	// 不参与余额计算，避免把未证实的 phylum/classField 等当成扣款条件。
	reason := ""
	if len(a) > 5 {
		reason = asString(a[5])
	}
	args = append(args, reason)
	return realChangeCurrency(args...)
}

func realGetItemCount(a ...luahost.Value) (luahost.Value, error) {
	if len(a) < 2 {
		return 0, nil
	}
	actor := DefaultActorManager.Get(asActorID(a[0]))
	if actor == nil {
		return 0, nil
	}
	actor.mu.RLock()
	defer actor.mu.RUnlock()
	return actor.Items[asInt(a[1])], nil
}

func realCostItem(a ...luahost.Value) (luahost.Value, error) {
	if len(a) < 3 {
		return false, nil
	}
	actor := DefaultActorManager.Get(asActorID(a[0]))
	if actor == nil {
		return false, nil
	}
	ok, err := actor.CostItem(asInt(a[1]), asInt(a[2]))
	return ok, err
}

func truthy(v luahost.Value) bool {
	switch x := v.(type) {
	case nil:
		return false
	case bool:
		return x
	case int:
		return x != 0
	case int64:
		return x != 0
	case float64:
		return x != 0
	case string:
		return x != ""
	default:
		return true
	}
}

func realCheckItemNum(a ...luahost.Value) (luahost.Value, error) {
	if len(a) < 3 || (len(a) > 3 && truthy(a[3])) {
		return false, nil
	}
	itemID, count := asInt(a[1]), asInt(a[2])
	if itemID <= 0 || count <= 0 {
		return false, nil
	}
	actor := DefaultActorManager.Get(asActorID(a[0]))
	if actor == nil {
		return false, nil
	}
	actor.mu.RLock()
	defer actor.mu.RUnlock()
	return actor.Items[itemID] >= count, nil
}

func realConsumeItem(a ...luahost.Value) (luahost.Value, error) {
	if len(a) < 3 || (len(a) > 3 && truthy(a[3])) {
		return false, nil
	}
	itemID, count := asInt(a[1]), asInt(a[2])
	if itemID <= 0 || count <= 0 {
		return false, nil
	}
	actor := DefaultActorManager.Get(asActorID(a[0]))
	if actor == nil {
		return false, nil
	}
	return actor.CostItem(itemID, count)
}

func realGiveItem(a ...luahost.Value) (luahost.Value, error) {
	if len(a) < 3 {
		return false, nil
	}
	actor := DefaultActorManager.GetOrCreate(asActorID(a[0]))
	ok, err := actor.AddItem(asInt(a[1]), asInt(a[2]))
	return ok, err
}

// getStaticVar(actorId) → Lua table。首次通过宿主创建空表并缓存，
// 后续返回同一表（Lua 表引用语义，业务层写字段即写时标脏）。
func realGetStaticVar(a ...luahost.Value) (luahost.Value, error) {
	if len(a) == 0 {
		return nil, nil
	}
	actor := DefaultActorManager.GetOrCreate(asActorID(a[0]))
	if actor.Static != nil {
		return actor.Static, nil
	}
	L := CurrentInterpreter()
	if L == nil {
		return nil, nil
	}
	v, err := L.DoString("{}")
	if err != nil {
		return nil, err
	}
	actor.Static = v
	actor.Dirty = true
	actor.StaticDirty = true
	return actor.Static, nil
}

// ClientConfigSnapshot 返回客户端配置的 key/val 快照（按 key 升序，稳定发包序）。
func (a *Actor) ClientConfigSnapshot() [][2]int {
	a.mu.RLock()
	defer a.mu.RUnlock()
	keys := make([]int, 0, len(a.ClientConfig))
	for k := range a.ClientConfig {
		keys = append(keys, k)
	}
	sort.Ints(keys)
	out := make([][2]int, 0, len(keys))
	for _, k := range keys {
		out = append(out, [2]int{k, a.ClientConfig[k]})
	}
	return out
}

func (a *Actor) SkillsSnapshot() map[int]int {
	a.mu.RLock()
	defer a.mu.RUnlock()
	skills := make(map[int]int, len(a.Skills))
	for skillID, level := range a.Skills {
		skills[skillID] = level
	}
	return skills
}

// SetClientConfig 更新单个客户端配置项（对应 clientconfig.lua onUpdateConfig）。
func (a *Actor) SetClientConfig(key, val int) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.ClientConfig == nil {
		a.ClientConfig = map[int]int{}
	}
	a.ClientConfig[key] = val
	a.Dirty = true
	a.ClientConfigDirty = true
}
