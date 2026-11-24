package engineapi

import (
	"fmt"
	"sync"
	"time"

	"github.com/xiaokentrl/ChuanQi/ltcq-go/internal/luahost"
	"github.com/xiaokentrl/ChuanQi/ltcq-go/internal/netframe"
)

// netroute.go：网络路由层（ROADMAP 4.5）。
// 上行：网关 cmd=5 帧 → NetId→actor → 构造 LDataPack → Fire OnNetMsg。
// 下行：LDataPack.flush → SendHook → NetId + 包字节 → 网关帧 → 连接写回。

// GateSession 是一个客户端会话在 gameworld 侧的映射。
type GateSession struct {
	SID     uint16
	NetID   []byte
	IP      string
	ActorID int // 登录后绑定（登录帧前为0）
}

// NetRouter 维护 sid↔actor 双向映射并驱动 OnNetMsg。
type NetRouter struct {
	L       luahost.Interpreter
	mu      sync.RWMutex
	bySID   map[uint16]*GateSession
	byActor map[int]*GateSession
	send    func(frame []byte) error // 下行写网关连接的回调
	// login 处理引擎级登录帧（CMD_Login=255）。nil 时登录帧仍走 OnNetMsg（静默丢弃）。
	// 登录鉴权非 Lua 协议，必须在 sysid/cmdid 拆包前分流（见 ROADMAP M2 实测修订）。
	login func(sid uint16, data []byte) error
}

func NewNetRouter(L luahost.Interpreter) *NetRouter {
	r := &NetRouter{L: L, bySID: map[uint16]*GateSession{}, byActor: map[int]*GateSession{}}
	// 安装回包 hook
	SetSendHook(func(actorID int, packet []byte) {
		sess := r.byActorGet(actorID)
		if sess == nil {
			return
		}
		// 回包网关帧 payload = NetId(8B) + 完整包字节（含 sysid/cmdid 头）
		payload := append([]byte(nil), sess.NetID...)
		payload = append(payload, packet...)
		frame := netframe.BuildGateFrame(netframe.GateCmdUserData, payload)
		if r.send != nil {
			_ = r.send(frame)
		}
	})
	return r
}

// SetSender 安装下行发送回调（由 6001 连接层提供）。
func (r *NetRouter) SetSender(fn func(frame []byte) error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.send = fn
}

// SetLoginHandler 安装引擎级登录帧处理器。cmd=5 数据中 [4]==CMD_Login(255) 时优先分流至此。
func (r *NetRouter) SetLoginHandler(fn func(sid uint16, data []byte) error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.login = fn
}

// SendToSession 把业务包字节封装为 cmd=5 网关帧发回指定 sid（登录回包用）。
// payload = NetId(8B) + 业务包（含 [sysid][cmdid] 头）。
func (r *NetRouter) SendToSession(sid uint16, packet []byte) error {
	sess := r.bySIDGet(sid)
	if sess == nil {
		return fmt.Errorf("会话不存在: sid=%d", sid)
	}
	payload := append(append([]byte(nil), sess.NetID...), packet...)
	frame := netframe.BuildGateFrame(netframe.GateCmdUserData, payload)
	r.mu.RLock()
	send := r.send
	r.mu.RUnlock()
	if send == nil {
		return fmt.Errorf("未安装下行发送回调")
	}
	return send(frame)
}

// OpenSession 处理 cmd=1 UserOpen：建立 sid 会话。
func (r *NetRouter) OpenSession(sid uint16, ip string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.bySID[sid] = &GateSession{SID: sid, NetID: netframe.BuildNetID(sid), IP: ip}
}

// BindActor 登录成功后把 sid 会话绑定到 actorID。
func (r *NetRouter) BindActor(sid uint16, actorID int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	s := r.bySID[sid]
	if s == nil {
		return
	}
	s.ActorID = actorID
	r.byActor[actorID] = s
}

// CloseSession 处理 cmd=2 UserClose：清理双向映射。
func (r *NetRouter) CloseSession(sid uint16) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if s := r.bySID[sid]; s != nil {
		delete(r.byActor, s.ActorID)
		delete(r.bySID, sid)
	}
}

func (r *NetRouter) byActorGet(actorID int) *GateSession {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.byActor[actorID]
}

// RouteUpstream 处理 cmd=5 UserData 帧的 body：
// body = NetId(8B) + 数据；数据 = [sysid(1B)][cmdid(1B)][业务体]。
// 构造只读 LDataPack（读游标在业务体起点），触发 Lua 的 OnNetMsg。
func (r *NetRouter) RouteUpstream(body []byte) error {
	if len(body) < netframe.NetIDLen+2 {
		return fmt.Errorf("UserData body 过短: %d", len(body))
	}
	sid := netframe.NetIDSid(body[:netframe.NetIDLen])
	data := body[netframe.NetIDLen:]

	// 登录帧是 C++ 引擎级协议，不走 Lua OnNetMsg。
	// 数据布局：[4B 标志][0xFF=CMD_Login][子命令...]；普通业务包第2字节才是 cmdid。
	if len(data) >= 5 && data[4] == cmdLogin {
		r.mu.RLock()
		login := r.login
		r.mu.RUnlock()
		if login != nil {
			return login(sid, data)
		}
	}

	sysid, cmdid := data[0], data[1]
	business := data[2:]

	sess := r.bySIDGet(sid)
	actorID := 0
	if sess != nil {
		actorID = sess.ActorID
	}
	// 构造 Lua 侧 pack（读游标在业务体起点）
	packHandle := NewPackFromBytes(business)
	return fireNetMsg(r.L, int(sysid), int(cmdid), actorID, packHandle)
}

func (r *NetRouter) bySIDGet(sid uint16) *GateSession {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.bySID[sid]
}

// fireNetMsg 调用 Lua 的 OnNetMsg(sysId, cmdId, actor, pack)，带 500ms 超时。
func fireNetMsg(L luahost.Interpreter, sysid, cmdid, actorID, packHandle int) error {
	_, err := L.PCall(500*time.Millisecond, "OnNetMsg", sysid, cmdid, actorID, packHandle)
	return err
}

// SessionCount 返回当前会话数（测试/监控）。
func (r *NetRouter) SessionCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.bySID)
}
