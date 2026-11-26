package engineapi

import (
	"encoding/binary"
	"fmt"
	"log/slog"
	"math"

	"github.com/xiaokentrl/ChuanQi/ltcq-go/internal/luahost"
	"github.com/xiaokentrl/ChuanQi/ltcq-go/internal/scene"
)

const aeUserLoginEventID = 13

const (
	cmdBase               = 0
	sBaseCmdActorBaseData = 1
	sBaseCmdRoleData      = 2
	sBaseCmdEnterScene    = 3
	sBaseCmdOtherEntity   = 4
	sBaseCmdFindPath      = 31
	sBaseCmdEntitySyncPos = 13
	sBaseCmdRemoveEntity  = 10
	sBaseCmdHpChange      = 9
	sBaseCmdNotifyFlyHp   = 32
	sBaseCmdUpdateMoney   = 5
	sBaseCmdUpdateExp     = 7
	sBaseCmdClientConfig  = 19 // sBaseCmd_ClientConfig / cBaseCmd_ClientConfig（protocol.lua:121,143）
)

const (
	currencyGold    = 1
	currencyYuanBao = 3
)

type ActorData struct {
	ActorID      int
	AccountID    int
	Account      string
	Name         string
	ServerID     uint32
	Job          int
	Sex          int
	Level        int
	Exp          int64
	HP           int
	MaxHP        int
	SceneID      int
	Gold         int64
	YuanBao      int64
	TotalPower   int64
	BagGridCount int
	VIPLevel     int
	Recharge     int
	CreateTime   int64
	AppID        string
	PfID         string
	GuildID      int
	GuildName    string
	ChapterLevel int
	MonthCard    int
	Reincarnate  int
	ZhuanSheng   int
	Feats        int64
	Soul         int64
	RuneShatter  int64
	RuneExchange int64
	Together1    int64
	Together2    int64
	WeiWang      int
	Status       int
}

type PostLoginHandler interface {
	HandlePostLogin(sid uint16, actorID int, account string, ready func() error) error
}

type postLoginManager struct {
	store  AccountStore
	router *NetRouter
	L      luahost.Interpreter
	World  *scene.World
	Combat *CombatManager
	Skills *SkillManager
}

func NewPostLoginManager(store AccountStore, router *NetRouter, L luahost.Interpreter, worlds ...*scene.World) PostLoginHandler {
	return newPostLoginManager(store, router, L, nil, worlds...)
}

func newPostLoginManager(store AccountStore, router *NetRouter, L luahost.Interpreter, skills *SkillManager, worlds ...*scene.World) PostLoginHandler {
	manager := &postLoginManager{store: store, router: router, L: L, Skills: skills}
	if len(worlds) > 0 {
		manager.World = worlds[0]
	}
	return manager
}

