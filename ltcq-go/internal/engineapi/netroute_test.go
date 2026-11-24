package engineapi

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/xiaokentrl/ChuanQi/ltcq-go/internal/luahost"
	"github.com/xiaokentrl/ChuanQi/ltcq-go/internal/netframe"
)

// newRouteEngine 组装 netroute 测试所需的完整 Lua 环境（枚举 + API + 五大注册表）。
// 调用方须 defer g.Close() 与 defer SetSendHook(nil)。
func newRouteEngine(t *testing.T) *luahost.GopherLua {
	t.Helper()
	root := testDataRoot(t)
	g := luahost.NewGopherLua(luahost.Config{DataRoot: root, OSType: "WIN"})
	if err := g.InjectEnums(); err != nil {
		g.Close()
		t.Skipf("资产不可达: %v", err)
	}
	if err := InstallAll(g); err != nil {
		g.Close()
		t.Fatal(err)
	}
	InstallSystemStubs(g)
	if _, err := LoadRegistries(g, root); err != nil {
		g.Close()
		t.Fatal(err)
	}
	return g
}

// TestNetRoute_FullChain 验证完整链路：
// cmd=5 网关帧 → ReadGateFrame → NetId→sid→actor → OnNetMsg → Lua readInt →
// allocPacket/writeInt/flush → SendHook → cmd=5 回包帧。
func TestNetRoute_FullChain(t *testing.T) {
	g := newRouteEngine(t)
	defer g.Close()
	defer SetSendHook(nil)

	var sent [][]byte
	router := NewNetRouter(g)
	router.SetSender(func(frame []byte) error {
		sent = append(sent, append([]byte(nil), frame...))
		return nil
	})

	const sid = uint16(0x1234)
	const actorID = 2001
	router.OpenSession(sid, "127.0.0.1")
	router.BindActor(sid, actorID)

	// 业务回调：读上行 int，回写 +1 的响应包
	if err := g.Load("netroute_full", `
		routeActor = -1
		routeSys, routeCmd = -1, -1
		routeVal = -1
		netmsgdispatcher.reg(10, 20, function(actor, pack)
			routeActor = actor
			routeVal = LDataPack.readInt(pack)
			local rsp = LDataPack.allocPacket(actor, 10, 21)
			LDataPack.writeInt(rsp, routeVal + 1)
			LDataPack.flush(rsp)
		end)
	`); err != nil {
		t.Fatalf("注册回调失败: %v", err)
	}

	// 合成上行帧：body = NetId(8B) + [sysid][cmdid] + int32(777)
	business := make([]byte, 4)
	binary.LittleEndian.PutUint32(business, 777)
	payload := append(netframe.BuildNetID(sid), 10, 20)
	payload = append(payload, business...)
	frame := netframe.BuildGateFrame(netframe.GateCmdUserData, payload)

	// 模拟 6001 帧 pump
	f, err := netframe.ReadGateFrame(bytes.NewReader(frame))
	if err != nil {
		t.Fatalf("ReadGateFrame 失败: %v", err)
	}
	if f.Cmd != netframe.GateCmdUserData {
		t.Fatalf("cmd=%d want %d", f.Cmd, netframe.GateCmdUserData)
	}
	if err := router.RouteUpstream(f.Body); err != nil {
		t.Fatalf("RouteUpstream 失败: %v", err)
	}

	// 校验 Lua 回调参数
	assertIntGlobal(t, g, "routeActor", actorID)
	assertIntGlobal(t, g, "routeVal", 777)

	// 校验下行回包帧
	if len(sent) != 1 {
		t.Fatalf("回包帧数 %d want 1", len(sent))
	}
	out := sent[0]
	cmd, bodyLen, err := netframe.ParseGateFrame(out)
	if err != nil {
		t.Fatalf("解析回包帧头失败: %v", err)
	}
	if cmd != netframe.GateCmdUserData {
		t.Errorf("回包 cmd=%d want %d", cmd, netframe.GateCmdUserData)
	}
	wantBodyLen := netframe.NetIDLen + 2 + 4 // NetId + [sysid][cmdid] + int32
	if bodyLen != wantBodyLen {
		t.Fatalf("回包 bodyLen=%d want %d", bodyLen, wantBodyLen)
	}
	body := out[netframe.GateFrameHeaderLen:]
	if got := netframe.NetIDSid(body[:netframe.NetIDLen]); got != sid {
		t.Errorf("回包 NetId sid=%d want %d", got, sid)
	}
	if body[netframe.NetIDLen] != 10 || body[netframe.NetIDLen+1] != 21 {
		t.Errorf("回包协议头 = %x %x want 0a 15", body[netframe.NetIDLen], body[netframe.NetIDLen+1])
	}
	if got := binary.LittleEndian.Uint32(body[netframe.NetIDLen+2:]); got != 778 {
		t.Errorf("回包 payload=%d want 778", got)
	}
}

