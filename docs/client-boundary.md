# 客户端可改性边界（client-boundary）

> egret 客户端的**游戏主体 main.min.js 是编译产物黑盒**（源码不在包内，无法重建）。
> 能改的只有"胶水层"。改黑盒 = 不可行，任何需求先对照本表归类。

## 可改层（已验证）

| 文件 | 内容 | 本工程已做的修改 |
|---|---|---|
| `h5game/index.js` | 游戏外壳：区服接口地址、loginType、公告文案、渠道分支 | 域名 localhost 化、显式 `loginType=0` |
| `h5game/login.min.js` | egret 登录场景（区服列表/登录框/公告拉取） | 修复 loginType 覆盖、域名改造 |
| `h5game/index*.html / indexlogin.html / game*.php` | 页面骨架、iframe 装载、start() 转发 | — |
| `h5game/web/**`（PHP） | 建号/区服/充值/公告接口 | 充值与 GM 接口的安全债在此层修 |
| `h5game/res/**` 静态资源 | 界面图片（**只换素材，别碰 JSON 里的数值结构**——218.92 之类是坐标不是 URL） | — |

## 黑盒层（不可改）

- `main.min.js`（游戏主体逻辑）、`libs/modules/egret*`（引擎）、资源打包结构
- 改动需求若命中黑盒 → 只能在**服务端（Lua/Go）**实现同等效果，或放弃

## 已验证的钩子（黑盒留给外部的入口）

1. `window.loginType / pf / serviceListdUrl / setServiceListdUrl` —— indexlogin.html 从 parent 继承
2. `parent.start(param)` —— 登录完成后跳转游戏（ip:port/user 参数在此注入）
3. `window.start / onSelectServer / Login` —— login.min.js 回调外层
4. `window.userData` —— URL 参数对象（indexlogin.html:72-84 解析）

> **将来 wss/TLS 的落点**：客户端 WS 地址由 `parent.start(param)` 注入的 srvaddr/srvport 决定——
> 协议头（ws/wss）是否硬编码在 main.min.js 内未验证，届时先抓包确认（触发条件见 ROADMAP §10）。

## 教训记录（本工程实战）

- login.min.js 内有**运行时全局配置段**（`window.loginType = 1` 硬编码），会覆盖 indexlogin.html 的继承值——
  改外围之前先 grep min.js 内部有无同名覆盖（本次排查耗时主要在此）
- nginx 对 js 设置 `expires 1h`：验证改动时 Ctrl+F5，否则旧缓存误导排查
