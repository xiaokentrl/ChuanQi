package engineapi

import (
	"github.com/xiaokentrl/ChuanQi/ltcq-go/internal/luahost"
)

// Engine 组装完整的 Go gameworld Lua 环境（阶段4 全部子项的集成入口）。
type Engine struct {
	Lua    *luahost.GopherLua
	Router *NetRouter
	Timer  *TimerDriver
	Login  *LoginManager // 引擎级登录状态机（dsn 非空时启用）
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
	e, err := Bootstrap(dataRoot, ostype, sendFn)
	if err != nil || dsn == "" {
		return e, err
	}
	store, err := NewMySQLAccountStore(dsn)
	if err != nil {
		e.Close()
		return nil, err
	}
	login := NewLoginManager(store, e.Router)
	e.Router.SetLoginHandler(login.HandleLogin)
	e.Login = login
	return e, nil
}

// Close 释放引擎资源。
func (e *Engine) Close() {
	if e.Login != nil && e.Login.store != nil {
		e.Login.store.Close()
	}
	if e.Lua != nil {
		e.Lua.Close()
	}
}
