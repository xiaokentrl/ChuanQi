package engineapi

import (
	"sync"

	"github.com/xiaokentrl/ChuanQi/ltcq-go/internal/luahost"
)

// actorstore.go：内存玩家实体（阶段4.6 骨架）。
// 完整领域逻辑（背包/属性/场景AOI）属后续工作；此处实现 T0 验证所需的核心：
// id/level/name/currency/items/staticVar + 脏标记，让 LActor API 从 stub 变真实读写。

// Actor 是内存中的玩家实体。
type Actor struct {
	ID       int
	Level    int
	Name     string
	Currency map[int]int   // NumericType → 数量
	Items    map[int]int   // itemId → 数量
	Static   luahost.Value // staticVar（Lua 表，宿主无关 Value；持久化时序列化）
	Dirty    bool          // staticVar 脏标记（写时标脏）
}

func newActor(id int) *Actor {
	return &Actor{
		ID:       id,
		Level:    1,
		Name:     "",
		Currency: map[int]int{},
		Items:    map[int]int{},
		Dirty:    false,
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

// DefaultActorManager 全局默认管理器（engineapi 包级使用）。
var DefaultActorManager = NewActorManager()

// 以下为真实 LActor API 实现（替换 actor.go 中的 stub）。

func realGetActorId(a ...luahost.Value) (luahost.Value, error) {
	if len(a) == 0 {
		return 0, nil
	}
	// actor 句柄即 id（骨架）；未来实体指针时改为从句柄查
	return asActorID(a[0]), nil
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
	return actor.Currency[asInt(a[1])], nil
}

func realChangeCurrency(a ...luahost.Value) (luahost.Value, error) {
	if len(a) < 3 {
		return nil, nil
	}
	actor := DefaultActorManager.GetOrCreate(asActorID(a[0]))
	typ := asInt(a[1])
	delta := asInt(a[2])
	actor.Currency[typ] += delta
	return nil, nil
}

func realGetItemCount(a ...luahost.Value) (luahost.Value, error) {
	if len(a) < 2 {
		return 0, nil
	}
	actor := DefaultActorManager.Get(asActorID(a[0]))
	if actor == nil {
		return 0, nil
	}
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
	itemID := asInt(a[1])
	count := asInt(a[2])
	if actor.Items[itemID] < count {
		return false, nil // 不足不扣
	}
	actor.Items[itemID] -= count
	return true, nil
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
	return actor.Static, nil
}