// TestNetRoute_SessionMapping 验证 sid↔actor 双向映射与清理。
func TestNetRoute_SessionMapping(t *testing.T) {
	g := luahost.NewGopherLua(luahost.Config{OSType: "WIN"})
	defer g.Close()
	defer SetSendHook(nil)

	router := NewNetRouter(g)
	router.OpenSession(1, "1.1.1.1")
	router.OpenSession(2, "2.2.2.2")
	if n := router.SessionCount(); n != 2 {
		t.Fatalf("SessionCount=%d want 2", n)
	}

	router.BindActor(1, 100)
	if s := router.byActorGet(100); s == nil || s.SID != 1 {
		t.Fatalf("byActor(100)=%v want sid=1", s)
	}
	if s := router.bySIDGet(1); s == nil || s.ActorID != 100 {
		t.Fatalf("bySID(1)=%v want actor=100", s)
	}

	router.CloseSession(1)
	if n := router.SessionCount(); n != 1 {
		t.Errorf("CloseSession 后 SessionCount=%d want 1", n)
	}
	if s := router.byActorGet(100); s != nil {
		t.Errorf("CloseSession 后 byActor(100) 仍存在: %v", s)
	}
}

// TestNetRoute_UnboundActor 验证未绑定 actor 的上行帧仍以 actor=0 触发 OnNetMsg。
func TestNetRoute_UnboundActor(t *testing.T) {
	g := newRouteEngine(t)
	defer g.Close()
	defer SetSendHook(nil)

	router := NewNetRouter(g)
	router.OpenSession(7, "")
	if err := g.Load("netroute_unbound", `
		unboundActor = -1
		netmsgdispatcher.reg(11, 12, function(actor, pack) unboundActor = actor end)
	`); err != nil {
		t.Fatal(err)
	}
	body := append(netframe.BuildNetID(7), 11, 12, 0)
	if err := router.RouteUpstream(body); err != nil {
		t.Fatal(err)
	}
	assertIntGlobal(t, g, "unboundActor", 0)
}

// TestNetRoute_ShortBody 验证过短 body 被拒绝且不触发回调。
func TestNetRoute_ShortBody(t *testing.T) {
	g := luahost.NewGopherLua(luahost.Config{OSType: "WIN"})
	defer g.Close()
	defer SetSendHook(nil)

	router := NewNetRouter(g)
	if err := router.RouteUpstream([]byte{1, 2, 3}); err == nil {
		t.Error("过短 body 应返回错误")
	}
}

func assertIntGlobal(t *testing.T, g *luahost.GopherLua, name string, want int) {
	t.Helper()
	v, err := g.DoString(name)
	if err != nil {
		t.Fatalf("读取全局 %s 失败: %v", name, err)
	}
	got, ok := v.(int)
	if !ok {
		t.Fatalf("全局 %s 类型 %T want int", name, v)
	}
	if got != want {
		t.Errorf("%s=%d want %d", name, got, want)
	}
}
