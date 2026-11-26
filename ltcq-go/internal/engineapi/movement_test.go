package engineapi

import (
	"encoding/binary"
	"math"
	"testing"

	"github.com/xiaokentrl/ChuanQi/ltcq-go/internal/luahost"
	"github.com/xiaokentrl/ChuanQi/ltcq-go/internal/netframe"
)

func movementBody(handle float64, points ...[2]int32) []byte {
	body := binary.LittleEndian.AppendUint64(nil, math.Float64bits(handle))
	body = binary.LittleEndian.AppendUint32(body, uint32(len(points)))
	for _, point := range points {
		body = binary.LittleEndian.AppendUint32(body, uint32(point[0]))
		body = binary.LittleEndian.AppendUint32(body, uint32(point[1]))
	}
	return body
}

func TestMovement_FullChain(t *testing.T) {
	DefaultActorManager = NewActorManager()
	g := luahost.NewGopherLua(luahost.Config{OSType: "WIN"})
	defer g.Close()
	defer SetSendHook(nil)
	router := NewNetRouter(g)
	router.SetMovementHandler(NewMovementManager(router).HandleMove)
	var frames [][]byte
	router.SetSender(func(frame []byte) error {
		frames = append(frames, append([]byte(nil), frame...))
		return nil
	})
	const sid, actorID = uint16(41), 2001
	router.OpenSession(sid, "127.0.0.1")
	router.SetAccount(sid, "test", 1, 1)
	router.BindActor(sid, actorID)
	(&postLoginManager{}).applyActorData(&ActorData{ActorID: actorID, SceneID: 1, Level: 55})

	body := clientBusinessBody(sid, 0, sBaseCmdFindPath, movementBody(actorID, [2]int32{60 * 64, 37 * 64}, [2]int32{60 * 64, 40 * 64}))
	if err := router.RouteUpstream(body); err != nil {
		t.Fatal(err)
	}
	actor := DefaultActorManager.Get(actorID)
	if actor.X != 60*64 || actor.Y != 40*64 {
		t.Fatalf("服务端坐标=(%d,%d)", actor.X, actor.Y)
	}
	if len(frames) != 1 {
		t.Fatalf("移动回包数=%d", len(frames))
	}
	cmd, bodyLen, err := netframe.ParseGateFrame(frames[0])
	if err != nil || cmd != netframe.GateCmdUserData {
		t.Fatalf("移动网关帧错误: cmd=%d err=%v", cmd, err)
	}
	payload := frames[0][netframe.GateFrameHeaderLen : netframe.GateFrameHeaderLen+bodyLen]
	packet := payload[netframe.NetIDLen:]
	if len(packet) != 24 || packet[0] != 0 || packet[1] != sBaseCmdEntitySyncPos {
		t.Fatalf("移动回包布局错误: %x", packet)
	}
	if math.Float64frombits(binary.LittleEndian.Uint64(packet[2:10])) != actorID || binary.LittleEndian.Uint16(packet[10:12]) != 1 {
		t.Fatalf("移动回包句柄/模式错误: %x", packet)
	}
	if binary.LittleEndian.Uint32(packet[12:16]) != 60*64 || binary.LittleEndian.Uint32(packet[16:20]) != 40*64 {
		t.Fatalf("移动回包坐标错误: %x", packet)
	}
	if binary.LittleEndian.Uint32(packet[20:24]) != ((4*64 + 3*64) / 4) {
		t.Fatalf("移动回包时长错误: %d", binary.LittleEndian.Uint32(packet[20:24]))
	}
}

func TestMovement_RejectsInvalidRequests(t *testing.T) {
	valid := movementBody(2001, [2]int32{60 * 64, 40 * 64})
	tests := []struct {
		name string
		body []byte
	}{
		{"wrong_handle", movementBody(2002, [2]int32{60 * 64, 40 * 64})},
		{"zero_count", movementBody(2001)},
		{"negative_coordinate", movementBody(2001, [2]int32{-1, 100})},
		{"out_of_bounds", movementBody(2001, [2]int32{64 * 64, 100})},
		{"truncated", valid[:len(valid)-1]},
		{"trailing_data", append(append([]byte(nil), valid...), 0)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			DefaultActorManager = NewActorManager()
			g := luahost.NewGopherLua(luahost.Config{OSType: "WIN"})
			defer g.Close()
			defer SetSendHook(nil)
			router := NewNetRouter(g)
			router.SetMovementHandler(NewMovementManager(router).HandleMove)
			sent := 0
			router.SetSender(func([]byte) error { sent++; return nil })
			const sid, actorID = uint16(42), 2001
			router.OpenSession(sid, "")
			router.SetAccount(sid, "test", 1, 1)
			router.BindActor(sid, actorID)
			(&postLoginManager{}).applyActorData(&ActorData{ActorID: actorID, SceneID: 1})
			if err := router.RouteUpstream(clientBusinessBody(sid, 0, sBaseCmdFindPath, tt.body)); err == nil {
				t.Fatal("非法移动请求应返回错误")
			}
			actor := DefaultActorManager.Get(actorID)
			if actor.X != 56*64 || actor.Y != 37*64 || sent != 0 {
				t.Fatalf("非法请求改变了状态: pos=(%d,%d) sent=%d", actor.X, actor.Y, sent)
			}
		})
	}
}

func TestMovement_RejectsDeadPlayer(t *testing.T) {
	previousActors := DefaultActorManager
	DefaultActorManager = NewActorManager()
	t.Cleanup(func() { DefaultActorManager = previousActors })
	g := luahost.NewGopherLua(luahost.Config{OSType: "WIN"})
	defer g.Close()
	defer SetSendHook(nil)
	router := NewNetRouter(g)
	router.SetMovementHandler(NewMovementManager(router).HandleMove)
	sent := 0
	router.SetSender(func([]byte) error { sent++; return nil })
	const sid, actorID = uint16(44), 2001
	router.OpenSession(sid, "")
	router.SetAccount(sid, "test", 1, 1)
	router.BindActor(sid, actorID)
	(&postLoginManager{}).applyActorData(&ActorData{ActorID: actorID, SceneID: 1})
	actor := DefaultActorManager.Get(actorID)
	actor.HP = 0
	prevX, prevY := actor.X, actor.Y
	body := clientBusinessBody(sid, 0, sBaseCmdFindPath, movementBody(actorID, [2]int32{60 * 64, 40 * 64}))
	if err := router.RouteUpstream(body); err == nil {
		t.Fatal("死亡玩家移动请求应返回错误")
	}
	if actor.X != prevX || actor.Y != prevY || sent != 0 {
		t.Fatalf("死亡移动改变了状态: pos=(%d,%d) sent=%d", actor.X, actor.Y, sent)
	}
}

func TestMovement_RejectsUnboundSession(t *testing.T) {
	g := luahost.NewGopherLua(luahost.Config{OSType: "WIN"})
	defer g.Close()
	router := NewNetRouter(g)
	router.SetMovementHandler(NewMovementManager(router).HandleMove)
	router.OpenSession(43, "")
	if err := router.RouteUpstream(clientBusinessBody(43, 0, sBaseCmdFindPath, movementBody(2001, [2]int32{100, 100}))); err == nil {
		t.Fatal("未绑定会话不应移动")
	}
}
