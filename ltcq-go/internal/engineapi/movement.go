package engineapi

import (
	"encoding/binary"
	"fmt"
	"log/slog"
	"math"

	"github.com/xiaokentrl/ChuanQi/ltcq-go/internal/scene"
)

type MovementManager struct {
	router *NetRouter
	World  *scene.World
}

func NewMovementManager(router *NetRouter, worlds ...*scene.World) *MovementManager {
	manager := &MovementManager{router: router}
	if len(worlds) > 0 {
		manager.World = worlds[0]
	}
	return manager
}

func (m *MovementManager) HandleMove(sid uint16, actorID int, body []byte) error {
	sess := m.router.bySIDGet(sid)
	if sess == nil || !sess.Authenticated || sess.ActorID != actorID || actorID == 0 {
		return fmt.Errorf("移动会话未绑定: sid=%d actor=%d", sid, actorID)
	}
	if len(body) < 12 {
		return fmt.Errorf("移动包过短: %d", len(body))
	}
	if handle := math.Float64frombits(binary.LittleEndian.Uint64(body[:8])); handle != float64(actorID) {
		return fmt.Errorf("移动句柄不匹配: handle=%g actor=%d", handle, actorID)
	}
	count := int(int32(binary.LittleEndian.Uint32(body[8:12])))
	if count < 1 || count > 256 || len(body) != 12+count*8 {
		return fmt.Errorf("移动路径非法: count=%d len=%d", count, len(body))
	}

	actor := DefaultActorManager.Get(actorID)
	if actor == nil {
		return fmt.Errorf("角色不存在: %d", actorID)
	}
	actor.mu.RLock()
	sceneID := actor.SceneID
	hp := actor.HP
	actor.mu.RUnlock()
	if hp <= 0 {
		return fmt.Errorf("玩家已死亡，不能移动: %d", actorID)
	}
	_, _, _, width, height, _, _ := sceneSpawn(sceneID)
	if m.World != nil {
		cfg, err := m.World.Spawn(sceneID)
		if err != nil {
			return err
		}
		width, height = cfg.Width, cfg.Height
	}
	actor.mu.RLock()
	prevX, prevY := actor.X, actor.Y
	actor.mu.RUnlock()
	distance := 0
	x, y := prevX, prevY
	for i := 0; i < count; i++ {
		off := 12 + i*8
		px := int(int32(binary.LittleEndian.Uint32(body[off : off+4])))
		py := int(int32(binary.LittleEndian.Uint32(body[off+4 : off+8])))
		if px < 0 || py < 0 || px >= width || py >= height {
			return fmt.Errorf("移动坐标越界: (%d,%d) scene=%d", px, py, sceneID)
		}
		distance += absInt(px-x) + absInt(py-y)
		x, y = px, py
	}
	actor.mu.Lock()
	if actor.HP <= 0 {
		actor.mu.Unlock()
		return fmt.Errorf("玩家已死亡，不能移动: %d", actorID)
	}
	actor.X, actor.Y = x, y
	var deliveries []scene.Delivery
	if m.World != nil {
		var err error
		deliveries, err = m.World.MovePlayer(actorID, x, y)
		if err != nil {
			actor.X, actor.Y = prevX, prevY
			actor.mu.Unlock()
			slog.Warn("移动 AOI 更新失败", "actor", actorID, "err", err)
			return nil
		}
	}
	actor.mu.Unlock()
	mode := uint16(1)
	duration := distance / 4
	if duration <= 0 {
		mode, duration = 0, 0
	} else if duration > 3000 {
		duration = 3000
	}
	packet := buildEntitySyncPosition(float64(actorID), x, y, mode, duration)
	sendErr := m.router.SendToSession(sid, packet)
	if m.World != nil {
		if err := m.World.Dispatch(deliveries); err != nil {
			slog.Warn("移动 AOI 广播失败", "actor", actorID, "err", err)
		}
	}
	return sendErr
}

func buildEntitySyncPosition(handle float64, x, y int, mode uint16, duration int) []byte {
	p := []byte{cmdBase, sBaseCmdEntitySyncPos}
	p = binary.LittleEndian.AppendUint64(p, math.Float64bits(handle))
	p = binary.LittleEndian.AppendUint16(p, mode)
	p = binary.LittleEndian.AppendUint32(p, uint32(x))
	p = binary.LittleEndian.AppendUint32(p, uint32(y))
	if mode == 1 {
		p = binary.LittleEndian.AppendUint32(p, uint32(duration))
	}
	return p
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
