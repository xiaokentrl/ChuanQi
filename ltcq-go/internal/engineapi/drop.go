package engineapi

import (
	"encoding/binary"
	"fmt"
	"sync"
	"time"
)

// 掉落拾取链路（A2，协议依据 RUNBOOK 五十七/golden 三类抓包）：
// 击杀 → OtherEntity 下发（type=6 地面掉落）→ 客户端拾取上行 → RemoveEntity + BagAdd。
const (
	sBaseCmdGetReward    = 1  // 拾取上行（客户端 [00][01][flag]）
	entityTypeDropItem   = 6  // EntityType.DropItem（A2 逆向）
	dropRewardTypeItem   = 1  // RewardData.type：物品
)

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
	m.drops[h] = d
	return h, buildOtherEntityFrame(d)
}

// Pickup 处理拾取上行：移除掉落并返回物品数据（供 BagAdd）。
func (m *DropManager) Pickup(handle uint64) (itemID, count, sceneID int, ok bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	d, ok := m.drops[handle]
	if !ok {
		return 0, 0, 0, false
	}
	itemID, count, sceneID = d.ItemID, d.Count, d.SceneID
	delete(m.drops, handle)
	return itemID, count, sceneID, true
}

// RemovePacket 构造实体消失包（[00][0a] + handle 8B）。
func (m *DropManager) RemovePacket(handle uint64) []byte {
	p := []byte{cmdBase, sBaseCmdRemoveEntity}
	p = binary.LittleEndian.AppendUint64(p, handle)
	return p
}

// Pending 返回场景内未过期掉落（OtherEntity 重发用）。
func (m *DropManager) Pending(sceneID int) []*DropBag {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()
	var out []*DropBag
	for _, d := range m.drops {
		if d.SceneID == sceneID && now.Before(d.ExpireAt) {
			out = append(out, d)
		}
	}
	return out
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

// buildDropRemoveFrame 构造实体消失包 [00][0a] + handle 8B。
func buildDropRemoveFrame(handle uint64) []byte {
	p := []byte{cmdBase, sBaseCmdRemoveEntity}
	p = binary.LittleEndian.AppendUint64(p, handle)
	return p
}
