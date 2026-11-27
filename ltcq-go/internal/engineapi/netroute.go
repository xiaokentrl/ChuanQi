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
	SID           uint16
	NetID         []byte
	IP            string
	Account       string
	AccountID     int
	ServerID      uint32
	Authenticated bool
	ActorID       int
}

func cloneGateSession(s *GateSession) *GateSession {
	if s == nil {
		return nil
	}
	copy := *s
	copy.NetID = append([]byte(nil), s.NetID...)
	return &copy
}

// NetRouter 维护 sid↔actor 双向映射并驱动 OnNetMsg。
type NetRouter struct {
	L       luahost.Interpreter
	mu      sync.RWMutex
	bySID   map[uint16]*GateSession
	byActor map[int]*GateSession
	send    func(frame []byte) error // 下行写网关连接的回调
	// 同号重登绑定与旧连接离场清理必须串行，避免清理掉新会话。
	lifecycleMu sync.Mutex
	// login 处理引擎级登录帧（CMD_Login=255）。nil 时登录帧仍走 OnNetMsg（静默丢弃）。
	// 登录鉴权非 Lua 协议，必须在 sysid/cmdid 拆包前分流（见 ROADMAP M2 实测修订）。
	login        func(sid uint16, data []byte) error
	move         func(sid uint16, actorID int, body []byte) error
	bag          func(sid uint16, actorID int, body []byte) error
	skill        func(sid uint16, actorID int, cmdid uint16, body []byte) error
	clientConfig func(sid uint16, actorID int, body []byte) error
	dropPickup   func(sid uint16, actorID int, body []byte) error
	close        func(sid uint16, actorID int)
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

// SetMovementHandler 安装 [00][1f] 移动请求处理器。
func (r *NetRouter) SetMovementHandler(fn func(sid uint16, actorID int, body []byte) error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.move = fn
}

func (r *NetRouter) SetBagHandler(fn func(sid uint16, actorID int, body []byte) error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.bag = fn
}

func (r *NetRouter) SetSkillHandler(fn func(sid uint16, actorID int, cmdid uint16, body []byte) error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.skill = fn
}

// SetClientConfigHandler 安装 [00][13] 客户端配置更新请求处理器（clientconfig.lua onUpdateConfig）。
func (r *NetRouter) SetClientConfigHandler(fn func(sid uint16, actorID int, body []byte) error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.clientConfig = fn
}

// SetDropPickupHandler 安装地面掉落拾取上行处理器（[01][01] flag=0，UserFb.sendGetReward，
// golden RUNBOOK 五十五）。flag≠0 的副本领奖不在此分流，仍走 OnNetMsg。
func (r *NetRouter) SetDropPickupHandler(fn func(sid uint16, actorID int, body []byte) error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.dropPickup = fn
}

func (r *NetRouter) SetCloseHandler(fn func(sid uint16, actorID int)) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.close = fn
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
	r.unbindLocked(sid)
	r.bySID[sid] = &GateSession{SID: sid, NetID: netframe.BuildNetID(sid), IP: ip}
}

func (r *NetRouter) SetAccount(sid uint16, account string, accountID int, serverID uint32) {
	r.mu.Lock()
	defer r.mu.Unlock()
	s := r.bySID[sid]
	if s == nil {
		return
	}
	r.unbindLocked(sid)
	s.Account = account
	s.AccountID = accountID
	s.ServerID = serverID
	s.Authenticated = true
}

func (r *NetRouter) AuthenticatedSession(sid uint16) *GateSession {
	r.mu.RLock()
	defer r.mu.RUnlock()
	s := r.bySID[sid]
	if s == nil || !s.Authenticated {
		return nil
	}
	return cloneGateSession(s)
}

func (r *NetRouter) BindActor(sid uint16, actorID int) {
	r.lifecycleMu.Lock()
	defer r.lifecycleMu.Unlock()
	r.mu.Lock()
	defer r.mu.Unlock()
	s := r.bySID[sid]
	if s == nil {
		return
	}
	r.unbindLocked(sid)
	s.ActorID = actorID
	if previous := r.byActor[actorID]; previous != nil && previous.SID != sid {
		previous.ActorID = 0
	}
	r.byActor[actorID] = s
}