func (m *postLoginManager) HandlePostLogin(sid uint16, actorID int, account string, ready func() error) error {
	data, err := m.store.LoadActorData(actorID)
	if err != nil {
		return fmt.Errorf("角色数据加载失败: %w", err)
	}
	if data.Account != account {
		return fmt.Errorf("角色账号不匹配")
	}
	if sess := m.router.AuthenticatedSession(sid); sess != nil {
		data.ServerID = sess.ServerID
	}
	if m.World != nil {
		if data.SceneID == 0 {
			data.SceneID = 1
		}
		if _, err := m.World.Spawn(data.SceneID); err != nil {
			return fmt.Errorf("角色场景无效: %w", err)
		}
	}
	spawn := m.spawn(data.SceneID)
	items, bagBase, err := m.loadBag(actorID)
	if err != nil {
		return fmt.Errorf("背包数据加载失败: %w", err)
	}
	m.applyActorDataAt(data, spawn)
	actor := DefaultActorManager.Get(actorID)
	if actor == nil {
		return fmt.Errorf("内存角色不存在: %d", actorID)
	}
	codec := tableCodecOf(m.L)
	if err := restoreActorPersist(actor, m.store, codec); err != nil {
		return err
	}
	if err := m.Skills.EnsureInitialized(actor, m.store, codec); err != nil {
		return fmt.Errorf("角色技能初始化失败: %w", err)
	}
	demonData, err := loadDemonCircleData(actor, codec)
	if err != nil {
		slog.Warn("法阵静态数据读取失败，使用默认值", "actor", actorID, "err", err)
	}
	var bagStore BagStore
	if store, ok := m.store.(BagStore); ok {
		bagStore = store
	}
	actor.ReplaceBag(items, bagBase, bagStore)
	if store, ok := m.store.(VIPLevelStore); ok {
		actor.SetVIPStore(store)
	}
	if store, ok := m.store.(CurrencyStore); ok {
		actor.SetCurrencyStore(store)
	} else {
		actor.SetCurrencyStore(nil)
	}
	actor.SetBagNotifier(func(packet []byte) error { return m.router.SendToSession(sid, packet) })
	packets := [][]byte{
		buildActorBaseData(data),
		buildRoleDataWithDemonSkills(data, demonData, actor),
		buildEnterSceneAt(data, spawn),
		buildOtherEntityAt(data, spawn),
	}
	packets = append(packets, buildClientConfig(actor))
	packets = append(packets, buildBagInitPackets(actorID, actor.BagSnapshot())...)
	if ready != nil {
		if err := ready(); err != nil {
			return err
		}
	}
	for _, packet := range packets {
		if err := m.router.SendToSession(sid, packet); err != nil {
			return err
		}
	}
	if m.World != nil {
		x, y := spawn.x, spawn.y
		observer := func(delivery scene.Delivery) error {
			packet, err := m.buildWorldPacket(delivery.Event)
			if err != nil {
				return err
			}
			return m.router.SendToSession(sid, packet)
		}
		actor.mu.RLock()
		hp, maxHP := actor.HP, actor.MaxHP
		actor.mu.RUnlock()
		deliveries, err := m.World.AddPlayerWithHealth(actorID, data.SceneID, x, y, hp, maxHP, observer)
		if err != nil {
			return fmt.Errorf("加入场景 AOI 失败: %w", err)
		}
		if err := m.World.Dispatch(deliveries); err != nil {
			slog.Warn("场景 AOI 进入广播失败", "actor", actorID, "err", err)
		}
	}
	if m.L != nil {
		if _, err := FirePCall(m.L, 500, "OnActorEvent", actorID, aeUserLoginEventID); err != nil {
			if m.Combat != nil {
				m.Combat.RemovePlayerAI(actorID)
			}
			if m.World != nil {
				if dispatchErr := m.World.Dispatch(m.World.RemovePlayer(actorID)); dispatchErr != nil {
					slog.Warn("场景 AOI 登录回滚失败", "actor", actorID, "err", dispatchErr)
				}
			}
			return fmt.Errorf("aeUserLogin 事件链触发失败: %w", err)
		}
		// 登录事件里的 Lua 会初始化/改写 staticVar；此处落一次快照，
		// 避免 Lua 表字段写入无法被 Go setter 拦截导致的数据丢失。
		markStaticDirty(actor)
		if err := flushDirty(actor, m.store, codec); err != nil {
			slog.Warn("登录后持久化刷新失败", "actor", actorID, "err", err)
		}
	}
	return nil
}

// handleClientConfig 处理 cBaseCmd_ClientConfig(19) 更新请求，对齐 clientconfig.lua:28-48：
// body = [4B key][4B val]；更新 Actor.ClientConfig（静态数据，标脏待持久化）。
func handleClientConfig(sid uint16, actorID int, body []byte) error {
	if len(body) < 8 {
		return fmt.Errorf("ClientConfig 请求体过短: %d", len(body))
	}
	key := int(binary.LittleEndian.Uint32(body[0:4]))
	val := int(binary.LittleEndian.Uint32(body[4:8]))
	actor := DefaultActorManager.Get(actorID)
	if actor == nil {
		return fmt.Errorf("ClientConfig actor 不存在: %d", actorID)
	}
	actor.SetClientConfig(key, val)
	slog.Info("客户端配置更新", "actorID", actorID, "key", key, "val", val)
	return nil
}

type sceneSpawnResult struct {
	mapID, x, y, width, height int
	name, desc                 string
}

func (m *postLoginManager) spawn(sceneID int) sceneSpawnResult {
	mapID, x, y, width, height, name, desc := sceneSpawn(sceneID)
	if m.World != nil {
		if sceneID == 0 {
			sceneID = 1
		}
		if cfg, err := m.World.Spawn(sceneID); err == nil {
			mapID, x, y, width, height = sceneID, cfg.SpawnX, cfg.SpawnY, cfg.Width, cfg.Height
		}
	}
	return sceneSpawnResult{mapID: mapID, x: x, y: y, width: width, height: height, name: name, desc: desc}
}

