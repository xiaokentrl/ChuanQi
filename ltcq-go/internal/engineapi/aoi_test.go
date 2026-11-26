package engineapi

import (
	"bytes"
	"encoding/binary"
	"math"
	"testing"

	"github.com/xiaokentrl/ChuanQi/ltcq-go/internal/luahost"
	"github.com/xiaokentrl/ChuanQi/ltcq-go/internal/netframe"
	"github.com/xiaokentrl/ChuanQi/ltcq-go/internal/scene"
)

type aoiTestConnection struct {
	sid     uint16
	actorID int
	router  *NetRouter
	packets [][]byte
}

func newAOITestConnection(t *testing.T, world *scene.World, store *mockAccountStore, sid uint16, actorID int) *aoiTestConnection {
	t.Helper()
	conn := &aoiTestConnection{sid: sid, actorID: actorID}
	conn.router = NewNetRouter(nil)
	conn.router.SetSender(func(frame []byte) error {
		conn.packets = append(conn.packets, append([]byte(nil), frame[netframe.GateFrameHeaderLen+netframe.NetIDLen:]...))
		return nil
	})
	conn.router.OpenSession(sid, "127.0.0.1")
	conn.router.SetAccount(sid, "帅哥", 121102, 1)
	conn.router.BindActor(sid, actorID)
	movement := NewMovementManager(conn.router, world)
	conn.router.SetMovementHandler(movement.HandleMove)
	installWorldCloseHandler(conn.router, world, nil, store, nil)
	post := NewPostLoginManager(store, conn.router, nil, world)
	if err := post.HandlePostLogin(sid, actorID, "帅哥", nil); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.router.CloseAll() })
	return conn
}

func (c *aoiTestConnection) move(t *testing.T, x, y int32) {
	t.Helper()
	body := clientBusinessBody(c.sid, 0, sBaseCmdFindPath, movementBody(float64(c.actorID), [2]int32{x, y}))
	if err := c.router.RouteUpstream(body); err != nil {
		t.Fatal(err)
	}
}

func (c *aoiTestConnection) lastPacket(t *testing.T) []byte {
	t.Helper()
	if len(c.packets) == 0 {
		t.Fatal("连接未收到场景事件")
	}
	return c.packets[len(c.packets)-1]
}

func assertEntitySync(t *testing.T, packet []byte, handle int, x, y uint32) {
	t.Helper()
	if len(packet) < 20 || packet[0] != cmdBase || packet[1] != sBaseCmdEntitySyncPos {
		t.Fatalf("位置同步包错误: %x", packet)
	}
	if math.Float64frombits(binary.LittleEndian.Uint64(packet[2:10])) != float64(handle) ||
		binary.LittleEndian.Uint32(packet[12:16]) != x || binary.LittleEndian.Uint32(packet[16:20]) != y {
		t.Fatalf("位置同步字段错误: %x", packet)
	}
}

func assertEntityRemoved(t *testing.T, packet []byte, handle int) {
	t.Helper()
	if len(packet) != 10 || !bytes.Equal(packet[:2], []byte{cmdBase, sBaseCmdRemoveEntity}) ||
		math.Float64frombits(binary.LittleEndian.Uint64(packet[2:10])) != float64(handle) {
		t.Fatalf("实体消失包错误: %x", packet)
	}
}