func (r *NetRouter) UnbindActor(sid uint16) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.unbindLocked(sid)
}

func (r *NetRouter) unbindLocked(sid uint16) {
	s := r.bySID[sid]
	if s != nil && s.ActorID != 0 && r.byActor[s.ActorID] == s {
		delete(r.byActor, s.ActorID)
	}
	if s != nil {
		s.ActorID = 0
	}
}

func (r *NetRouter) CloseSession(sid uint16) {
	r.lifecycleMu.Lock()
	defer r.lifecycleMu.Unlock()
	r.mu.Lock()
	actorID := 0
	if session := r.bySID[sid]; session != nil {
		actorID = session.ActorID
	}
	r.unbindLocked(sid)
	delete(r.bySID, sid)
	closeHandler := r.close
	r.mu.Unlock()
	if closeHandler != nil {
		closeHandler(sid, actorID)
	}
}

func (r *NetRouter) CloseAll() {
	r.mu.RLock()
	sids := make([]uint16, 0, len(r.bySID))
	for sid := range r.bySID {
		sids = append(sids, sid)
	}
	r.mu.RUnlock()
	for _, sid := range sids {
		r.CloseSession(sid)
	}
}

func (r *NetRouter) byActorGet(actorID int) *GateSession {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return cloneGateSession(r.byActor[actorID])
}

// RouteUpstream 处理 cmd=5 UserData 帧的 body：
// body = NetId(8B) + seq(4B) + [sysid][cmdid] + 业务体。
// 构造只读 LDataPack（读游标在业务体起点），触发 Lua 的 OnNetMsg。
func (r *NetRouter) RouteUpstream(body []byte) error {
	if len(body) < netframe.NetIDLen+6 {
		return fmt.Errorf("UserData body 过短: %d", len(body))
	}
	sid := netframe.NetIDSid(body[:netframe.NetIDLen])
	data := body[netframe.NetIDLen:]

	if len(data) >= 5 && data[4] == cmdLogin {
		r.mu.RLock()
		login := r.login
		r.mu.RUnlock()
		if login == nil {
			return nil
		}
		return login(sid, data)
	}
	if len(data) < 6 {
		return fmt.Errorf("业务包头过短: %d", len(data))
	}

	sysid, cmdid := data[4], data[5]
	business := data[6:]
	sess := r.bySIDGet(sid)
	actorID := 0
	if sess != nil {
		actorID = sess.ActorID
	}
	if sysid == cmdBase && cmdid == sBaseCmdFindPath {
		r.mu.RLock()
		move := r.move
		r.mu.RUnlock()
		if move != nil {
			return move(sid, actorID, business)
		}
	}
	if sysid == cmdBag && cmdid == sBagCmdUseItem {
		r.mu.RLock()
		bag := r.bag
		r.mu.RUnlock()
		if bag != nil {
			return bag(sid, actorID, business)
		}
	}
	if sysid == cmdSkill && (cmdid == cSkillCmdCastSkill || cmdid == cSkillCmdUpgradeSkill || cmdid == cSkillCmdUpgradeAll) {
		r.mu.RLock()
		skill := r.skill
		r.mu.RUnlock()
		if skill != nil {
			return skill(sid, actorID, uint16(cmdid), business)
		}
	}
	if sysid == cmdFuben && cmdid == sFubenCmdGetReward && len(business) > 0 && business[0] == 0 {
		r.mu.RLock()
		drop := r.dropPickup
		r.mu.RUnlock()
		if drop != nil {
			return drop(sid, actorID, business)
		}
	}
	if sysid == cmdBase && cmdid == sBaseCmdClientConfig {
		r.mu.RLock()
		cc := r.clientConfig
		r.mu.RUnlock()
		if cc != nil {
			return cc(sid, actorID, business)
		}
	}

	packHandle := NewPackFromBytes(business)
	return fireNetMsg(r.L, int(sysid), int(cmdid), actorID, packHandle)
}

func (r *NetRouter) bySIDGet(sid uint16) *GateSession {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return cloneGateSession(r.bySID[sid])
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
