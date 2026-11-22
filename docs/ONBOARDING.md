# 新维护者上手（ONBOARDING）

> 目标：30 分钟内完成"改一行代码 → 验证生效"的完整闭环。
> 约定：代码注释与文档统一**中文**，标识符**英文**；每个包必须有 doc.go。

## 一、环境速查

| 组件 | 版本/位置 |
|---|---|
| Go | 1.27.4 `D:\tools\go`（GOPROXY=goproxy.cn 已配） |
| MySQL | 5.7.44（EServer 内置 `D:\tools\EServer-data\childApp\server\mysql-5.7\bin`，root/123456） |
| PHP/nginx | EServer，站点 80 端口（vhost: `EServer-data\childApp\server\nginx\conf\vhosts\localhost_80.conf`） |
| 原版基准 | `data/server/s1/Debug/` 四件套（按 dbserver→gameworld→gateway→loggerserver 顺序启动） |
| 客户端 | http://localhost/ （hosts 已配 www.wkzy.net→127.0.0.1，但统一用 localhost） |

## 二、30 分钟闭环（改一行 Lua 看效果）

1. 打开 `data/server/s1/Debug/gameworld/data/functions/systems/` 任选一个系统 Lua
2. 改一处文案或数值（如商店价格表）
3. 重启 gameworld 进程（见 RUNBOOK 启动顺序）
4. 浏览器 http://localhost/ 登录进游戏验证
5. 完事——**Lua 层改动无需编译、无需测试门禁**（热更层就是为此设计的）

## 三、变更影响分级表（改哪里就要跑什么）

| 你改了什么 | 必须通过 | 说明 |
|---|---|---|
| `data/**/*.lua` 脚本层 | 重启生效即可 | 热更层，零门槛，最常改 |
| `data/**/*.config/.txt` 配置 | 重启 + 抽查相关系统 | xlsx 导出的 Lua 表 |
| `ltcq-go/internal/engineapi/*` 单个绑定 | 该绑定单测 + golden 编解码测试 | 契约层，改错=客户端协议错 |
| `ltcq-go/internal/netframe/*` 帧处理 | **全量对拍回归** | 字节层，动一行可能破坏兼容 |
| `ltcq-go/internal/luahost/*` | 726 文件加载冒烟 + 全量回归 | 内核层，非必要不动 |

## 四、代码规范

- engineapi 绑定一律走 **registry 表驱动**（`internal/engineapi/registry.go` 单一事实源），不手写 Register
- 语义注释格式：`// 原版: <C++位置> | Lua调用: N处 | 规格: engine-api.md#节名`
- 函数 ≤80 行、圈复杂度 ≤25、包偏移必须用命名常量（禁魔法数字）——CI 强制
- 任何选型/协议/格式决策 → 写 `docs/adr/NNN-*.md`（含否决理由）

## 五、关键概念 30 秒速成

- **五大注册表**：netmsgdispatcher(客户端消息)/actorevent(玩家事件)/monevent(怪物事件)/csmsgdispatcher(跨服)/dbretdispatcher(DB回调)——全事件驱动无主循环
- **LDataPack 范式**：allocPacket(actor, CMD, subcmd) → writeXxx → flush
- **staticVar**：玩家持久化 Lua 表（写时标脏，定时/登出序列化落库）
- **金色字节（golden）**：从原版抓的真实包字节，作为编解码测试基准
- **对拍**：Go 服务先挂原版其余组件跑，逐字节 diff，过了才单独上线