func TestAOISharedWorldAcrossConnections(t *testing.T) {
	previousActors := DefaultActorManager
	DefaultActorManager = NewActorManager()
	t.Cleanup(func() { DefaultActorManager = previousActors; SetSendHook(nil) })

	world := scene.NewWorld()
	if err := world.SetScenes(map[int]scene.SceneConfig{
		1: {Width: 128 * scene.CellSize, Height: 128 * scene.CellSize, SpawnX: 10 * scene.CellSize, SpawnY: 10 * scene.CellSize},
		2: {Width: 64 * scene.CellSize, Height: 64 * scene.CellSize, SpawnX: 5 * scene.CellSize, SpawnY: 5 * scene.CellSize},
	}); err != nil {
		t.Fatal(err)
	}
	store := newMockStore()
	store.actors[98766] = &ActorData{ActorID: 98766, AccountID: 121102, Account: "帅哥", Name: "二号角色", Job: 2, Sex: 1, Level: 20, SceneID: 1}
	store.actors[98767] = &ActorData{ActorID: 98767, AccountID: 121102, Account: "帅哥", Name: "其他场景", Job: 3, Level: 10, SceneID: 2}

	first := newAOITestConnection(t, world, store, 101, 98765)
	if len(first.packets) != 8 {
		t.Fatalf("首个角色登录回包数=%d", len(first.packets))
	}
	second := newAOITestConnection(t, world, store, 102, 98766)
	if len(second.packets) != 9 || !bytes.Equal(second.lastPacket(t)[:2], []byte{cmdBase, sBaseCmdOtherEntity}) {
		t.Fatalf("第二个角色未收到首个实体: %x", second.packets)
	}
	if len(first.packets) != 9 || !bytes.Equal(first.lastPacket(t)[:2], []byte{cmdBase, sBaseCmdOtherEntity}) {
		t.Fatalf("首个角色未收到第二个实体: %x", first.packets)
	}

	otherScene := newAOITestConnection(t, world, store, 103, 98767)
	if len(otherScene.packets) != 8 || len(first.packets) != 9 || len(second.packets) != 9 {
		t.Fatal("不同场景角色错误地触发了 AOI 广播")
	}

	first.move(t, 12*scene.CellSize, 10*scene.CellSize)
	assertEntitySync(t, first.lastPacket(t), 98765, 12*scene.CellSize, 10*scene.CellSize)
	assertEntitySync(t, second.lastPacket(t), 98765, 12*scene.CellSize, 10*scene.CellSize)

	first.move(t, 30*scene.CellSize, 10*scene.CellSize)
	if len(first.packets) < 2 {
		t.Fatalf("离开视野回包不足: %x", first.packets)
	}
	assertEntitySync(t, first.packets[len(first.packets)-2], 98765, 30*scene.CellSize, 10*scene.CellSize)
	assertEntityRemoved(t, second.lastPacket(t), 98765)
	assertEntityRemoved(t, first.lastPacket(t), 98766)

	first.move(t, 18*scene.CellSize, 10*scene.CellSize)
	assertEntitySync(t, first.packets[len(first.packets)-2], 98765, 18*scene.CellSize, 10*scene.CellSize)
	secondEnter := second.lastPacket(t)
	if len(secondEnter) < 12 || !bytes.Equal(secondEnter[:2], []byte{cmdBase, sBaseCmdOtherEntity}) ||
		math.Float64frombits(binary.LittleEndian.Uint64(secondEnter[4:12])) != 98765 {
		t.Fatalf("重新进入视野应发送完整实体: %x", secondEnter)
	}
	if packet := first.lastPacket(t); !bytes.Equal(packet[:2], []byte{cmdBase, sBaseCmdOtherEntity}) {
		t.Fatalf("重新进入视野未发送实体: %x", packet)
	}

	second.router.CloseSession(second.sid)
	assertEntityRemoved(t, first.lastPacket(t), 98766)
	if _, ok := world.Entity(98766); ok {
		t.Fatal("断线后角色仍留在共享场景")
	}
	if second.router.SessionCount() != 0 {
		t.Fatal("断线后路由会话未清理")
	}
	if _, ok := world.Entity(98765); !ok {
		t.Fatal("其他角色断线误删了当前角色")
	}
	first.router.CloseAll()
	if _, ok := world.Entity(98765); ok {
		t.Fatal("网关连接结束后角色仍留在共享场景")
	}
}

