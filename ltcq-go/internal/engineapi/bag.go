package engineapi

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"log/slog"
	"math"
	"sort"
)

const (
	cmdBag              = 3
	sBagCmdInitData     = 1
	sBagCmdDeleteItem   = 3
	sBagCmdAddItem      = 4
	sBagCmdUpdateItem   = 5
	sBagCmdUseItem      = 6
	bagTypeOther        = 0
	bagItemBytes        = 144
	bagBlobMagic        = "LTGOBAG1"
	bagUseResultInvalid = 5
)

type BagStore interface {
	LoadBagBlob(actorID int) ([]byte, error)
	SaveBagBlob(actorID int, blob []byte) error
}

func (s *MySQLAccountStore) LoadBagBlob(actorID int) ([]byte, error) {
	var blob []byte
	if err := s.db.QueryRow("SELECT variable FROM actorvariable WHERE actorid=?", actorID).Scan(&blob); err != nil {
		return nil, err
	}
	return blob, nil
}

func (s *MySQLAccountStore) SaveBagBlob(actorID int, blob []byte) error {
	res, err := s.db.Exec("UPDATE actorvariable SET variable=? WHERE actorid=?", blob, actorID)
	if err != nil {
		return err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		var exists int
		if err := s.db.QueryRow("SELECT 1 FROM actorvariable WHERE actorid=?", actorID).Scan(&exists); err != nil {
			return err
		}
		if exists != 1 {
			return fmt.Errorf("背包存档未命中: actor=%d", actorID)
		}
	}
	return nil
}

func decodeBagBlob(blob []byte) (map[int]int, []byte, error) {
	items := map[int]int{}
	start := bytes.LastIndex(blob, []byte(bagBlobMagic))
	if start < 0 {
		return items, append([]byte(nil), blob...), nil
	}
	if len(blob)-start < 16 {
		return nil, nil, fmt.Errorf("背包存档尾部过短")
	}
	payloadLen := int(binary.LittleEndian.Uint32(blob[start+8 : start+12]))
	crcAt := start + 12 + payloadLen
	if payloadLen < 4 || crcAt+4 != len(blob) {
		return nil, nil, fmt.Errorf("背包存档长度非法")
	}
	wantCRC := binary.LittleEndian.Uint32(blob[crcAt:])
	if got := crc32.ChecksumIEEE(blob[start:crcAt]); got != wantCRC {
		return nil, nil, fmt.Errorf("背包存档校验失败")
	}
	payload := blob[start+12 : crcAt]
	count := int(binary.LittleEndian.Uint32(payload[:4]))
	if count > (len(payload)-4)/8 || (len(payload)-4)%8 != 0 {
		return nil, nil, fmt.Errorf("背包存档条目数非法")
	}
	off := 4
	for range count {
		itemID := int(int32(binary.LittleEndian.Uint32(payload[off : off+4])))
		itemCount := int(int32(binary.LittleEndian.Uint32(payload[off+4 : off+8])))
		if itemID <= 0 || itemCount <= 0 {
			return nil, nil, fmt.Errorf("背包存档条目非法: item=%d count=%d", itemID, itemCount)
		}
		if _, exists := items[itemID]; exists {
			return nil, nil, fmt.Errorf("背包存档物品重复: item=%d", itemID)
		}
		items[itemID] = itemCount
		off += 8
	}
	return items, append([]byte(nil), blob[:start]...), nil
}

func encodeBagBlob(base []byte, items map[int]int) ([]byte, error) {
	ids := make([]int, 0, len(items))
	for itemID, count := range items {
		if itemID <= 0 || itemID > math.MaxInt32 || count <= 0 || count > math.MaxInt32 {
			return nil, fmt.Errorf("背包条目非法: item=%d count=%d", itemID, count)
		}
		ids = append(ids, itemID)
	}
	sort.Ints(ids)
	payload := binary.LittleEndian.AppendUint32(nil, uint32(len(ids)))
	for _, itemID := range ids {
		payload = binary.LittleEndian.AppendUint32(payload, uint32(itemID))
		payload = binary.LittleEndian.AppendUint32(payload, uint32(items[itemID]))
	}
	blob := append([]byte(nil), base...)
	blob = append(blob, bagBlobMagic...)
	blob = binary.LittleEndian.AppendUint32(blob, uint32(len(payload)))
	blob = append(blob, payload...)
	return binary.LittleEndian.AppendUint32(blob, crc32.ChecksumIEEE(blob[len(base):])), nil
}