func (m *postLoginManager) applyActorData(d *ActorData) {
	m.applyActorDataAt(d, m.spawn(d.SceneID))
}

func (m *postLoginManager) loadBag(actorID int) (map[int]int, []byte, error) {
	store, ok := m.store.(BagStore)
	if !ok {
		return map[int]int{}, nil, nil
	}
	blob, err := store.LoadBagBlob(actorID)
	if err != nil {
		return nil, nil, err
	}
	return decodeBagBlob(blob)
}

func (m *postLoginManager) applyActorDataAt(d *ActorData, spawn sceneSpawnResult) {
	actor := DefaultActorManager.GetOrCreate(d.ActorID)
	actor.mu.Lock()
	defer actor.mu.Unlock()
	x, y := spawn.x, spawn.y
	d.HP, d.MaxHP = 100, 100
	actor.Name = d.Name
	actor.Account = d.Account
	actor.AccountID = d.AccountID
	actor.Level = d.Level
	actor.Exp = d.Exp
	actor.ServerID = d.ServerID
	actor.TotalPower = d.TotalPower
	actor.Job = d.Job
	actor.ZhuanSheng = d.ZhuanSheng
	actor.Sex = d.Sex
	actor.SceneID = d.SceneID
	actor.X, actor.Y = x, y
	actor.HP, actor.MaxHP = 100, 100
	actor.VIPLevel = d.VIPLevel
	actor.Recharge = d.Recharge
	actor.CreateTime = d.CreateTime
	actor.AppID = d.AppID
	actor.PfID = d.PfID
	actor.GuildID = d.GuildID
	actor.GuildName = d.GuildName
	actor.ChapterLevel = d.ChapterLevel
	actor.MonthCard = d.MonthCard
	actor.Reincarnate = d.Reincarnate
	if actor.Currency == nil {
		actor.Currency = map[int]int{}
	}
	actor.Currency[currencyGold] = int(d.Gold)
	actor.Currency[currencyYuanBao] = int(d.YuanBao)
}

// buildClientConfig 构造 sBaseCmd_ClientConfig(19) 包，对齐 clientconfig.lua:50-62 的 onLogin：
//
//	[2B 配置项数量][每项 4B key + 4B val]
//
// 原版从 staticVar.cliCfg（key 0/1 来自 p1/p2）读取；Go 侧用 Actor.ClientConfig 承载。
func buildClientConfig(actor *Actor) []byte {
	p := []byte{cmdBase, sBaseCmdClientConfig}
	items := actor.ClientConfigSnapshot()
	p = binary.LittleEndian.AppendUint16(p, uint16(len(items)))
	for _, kv := range items {
		p = binary.LittleEndian.AppendUint32(p, uint32(kv[0]))
		p = binary.LittleEndian.AppendUint32(p, uint32(kv[1]))
	}
	return p
}

func buildActorBaseData(d *ActorData) []byte {
	p := []byte{cmdBase, sBaseCmdActorBaseData}
	p = binary.LittleEndian.AppendUint64(p, math.Float64bits(float64(d.ActorID)))
	p = binary.LittleEndian.AppendUint32(p, uint32(d.ActorID))
	p = binary.LittleEndian.AppendUint32(p, d.ServerID)
	name := make([]byte, 33)
	copy(name, []byte(d.Name))
	p = append(p, name...)
	p = binary.LittleEndian.AppendUint32(p, uint32(d.Level))
	p = binary.LittleEndian.AppendUint32(p, uint32(d.Exp))
	p = binary.LittleEndian.AppendUint64(p, 0)
	p = binary.LittleEndian.AppendUint64(p, uint64(d.Gold))
	p = binary.LittleEndian.AppendUint64(p, uint64(d.YuanBao))
	p = binary.LittleEndian.AppendUint32(p, uint32(d.VIPLevel))
	p = binary.LittleEndian.AppendUint64(p, uint64(d.Soul))
	p = binary.LittleEndian.AppendUint32(p, uint32(d.BagGridCount))
	p = binary.LittleEndian.AppendUint64(p, uint64(d.Feats))
	p = binary.LittleEndian.AppendUint64(p, uint64(d.RuneShatter))
	p = binary.LittleEndian.AppendUint64(p, uint64(d.RuneExchange))
	p = binary.LittleEndian.AppendUint64(p, uint64(d.Together1))
	p = binary.LittleEndian.AppendUint64(p, uint64(d.Together2))
	p = binary.LittleEndian.AppendUint32(p, uint32(d.WeiWang))
	return p
}

