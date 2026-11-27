package engineapi

import (
	"os"
	"bytes"
	"encoding/binary"
	"errors"
	"math"
	"reflect"
	"testing"

	"github.com/xiaokentrl/ChuanQi/ltcq-go/internal/luahost"
	"github.com/xiaokentrl/ChuanQi/ltcq-go/internal/netframe"
)

type memoryBagStore struct {
	blob  []byte
	fail  bool
	saves int
}

func (s *memoryBagStore) LoadBagBlob(int) ([]byte, error) {
	return append([]byte(nil), s.blob...), nil
}

func (s *memoryBagStore) SaveBagBlob(_ int, blob []byte) error {
	s.saves++
	if s.fail {
		return errors.New("存档失败")
	}
	s.blob = append([]byte(nil), blob...)
	return nil
}

func TestBagBlobRoundTripPreservesUnknownPrefix(t *testing.T) {
	prefix := append([]byte{0xaa, 0xbb}, []byte(bagBlobMagic)...)
	want := map[int]int{1002: 1, 1001: 3}
	blob, err := encodeBagBlob(prefix, want)
	if err != nil {
		t.Fatal(err)
	}
	got, base, err := decodeBagBlob(blob)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(base, prefix) || !reflect.DeepEqual(got, want) {
		t.Fatalf("解码结果=%v base=%x", got, base)
	}
	again, err := encodeBagBlob(base, got)
	if err != nil || !bytes.Equal(again, blob) {
		t.Fatalf("背包存档不稳定: %x %v", again, err)
	}

	blob[len(blob)-1] ^= 0xff
	if _, _, err := decodeBagBlob(blob); err == nil {
		t.Fatal("损坏的背包校验和应拒绝加载")
	}
}

func TestActorItemChangesPersistAndNotifyAtomically(t *testing.T) {
	store := &memoryBagStore{}
	actor := newActor(42)
	actor.ReplaceBag(nil, []byte("staticVar"), store)
	var packets [][]byte
	actor.SetBagNotifier(func(packet []byte) error {
		packets = append(packets, append([]byte(nil), packet...))
		return nil
	})

	steps := []struct {
		add   bool
		count int
		cmd   byte
	}{
		{add: true, count: 2, cmd: sBagCmdAddItem},
		{add: true, count: 1, cmd: sBagCmdUpdateItem},
		{add: false, count: 1, cmd: sBagCmdUpdateItem},
		{add: false, count: 2, cmd: sBagCmdDeleteItem},
	}
	for _, step := range steps {
		var ok bool
		var err error
		if step.add {
			ok, err = actor.AddItem(1001, step.count)
		} else {
			ok, err = actor.CostItem(1001, step.count)
		}
		if err != nil || !ok {
			t.Fatalf("物品变更失败: ok=%v err=%v", ok, err)
		}
		if got := packets[len(packets)-1]; len(got) < 2 || got[0] != cmdBag || got[1] != step.cmd {
			t.Fatalf("背包通知错误: %x", got)
		}
	}
	if store.saves != len(steps) || len(actor.Items) != 0 {
		t.Fatalf("存档或内存状态错误: saves=%d items=%v", store.saves, actor.Items)
	}
	items, base, err := decodeBagBlob(store.blob)
	if err != nil || len(items) != 0 || !bytes.Equal(base, []byte("staticVar")) {
		t.Fatalf("持久化内容错误: items=%v base=%q err=%v", items, base, err)
	}

	beforePackets := len(packets)
	if ok, err := actor.CostItem(1001, 1); err != nil || ok {
		t.Fatalf("不足扣减应失败: ok=%v err=%v", ok, err)
	}
	if ok, err := actor.AddItem(1001, -1); err != nil || ok {
		t.Fatalf("负数发放应失败: ok=%v err=%v", ok, err)
	}
	if ok, err := actor.CostItem(1001, 0); err != nil || ok {
		t.Fatalf("零扣减应失败: ok=%v err=%v", ok, err)
	}
	if len(packets) != beforePackets || store.saves != len(steps) {
		t.Fatal("失败请求不应通知或存档")
	}

	store.fail = true
	if ok, err := actor.AddItem(1001, 5); err == nil || ok {
		t.Fatalf("存档失败应返回错误: ok=%v err=%v", ok, err)
	}
	if len(actor.Items) != 0 || len(packets) != beforePackets {
		t.Fatal("存档失败不应修改内存或发送通知")
	}
}