func TestAOIMonsterVisibilityAcrossConnections(t *testing.T) {
	previousActors := DefaultActorManager
	DefaultActorManager = NewActorManager()
	t.Cleanup(func() { DefaultActorManager = previousActors; SetSendHook(nil) })

	world := scene.NewWorld()
	if err := world.SetScenes(map[int]scene.SceneConfig{
		1: {Width: 128 * scene.CellSize, Height: 128 * scene.CellSize, SpawnX: 10 * scene.CellSize, SpawnY: 10 * scene.CellSize},
	}); err != nil {
		t.Fatal(err)
	}
	const monsterID = 1 << 30
	if deliveries, err := world.AddMonster(monsterID, 1, 31001, 11*scene.CellSize, 10*scene.CellSize, 127738, 127738); err != nil || len(deliveries) != 0 {
		t.Fatalf("无观察者刷怪不应广播: deliveries=%+v err=%v", deliveries, err)
	}
	store := newMockStore()
	store.actors[98766] = &ActorData{ActorID: 98766, AccountID: 121102, Account: "帅哥", Name: "二号角色", Job: 2, SceneID: 1}

	first := newAOITestConnection(t, world, store, 101, 98765)
	if len(first.packets) != 9 || !bytes.Equal(first.lastPacket(t), buildMonsterEntity(scene.Entity{
		ID: monsterID, Kind: scene.EntityMonster, SceneID: 1, X: 11 * scene.CellSize, Y: 10 * scene.CellSize, ConfigID: 31001, HP: 127738, MaxHP: 127738,
	})) {
		t.Fatalf("登录未收到怪物实体: %x", first.packets)
	}
	second := newAOITestConnection(t, world, store, 102, 98766)
	foundMonster := false
	for _, packet := range second.packets {
		if len(packet) >= 16 && bytes.Equal(packet[:2], []byte{cmdBase, sBaseCmdOtherEntity}) &&
			binary.LittleEndian.Uint16(packet[2:4]) == 2 && math.Float64frombits(binary.LittleEndian.Uint64(packet[4:12])) == monsterID {
			foundMonster = true
		}
	}
	if !foundMonster {
		t.Fatalf("第二个连接登录未收到怪物: %x", second.packets)
	}

	first.move(t, 30*scene.CellSize, 10*scene.CellSize)
	assertEntitySync(t, first.packets[len(first.packets)-3], 98765, 30*scene.CellSize, 10*scene.CellSize)
	assertMonsterRemoved := func(packet []byte) bool {
		return len(packet) == 10 && bytes.Equal(packet[:2], []byte{cmdBase, sBaseCmdRemoveEntity}) &&
			math.Float64frombits(binary.LittleEndian.Uint64(packet[2:10])) == monsterID
	}
	if !assertMonsterRemoved(first.packets[len(first.packets)-2]) && !assertMonsterRemoved(first.packets[len(first.packets)-1]) {
		t.Fatalf("离开怪物视野未发送消失包: %x", first.packets[len(first.packets)-3:])
	}
	first.move(t, 18*scene.CellSize, 10*scene.CellSize)
	assertEntitySync(t, first.packets[len(first.packets)-3], 98765, 18*scene.CellSize, 10*scene.CellSize)
	assertMonsterEnter := func(packet []byte) bool {
		return len(packet) >= 16 && bytes.Equal(packet[:2], []byte{cmdBase, sBaseCmdOtherEntity}) &&
			binary.LittleEndian.Uint16(packet[2:4]) == 2 && math.Float64frombits(binary.LittleEndian.Uint64(packet[4:12])) == monsterID
	}
	if !assertMonsterEnter(first.packets[len(first.packets)-2]) && !assertMonsterEnter(first.packets[len(first.packets)-1]) {
		t.Fatalf("重新进入怪物视野未发送实体: %x", first.packets[len(first.packets)-3:])
	}

	first.router.CloseSession(first.sid)
	if _, ok := world.Entity(monsterID); !ok {
		t.Fatal("玩家断线误删了怪物")
	}
	if _, ok := world.Entity(98766); !ok {
		t.Fatal("其他玩家断线误删了当前玩家")
	}
	second.router.CloseAll()
	if _, ok := world.Entity(monsterID); !ok {
		t.Fatal("所有玩家离线后静态怪物应保留")
	}
}

func TestPostLoginRollsBackWorldOnLuaError(t *testing.T) {
	previousActors := DefaultActorManager
	DefaultActorManager = NewActorManager()
	t.Cleanup(func() { DefaultActorManager = previousActors; SetSendHook(nil) })

	world := scene.NewWorld()
	if err := world.SetScenes(map[int]scene.SceneConfig{
		1: {Width: 64, Height: 64, SpawnX: 0, SpawnY: 0},
	}); err != nil {
		t.Fatal(err)
	}
	g := luahost.NewGopherLua(luahost.Config{OSType: "WIN"})
	t.Cleanup(g.Close)
	if err := g.Load("stub", `function OnActorEvent() error("login event failed") end`); err != nil {
		t.Fatal(err)
	}
	store := newMockStore()
	router := NewNetRouter(nil)
	router.OpenSession(101, "")
	router.SetAccount(101, "帅哥", 121102, 1)
	router.BindActor(101, 98765)
	router.SetSender(func([]byte) error { return nil })
	post := NewPostLoginManager(store, router, g, world)
	if err := post.HandlePostLogin(101, 98765, "帅哥", nil); err == nil {
		t.Fatal("Lua 登录事件失败应返回错误")
	}
	if _, ok := world.Entity(98765); ok {
		t.Fatal("登录事件失败后角色仍留在共享场景")
	}
}

func TestPostLoginRejectsUnknownWorldSceneBeforeReply(t *testing.T) {
	previousActors := DefaultActorManager
	DefaultActorManager = NewActorManager()
	t.Cleanup(func() { DefaultActorManager = previousActors; SetSendHook(nil) })

	world := scene.NewWorld()
	if err := world.SetScenes(map[int]scene.SceneConfig{
		1: {Width: 64, Height: 64, SpawnX: 0, SpawnY: 0},
	}); err != nil {
		t.Fatal(err)
	}
	store := newMockStore()
	actor := store.actors[98765]
	actor.SceneID = 99
	router := NewNetRouter(nil)
	router.OpenSession(101, "")
	router.SetAccount(101, "帅哥", 121102, 1)
	sent := 0
	router.SetSender(func([]byte) error { sent++; return nil })
	post := NewPostLoginManager(store, router, nil, world)
	if err := post.HandlePostLogin(101, 98765, "帅哥", nil); err == nil {
		t.Fatal("未配置场景应拒绝进入")
	}
	if sent != 0 {
		t.Fatal("场景校验失败前不应发送进游戏成功包")
	}
}