func buildRoleData(d *ActorData) []byte {
	return buildRoleDataWithDemon(d, demonCircleData{})
}

func buildRoleDataWithDemon(d *ActorData, demon demonCircleData) []byte {
	return buildRoleDataWithDemonSkills(d, demon, nil)
}

func buildRoleDataWithDemonSkills(d *ActorData, demon demonCircleData, actor *Actor) []byte {
	p := []byte{cmdBase, sBaseCmdRoleData}
	p = binary.LittleEndian.AppendUint16(p, 1)
	p = binary.LittleEndian.AppendUint32(p, 0)
	p = binary.LittleEndian.AppendUint32(p, 0)
	job := d.Job
	if job < 1 || job > 3 {
		job = 1
	}
	p = binary.LittleEndian.AppendUint32(p, uint32(job))
	p = binary.LittleEndian.AppendUint32(p, uint32(d.Sex))
	p = binary.LittleEndian.AppendUint64(p, uint64(d.TotalPower))
	skills := map[int]int{}
	skillJob := job
	if actor != nil {
		actor.mu.RLock()
		skillJob = actor.Job
		for id, level := range actor.Skills {
			skills[id] = level
		}
		actor.mu.RUnlock()
	}
	if skillJob >= 1 && skillJob <= 3 {
		for slot := 1; slot <= 5; slot++ {
			p = binary.LittleEndian.AppendUint32(p, uint32(skills[skillBaseID(skillJob, slot)]))
		}
	} else {
		p = append(p, make([]byte, 20)...)
	}
	p = append(p, make([]byte, 20)...)
	p = binary.LittleEndian.AppendUint32(p, uint32(d.ZhuanSheng))
	p = append(p, make([]byte, 6)...)
	p = append(p, make([]byte, 13*(7*4+144))...)
	p = append(p, make([]byte, 4*4)...)
	p = append(p, make([]byte, 20)...)
	p = append(p, make([]byte, 8)...)
	p = append(p, make([]byte, 13)...)
	p = append(p, make([]byte, 8*144)...)
	p = append(p, make([]byte, 8*4)...)
	p = binary.LittleEndian.AppendUint32(p, 0)
	p = binary.LittleEndian.AppendUint16(p, 0)
	p = binary.LittleEndian.AppendUint16(p, 0)
	p = binary.LittleEndian.AppendUint16(p, 0)
	p = append(p, make([]byte, 3*4)...)
	p = append(p, make([]byte, 8*2)...)
	p = append(p, make([]byte, 8*5*2)...)
	return appendDemonCircleData(p, demon)
}

// 法阵数据布局常量（权威：demoncircle.lua:1160 packDemonCircleData；
// config/demoncir/demoncirbase.config / demoncirskill.config）。
const (
	demonCircleCount   = 6 // DemonCirBaseConfig 子法阵数量
	demonTezhiCount    = 4 // DemonCirSkillConfig 特质数量
	demonQiannengCount = 5 // 每个子阵潜能数量
)

type demonCircleSubData struct {
	EyeLevel     uint16
	NutritionLv  uint16
	NutritionExp uint32
	StudyCount   uint16
	TezhiLevel   [demonTezhiCount]uint16
	ExtraValue   [demonQiannengCount]uint32
}

type demonCircleData struct {
	HuanhuaID uint16
	Subs      [demonCircleCount]demonCircleSubData
}

// demonQiannengStudyRequirements 来自 demoncirqianneng.config 的 count 字段。
// 原版 GetQiannengActiveFlag：arrayeyelv>0 且 studycount>=count 才激活。
var demonQiannengStudyRequirements = map[uint16]uint16{
	1: 0, 2: 0,
	11: 5, 12: 5,
	21: 10, 22: 10,
	31: 15, 32: 15,
	41: 20, 51: 20, 61: 20, 71: 20, 81: 20, 91: 20,
}