func TestBagPacketLayouts(t *testing.T) {
	const actorID, itemID, count = 65537, 31001, 3
	init := buildBagInitPackets(actorID, map[int]int{itemID: count})
	if len(init) != 3 || len(init[0]) != 8+bagItemBytes || len(init[1]) != 8 || len(init[2]) != 8 {
		t.Fatalf("背包初始化长度错误: %d %d %d", len(init[0]), len(init[1]), len(init[2]))
	}
	for bagType, packet := range init {
		if packet[0] != cmdBag || packet[1] != sBagCmdInitData || packet[2] != 1 || packet[3] != byte(bagType) {
			t.Fatalf("背包初始化头错误: %x", packet)
		}
	}
	item := init[0][8:]
	if got := math.Float64frombits(binary.LittleEndian.Uint64(item[:8])); math.Float64bits(got) != bagItemHandle(actorID, itemID) {
		t.Fatalf("物品句柄错误: %x", item[:8])
	}
	if binary.LittleEndian.Uint32(item[8:12]) != itemID || binary.LittleEndian.Uint32(item[12:16]) != count {
		t.Fatalf("物品字段错误: %x", item[:16])
	}

	add := buildBagAddItem(actorID, itemID, count)
	update := buildBagUpdateItem(actorID, itemID, count-1)
	remove := buildBagDeleteItem(actorID, itemID)
	if len(add) != 148 || len(update) != 16 || len(remove) != 11 {
		t.Fatalf("物品变更包长度错误: add=%d update=%d delete=%d", len(add), len(update), len(remove))
	}
	if add[2] != bagTypeOther || update[2] != bagTypeOther || remove[2] != bagTypeOther {
		t.Fatal("物品变更包 bagType 错误")
	}
}

func TestBagUseRouteConsumesItemAndRepliesResult(t *testing.T) {
	previousActors := DefaultActorManager
	DefaultActorManager = NewActorManager()
	t.Cleanup(func() { DefaultActorManager = previousActors; SetSendHook(nil) })

	actor := DefaultActorManager.GetOrCreate(65537)
	actor.ReplaceBag(map[int]int{31001: 2}, nil, nil)
	var packets [][]byte
	router := NewNetRouter(nil)
	router.SetSender(func(frame []byte) error {
		packets = append(packets, append([]byte(nil), frame[netframe.GateFrameHeaderLen+netframe.NetIDLen:]...))
		return nil
	})
	router.OpenSession(1, "127.0.0.1")
	router.BindActor(1, 65537)
	actor.SetBagNotifier(func(packet []byte) error { return router.SendToSession(1, packet) })
	router.SetBagHandler(newBagUseHandler(router))
	body := binary.LittleEndian.AppendUint32(nil, 31001)
	body = binary.LittleEndian.AppendUint32(body, 1)
	if err := router.RouteUpstream(clientBusinessBody(1, cmdBag, sBagCmdUseItem, body)); err != nil {
		t.Fatal(err)
	}
	if actor.Items[31001] != 1 || len(packets) != 2 || !bytes.Equal(packets[0][:2], []byte{cmdBag, sBagCmdUpdateItem}) || !bytes.Equal(packets[1], []byte{cmdBag, sBagCmdUseItem, 0}) {
		t.Fatalf("使用物品结果错误: items=%v packets=%x", actor.Items, packets)
	}
	if err := router.RouteUpstream(clientBusinessBody(1, cmdBag, sBagCmdUseItem, body)); err != nil {
		t.Fatal(err)
	}
	if actor.Items[31001] != 0 || len(packets) != 4 || !bytes.Equal(packets[2][:2], []byte{cmdBag, sBagCmdDeleteItem}) || !bytes.Equal(packets[3], []byte{cmdBag, sBagCmdUseItem, 0}) {
		t.Fatalf("扣至零结果错误: items=%v packets=%x", actor.Items, packets)
	}
	if err := router.RouteUpstream(clientBusinessBody(1, cmdBag, sBagCmdUseItem, body)); err != nil {
		t.Fatal(err)
	}
	if len(packets) != 5 || !bytes.Equal(packets[4], []byte{cmdBag, sBagCmdUseItem, bagUseResultInvalid}) {
		t.Fatalf("数量不足结果错误: %x", packets)
	}
}

