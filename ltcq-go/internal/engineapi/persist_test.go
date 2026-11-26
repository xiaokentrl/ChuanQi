package engineapi

import (
	"encoding/binary"
	"testing"

	"github.com/xiaokentrl/ChuanQi/ltcq-go/internal/luahost"
	"github.com/xiaokentrl/ChuanQi/ltcq-go/internal/netframe"
)

type persistMockStore struct {
	*mockAccountStore
	persisted map[int]actorPersistData
	saveCalls int
}

func newPersistMockStore() *persistMockStore {
	return &persistMockStore{
		mockAccountStore: newMockStore(),
		persisted:        map[int]actorPersistData{},
	}
}

func (s *persistMockStore) LoadActorPersist(actorID int) (*actorPersistData, error) {
	p, ok := s.persisted[actorID]
	if !ok {
		return defaultActorPersistData(), nil
	}
	copied := actorPersistData{Version: p.Version, ClientConfig: map[int]int{}, Skills: map[int]int{}, Static: p.Static}
	for key, val := range p.ClientConfig {
		copied.ClientConfig[key] = val
	}
	for skillID, level := range p.Skills {
		copied.Skills[skillID] = level
	}
	return &copied, nil
}

func (s *persistMockStore) SaveActorPersist(actorID int, data *actorPersistData) error {
	s.saveCalls++
	stored := actorPersistData{Version: actorPersistVersion, ClientConfig: map[int]int{}, Skills: map[int]int{}}
	if data != nil {
		stored.Version = data.Version
		stored.Static = data.Static
		for key, val := range data.ClientConfig {
			stored.ClientConfig[key] = val
		}
		for skillID, level := range data.Skills {
			stored.Skills[skillID] = level
		}
	}
	s.persisted[actorID] = stored
	return nil
}

func TestFlushDirtyPersistsClientConfig(t *testing.T) {
	DefaultActorManager = NewActorManager()
	store := newPersistMockStore()
	actor := DefaultActorManager.GetOrCreate(98765)

	actor.SetClientConfig(5, 777)
	if !actor.Dirty || !actor.ClientConfigDirty {
		t.Fatal("ClientConfig 更新应同时标记总脏与待落库")
	}
	if err := flushDirty(actor, store, nil); err != nil {
		t.Fatal(err)
	}
	if actor.ClientConfigDirty || actor.Dirty {
		t.Fatal("落库成功后应清除 ClientConfigDirty 与总脏标记")
	}
	got, err := store.LoadActorPersist(98765)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.ClientConfig) != 1 || got.ClientConfig[5] != 777 {
		t.Fatalf("落库数据错误: %+v", got)
	}
	if err := flushDirty(actor, store, nil); err != nil {
		t.Fatal(err)
	}
	if store.saveCalls != 1 {
		t.Fatalf("无新增脏字段时不应重复落库: saveCalls=%d", store.saveCalls)
	}
}

func TestFlushDirtyPersistsSkills(t *testing.T) {
	DefaultActorManager = NewActorManager()
	store := newPersistMockStore()
	actor := DefaultActorManager.GetOrCreate(98765)

	actor.mu.Lock()
	actor.Skills[101] = 2
	actor.SkillsDirty = true
	actor.Dirty = true
	actor.mu.Unlock()
	if err := flushDirty(actor, store, nil); err != nil {
		t.Fatal(err)
	}
	got, err := store.LoadActorPersist(98765)
	if err != nil {
		t.Fatal(err)
	}
	if got.Skills[101] != 2 {
		t.Fatalf("技能等级未落库: %+v", got.Skills)
	}

	DefaultActorManager = NewActorManager()
	restored := DefaultActorManager.GetOrCreate(98765)
	if err := restoreActorPersist(restored, store, nil); err != nil {
		t.Fatal(err)
	}
	if restored.Skills[101] != 2 || restored.SkillsDirty || restored.Dirty {
		t.Fatalf("技能等级还原错误: %+v", restored)
	}
}

func TestClientConfigHandlerFlushesDirty(t *testing.T) {
	DefaultActorManager = NewActorManager()
	store := newPersistMockStore()
	DefaultActorManager.GetOrCreate(98765)

	body := make([]byte, 8)
	binary.LittleEndian.PutUint32(body[0:4], 5)
	binary.LittleEndian.PutUint32(body[4:8], 777)
	if err := newClientConfigHandler(store, nil)(1, 98765, body); err != nil {
		t.Fatal(err)
	}
	got, err := store.LoadActorPersist(98765)
	if err != nil {
		t.Fatal(err)
	}
	if got.ClientConfig[5] != 777 {
		t.Fatalf("路由处理器未落库 ClientConfig: %+v", got)
	}
}