func loadDemonCircleData(actor *Actor, codec luahost.TableCodec) (demonCircleData, error) {
	var out demonCircleData
	if actor == nil || codec == nil {
		return out, nil
	}
	actor.mu.RLock()
	static := actor.Static
	actor.mu.RUnlock()
	if static == nil {
		return out, nil
	}
	root, err := codec.EncodeValue(static)
	if err != nil {
		return out, err
	}
	demons, ok := root.Field("demoncircledata")
	if !ok {
		return out, nil
	}
	role, ok := demons.Index(0)
	if !ok {
		return out, nil
	}
	if node, ok := role.Field("huanhuaid"); ok {
		out.HuanhuaID = demonUint16(node.AsInt())
	}
	subs, ok := role.Field("subdemoncircleinfo")
	if !ok {
		return out, nil
	}
	for sub := 0; sub < demonCircleCount; sub++ {
		subNode, ok := subs.Index(sub + 1)
		if !ok {
			continue
		}
		target := &out.Subs[sub]
		if node, ok := subNode.Field("arrayeyelv"); ok {
			target.EyeLevel = demonUint16(node.AsInt())
		}
		if node, ok := subNode.Field("nutritionlv"); ok {
			target.NutritionLv = demonUint16(node.AsInt())
		}
		if node, ok := subNode.Field("nutritionexp"); ok {
			target.NutritionExp = demonUint32(node.AsInt())
		}
		if node, ok := subNode.Field("studycount"); ok {
			target.StudyCount = demonUint16(node.AsInt())
		}
		if tezhiNode, ok := subNode.Field("tezhiinfo"); ok {
			for tezhi := 0; tezhi < demonTezhiCount; tezhi++ {
				if node, ok := tezhiNode.Index(tezhi + 1); ok {
					target.TezhiLevel[tezhi] = demonUint16(node.AsInt())
				}
			}
		}
		if potentialNode, ok := subNode.Field("potentialpoint"); ok {
			for idx, qid := range demonQiannengIDs[sub] {
				qnode, ok := potentialNode.Index(int(qid))
				if !ok {
					continue
				}
				if node, ok := qnode.Field("extravalue"); ok {
					target.ExtraValue[idx] = demonUint32(node.AsInt())
				}
			}
		}
	}
	return out, nil
}

func demonUint16(v int) uint16 {
	if v <= 0 {
		return 0
	}
	if v > 0xffff {
		return 0xffff
	}
	return uint16(v)
}

func demonUint32(v int) uint32 {
	if v <= 0 {
		return 0
	}
	return uint32(v)
}

// 每个子阵的潜能 id（demoncirbase.config qianneng 字段；客户端按此 id 索引 QNData）。
var demonQiannengIDs = [demonCircleCount][demonQiannengCount]uint16{
	{1, 11, 21, 31, 41},
	{1, 11, 21, 31, 51},
	{2, 12, 22, 32, 61},
	{2, 12, 22, 32, 71},
	{1, 11, 21, 31, 81},
	{1, 11, 21, 31, 91},
}

