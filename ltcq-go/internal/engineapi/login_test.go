package engineapi

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"testing"

	"github.com/xiaokentrl/ChuanQi/ltcq-go/internal/luahost"
	"github.com/xiaokentrl/ChuanQi/ltcq-go/internal/netframe"
)

const testPassword = "e10adc3949ba59abbe56e057f20f883e"

type mockAccount struct {
	id     int
	passwd string
}

type mockAccountStore struct {
	accounts        map[string]mockAccount
	actors          map[int]*ActorData
	nextID          int
	vipSaveErr      error
	currencySaveErr error
	currencyCalls   int
	currencyType    int
	currencyAmount  int64
}

func (m *mockAccountStore) CheckPassword(account, md5pass string) (int, bool, error) {
	a, ok := m.accounts[account]
	if !ok {
		return 0, false, nil
	}
	return a.id, a.passwd == md5pass, nil
}

func (m *mockAccountStore) ListActors(account string) ([]ActorData, error) {
	var actors []ActorData
	for _, actor := range m.actors {
		if actor.Account == account {
			actors = append(actors, *actor)
		}
	}
	return actors, nil
}

func (m *mockAccountStore) LoadActorData(actorID int) (*ActorData, error) {
	actor := m.actors[actorID]
	if actor == nil {
		return nil, fmt.Errorf("actor %d not found", actorID)
	}
	copy := *actor
	return &copy, nil
}

func (m *mockAccountStore) CreateActor(account string, accountID int, name string, job, sex int) (int, error) {
	for _, actor := range m.actors {
		if actor.Name == name {
			return 0, fmt.Errorf("角色名已存在")
		}
	}
	m.nextID++
	id := 900000 + m.nextID
	m.actors[id] = &ActorData{
		ActorID: id, AccountID: accountID, Account: account, Name: name,
		Job: job, Sex: sex, Level: 1, SceneID: 1, Status: 2,
	}
	return id, nil
}

func (m *mockAccountStore) SaveProgression(actorID int, level int, exp, gold int64) error {
	actor := m.actors[actorID]
	if actor == nil {
		return fmt.Errorf("actor %d not found", actorID)
	}
	actor.Level, actor.Exp, actor.Gold = level, exp, gold
	return nil
}

func (m *mockAccountStore) SaveVIPLevel(actorID, level int) error {
	if m.vipSaveErr != nil {
		return m.vipSaveErr
	}
	actor := m.actors[actorID]
	if actor == nil {
		return fmt.Errorf("actor %d not found", actorID)
	}
	actor.VIPLevel = level
	return nil
}

func (m *mockAccountStore) SaveCurrency(actorID, currencyType int, amount int64) error {
	if m.currencySaveErr != nil {
		return m.currencySaveErr
	}
	actor := m.actors[actorID]
	if actor == nil {
		return fmt.Errorf("actor %d not found", actorID)
	}
	switch currencyType {
	case currencyGold:
		actor.Gold = amount
	case currencyYuanBao:
		actor.YuanBao = amount
	default:
		return fmt.Errorf("unsupported currency type %d", currencyType)
	}
	m.currencyCalls++
	m.currencyType = currencyType
	m.currencyAmount = amount
	return nil
}

func (m *mockAccountStore) Close() {}

func newMockStore() *mockAccountStore {
	return &mockAccountStore{
		accounts: map[string]mockAccount{"帅哥": {id: 121102, passwd: testPassword}},
		actors: map[int]*ActorData{
			98765: {ActorID: 98765, AccountID: 121102, Account: "帅哥", Name: "帅哥", Job: 1, Sex: 0, Level: 55, SceneID: 1, TotalPower: 12345, BagGridCount: 80},
		},
		nextID: 0,
	}
}

func buildLoginFrameBytes(account, md5pass string) []byte {
	p := binary.LittleEndian.AppendUint32(nil, 1)
	p = append(p, cmdLogin, loginSubCmdResult, 1, 0, 0, 0)
	p = appendEgretString(p, account)
	p = appendEgretString(p, md5pass)
	return p
}

func buildRequest(subcmd byte, body ...[]byte) []byte {
	p := binary.LittleEndian.AppendUint32(nil, 1)
	p = append(p, cmdLogin, subcmd)
	for _, part := range body {
		p = append(p, part...)
	}
	return p
}

func buildEnterRequest(actorID int) []byte {
	body := binary.LittleEndian.AppendUint32(nil, uint32(actorID))
	for range 3 {
		body = appendEgretString(body, "")
	}
	return buildRequest(loginSubCmdEnterGame, body)
}

type loginFixture struct {
	router  *NetRouter
	store   *mockAccountStore
	frames  [][]byte
	packets [][]byte
}