func TestPostLoginRestoresPersistedClientConfig(t *testing.T) {
	DefaultActorManager = NewActorManager()
	store := newPersistMockStore()
	store.persisted[98765] = actorPersistData{
		Version:      actorPersistVersion,
		ClientConfig: map[int]int{5: 777},
	}

	var packets [][]byte
	router := NewNetRouter(nil)
	defer SetSendHook(nil)
	router.SetSender(func(frame []byte) error {
		packets = append(packets, append([]byte(nil), frame[netframe.GateFrameHeaderLen+netframe.NetIDLen:]...))
		return nil
	})
	router.OpenSession(553, "127.0.0.1")
	router.SetAccount(553, "帅哥", 121102, 1)

	post := NewPostLoginManager(store, router, nil)
	if err := post.HandlePostLogin(553, 98765, "帅哥", nil); err != nil {
		t.Fatal(err)
	}
	var clientConfig []byte
	for _, packet := range packets {
		if len(packet) >= 2 && packet[0] == cmdBase && packet[1] == sBaseCmdClientConfig {
			clientConfig = packet
			break
		}
	}
	if clientConfig == nil {
		t.Fatalf("未找到 ClientConfig 回包: %x", packets)
	}
	if len(clientConfig) < 10 || binary.LittleEndian.Uint16(clientConfig[2:4]) != 1 {
		t.Fatalf("ClientConfig 回包长度/条目数错误: %x", clientConfig)
	}
	if key := int(binary.LittleEndian.Uint32(clientConfig[4:8])); key != 5 {
		t.Fatalf("ClientConfig key=%d want 5", key)
	}
	if val := int(binary.LittleEndian.Uint32(clientConfig[8:12])); val != 777 {
		t.Fatalf("ClientConfig val=%d want 777", val)
	}
	if actor := DefaultActorManager.Get(98765); actor == nil || actor.ClientConfigDirty {
		t.Fatalf("登录还原不应标脏: %+v", actor)
	}
}

func TestStaticVarPersistsAcrossRelogin(t *testing.T) {
	DefaultActorManager = NewActorManager()
	store := newPersistMockStore()

	g1 := luahost.NewGopherLua(luahost.Config{OSType: "WIN"})
	defer g1.Close()
	if err := InstallAll(g1); err != nil {
		t.Fatal(err)
	}
	if err := g1.Load("login-write", `
		function OnActorEvent(actor, eid)
			local st = LActor.getStaticVar(actor)
			st.demoncircledata = {
				[0] = {
					huanhuaid = 2,
					subdemoncircleinfo = { [1] = { arrayeyelv = 3, nutritionexp = 42 } },
				},
			}
		end
	`); err != nil {
		t.Fatal(err)
	}
	router1 := NewNetRouter(g1)
	defer SetSendHook(nil)
	router1.SetSender(func([]byte) error { return nil })
	router1.OpenSession(553, "127.0.0.1")
	router1.SetAccount(553, "帅哥", 121102, 1)
	post1 := NewPostLoginManager(store, router1, g1)
	if err := post1.HandlePostLogin(553, 98765, "帅哥", nil); err != nil {
		t.Fatal(err)
	}
	saved, err := store.LoadActorPersist(98765)
	if err != nil {
		t.Fatal(err)
	}
	if saved.Static == nil {
		t.Fatal("登录事件写入的 staticVar 未落库")
	}

	DefaultActorManager = NewActorManager()
	g2 := luahost.NewGopherLua(luahost.Config{OSType: "WIN"})
	defer g2.Close()
	if err := InstallAll(g2); err != nil {
		t.Fatal(err)
	}
	if err := g2.Load("login-read", `function OnActorEvent(actor, eid) end`); err != nil {
		t.Fatal(err)
	}
	router2 := NewNetRouter(g2)
	router2.SetSender(func([]byte) error { return nil })
	router2.OpenSession(554, "127.0.0.1")
	router2.SetAccount(554, "帅哥", 121102, 1)
	post2 := NewPostLoginManager(store, router2, g2)
	if err := post2.HandlePostLogin(554, 98765, "帅哥", nil); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		expr string
		want int
	}{
		{`(function() local d=LActor.getStaticVar(98765).demoncircledata[0]; return d.huanhuaid end)()`, 2},
		{`(function() local d=LActor.getStaticVar(98765).demoncircledata[0]; return d.subdemoncircleinfo[1].arrayeyelv end)()`, 3},
		{`(function() local d=LActor.getStaticVar(98765).demoncircledata[0]; return d.subdemoncircleinfo[1].nutritionexp end)()`, 42},
	} {
		got, err := g2.DoString(tc.expr)
		if err != nil {
			t.Fatal(err)
		}
		if value, ok := got.(int); !ok || value != tc.want {
			t.Fatalf("%s = %v want %d", tc.expr, got, tc.want)
		}
	}
}
