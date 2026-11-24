package engineapi

import (
	"encoding/binary"
	"testing"

	"github.com/xiaokentrl/ChuanQi/ltcq-go/internal/luahost"
	"github.com/xiaokentrl/ChuanQi/ltcq-go/internal/netframe"
)

// mockAccountStore 单测用账号存储（覆盖成功/密码错/无账号）。
type mockAccountStore struct {
	accounts map[string]struct {
		id     int
		passwd string
		actor  int
	}
}

func (m *mockAccountStore) CheckPassword(account, md5pass string) (int, bool, error) {
	a, ok := m.accounts[account]
	if !ok {
		return 0, false, nil
	}
	return a.id, a.passwd == md5pass, nil
}
func (m *mockAccountStore) ActorIDByAccount(account string) (int, error) {
	return m.accounts[account].actor, nil
}
func (m *mockAccountStore) Close() {}

// buildLoginFrameBytes 按 golden 布局构造登录帧（NetId 之后的 54B）。
func buildLoginFrameBytes(account, md5pass string) []byte {
	b := make([]byte, 0, 64)
	flag := make([]byte, 4)
	binary.LittleEndian.PutUint32(flag, 1)
	b = append(b, flag...)
	b = append(b, cmdLogin, 0x01, 0x01, 0, 0, 0) // ff 01 01 00 00 00
	l := make([]byte, 2)
	binary.LittleEndian.PutUint16(l, uint16(len(account)))
	b = append(b, l...)
	b = append(b, []byte(account)...)
	b = append(b, 0, 0x20, 0)         // 3B 分隔段
	b = append(b, []byte(md5pass)...) // 32B md5
	b = append(b, 0)                  // 尾字节
	return b
}

func TestParseLoginFrame_Golden(t *testing.T) {
	data := buildLoginFrameBytes("帅哥", "e10adc3949ba59abbe56e057f20f883e")
	f, err := ParseLoginFrame(data)
	if err != nil {
		t.Fatal(err)
	}
	if f.Flag != 1 || f.Cmd != cmdLogin || f.SubCmd != 1 {
		t.Errorf("头字段 flag=%d cmd=%02x sub=%d", f.Flag, f.Cmd, f.SubCmd)
	}
	if f.Account != "帅哥" {
		t.Errorf("账号=%q want 帅哥", f.Account)
	}
	if f.Password != "e10adc3949ba59abbe56e057f20f883e" {
		t.Errorf("密码=%q", f.Password)
	}
}

func TestLoginManager_SuccessAndReply(t *testing.T) {
	store := &mockAccountStore{accounts: map[string]struct {
		id     int
		passwd string
		actor  int
	}{"帅哥": {121102, "e10adc3949ba59abbe56e057f20f883e", 98765}}}

	g := luahost.NewGopherLua(luahost.Config{OSType: "WIN"})
	defer g.Close()
	defer SetSendHook(nil)

	var frames [][]byte
	router := NewNetRouter(g)
	router.SetSender(func(frame []byte) error {
		frames = append(frames, append([]byte(nil), frame...))
		return nil
	})
	mgr := NewLoginManager(store, router)
	router.SetLoginHandler(mgr.HandleLogin)

	const sid = uint16(552)
	router.OpenSession(sid, "127.0.0.1")

	body := append(netframe.BuildNetID(sid), buildLoginFrameBytes("帅哥", "e10adc3949ba59abbe56e057f20f883e")...)
	if err := router.RouteUpstream(body); err != nil {
		t.Fatal(err)
	}

	if len(frames) != 1 {
		t.Fatalf("回包帧数 %d want 1", len(frames))
	}
	cmd, bodyLen, err := netframe.ParseGateFrame(frames[0])
	if err != nil {
		t.Fatal(err)
	}
	if cmd != netframe.GateCmdUserData || bodyLen != netframe.NetIDLen+3 {
		t.Fatalf("回包 cmd=%d bodyLen=%d", cmd, bodyLen)
	}
	b := frames[0][netframe.GateFrameHeaderLen:]
	if netframe.NetIDSid(b[:netframe.NetIDLen]) != sid {
		t.Error("回包 sid 不符")
	}
	want := []byte{cmdLogin, loginSubCmdResult, loginResultSuccess}
	for i, w := range want {
		if b[netframe.NetIDLen+i] != w {
			t.Errorf("回包[%d]=%02x want %02x", i, b[netframe.NetIDLen+i], w)
		}
	}
	if s := router.bySIDGet(sid); s == nil || s.ActorID != 98765 {
		t.Errorf("登录未绑定 actor: %v", s)
	}
}

func TestLoginManager_BadPassword(t *testing.T) {
	store := &mockAccountStore{accounts: map[string]struct {
		id     int
		passwd string
		actor  int
	}{"帅哥": {121102, "e10adc3949ba59abbe56e057f20f883e", 98765}}}
	g := luahost.NewGopherLua(luahost.Config{OSType: "WIN"})
	defer g.Close()
	defer SetSendHook(nil)

	var last []byte
	router := NewNetRouter(g)
	router.SetSender(func(frame []byte) error { last = frame; return nil })
	mgr := NewLoginManager(store, router)
	router.SetLoginHandler(mgr.HandleLogin)
	router.OpenSession(1, "")
	body := append(netframe.BuildNetID(1), buildLoginFrameBytes("帅哥", "00000000000000000000000000000000")...)
	if err := router.RouteUpstream(body); err != nil {
		t.Fatal(err)
	}
	b := last[netframe.GateFrameHeaderLen+netframe.NetIDLen:]
	if b[2] != loginResultNoAccount {
		t.Errorf("错误密码结果码=%02x want %02x", b[2], loginResultNoAccount)
	}
	if s := router.bySIDGet(1); s != nil && s.ActorID != 0 {
		t.Error("失败登录不应绑定 actor")
	}
}
