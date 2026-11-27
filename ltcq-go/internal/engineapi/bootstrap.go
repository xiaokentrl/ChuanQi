package engineapi

import (
	"log/slog"

	"github.com/xiaokentrl/ChuanQi/ltcq-go/internal/luahost"
	"github.com/xiaokentrl/ChuanQi/ltcq-go/internal/scene"
)

// Engine 组装完整的 Go gameworld Lua 环境（阶段4 全部子项的集成入口）。
type Engine struct {
	Lua    *luahost.GopherLua
	Router *NetRouter
	Timer  *TimerDriver
	Login  *LoginManager  // 引擎级登录状态机（dsn 非空时启用）
	World  *scene.World   // 进程级共享场景与 AOI
	Combat *CombatManager // 进程级共享战斗与 AI 状态
	Skills *SkillManager  // 进程级共享技能状态与冷却
}

// Bootstrap 创建并初始化完整引擎：
// 枚举注入 → API 注册 → System stub → 五大注册表 → 定时器 → 网络路由。
// dataRoot 为 gameworld/data 目录；sendFn 为下行写网关连接的回调（可为 nil）。
func Bootstrap(dataRoot, ostype string, sendFn func(frame []byte) error) (*Engine, error) {
	if ostype == "" {
		ostype = "WIN"
	}
	g := luahost.NewGopherLua(luahost.Config{DataRoot: dataRoot, OSType: ostype})

	// 1. 枚举（tt*/mt*/dt*/ae* + SystemId 占位）
	if err := g.InjectEnums(); err != nil {
		g.Close()
		return nil, err
	}
	// 2. L* API 全注册（LDataPack 真实 + LActor 骨架/真实）
	if err := InstallAll(g); err != nil {
		g.Close()
		return nil, err
	}
	// 3. System stub + 五大注册表
	InstallSystemStubs(g)
	if _, err := LoadRegistries(g, dataRoot); err != nil {
		g.Close()
		return nil, err
	}
	// 4. 定时器四层（postscripttimer 依赖 actorevent，已在步骤3加载）
	timer := NewTimerDriver(g)
	if err := timer.Install(dataRoot); err != nil {
		g.Close()
		return nil, err
	}
	// 5. 网络路由（安装 flush→网关帧 SendHook）
	router := NewNetRouter(g)
	if sendFn != nil {
		router.SetSender(sendFn)
	}

	return &Engine{Lua: g, Router: router, Timer: timer}, nil
}

// BootstrapWithLogin 在 Bootstrap 基础上启用引擎级登录状态机。
// dsn 为空则退化为无登录（仅转发/OnNetMsg）。
func BootstrapWithLogin(dataRoot, ostype, dsn string, sendFn func(frame []byte) error) (*Engine, error) {
	return bootstrapWithLogin(dataRoot, ostype, dsn, nil, nil, nil, sendFn)
}

func BootstrapWithLoginWorld(dataRoot, ostype, dsn string, world *scene.World, sendFn func(frame []byte) error) (*Engine, error) {
	return BootstrapWithLoginRuntime(dataRoot, ostype, dsn, world, nil, nil, sendFn)
}

func BootstrapWithLoginRuntime(dataRoot, ostype, dsn string, world *scene.World, combat *CombatManager, skills *SkillManager, sendFn func(frame []byte) error) (*Engine, error) {
	return bootstrapWithLogin(dataRoot, ostype, dsn, world, combat, skills, sendFn)
}

func bootstrapWithLogin(dataRoot, ostype, dsn string, world *scene.World, combat *CombatManager, skills *SkillManager, sendFn func(frame []byte) error) (*Engine, error) {
	e, err := Bootstrap(dataRoot, ostype, sendFn)
	if err != nil {
		return nil, err
	}
	setDefaultWorld(world)
	installCombatAPI(e.Lua, combat)
	e.Combat = combat
	if dsn == "" {
		return e, nil
	}
	store, err := NewMySQLAccountStore(dsn)
	if err != nil {
		e.Close()
		return nil, err
	}
	login := NewLoginManager(store, e.Router)
	postLogin := newPostLoginManager(store, e.Router, e.Lua, skills, world).(*postLoginManager)
	postLogin.Combat = combat
	login.SetPostLoginHandler(postLogin)
	e.Router.SetLoginHandler(login.HandleLogin)
	movement := NewMovementManager(e.Router, world)
	e.Router.SetMovementHandler(movement.HandleMove)
	e.Router.SetBagHandler(newBagUseHandler(e.Router))
	if combat != nil {
		// 地面掉落拾取上行（[01][01] flag=0）；combat 为 nil 时无掉落源，不安装。
		e.Router.SetDropPickupHandler(newDropPickupHandler(combat, world))
	}
	codec := tableCodecOf(e.Lua)
	e.Router.SetClientConfigHandler(newClientConfigHandler(store, codec))
	if skills != nil {
		e.Skills = skills
		e.Router.SetSkillHandler(func(sid uint16, actorID int, cmdid uint16, body []byte) error {
			return skills.Handle(sid, actorID, cmdid, body, e.Router, store, codec)
		})
	}
	if world != nil {
		e.World = world
		installWorldCloseHandler(e.Router, world, combat, store, codec)
	}
	e.Login = login
	e.Timer.Start()
	return e, nil
}

func installWorldCloseHandler(router *NetRouter, world *scene.World, combat *CombatManager, store AccountStore, codec luahost.TableCodec) {
	router.SetCloseHandler(func(_ uint16, actorID int) {
		if actorID == 0 {
			return
		}
		if combat != nil {
			combat.RemovePlayerAI(actorID)
		}
		if err := world.Dispatch(world.RemovePlayer(actorID)); err != nil {
			slog.Warn("场景 AOI 离场广播失败", "actor", actorID, "err", err)
		}
		actor := DefaultActorManager.Get(actorID)
		markStaticDirty(actor)
		if err := flushDirty(actor, store, codec); err != nil {
			slog.Warn("离场持久化刷新失败", "actor", actorID, "err", err)
		}
	})
}

// Close 释放引擎资源。
func (e *Engine) Close() {
	if e.Timer != nil {
		e.Timer.Close()
	}
	if e.Login != nil && e.Login.store != nil {
		e.Login.store.Close()
	}
	if e.Lua != nil {
		e.Lua.Close()
	}
}