// appendDemonCircleData 在 RoleData 末尾追加法阵数据段。
//
// 原版由 C++ 在组角色包时回调 Lua `_G.packDemonCircleData`（demoncircle.lua:1160），
// Go 侧无该 C++→Lua 回调，故在此直接按同布局组包；demon 为零值时全部填 0（未激活），
// 保证客户端 SubRoles.parser 能构造出 6 个 MatrixInfo，红点刷新不再读 undefined.Lv。
//
// 布局（全部小端）：
//
//	[2B huanhuaId][2B 子阵数=6]
//	×6 { [2B subId][2B Lv][2B WYLv][4B WYEXP]
//	     [2B 特质数=4] ×4{[2B id][2B level]}
//	     [2B 领悟次数][2B 潜能数=5] ×5{[2B id][2B 激活][4B 额外值]} }
func appendDemonCircleData(p []byte, demon demonCircleData) []byte {
	p = binary.LittleEndian.AppendUint16(p, demon.HuanhuaID)
	p = binary.LittleEndian.AppendUint16(p, demonCircleCount)
	for sub := 0; sub < demonCircleCount; sub++ {
		src := demon.Subs[sub]
		p = binary.LittleEndian.AppendUint16(p, uint16(sub+1)) // subId
		p = binary.LittleEndian.AppendUint16(p, src.EyeLevel)  // Lv 阵眼等级
		p = binary.LittleEndian.AppendUint16(p, src.NutritionLv)
		p = binary.LittleEndian.AppendUint32(p, src.NutritionExp)
		p = binary.LittleEndian.AppendUint16(p, demonTezhiCount)
		for tezhi := 1; tezhi <= demonTezhiCount; tezhi++ {
			p = binary.LittleEndian.AppendUint16(p, uint16(tezhi))           // 特质 id
			p = binary.LittleEndian.AppendUint16(p, src.TezhiLevel[tezhi-1]) // 特质等级
		}
		p = binary.LittleEndian.AppendUint16(p, src.StudyCount) // 领悟次数
		p = binary.LittleEndian.AppendUint16(p, demonQiannengCount)
		for idx, qid := range demonQiannengIDs[sub] {
			p = binary.LittleEndian.AppendUint16(p, qid)
			active := uint16(0)
			if src.EyeLevel > 0 && src.StudyCount >= demonQiannengStudyRequirements[qid] {
				active = 1
			}
			p = binary.LittleEndian.AppendUint16(p, active)
			p = binary.LittleEndian.AppendUint32(p, src.ExtraValue[idx])
		}
	}
	return p
}

func sceneSpawn(sceneID int) (mapID, x, y, width, height int, name, desc string) {
	if sceneID == 0 {
		sceneID = 1
	}
	mapID, width, height, name = sceneID, 32767, 32767, "场景"
	switch sceneID {
	case 1:
		mapID, x, y, width, height = 1, 56*64, 37*64, 63*64, 43*64
		name, desc = "比奇省", "主城"
	case 41:
		x, y, width, height = 77*64, 22*64, 83*64, 63*64
		name = "比奇矿洞"
	}
	return
}

func buildEnterScene(d *ActorData) []byte {
	mapID, x, y, _, _, name, desc := sceneSpawn(d.SceneID)
	return buildEnterSceneAt(d, sceneSpawnResult{mapID: mapID, x: x, y: y, name: name, desc: desc})
}

func buildEnterSceneAt(d *ActorData, spawn sceneSpawnResult) []byte {
	p := []byte{cmdBase, sBaseCmdEnterScene}
	p = binary.LittleEndian.AppendUint32(p, 0)
	p = binary.LittleEndian.AppendUint32(p, uint32(spawn.mapID))
	p = binary.LittleEndian.AppendUint16(p, uint16(spawn.x))
	p = binary.LittleEndian.AppendUint16(p, uint16(spawn.y))
	p = append(p, 0)
	p = appendEgretString(p, spawn.name)
	p = appendEgretString(p, spawn.desc)
	return p
}

func buildOtherEntity(d *ActorData) []byte {
	_, x, y, _, _, _, _ := sceneSpawn(d.SceneID)
	return buildOtherEntityAt(d, sceneSpawnResult{x: x, y: y})
}

func buildOtherEntityAt(d *ActorData, spawn sceneSpawnResult) []byte {
	hp, maxHP := d.HP, d.MaxHP
	if hp <= 0 || maxHP < hp {
		hp, maxHP = 100, 100
	}
	p := []byte{cmdBase, sBaseCmdOtherEntity}
	p = binary.LittleEndian.AppendUint16(p, 1)
	p = binary.LittleEndian.AppendUint64(p, math.Float64bits(float64(d.ActorID)))
	p = binary.LittleEndian.AppendUint32(p, 0)
	p = binary.LittleEndian.AppendUint64(p, math.Float64bits(float64(d.ActorID)))
	p = binary.LittleEndian.AppendUint32(p, uint32(spawn.x))
	p = binary.LittleEndian.AppendUint32(p, uint32(spawn.y))
	p = binary.LittleEndian.AppendUint16(p, 3)
	p = binary.LittleEndian.AppendUint64(p, math.Float64bits(float64(hp)))
	p = binary.LittleEndian.AppendUint64(p, math.Float64bits(float64(maxHP)))
	p = binary.LittleEndian.AppendUint64(p, math.Float64bits(float64(maxHP)))
	p = binary.LittleEndian.AppendUint16(p, 0)
	p = appendEgretString(p, d.Name)
	p = binary.LittleEndian.AppendUint32(p, d.ServerID)
	job := d.Job
	if job < 1 || job > 3 {
		job = 1
	}
	p = append(p, byte(job), byte(d.Sex))
	p = binary.LittleEndian.AppendUint32(p, uint32(d.Level))
	p = append(p, make([]byte, 8+4*4)...)
	p = appendEgretString(p, "")
	p = append(p, make([]byte, 3*4+4+2+8*4+4)...)
	return p
}

