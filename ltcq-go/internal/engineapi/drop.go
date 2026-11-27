package engineapi

import (
	"encoding/binary"
	"fmt"
	"log/slog"
	"sort"
	"sync"
	"time"

	"github.com/xiaokentrl/ChuanQi/ltcq-go/internal/scene"
)

// 掉落拾取链路（A2，golden 依据 RUNBOOK 五十五/五十六/五十七）：
// 击杀 → OtherEntity 下发（type=6 地面掉落）→ 客户端 UserFb.sendGetReward 拾取上行 → 消失包 + BagAdd。
const (
	// 拾取上行（golden 实测，五十五）：客户端不回传掉落 handle，只发
	// [sysid=1 CMD_Fuben/Guanqia][cmdid=1][flag]，普通地图 flag=fubenID=0、自动寻路贴近后多次发出；
	// 服务端按距离判定归属并结算（golden 一次触发伴随多帧消失包）。
	cmdFuben           = 1 // CMD_Fuben/Guanqia（_G.lua SystemId）
	sFubenCmdGetReward = 1 // UserFb.sendGetReward
	entityTypeDropItem = 6 // EntityType.DropItem（A2 逆向）
	dropRewardTypeItem = 1 // RewardData.type：物品
)

// dropPickupRadius 拾取判定半径：客户端贴掉落物后发 sendGetReward（flag=0），
// 服务端按曼哈顿距离归属，取 2 格。
const dropPickupRadius = 2 * scene.CellSize

// DropBag 一份地面掉落（A2 布局：handle/configID/masterHandle/x/y/RewardData）。
type DropBag struct {
	Handle       uint64 // 不透明句柄（f64 位模式，客户端按不透明值回传）
	ConfigID     uint32 // 掉落物 itemID（客户端显示用）
	MasterHandle uint64 // 归属玩家句柄
	X, Y         int
	SceneID      int
	ItemID       int
	Count        int
	ExpireAt     time.Time
}

func (d *DropBag) String() string {
	return fmt.Sprintf("drop{handle=%x item=%d count=%d @(%d,%d) scene=%d}", d.Handle, d.ItemID, d.Count, d.X, d.Y, d.SceneID)
}

// DropManager 管理全部未拾取掉落。
type DropManager struct {
	mu    sync.Mutex
	drops map[uint64]*DropBag
	next  uint64
}

func NewDropManager() *DropManager {
	return &DropManager{drops: map[uint64]*DropBag{}, next: 1}
}

// CreateDrop 在击杀位置创建地面掉落，返回句柄与 OtherEntity 下行帧。
func (m *DropManager) CreateDrop(sceneID, itemID, count, x, y int, master uint64) (uint64, []byte) {
	m.mu.Lock()
	defer m.mu.Unlock()
	h := m.next
	m.next++
	d := &DropBag{
		Handle:       handle64(h),
		ConfigID:     uint32(itemID),
		MasterHandle: master,
		X:            x,
		Y:            y,
		SceneID:      sceneID,
		ItemID:       itemID,
		Count:        count,
		ExpireAt:     time.Now().Add(60 * time.Second),
	}
	// map 键必须与 Handle 字段一致（handle64 位模式），否则按句柄拾取永远 miss。
	m.drops[d.Handle] = d
	return h, buildOtherEntityFrame(d)
}

// NearDrops 返回拾取点周围半径内的未过期掉落（近先序），供拾取上行结算。
func (m *DropManager) NearDrops(sceneID, x, y, radius int) []*DropBag {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()
	var out []*DropBag
	for _, d := range m.drops {
		if d.SceneID != sceneID || !now.Before(d.ExpireAt) {
			continue
		}
		if absInt(d.X-x)+absInt(d.Y-y) <= radius {
			out = append(out, d)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return absInt(out[i].X-x)+absInt(out[i].Y-y) < absInt(out[j].X-x)+absInt(out[j].Y-y)
	})
	return out
}

// Take 移除并返回指定掉落（拾取入包成功后调用）。
func (m *DropManager) Take(handle uint64) (*DropBag, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	d, ok := m.drops[handle]
	if !ok {
		return nil, false
	}
	delete(m.drops, handle)
	return d, true
}

// SweepExpired 移除并返回已过期掉落（清扫方负责广播消失包）。
func (m *DropManager) SweepExpired(now time.Time) []*DropBag {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []*DropBag
	for h, d := range m.drops {
		if !now.Before(d.ExpireAt) {
			out = append(out, d)
			delete(m.drops, h)
		}
	}
	return out
}

// RemovePacket 构造实体消失包（[00][0a] + handle 8B，golden 五十五：掉落物消失下行）。
func (m *DropManager) RemovePacket(handle uint64) []byte {
	p := []byte{cmdBase, sBaseCmdRemoveEntity}
	p = binary.LittleEndian.AppendUint64(p, handle)
	return p
}

// handle64 构造不透明掉落句柄（低 4B=索引，次 4B=栈标记；f64 位模式）。
func handle64(idx uint64) uint64 {
	return idx<<32 | 0x00010000
}

// buildOtherEntityFrame 构造 sBaseCmd_OtherEntity(0,4) 下行：
// [u16 count][count × {type:u32=6, f64 handle, u32 configID, f64 masterHandle, u32 x, u32 y, RewardData{type,id,count}}]。
func buildOtherEntityFrame(d *DropBag) []byte {
	p := binary.LittleEndian.AppendUint16(nil, 1) // count=1
	p = binary.LittleEndian.AppendUint32(p, entityTypeDropItem)
	p = binary.LittleEndian.AppendUint64(p, d.Handle)
	p = binary.LittleEndian.AppendUint32(p, d.ConfigID)
	p = binary.LittleEndian.AppendUint64(p, d.MasterHandle)
	p = binary.LittleEndian.AppendUint32(p, uint32(d.X))
	p = binary.LittleEndian.AppendUint32(p, uint32(d.Y))
	p = binary.LittleEndian.AppendUint32(p, dropRewardTypeItem)
	p = binary.LittleEndian.AppendUint32(p, uint32(d.ItemID))
	p = binary.LittleEndian.AppendUint32(p, uint32(d.Count))
	return append([]byte{cmdBase, sBaseCmdOtherEntity}, p...)
}

// newDropPickupHandler 拾取上行处理器（[01][01] flag=0，UserFb.sendGetReward）：
// 半径内掉落逐个结算——AddItem（BagAdd golden 布局经 BagNotifier 下发）成功后
// 移除掉落并广播消失包；入包失败（背包满等）时掉落留在地面等下次拾取。
func newDropPickupHandler(combat *CombatManager, world *scene.World) func(sid uint16, actorID int, body []byte) error {
	return func(_ uint16, actorID int, body []byte) error {
		if combat == nil || combat.drops == nil || world == nil || len(body) == 0 || body[0] != 0 {
			return nil
		}
		actor := DefaultActorManager.Get(actorID)
		if actor == nil {
			return nil
		}
		entity, ok := world.Entity(actorID)
		if !ok || entity.HP <= 0 {
			return nil
		}
		for _, d := range combat.drops.NearDrops(entity.SceneID, entity.X, entity.Y, dropPickupRadius) {
			got, err := actor.AddItem(d.ItemID, d.Count)
			if err != nil {
				slog.Warn("拾取入包失败", "actor", actorID, "drop", d, "err", err)
				continue
			}
			if !got {
				continue
			}
			if taken, ok := combat.drops.Take(d.Handle); ok {
				combat.deliverToScene(taken.SceneID, combat.drops.RemovePacket(taken.Handle))
			}
		}
		return nil
	}
}