func TestLuaGiveAndCostItem(t *testing.T) {
	DefaultActorManager = NewActorManager()
	actor := DefaultActorManager.GetOrCreate(7)
	actor.ReplaceBag(nil, nil, nil)
	g := luahost.NewGopherLua(luahost.Config{OSType: "WIN"})
	defer g.Close()
	if err := InstallAll(g); err != nil {
		t.Fatal(err)
	}
	for _, call := range []struct {
		name string
		args []int
	}{
		{name: "LActor.giveItem", args: []int{7, 1001, 2}},
		{name: "LActor.giveItem", args: []int{7, 1001, 3}},
		{name: "LActor.costItem", args: []int{7, 1001, 4}},
	} {
		values := make([]luahost.Value, len(call.args))
		for i, arg := range call.args {
			values[i] = arg
		}
		got, err := g.Call(call.name, values...)
		if err != nil || got != true {
			t.Fatalf("%s=%v err=%v", call.name, got, err)
		}
	}
	got, err := g.Call("LActor.getItemCount", 7, 1001)
	if err != nil || got != 1 {
		t.Fatalf("剩余物品=%v err=%v", got, err)
	}
}

func TestBagCapacity(t *testing.T) {
	a := newActor(1)
	a.SetBagCapacity(2)
	a.SetBagCapacity(2)
	resolverCalls := 0
	a.SetItemResolver(func(id int) (ItemInfo, bool) {
		resolverCalls++
		if id == 101001 {
			return ItemInfo{ID: id, BagTab: 1}, true // 装备走独立容量
		}
		return ItemInfo{ID: id, BagTab: 0}, true
	})

	ok, _ := a.AddItem(1, 1) // 普通槽 1/2
	if !ok {
		t.Fatal("普通物品 1 应加入")
	}
	ok, _ = a.AddItem(2, 1) // 普通槽 2/2
	if !ok {
		t.Fatal("普通物品 2 应加入")
	}
	ok, err := a.AddItem(3, 1) // 普通槽满 → 拒绝
	if ok || err == nil {
		t.Fatal("背包已满应拒绝")
	}
	ok, _ = a.AddItem(1, 1) // 已有叠加不占新槽
	if !ok {
		t.Fatal("叠加应通过")
	}
	ok, _ = a.AddItem(101001, 1) // 装备绕过普通容量
	if !ok {
		t.Fatal("装备应绕过普通容量")
	}
	if resolverCalls < 3 {
		t.Fatalf("resolver 调用过少: %d", resolverCalls)
	}
	if len(a.Items) != 3 {
		t.Fatalf("物品种类数不符: %d", len(a.Items))
	}
}

func TestBagCapacityUnlimited(t *testing.T) {
	a := newActor(1)
	for i := 1; i <= 5; i++ {
		if ok, _ := a.AddItem(1000+i, 1); !ok {
			t.Fatalf("物品 %d 应加入", i)
		}
	}
	if len(a.Items) != 5 {
		t.Fatalf("不限容量下物品数不符: %d", len(a.Items))
	}
}

func TestBagAddGoldenDiff(t *testing.T) {
	gold, err := os.ReadFile("../../reference/captures/golden_bagadd_152.bin")
	if err != nil {
		gold, err = os.ReadFile("../../reference/captures/golden_bagadd_152.bin")
	}
	if err != nil {
		t.Skip("golden 文件不可达")
	}
	// golden = [4B 会话前缀][148B BagAdd]
	body := gold[4:]
	mine := buildBagAddItem(1, 101001, 1)
	t.Logf("golden 长度=%d 我的长度=%d", len(body), len(mine))
	n := len(body)
	if len(mine) < n {
		n = len(mine)
	}
	for i := 0; i < n; i++ {
		if i >= len(mine) || i >= len(body) {
			break
		}
		if mine[i] != body[i] {
			t.Logf("差异 @%02x: mine=%02x gold=%02x", i, mine[i], body[i])
		}
	}
	t.Logf("mine 首尾: %x", mine[:min(24, len(mine))])
	t.Logf("gold 首尾: %x", body[:min(24, len(body))])
}