func buildMonsterEntity(entity scene.Entity) []byte {
	p := []byte{cmdBase, sBaseCmdOtherEntity}
	p = binary.LittleEndian.AppendUint16(p, 2)
	p = binary.LittleEndian.AppendUint64(p, math.Float64bits(float64(entity.ID)))
	p = binary.LittleEndian.AppendUint32(p, uint32(entity.ConfigID))
	p = binary.LittleEndian.AppendUint64(p, math.Float64bits(0))
	p = binary.LittleEndian.AppendUint32(p, uint32(entity.X))
	p = binary.LittleEndian.AppendUint32(p, uint32(entity.Y))
	p = binary.LittleEndian.AppendUint16(p, 3)
	for _, value := range []float64{float64(entity.HP), 0, float64(entity.MaxHP)} {
		p = binary.LittleEndian.AppendUint64(p, math.Float64bits(value))
	}
	p = binary.LittleEndian.AppendUint16(p, 0)
	p = binary.LittleEndian.AppendUint16(p, 0)
	return append(p, 0)
}

func buildHpChange(handle int, hp int) []byte {
	p := []byte{cmdBase, sBaseCmdHpChange}
	p = binary.LittleEndian.AppendUint64(p, math.Float64bits(float64(handle)))
	return binary.LittleEndian.AppendUint64(p, math.Float64bits(float64(hp)))
}

func buildNotifyFlyHp(attackerID, targetID, damage int) []byte {
	p := []byte{cmdBase, sBaseCmdNotifyFlyHp}
	p = binary.LittleEndian.AppendUint64(p, math.Float64bits(float64(attackerID)))
	p = binary.LittleEndian.AppendUint64(p, math.Float64bits(float64(targetID)))
	p = binary.LittleEndian.AppendUint64(p, math.Float64bits(float64(damage)))
	return binary.LittleEndian.AppendUint32(p, 0)
}

func (m *postLoginManager) buildWorldPacket(event scene.Event) ([]byte, error) {
	entity := event.Entity
	if event.Kind == scene.EventLeave {
		packet := []byte{cmdBase, sBaseCmdRemoveEntity}
		return binary.LittleEndian.AppendUint64(packet, math.Float64bits(float64(entity.ID))), nil
	}
	if event.Kind == scene.EventHP {
		return buildHpChange(entity.ID, entity.HP), nil
	}
	if event.Kind == scene.EventDamage {
		return buildNotifyFlyHp(event.Attacker.ID, entity.ID, event.Damage), nil
	}
	if event.Kind == scene.EventMoney {
		return buildGoldChange(currencyGold, event.Gold), nil
	}
	if event.Kind == scene.EventExp {
		return buildExpChange(event.Level, event.Exp, event.ExpAdded), nil
	}
	if event.Kind == scene.EventMove {
		distance := absInt(entity.X-event.From.X) + absInt(entity.Y-event.From.Y)
		mode, duration := uint16(0), 0
		if distance > 0 {
			mode, duration = 1, min(distance/4, 3000)
		}
		return buildEntitySyncPosition(float64(entity.ID), entity.X, entity.Y, mode, duration), nil
	}
	if entity.Kind == scene.EntityMonster {
		return buildMonsterEntity(entity), nil
	}
	actor := DefaultActorManager.Get(entity.ID)
	if actor == nil {
		return nil, fmt.Errorf("AOI 实体角色不存在: %d", entity.ID)
	}
	actor.mu.RLock()
	data := ActorData{
		ActorID: actor.ID, Name: actor.Name, ServerID: actor.ServerID,
		Job: actor.Job, Sex: actor.Sex, Level: actor.Level, SceneID: actor.SceneID,
		HP: actor.HP, MaxHP: actor.MaxHP,
	}
	actor.mu.RUnlock()
	return buildOtherEntityAt(&data, sceneSpawnResult{x: entity.X, y: entity.Y}), nil
}