func newLoginFixture(t *testing.T) *loginFixture {
	t.Helper()
	g := luahost.NewGopherLua(luahost.Config{OSType: "WIN"})
	t.Cleanup(g.Close)
	t.Cleanup(func() { SetSendHook(nil) })
	f := &loginFixture{store: newMockStore()}
	f.router = NewNetRouter(g)
	f.router.SetSender(func(frame []byte) error {
		f.frames = append(f.frames, append([]byte(nil), frame...))
		f.packets = append(f.packets, append([]byte(nil), frame[netframe.GateFrameHeaderLen+netframe.NetIDLen:]...))
		return nil
	})
	mgr := NewLoginManager(f.store, f.router)
	f.router.SetLoginHandler(mgr.HandleLogin)
	f.router.OpenSession(553, "127.0.0.1")
	return f
}

func (f *loginFixture) send(data []byte) error {
	return f.router.RouteUpstream(append(netframe.BuildNetID(553), data...))
}

func TestParseLoginFrame_RealClientEncoding(t *testing.T) {
	data := buildLoginFrameBytes("帅哥", testPassword)
	frame, err := ParseLoginFrame(data)
	if err != nil {
		t.Fatal(err)
	}
	if frame.Flag != 1 || frame.ServerID != 1 || frame.Account != "帅哥" || frame.Password != testPassword {
		t.Fatalf("unexpected frame: %+v", frame)
	}
	if !bytes.Equal(data, []byte{
		1, 0, 0, 0, 0xff, 1, 1, 0, 0, 0,
		6, 0, 0xe5, 0xb8, 0x85, 0xe5, 0x93, 0xa5, 0, 32, 0,
		'e', '1', '0', 'a', 'd', 'c', '3', '9', '4', '9', 'b', 'a', '5', '9', 'a', 'b',
		'b', 'e', '5', '6', 'e', '0', '5', '7', 'f', '2', '0', 'f', '8', '8', '3', 'e', 0,
	}) {
		t.Fatalf("登录帧编码与现网客户端不一致: %x", data)
	}
}

func TestLoginAuthenticationDoesNotCreateOrBindActor(t *testing.T) {
	f := newLoginFixture(t)
	if err := f.send(buildLoginFrameBytes("帅哥", testPassword)); err != nil {
		t.Fatal(err)
	}
	if len(f.packets) != 1 || !bytes.Equal(f.packets[0], []byte{0xff, 1, 0}) {
		t.Fatalf("登录回包 = %x", f.packets)
	}
	sess := f.router.AuthenticatedSession(553)
	if sess == nil || sess.ActorID != 0 || sess.Account != "帅哥" || sess.AccountID != 121102 || sess.ServerID != 1 {
		t.Fatalf("鉴权会话状态错误: %+v", sess)
	}
	if len(f.store.actors) != 1 {
		t.Fatal("ff01 不应创建角色")
	}
}

func TestRoleListUsesCapturedFF04Request(t *testing.T) {
	f := newLoginFixture(t)
	if err := f.send(buildLoginFrameBytes("帅哥", testPassword)); err != nil {
		t.Fatal(err)
	}
	if err := f.send([]byte{1, 0, 0, 0, 0xff, 4}); err != nil {
		t.Fatal(err)
	}
	if len(f.packets) != 2 {
		t.Fatalf("回包数=%d", len(f.packets))
	}
	p := f.packets[1]
	wantHead := binary.LittleEndian.AppendUint32([]byte{0xff, 4}, uint32(121102))
	wantHead = append(wantHead, 1)
	if !bytes.Equal(p[:7], wantHead) {
		t.Fatalf("ff04 头=%x", p[:7])
	}
	if got := binary.LittleEndian.Uint32(p[7:11]); got != 1 {
		t.Fatalf("角色数=%d", got)
	}
	name, off, err := parseEgretString(p, 15)
	if err != nil || name != "帅哥" || binary.LittleEndian.Uint32(p[off:off+4]) != 1 {
		t.Fatalf("角色摘要解析失败: name=%q off=%d err=%v", name, off, err)
	}
	if binary.LittleEndian.Uint32(p[off+4:off+8]) != 0 || binary.LittleEndian.Uint32(p[off+8:off+12]) != 55 || binary.LittleEndian.Uint64(p[off+12:off+20]) != 12345 {
		t.Fatalf("角色摘要字段错误: %x", p[off:])
	}
}