func (a *Actor) ReplaceBag(items map[int]int, base []byte, store BagStore) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.Items = make(map[int]int, len(items))
	for itemID, count := range items {
		a.Items[itemID] = count
	}
	a.bagBase = append([]byte(nil), base...)
	a.bagStore = store
}

func (a *Actor) SetBagNotifier(notify func([]byte) error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.bagNotify = notify
}

func (a *Actor) BagSnapshot() map[int]int {
	a.mu.RLock()
	defer a.mu.RUnlock()
	items := make(map[int]int, len(a.Items))
	for itemID, count := range a.Items {
		items[itemID] = count
	}
	return items
}

func (a *Actor) AddItem(itemID, count int) (bool, error) {
	if itemID <= 0 || count <= 0 {
		return false, nil
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	old := a.Items[itemID]
	if old < 0 || count > int(^uint(0)>>1)-old {
		return false, fmt.Errorf("物品数量溢出: item=%d count=%d", itemID, count)
	}
	// 容量检查（BagBaseConfig.baseSize=200 语义：不同物品种类数）；已叠加物品不占新槽。
	// 装备物品（bagTab=1）走独立装备背包容量，待装备容量证据后启用。
	var isEquip bool
	if a.itemResolver != nil {
		if info, ok := a.itemResolver(itemID); ok {
			isEquip = info.BagTab == 1
		}
	}
	if old == 0 && !isEquip && a.bagCapacity > 0 && len(a.Items)+1 > a.bagCapacity {
		return false, fmt.Errorf("背包已满: %d/%d", len(a.Items), a.bagCapacity)
	}
	next := make(map[int]int, len(a.Items)+1)
	for id, value := range a.Items {
		next[id] = value
	}
	next[itemID] = old + count
	if err := a.saveBagLocked(next); err != nil {
		return false, err
	}
	a.Items = next
	packet := buildBagAddItem(a.ID, itemID, next[itemID])
	if old > 0 {
		packet = buildBagUpdateItem(a.ID, itemID, next[itemID])
	}
	a.notifyBagLocked(packet)
	return true, nil
}

func (a *Actor) CostItem(itemID, count int) (bool, error) {
	if itemID <= 0 || count <= 0 {
		return false, nil
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	old := a.Items[itemID]
	if old < count {
		return false, nil
	}
	next := make(map[int]int, len(a.Items))
	for id, value := range a.Items {
		next[id] = value
	}
	if nextCount := old - count; nextCount > 0 {
		next[itemID] = nextCount
	} else {
		delete(next, itemID)
	}
	if err := a.saveBagLocked(next); err != nil {
		return false, err
	}
	a.Items = next
	if next[itemID] > 0 {
		a.notifyBagLocked(buildBagUpdateItem(a.ID, itemID, next[itemID]))
	} else {
		a.notifyBagLocked(buildBagDeleteItem(a.ID, itemID))
	}
	return true, nil
}

func (a *Actor) saveBagLocked(items map[int]int) error {
	if a.bagStore == nil {
		return nil
	}
	blob, err := encodeBagBlob(a.bagBase, items)
	if err != nil {
		return err
	}
	return a.bagStore.SaveBagBlob(a.ID, blob)
}

func (a *Actor) notifyBagLocked(packet []byte) {
	if a.bagNotify == nil {
		return
	}
	if err := a.bagNotify(packet); err != nil {
		slog.Warn("背包变更通知失败", "actor", a.ID, "err", err)
	}
}

func bagItemHandle(actorID, itemID int) uint64 {
	return uint64(uint32(itemID))<<32 | uint64(uint32(actorID))
}

func appendBagItem(p []byte, actorID, itemID, count int) []byte {
	p = binary.LittleEndian.AppendUint64(p, math.Float64bits(math.Float64frombits(bagItemHandle(actorID, itemID))))
	p = binary.LittleEndian.AppendUint32(p, uint32(itemID))
	p = binary.LittleEndian.AppendUint32(p, uint32(count))
	for range 16 {
		p = binary.LittleEndian.AppendUint32(p, 0)
		p = binary.LittleEndian.AppendUint32(p, 0)
	}
	return p
}

func buildBagAddItem(actorID, itemID, count int) []byte {
	p := []byte{cmdBag, sBagCmdAddItem, bagTypeOther}
	p = appendBagItem(p, actorID, itemID, count)
	return append(p, 0)
}

func buildBagUpdateItem(actorID, itemID, count int) []byte {
	p := []byte{cmdBag, sBagCmdUpdateItem, bagTypeOther}
	p = binary.LittleEndian.AppendUint64(p, math.Float64bits(math.Float64frombits(bagItemHandle(actorID, itemID))))
	p = binary.LittleEndian.AppendUint32(p, uint32(count))
	return append(p, 0)
}

func buildBagDeleteItem(actorID, itemID int) []byte {
	p := []byte{cmdBag, sBagCmdDeleteItem, bagTypeOther}
	return binary.LittleEndian.AppendUint64(p, math.Float64bits(math.Float64frombits(bagItemHandle(actorID, itemID))))
}

func buildBagInitPackets(actorID int, items map[int]int) [][]byte {
	ids := make([]int, 0, len(items))
	for itemID := range items {
		ids = append(ids, itemID)
	}
	sort.Ints(ids)
	packets := make([][]byte, 0, 3)
	for bagType := 0; bagType <= 2; bagType++ {
		p := []byte{cmdBag, sBagCmdInitData, 1, byte(bagType)}
		count := 0
		if bagType == bagTypeOther {
			count = len(ids)
		}
		p = binary.LittleEndian.AppendUint32(p, uint32(count))
		if bagType == bagTypeOther {
			for _, itemID := range ids {
				p = appendBagItem(p, actorID, itemID, items[itemID])
			}
		}
		packets = append(packets, p)
	}
	return packets
}

func buildBagUseResult(ok bool) []byte {
	result := byte(bagUseResultInvalid)
	if ok {
		result = 0
	}
	return []byte{cmdBag, sBagCmdUseItem, result}
}

func newBagUseHandler(router *NetRouter) func(uint16, int, []byte) error {
	return func(sid uint16, actorID int, body []byte) error {
		if actorID <= 0 || len(body) < 8 {
			return fmt.Errorf("背包使用请求非法: actor=%d len=%d", actorID, len(body))
		}
		itemID := int(int32(binary.LittleEndian.Uint32(body[:4])))
		count := int(int32(binary.LittleEndian.Uint32(body[4:8])))
		actor := DefaultActorManager.Get(actorID)
		if actor == nil {
			return fmt.Errorf("背包角色不存在: %d", actorID)
		}
		ok, err := actor.CostItem(itemID, count)
		if err != nil {
			return err
		}
		return router.SendToSession(sid, buildBagUseResult(ok))
	}
}

// SetBagCapacity 设置普通背包容量（BagBaseConfig.baseSize=200；0=不限）。
func (a *Actor) SetBagCapacity(n int) {
	a.mu.Lock()
	a.bagCapacity = n
	a.mu.Unlock()
}

// SetItemResolver 注入物品判定（BagTab 路由；由引擎 Bootstrap 时接 VM 配置）。
func (a *Actor) SetItemResolver(r func(int) (ItemInfo, bool)) {
	a.mu.Lock()
	a.itemResolver = r
	a.mu.Unlock()
}