func TestCreateRoleAndEnterGameChain(t *testing.T) {
	f := newLoginFixture(t)
	if err := f.send(buildLoginFrameBytes("帅哥", testPassword)); err != nil {
		t.Fatal(err)
	}
	if err := f.send([]byte{1, 0, 0, 0, 0xff, 4}); err != nil {
		t.Fatal(err)
	}
	// 使用无角色账号验证 status=0 和后续 ff02/ff05。
	f.store.accounts["新号"] = mockAccount{id: 777, passwd: testPassword}
	if err := f.send(buildLoginFrameBytes("新号", testPassword)); err != nil {
		t.Fatal(err)
	}
	if err := f.send([]byte{1, 0, 0, 0, 0xff, 4}); err != nil {
		t.Fatal(err)
	}
	if got := f.packets[len(f.packets)-1]; len(got) != 7 || got[6] != 0 {
		t.Fatalf("无角色 ff04=%x", got)
	}
	create := appendEgretString(nil, "测试角色")
	create = append(create, 0, 2, 0)
	for range 3 {
		create = appendEgretString(create, "")
	}
	if err := f.send(buildRequest(loginSubCmdCreate, create)); err != nil {
		t.Fatal(err)
	}
	result := f.packets[len(f.packets)-1]
	if result[0] != 0xff || result[1] != 2 || result[6] != 0 {
		t.Fatalf("创角回包=%x", result)
	}
	actorID := int(binary.LittleEndian.Uint32(result[2:6]))
	if err := f.send(buildEnterRequest(actorID)); err != nil {
		t.Fatal(err)
	}
	if got := f.router.AuthenticatedSession(553); got.ActorID != actorID {
		t.Fatalf("ff05 未绑定角色: %+v", got)
	}
	if got := f.packets[len(f.packets)-1]; !bytes.Equal(got, []byte{0xff, 5, 1}) {
		t.Fatalf("进游戏回包=%x", got)
	}
}

func TestEnterGameRepliesSuccessBeforeScenePackets(t *testing.T) {
	f := newLoginFixture(t)
	DefaultActorManager = NewActorManager()
	mgr := NewLoginManager(f.store, f.router)
	mgr.SetPostLoginHandler(NewPostLoginManager(f.store, f.router, nil))
	f.router.SetLoginHandler(mgr.HandleLogin)
	if err := f.send(buildLoginFrameBytes("帅哥", testPassword)); err != nil {
		t.Fatal(err)
	}
	f.packets = nil
	if err := f.send(buildEnterRequest(98765)); err != nil {
		t.Fatal(err)
	}
	if len(f.packets) != 9 {
		t.Fatalf("进游戏回包数=%d", len(f.packets))
	}
	if !bytes.Equal(f.packets[0], []byte{0xff, 5, 1}) {
		t.Fatalf("ff05 必须先于场景包: %x", f.packets[0])
	}
	// {0,19} 是 ClientConfig 初始化包（clientconfig.lua onLogin），在场景包后、背包前。
	wantHeaders := [][2]byte{{0, 1}, {0, 2}, {0, 3}, {0, 4}, {0, 19}, {3, 1}, {3, 1}, {3, 1}}
	for i, want := range wantHeaders {
		got := f.packets[i+1]
		if len(got) < 2 || got[0] != want[0] || got[1] != want[1] {
			t.Fatalf("场景包 %d 头=%x", i, got)
		}
	}
	for i, bagType := range []byte{0, 1, 2} {
		if got := f.packets[i+6]; len(got) < 8 || got[2] != 1 || got[3] != bagType || binary.LittleEndian.Uint32(got[4:8]) != 0 {
			t.Fatalf("背包初始化包 %d=%x", bagType, got)
		}
	}
}

func TestEnterGameRejectsOtherAccountsActor(t *testing.T) {
	f := newLoginFixture(t)
	f.store.accounts["其他账号"] = mockAccount{id: 778, passwd: testPassword}
	if err := f.send(buildLoginFrameBytes("其他账号", testPassword)); err != nil {
		t.Fatal(err)
	}
	if err := f.send(buildEnterRequest(98765)); err != nil {
		t.Fatal(err)
	}
	if got := f.packets[len(f.packets)-1]; !bytes.Equal(got, []byte{0xff, 5, 0}) {
		t.Fatalf("越权进游戏回包=%x", got)
	}
	if sess := f.router.AuthenticatedSession(553); sess.ActorID != 0 {
		t.Fatal("越权请求不应绑定 Actor")
	}
}

func TestRoleCommandsRequireAuthentication(t *testing.T) {
	f := newLoginFixture(t)
	for _, request := range [][]byte{{1, 0, 0, 0, 0xff, 4}, buildEnterRequest(98765)} {
		if err := f.send(request); err == nil {
			t.Fatal("未鉴权会话应拒绝角色命令")
		}
	}
	if len(f.packets) != 0 {
		t.Fatal("未鉴权会话不应收到回包")
	}
}

func TestBadPasswordLeavesSessionUnauthenticated(t *testing.T) {
	f := newLoginFixture(t)
	if err := f.send(buildLoginFrameBytes("帅哥", "00000000000000000000000000000000")); err != nil {
		t.Fatal(err)
	}
	if got := f.packets[0]; !bytes.Equal(got, []byte{0xff, 1, 2}) {
		t.Fatalf("密码错误回包=%x", got)
	}
	if f.router.AuthenticatedSession(553) != nil {
		t.Fatal("密码错误不应鉴权")
	}
}
