# 运维手册（RUNBOOK）

## 一、启动顺序（与原版 run.sh 一致，顺序不可乱）

```
dbserver → gameworld → gateway → loggerserver
（gameworld 启动后等 "load global function!!!!" 与 "all game config load complete" 日志）
```

- C++ 版：`data/server/s1/Debug/<svc>/<svc>.exe`（Linux 用 `s1/run.sh`；Linux 部署记得 chmod +x——历史事故：nohup.out 记录过"权限不够"）
- Go 版：`ltcq-go` 构建产物，配置沿用同名 .txt
- 停止：先 gateway（断新连接）→ gameworld（存盘）→ dbserver/loggerserver

## 二、端口表

| 端口 | 服务 | 说明 |
|---|---|---|
| 9002 | gateway | 客户端 WS（MaxSession 8192，s1；s2 9002、s999 9099） |
| 6001 | gameworld | 网关后端 |
| 5001 | dbserver | 存取 |
| 7001 | loggerserver | 日志 |
| 10001 | gameworld 内部 HTTP | 管理口（127.0.0.1） |
| 3306 | MySQL | 三个库：actor_s1/globaldata/log_s1（+center） |
| 80 | nginx | PHP 门户（EServer） |

## 三、健康检查（Go 版）

每个 Go 服务暴露 `http://127.0.0.1:<port+10000>/healthz`：返回进程名/版本/在线数/最后心跳。
`/metrics`：Prometheus 格式（在线数、包 p99、Lua VM 内存、DB 连接池、错误率）。
告警基线：单区在线 >70% 容量、包 p99 >10ms、错误率突增。

## 四、备份与恢复

```
备份：每日 mysqldump --databases actor_s1 globaldata center log_s1 | 保留 7 天
     + binlog 实时（恢复点 = 最近 dump + binlog 重放）
恢复：停服 → 导入最近 dump → 重放 binlog 至目标时刻 → 起服
Go 版数据格式变更：走 migrations/NNN_*.sql 顺序执行
```

## 五、常见故障表（本工程已实际踩过）

| 现象 | 原因 | 处置 |
|---|---|---|
| 客户端无登录框 | login.min.js 里 `window.loginType` 被硬编码 1（渠道SDK模式） | 确保为 0（已修复）；浏览器 Ctrl+F5 清缓存（js 有 1h 缓存头） |
| 浏览器报 `WebSocket connection to ws://127.0.0.1:9001 failed` | 网关进程未启动；或区服列表下发端口与网关实际监听端口不符（详见 §七） | 确认网关已启动且监听端口与 `web/server_list/config.php` 一致（s1=9002）；WebSocket 握手应返回 101 |
| 进游戏卡进度条 | gameworld 连不上 dbserver（`wait for db client` / `connect server time out`），多因端口错配或遗留 tcpcap 抓包进程占用（详见 §七） | 核对 `GameWorld.txt` 的 DbServer 端口=5001；停掉遗留 tcpcap；按 §一干净重启整链 |
| nginx reload 报 Access denied | nginx 以管理员启动，普通权限发不出信号 | 统一用普通权限启动；`nginx.exe -t && nginx.exe -s reload` |
| gameworld 卡 "wait for db client" | dbserver 未先启动，或 DbServer 端口被改错、dbserver 半死（详见 §七） | 按 §一顺序重启；仍失败则核对 §七的端口与进程占用 |
| gameworld 报 envir/quest 缺失 | 旧版路径死引用（已确认死代码区），引擎容错跳过 | 无需处理；不要去补造旧目录 |
| 启动大量 ItemConfig/EquipConfig ERR | 配置校验警告，引擎容错 | 正常现象，对照原版基线日志 |
| loggerserver "connect to db server succ" 但表不存在 | log_s1 未建表 | 从 loggerserver/data/main.lua 提取建表 SQL（提取脚本见会话记录，占位符 %d%02d→年月） |
| git push 卡死 | GitHub 直连不稳 | 用 SSH remote；分批提交 |
| Python 假命令（exit 49） | Microsoft Store 占位符 | 用 `D:\tools\python\python.exe` |

## 六、发布与回滚

- 版本兼容矩阵见 compatibility.md；发布前跑对应列回归
- 金丝雀：新区先上新版本 → 稳定后全区滚动
- 回滚 = 回退二进制版本 + 数据按 §四恢复到发布前 dump

## 七、实战排障记录：进不去游戏 / 卡进度条（2026-11-23）

### 症状与现象
- 浏览器控制台：`WebSocket connection to 'ws://127.0.0.1:9001/' failed`（socket.min.js:1）
- 修复端口后：能进登录页、能登录，但"开始游戏"后卡在进度条

### 根因链（四层叠加，逐层暴露）
1. **网关未启动**：9001/9002 均无进程监听，四个后端服务在跑但唯独缺 gateway。
2. **端口配置矛盾**：区服列表 `web/server_list/config.php` 下发 `9001`，而 C++ 网关 `GateWay.txt` 实际监听 `9002`，二者不符。
3. **前端胶水层缺陷（隐患，非本次主因）**：登录页依赖外网 `libs.baidu.com` 的 jQuery；`index.js` 在 `?uid=xxx` 路径下 userData 只剩 uid，缺 `sdkloginmodel/channelExt/game_appid/pid` 等登录包必需字段。
4. **db 链路错配（卡进度条主因）**：
   - 遗留的抓包工具 `tcpcap.exe -listen 127.0.0.1:5001 -to 127.0.0.1:5002` 仍占用 5001；
   - `GameWorld.txt` 的 DbServer 端口被改为 `5002`，而真正的 dbserver 监听 `5001`，中间经 tcpcap 转发形成错配；
   - dbserver 长期处于半死状态（boot.log 空），gameworld 持续刷 `wait for db client` / `connect server time out!`，登录请求无法回应 → 客户端卡进度条。

### 诊断命令（Windows / PowerShell）
```
# 看进程与端口占用
Get-Process | Where-Object { $_.ProcessName -match 'gateway|gameworld|dbserver|loggerserver|tcpcap' }
Get-NetTCPConnection -State Listen | Where-Object { $_.LocalPort -in @(9001,9002,6001,5001,7001) }
Get-CimInstance Win32_Process -Filter "ProcessId=<PID>" | Select CommandLine   # 查进程启动参数，识别 tcpcap
# 看 Established 链路是否打通
Get-NetTCPConnection | Where-Object { $_.RemotePort -eq 5001 -and $_.State -eq 'Established' }
Get-NetTCPConnection | Where-Object { $_.RemotePort -eq 6001 -and $_.State -eq 'Established' }
# 测接口与 WS 握手
Invoke-WebRequest 'http://localhost/web/server_list/Serverlist.php/?account=test'   # 看下发 port
node -e "..."  # 发标准 WS 握手，期望 HTTP/1.1 101 Switching Protocols
```
> 坑：`Start-Process -WindowStyle Hidden` 启动 C++ gameworld 会挂起（CPU 增量≈0、不写日志）；用普通 `Start-Process` 或输出重定向启动才正常。

### 修复步骤（全部完成）
1. 启动 C++ 网关 `data/server/s1/Debug/gateway/gateway.exe`（监听 9002）。
2. `web/server_list/config.php` 端口 `9001 → 9002`；同步更新本 RUNBOOK §二端口表。
3. `login1.php`/`login2.php`/`index1.php`/`index2.php` 外网 jQuery 改为本地 `/static/js/jquery-2.1.4.min.js`。
4. `index.js` userData 改为"默认字段 + URL 参数合并"，保证 `?uid=xxx` 路径字段完整，并在只带 uid 时同步 user_id/openId。
5. 停掉遗留 `tcpcap`（PID 4712），`GameWorld.txt` DbServer 端口 `5002 → 5001`。
6. 按 §一顺序干净重启：`dbserver → gameworld`（停旧进程后按序启动，用输出重定向捕获日志确认 `all game config load complete!`）。

### 最终验收状态
```
dbserver :5001 / gameworld :6001 / gateway :9002 / loggerserver :7001  均监听
gameworld → dbserver:5001   Established
gateway   → gameworld:6001  Established
WS 握手返回 HTTP/1.1 101
```

### 防复发要点
- 对拍/抓包用的 `tcpcap`、`wsprobe` 等工具用完即关，避免长期占用 5001/6001 造成端口错配。
- 网关端口以 `GateWay.txt` 为准，区服列表下发端口必须与之相同（s1=9002、s2=9002、s999=9099）；Go 网关默认 9001 与 C++ 不一致，切换网关时需同步改。
- gameworld 的 DbServer 端口应始终 = dbserver 的 DBService 端口（5001）；改动后必须重启 gameworld 才生效。
- 前端 js 有 1h 缓存，每次改完用 Ctrl+F5 验证；避免登录页依赖外网 CDN。

### Go 网关登录验证（2026-11-23 补充）
用 wsprobe 探针验证 Go 网关时，踩到几个坑，记录在案：
1. **端口默认值**：Go 网关默认曾为 9001/6002，与现网 9002/6001 不符。已改为 `listen=0.0.0.0:9002`、`backend=127.0.0.1:6001`；wsprobe 默认 addr 也改为 9002。
2. **wsprobe 登录数据**：探针硬编码的登录数据首 4 字节曾为 `00 00 00 00`，gameworld 会静默拒登；golden 真实值是 `01 00 00 00`。已修正，但探针 auth 帧仍只有 4B（真实客户端 58B），故**探针只能验证握手/转发，不能作为登录成功的判据**。
3. **gameworld 就绪判断**：gameworld 配置加载约需 20–40s（内存升至 ~600MB），未完成前登录帧无回包（started_=false，20s 超时）。验证前先确认日志出现 `all game config load complete!`。
4. **已验证字节一致**：Go 网关发出的注册包、cmd=1 UserOpen、cmd=5 UserData 与 C++ golden `gate_upstream.raw` 逐字节一致；gameworld 日志显示 `registe success (gateway_s1)`。
5. **未解决**：即便 gameworld 就绪且字节一致，探针登录仍无回包（i/o timeout）——疑因探针 auth 帧 4B ≠ 真实 58B，或 gameworld 会话处理差异。**Go 网关端到端登录尚未跑通，M2 未完成**；需用真实浏览器客户端抓包对拍，而非探针。
6. **g2c 录制 bug（已修 2026-11-23）**：原 `gwsession.go` 每会话 g2c 文件只 open 不写且不 close（88 个 golden 全 0 字节、文件被持有）。已改为回向 pump 统一写 `gate_downstream.raw`（完整下行帧含14B头），新增 `readRawGateFrame` 与单元测试；每会话 g2c 空文件逻辑已移除。
7. **★ 隔离端口对照实测（2026-11-23，零中断）**：
   - 方法：Go 网关 `:19002` + Go gameworld `:16001`（注册名 `gateway_s1`），现网 C++ 全部不动（9002/6001/5001 未受影响）。探针 `wsprobe -addr 127.0.0.1:19002`。
   - **C++ 基线**：探针打现网 9002 有登录回包 `0a bb 09 1c ff 01 00`（skey + `ff 01 00`=`CMD_Login(255)/子命令1/成功`）——**探针可验证登录握手，此前"探针不能作登录判据"结论需按此修正**（卡进度条是 egret 客户端参数流缺 `cd cd 00 00 00 00` 字段段，非服务端）。
   - **Go 全栈**：握手/skey/注册全正常；登录帧后 i/o timeout。Go gameworld 诊断日志实证帧已到达：`收到上行数据帧 sid=552 sysid=1 cmdid=0 len=62`。
   - **根因定案**：Go gameworld 把登录帧按 `[sysid=1][cmdid=0]` 路由到 Lua `OnNetMsg`，但登录鉴权（`CMD_Login=255`，账号+MD5 校验、`openuser`、`password ok`）是 **C++ 引擎级 `GameGate::OnRecv`/`Actor::Onlogin` 处理**，Lua 侧无 `netmsgdispatcher.reg(1,0,...)`（全局仅 actorevent 的 `aeUserLogin=13` 回调）→ 无处理器 → 静默无回包。**Go 网关侧协议零问题；缺的是 Go gameworld 的引擎级登录状态机**（M4 范畴，非网关 M2）。
   - 登录帧 54B 结构：首 4B `01 00 00 00`（登录类型标志，必需为1）+ `ff 01 01 00 00 00` + 账号长度(2B LE) + 账号UTF-8 + `00 20 00` + md5(32B) + `00`。
8. **★ M2 登录状态机复刻完成（2026-11-23，Go 全栈字节对齐）**：
   - Go gameworld 新增引擎级登录状态机（`internal/engineapi/login.go`）：在 NetRouter 按 `[4]==0xff` 分流登录帧（不走 Lua OnNetMsg）→ 解析 54B → 查 `actor_s1.globaluser` 比对 MD5 → 查 `actors` 取 actorid → `BindActor` → 回包 `[ff][01][00]`。启动加 `-dsn root:123456@tcp(127.0.0.1:3306)/actor_s1?charset=utf8`。
   - Go gateway 回向补 **skey 回显**（`cmd/gateway/main.go`）：登录结果包 `[ff][01][result]` 前加该 sid 的 skey 4B（小端），对齐 C++ 实测 `0a bb 09 1c ff 01 00`。其他回包保持原样。**此前 Go 回包仅 len=3 无 skey，真实客户端会校验失败卡进度条——这是比探针更隐蔽的一层 bug**。
   - 隔离端口端到端实证：Go 网关 :19002 + Go gameworld :16001 + 真实 MySQL，探针 `帅哥` 登录回包 `71 a0 97 68 ff 01 00`（skey=`6897a071` 小端 + `ff 01 00`），**len=7 与 C++ 现网同构**；gameworld 日志 `密码校验通过 account=帅哥 accountID=121102` / `登录绑定 actor sid=552 actorID=1`。
   - 单测 3 项：`ParseLoginFrame` golden / 成功回包字节+绑定 / 密码错误拒绝；全量 gofmt/vet/test 通过；现网服务全程未动。
- 排障后务必恢复 C++ 网关（停 Go 网关 → 启 `gateway.exe`），避免游戏服务中断。

## 八、登录后进场景验证（2026-11-24）

### Go 登录后业务链

1. `internal/engineapi/login.go`：`ff01` 只鉴权，不创建或绑定角色；`ff04` 返回角色列表；`ff02` 校验职业/性别后创建角色及关联记录；`ff05` 校验角色归属后绑定 Actor，并调用登录后处理器。
2. `internal/engineapi/postlogin.go`：加载角色并填充内存 Actor，依次下发 `ActorBaseData`（`[00][01]`）、`RoleData`（`[00][02]`）、`EnterScene`（`[00][03]`）、`OtherEntity`（`[00][04]`），最后触发 Lua `OnActorEvent(aeUserLogin=13)`。
3. **回包顺序是关键**：真实客户端在收到 `ff05 result=1` 后才由 `RoleMgr.doEnterGame` 创建 `MainScene`。因此必须先发 `ff05` 成功包，再发四个场景包；否则 `GameLogic.postEnterMap` 会在 `this.gamescene` 尚未创建时访问 `this.gamescene.map`。
4. `cmd/gateway/main.go`：所有 `cmd=5` 业务包回向均补会话 skey 4B。

### 隔离端口真实浏览器验证

- 服务：Go gateway `127.0.0.1:19002` → Go gameworld `127.0.0.1:16001`；C++ `9002/6001/5001/7001` 全程未改。
- 客户端启动参数必须保留原始中文账号（不要 `encodeURIComponent`），否则 Egret 会把百分号编码串作为账号发送。
- WebSocket 实测顺序：

  ```text
  [skey] ff 01 00
  [skey] ff 04 ... status=1
  [skey] ff 05 01
  [skey] 00 01 ... ActorBaseData
  [skey] 00 02 ... RoleData
  [skey] 00 03 ... EnterScene
  [skey] 00 04 ... OtherEntity
  ```

- 客户端运行日志明确出现 `[E2E] scene=MainScene`；`Actor.postInit`、`SubRoles.doSubRole`、`Role.parser`、`GameMap.parser`、`GameLogic.postEnterMap` 均进入并正常返回。
- 首轮黑屏已定位并修复：`ActorBaseData` 多写了 `00 01`，导致客户端字段错位并在 `UIView2.expChange` 同步抛异常；`EnterScene` 的首个 int 是副本 ID，必须为 `0`，第二个 int 才是地图 ID；场景配置出生点是格子坐标，下发前必须乘客户端 `GameMap.CELL_SIZE=64` 转为像素。
- 最终真实浏览器结果：`scene=MainScene`、`fubenID=0`、`mapID=41`、`mapX=4928`、`mapY=1408`、地图 `map010`，缩略图加载完成，24 张地图分块已挂载，镜头位于 `(-4638,-883)`；截图可见完整地图与 UI。约 12 秒内 `SystemTicker.update=3951`、`render=1980`，WebSocket 与资源加载无失败，CDP  Evaluate 和截图均成功。
- 证据目录：`ltcq-go/reference/captures/browser_render_debug_20261124/`，`first_frame.png` 为最终地图画面，`first_frame_events.json` 记录渲染/ticker/资源证据，`gate_downstream.raw` 记录网关下行帧；此前 `browser_e2e_scene_20261124_2050/` 保留 `MainScene` 首次进入证据。
- 唯一非致命异常是 headless 浏览器音频自动播放被策略拒绝，不影响地图与 UI 渲染。
- **该阶段边界**：真实浏览器已证明登录、进入场景、主角色显示与点击移动闭环；AOI、其他玩家同步、怪物/AI/战斗、掉落/拾取、完整持久化及重连链路仍未接通。
- 回归测试：`go test ./...` 全绿；`TestBuildActorBaseData_ClientParserLayout`、`TestBuildEnterScene_ClientParserLayout`、`TestBuildEnterScene_MineEntryCoordinates` 与 `TestPostLoginSendsFourPacketsBeforeLoginEvent` 固化包布局、出生坐标及回包顺序。

### 验证后恢复

已停止隔离 Go gateway/gameworld 与测试用 headless Edge，并将 `data/h5game/web/server_list/config.php` 的区服端口从 `19002` 恢复为 `9002`。C++ 网关继续监听 `9002`，无需重复启动。

## 九、主角色显示与移动验证（2026-11-25）

### 协议修复

- `OtherEntity` 的 `type/handle` 只出现一次；`postCreateOtherEntity` 已消费该前缀，`Role.parserBase` 会再次消费对应的 10B，重复写入会导致后续字段整体错位。
- 实体 handle 和属性值使用 IEEE-754 `float64` 位模式（`math.Float64bits`），不能直接写整数。至少发送 HP、MP、maxHP 三个属性，否则主角色无法通过 `getNoDieRole()`。
- 主角色 `Role=1`、`masterHandle=Actor.handle`、`configID=0`，使其与已有主 SubRole 合并。
- 移动上行 `[00][1f]`：`handle:f64, count:i32, count × (x:i32, y:i32)`；下行 `[00][0d]`：`handle:f64, mode:i16`，mode 0 带 `x:i32,y:i32`，mode 1 再带 `duration:i32`。Go 侧校验鉴权、Actor 绑定、handle 所有权、路径长度、精确包长和地图边界。

### 真实浏览器验收

- 隔离服务：gateway `127.0.0.1:19002` → gameworld `127.0.0.1:16001`；C++ 服务全程未改。
- 主角色“有点有你”在 `MainScene` 创建、合并并挂载，handle/masterHandle 均为 1，HP/maxHP 为 100/100；前后截图均可见角色。
- 点击地图后客户端调用 `sendFindPathToServer`，浏览器实收上行 `[00][1f]` 和下行 `[00][0d]`；角色坐标由 `(4928,1408)` 更新为 `(5044,1478)`，移动时长 46。
- 客户端普通场景会屏蔽点击移动（`MoveCC.mapClick` 要求 `GameMap.sceneInMine()`）。为验证移动业务链，仅在临时测试页面临时设置 `GameMap.fubenID=GameMap.mapID`，未修改服务端、客户端文件或共享数据库。该覆盖记录在证明 JSON 中，不代表普通地图 41 的线上行为。
- **普通地图 41 必须保持 `fubenID=0,mapID=41`**，与 C++ 抓包一致；采矿副本实际为 `fubenID=50000,mapID=7000`，不能把 41 伪装成采矿副本。
- 证据目录：`ltcq-go/reference/captures/browser_role_move_20261124/`。`role_move_proof.json` 记录 `upstreamFindPath=1`、`otherEntity=1`、`syncPos=1`，`role_move_events.json` 保留浏览器 WebSocket 帧原文，`role_before_move.png` / `role_after_move.png` 为前后截图。捕获中仍有一条 Egret 音频自动播放策略异常，不影响场景和移动。

### 验证后恢复

已关闭临时 Edge（CDP `9223` 无响应），停止 Go gateway/gameworld（`19002/16001` 不再监听）。复查 C++ `9002/6001/5001/7001` 与门户 `80` 均在监听，区服配置仍为 `9002`。`go test ./...`、`go vet ./...` 和 `git diff --check` 通过。

## 十、共享场景与 AOI 基础闭环（2026-11-25）

### 已实现

- `internal/scene.World` 作为 gameworld 进程级共享场景，所有网关连接共用同一份实体、观察者和地图配置；每个连接仍保留独立 Lua VM。
- `config/instance/scene.config` 由 gopher-lua 预处理并执行，`sizeX/sizeY/enterX/enterY` 从格子单位换算为 64 像素单位；地图 41 出生点保持 `(77,22)` 格。
- 默认视野为 576 像素方形范围。登录时双向发送视野内 `OtherEntity`，移动时广播 `[00][0d]`，离开/进入视野时发送 `[00][0a]` 或完整实体。
- `NetRouter.CloseSession` 与网关连接 EOF 的 `CloseAll` 均触发共享场景移除和离场广播；路由锁在调用 AOI 观察者前释放。
- 移动广播不持有 Actor 写锁，避免移动更新与 AOI 实体打包形成嵌套锁死。

### 验证结果与边界

- `TestAOISharedWorldAcrossConnections` 使用两个独立 Router/连接共享一个 World，验证登录互见、跨连接移动同步、离开/重新进入视野、场景隔离和断线移除。
- `go test ./...` 与 `go vet ./...` 通过；`git diff --check` 通过。
- `go test -race ./internal/scene ./internal/engineapi` 未运行成功：当前 Windows 环境 `CGO_ENABLED=0`，且未找到 gcc/clang；本机也没有可用 WSL 发行版。该检查仍需在具备 CGO/C 编译器的环境补做。
- 本阶段是双连接协议集成验证，不是双浏览器真实客户端验收；尚未启动隔离服务，也未改动门户或共享 Actor 数据。复查 C++ `9002/6001/5001/7001` 仍在监听，`19002/16001` 未监听，区服端口仍为 `9002`。
- 当前 AOI 使用矩形视野扫描，尚未做空间网格性能优化；已支持最小静态怪物生成与外观下发。怪物 AI、巡逻、攻击、伤害、死亡、掉落/拾取、完整持久化及重连仍未实现。

## 十一、最小怪物生成与外观闭环（2026-11-25）

### 已实现

- `internal/scene.World` 新增静态怪物实体：登录和进入视野时下发 `OtherEntity(type=2)`，离开视野或移除时下发 `EntityDisapear`；怪物没有观察者，不会反向接收玩家事件。
- `internal/engineapi/monsterconfig.go` 加载原版 `monsters.config` 与 `chaptermonster.config`，后者覆盖同 ID；`MonsterManager.SpawnAll` 先校验全部刷新点，再按格子坐标乘 64 生成怪物。
- 怪物 handle 从 `1<<30` 开始分配；HP/maxHP 来自服务端配置。名称和外观由客户端按 `configID` 从自己的 `GlobalConfig.MonstersConfig` 读取，服务端包内不发送名称、avatar 或 scale。
- 怪物 `OtherEntity` 负载固定为 63 字节：`type:u16, handle:f64, configID:i32, masterHandle:f64, x:i32, y:i32, attrCount:u16, HP/MP/maxHP:3×f64, extraIntCount:u16, lyMarkLv:u16, lySkillCount:u8`。
- gameworld 新增可选 `-monster-spawns`；未指定时不刷怪。开发示例 `ltcq-go/config/monster_spawns.example.json` 在服务端场景 1 的 `(55,37)` 格生成一只基础怪物 `31001`（像素坐标 `3520,2368`），不是原版公开地图刷怪数据。
- 原版公共地图没有自动刷怪事件，地图 41 刷怪表缺失；本阶段只提供显式静态刷新配置，不冒充原版刷怪逻辑。
- 原版场景表存在边界非法条目（如 1002 的出生点越界、1005 的出生点等于宽度）。加载时记录警告并跳过这些不可用条目；若没有任何有效场景则拒绝启动，地图 41 配置保持不变。

### 配置选择与协议边界

- 服务端章节怪物 `10001` 不在客户端 `GlobalConfig.MonstersConfig` 中。真实浏览器按该 ID 创建模型时会在名称/avatar getter 处失败，因此开发示例改用客户端已加载的基础怪物 `31001`。
- 原版基础怪物表没有显式 avatar/scale 字段，客户端配置由预处理补齐；服务端无需复制这些客户端外观字段，只需保证所选 configID 存在于客户端基础表且 HP 合法。
- 本次真实浏览器进入普通地图时 `GameMap.mapID=1`、`fubenID=0`、`sceneInMine()=false`；怪物与玩家位于同一服务端场景 1。`afterCreateOtherEntity` 会在采矿副本状态隐藏怪物，不能用 mine-state override 伪造本阶段验收。

### 测试与运行

- `TestLoadOriginalMonsterCatalog` 验证原版目录可加载；`TestMonsterManagerSpawnsInPixelCoordinates` 与原子预校验测试覆盖坐标、数量和 handle。
- `TestBuildMonsterEntityClientLayout` 固化 63 字节布局；`TestAOIMonsterVisibilityAcrossConnections` 覆盖双连接登录、视野离开/重进及断线后怪物保留。
- `TestLoadOriginalSceneWorld` 覆盖原版场景资产加载与容错跳过。
- 隔离启动示例：

  ```bash
  go run ./cmd/gameworld -listen 127.0.0.1:16001 \
    -data ../data/server/s1/Debug/gameworld/data -ostype WIN \
    -dsn 'root:123456@tcp(127.0.0.1:3306)/actor_s1?charset=utf8' \
    -monster-spawns config/monster_spawns.example.json
  go run ./cmd/gateway -listen 127.0.0.1:19002 \
    -backend 127.0.0.1:16001 -gatename gateway_s1
  ```

- 当前仍未实现怪物 AI、巡逻、攻击、伤害、死亡、掉落/拾取及持久化；`go test -race` 仍受本机 CGO/编译器缺失限制。

### 真实浏览器验收与恢复（2026-11-25）

- 隔离服务使用 Go gateway `127.0.0.1:19002` → Go gameworld `127.0.0.1:16001`，门户临时指向 `19002`；C++ 基线端口未改。
- 网关录制 `ltcq-go/reference/captures/browser_monster_20261125/gate_downstream.raw` 中有两条怪物实体帧（sid 552/553）：`type=2`、handle `1073741824`、configID `31001`、masterHandle `0`、坐标 `(3520,2368)`、属性 `[127738,0,127738]`；含 `[00][04]` 的实体负载为 63 字节。
- 真实浏览器创建两个可见实体：玩家 handle `9502731`、坐标 `(3584,2368)`；怪物 handle `1073741824`、名称 `兽人统领`、avatar `10004`、客户端 level 20、HP/maxHP `127738`、血条值 `127738`，已挂载舞台且 `sceneInMine()=false`。
- 最终截图 `ltcq-go/reference/captures/browser_monster_20261125/monster_31001_rendered.png` 可见兽人统领、玩家、怪物血条与红色脚底光环；结构化证据为同目录 `monster_31001_proof.json`。
- 验收后将 `data/h5game/web/server_list/config.php` 恢复为 `9002`，停止隔离 gateway/gameworld 并关闭测试浏览器页；`19002/16001` 不再监听。C++ 服务链按 dbserver → gameworld → gateway → loggerserver 恢复，最终复查 `9002/6001/5001/7001` 与门户 `80`。

## 十二、最小自动战斗与死亡消失闭环（2026-11-25）

### 触发选择与实现

- 客户端 `main.min.js` 中 `sendUseSkill` 没有调用点；点击怪物只走选中/寻路逻辑，没有独立的普通攻击上行。`[02][01]` 已确认是技能结果，不能冒充普通攻击请求。
- 因此本阶段采用**服务端权威自动战斗**：玩家进入攻击范围后，由 gameworld 每 500ms 选择同场景、范围内最近的存活怪物攻击一次，不伪造客户端攻击包。
- `internal/scene/world.go` 在同一把锁内校验攻击者、目标、场景和 Manhattan 距离，扣减 HP，并在死亡时删除怪物；最终伤害钳制为剩余 HP，重复攻击不能造成超额伤害。
- 每次命中按固定顺序广播 `EventHP` → `EventDamage`；死亡追加 `EventLeave`。即使某个观察者发送失败，已生效的 HP 与死亡结果不回滚。
- `internal/engineapi/combat.go` 的 ticker 由 `cmd/gameworld` 进程级启动一次，不随网关连接或 Lua VM 重复运行。
- 默认参数：伤害 100、攻击距离 `2*64=128` 像素、攻击间隔 500ms。伤害数值和距离目前是开发常量，不是完整职业/装备公式。

### 下行协议

| 命令 | 负载（不含 `skey/sysid/cmdid`） |
| --- | --- |
| `[00][09]` HP 变化 | `handle:f64, currentHP:f64` |
| `[00][20]` 飘字 | `attacker:f64, target:f64, damage:f64, damageType:u32` |
| `[00][10]` 实体消失 | `handle:f64` |

对应完整业务负载分别为 18、30、14 字节（含两字节命令头）。当前 `damageType=0`。

### 开发刷新配置

- `ltcq-go/config/monster_spawns.example.json` 显式在服务端场景 2 的 `(6,6)` 格生成一只 `31001` 兽人统领，像素坐标 `(384,384)`；测试角色位于 `(256,384)`，距离恰好 128。
- 该配置增加开发专用 `hp` 覆盖，将原目录 HP `127738` 降为 `2000`，用于约 10 秒完成 20 次命中并死亡。负覆盖会在生成前被拒绝。
- 场景、坐标和 HP 覆盖均**不是原版公开地图刷怪数据**，也没有修改共享 Actor 数据库来迁就测试。

### 真实浏览器验收

- 使用干净 Edge 配置，经真实公告关闭与开始按钮进入游戏；网关 `19002` → gameworld `16001`，门户仅临时指向 `19002`。
- 浏览器状态：`GameSceneView`、`mapID=2`、`fubenID=0`、`sceneInMine()=false`；玩家 handle `65537`，怪物 handle `1073741824`、configID `31001`。
- 实收 20 个 `[00][09]`，HP 从 1900 依次降至 0；20 个 `[00][20]` 均为攻击者 `65537` → 怪物 `1073741824`、伤害 100、类型 0。
- 客户端捕获 20 次伤害回调，每次 `amount=-100,type=0`；死亡时收到一次消失回调和一个 `[00][10]`。
- 最终 `EntityManager` 中只剩玩家，怪物不在实体表中，且已从 Egret 渲染舞台移除。`combat_browser_proof.json` 的 9 项验收全部为 `true`，共记录 46 个战斗帧、67 个 WebSocket 帧。
- 证据目录：`ltcq-go/reference/captures/browser_attack_20261125/`。`combat_monster_initial.png` 显示怪物在场，`combat_monster_removed.png` 显示死亡动画结束后的最终画面；`combat_damage_visible.png` 为中途状态，飘字以结构化回调证据为准。`gate_downstream.raw`、`gateway.log`、`gameworld.log` 和 `verify_combat.mjs` 可复现抓包与浏览器断言。

### 边界与回归

- 已覆盖伤害与死亡顺序、多观察者、场景/距离/归属校验、并发死亡只结算一次、广播失败不回滚状态、最近目标选择和协议精确布局。
- `go test ./...`、`go vet ./...`、`git diff --check` 通过。`go test -race` 仍受当前 Windows 环境 `CGO_ENABLED=0` 且无 C 编译器限制。
- 本阶段尚未实现技能树、怪物反击/AI、掉落、经验、复活、副本结算、完整持久化和怪物重生；这些缺口由后续里程碑继续收敛。

## 十三、击杀奖励、进度存档与怪物重生闭环（2026-11-25）

### 已实现

- `internal/scene/world.go` 的 `AttackMonsterResult` 只在本次攻击实际击杀怪物时返回 `Killed=true`；`CombatManager` 仅依据该结果结算奖励，并发命中同一怪物不会重复发奖。
- `MonsterSpawn` 新增 `exp`、`gold`、`respawnSeconds`。开发示例每只 `31001` 提供 100 经验、1000 金币，死亡 3 秒后按原坐标、原句柄和满 HP 重生；`respawnSeconds<=0` 表示不重生。
- `ProgressionManager` 先更新内存 Actor，再以绝对 level/exp/gold 写入 `actors` 表。存档失败会标记 dirty，在后续战斗 tick 和停机时重试；重试使用最新快照，不会重复累加奖励。
- 经验阈值从原版 `config/actor/exp.config` 加载，支持升级扣减阈值和满级封顶；协议字段限制在 signed int32 范围内。
- `cmd/gameworld` 每进程只启动一个战斗 ticker、一个共享怪物管理器和一个奖励存档管理器，不随网关连接重复创建。

### 下行协议

| 命令 | 负载（不含 `skey/sysid/cmdid`） |
| --- | --- |
| `[00][05]` 金币变化 | `moneyType:i16, absoluteGold:u64`；普通金币 `moneyType=1` |
| `[00][07]` 经验变化 | `level:i32, exp:i32, addedExp:i32` |
| `[00][04]` 怪物重生 | 重发完整 `OtherEntity`，无专门重生命令 |
| `[00][10]` 死亡消失 | `handle:f64` |

致命一击的广播顺序为 HP=0 → 伤害飘字 → 实体消失 → 金币 → 经验；奖励包只发给击杀者。

### 隔离真实浏览器验收

- 使用独立库 `actor_s1_m5d_20261125`，克隆账号 `haohaiyao` / Actor `65537`，初始重置为 51 级、0 经验、0 金币、场景 2；现网 `actor_s1` 未修改。
- 隔离服务为 Go gateway `127.0.0.1:19002` → Go gameworld `127.0.0.1:16001`，门户仅临时指向 `19002`。真实 Edge 经公告关闭和开始按钮进入 `GameSceneView`。
- 首轮 20 次命中后 HP 归零，实收一个 `[00][10]`、一个金币绝对值为 1000 的 `[00][05]` 和一个 `addedExp=100,exp=100` 的 `[00][07]`；客户端 Actor 同步为 100 经验、1000 金币。
- 3 秒后重发同句柄 `1073741824` 的 `[00][04]`，HP/maxHP 均为 2000，客户端实体重新挂载舞台。第二轮再次死亡，奖励累计为 200 经验、2000 金币，没有重复发奖。
- 显式关闭游戏 WebSocket 后重登，网关记录 sid `552` close、随后 sid `553` 鉴权；重登 `ActorBaseData` 和客户端 Actor 均为 51 级、200 经验、2000 金币，隔离库查询结果一致。
- `reward_browser_proof.json` 的 17 项验收全部为 `true`：两轮共 40 次客户端伤害回调、两次 HP 归零、两次消失、两次金币/经验奖励；另有一条重登后的伤害帧。证据共记录 141 个 WebSocket 帧。
- 证据目录：`ltcq-go/reference/captures/browser_reward_20261125/`。`reward_monster_initial.png`、`reward_first_kill.png`、`reward_monster_respawned.png`、`reward_second_kill.png`、`reward_relogin_persisted.png` 分别保存战前、首杀、重生、二杀和重登画面；`verify_reward.mjs`、`gameworld.log`、`gateway.log`、`verifier.log` 可复现验收。

### 边界与恢复

- 奖励数值、HP 覆盖和刷新坐标是开发示例，不是原版公开刷怪表或完整职业公式。当前只持久化 level/exp/gold，不代表背包、任务、技能等完整玩法存档。
- 仍未实现掉落/拾取、怪物反击与巡逻 AI、技能树、复活和副本结算。
- 回归覆盖奖励包布局、升级/封顶、非法奖励、存档失败重试、同句柄满血重生、致死攻击只发奖一次和完整战斗包顺序；`go test ./...`、`go vet ./...`、`git diff --check` 通过。`go test -race` 仍受当前 Windows 环境 `CGO_ENABLED=0` 且无 C 编译器限制。
- 验收后已将门户恢复为 `9002`，停止 Go gateway/gameworld 与测试 Edge，复查 C++ `9002/6001/5001/7001` 和门户 `80` 正常监听；隔离库及临时二进制、浏览器配置均已清理。

## 十四、怪物受击反击与玩家 HP（2026-11-25）

### 已实现

- `internal/scene/world.go` 新增 `AttackPlayerResult`，在怪物存活、目标玩家存活、同场景且位于攻击范围内时造成伤害；HP 最低钳制为 0，超量伤害不会变为负数。
- `internal/engineapi/combat.go` 在每个 500 ms tick 中依次执行进度存档、玩家自动攻击、怪物反击、到期重生。只有成功命中怪物才记录仇恨；怪物只反击当前目标，不主动索敌或追逐。
- 目标死亡、断线、离开场景或超出范围时清除仇恨。每只怪物使用独立攻击时间表，攻击伤害、范围、间隔由 `MonsterSpawn.attackDamage`、`attackRange`、`attackIntervalSeconds` 配置；无效组合在怪物生成前拒绝。
- 怪物伤害写回 World 玩家实体和内存 Actor，移动会保留 HP/MaxHP。默认 HP/MaxHP 为 100；登录进场景前重置为 100，本阶段不持久化 HP 变化。
- 客户端伤害回调、受害者 HP 包和其他可见玩家观察者使用同一组广播。玩家 HP 归零后实体仍保留，不触发死亡或复活流程。

### 下行协议

| 命令 | 负载（不含 `skey/sysid/cmdid`） | 语义 |
| --- | --- | --- |
| `[00][09]` HP 变化 | `handle:f64, currentHP:f64` | 玩家或怪物当前 HP |
| `[00][20]` 飘字 | `attacker:f64, target:f64, damage:f64, damageType:u32` | 复用普通伤害包表示怪物反击，`damageType=0` |

对玩家的每次反击先发送 HP 包，再发送伤害飘字；AOI 默认排除受害者，因此受害者需要显式投递，其他可见玩家仍接收战斗广播。开发示例配置为 HP 2000、伤害 10、距离 128、间隔 2 秒；玩家以每 500 ms 一次、伤害 100 攻击，共约 10 秒击杀。

### 隔离真实浏览器验收

- 使用隔离库 `actor_s1_m5e_20261125_01`、现有账号 `abc`、Actor `65537`，初始为 51 级、0 经验、0 金币、场景 2；未修改现网 `actor_s1` 或 `globaluser`，隔离库仅克隆既有表，最终登录使用现有账号 `abc`。
- 真实 Edge 经公告关闭与开始按钮进入场景，只有一个 `ws://127.0.0.1:19002/` 连接和一条登录帧。验证脚本检查 socket/loading 状态，避免客户端自动登录后再次点击开始造成重复连接。
- 实收 20 次玩家伤害、5 次怪物反击；玩家 HP 依次为 `90 → 80 → 70 → 60 → 50`，四次反击间隔约 `1.999–2.001` 秒。怪物 HP 归零一次，玩家仍以 50/100 HP 留在场景。
- 击杀仍广播一次金币绝对值 1000、一次经验 `addedExp=100,exp=100` 和一次实体消失；客户端伤害回调共 25 次，无控制台错误。
- `retaliation_browser_proof.json` 的 14 项验收全部为 `true`，记录 79 个 WebSocket 帧。证据目录为 `ltcq-go/reference/captures/browser_retaliation_20261125/`；`retaliation_monster_initial.png`、`retaliation_damage_visible.png`、`retaliation_first_kill.png` 保存战前、受击和击杀画面，`verify_retaliation.mjs`、`gameworld.log`、`gateway.log`、`verifier.log` 保留验收素材。

### 边界、回归与恢复

- 本阶段仅实现受击后的最小仇恨反击，不含巡逻、主动索敌、追击、技能、玩家死亡/复活、掉落/拾取和完整玩法持久化。
- 单元测试覆盖无仇恨不反击、伤害与 HP 广播、范围/场景/存活校验、HP 钳制、零血玩家保留、断线与离开范围清仇恨、攻击间隔及 ticker 提前到达时的 50 ms 调度容差；`go test ./...`、`go vet ./...` 通过。`go test -race` 仍受当前 Windows 环境 `CGO_ENABLED=0` 且无 C 编译器限制。
- 验收后已停止隔离 Go gateway/gameworld 和测试 Edge，删除隔离库及 `.tmp_m5e` 临时二进制、查询脚本和浏览器配置；门户恢复 `9002`，复查 `16001/19002/9223` 关闭，C++ `9002/6001/5001/7001` 与门户 `80` 正常监听。

## 十五、玩家死亡冻结与安全复活闭环（2026-11-25）

### 已实现

- `internal/scene/world.go` 的 `MovePlayer` 拒绝 HP 为 0 的玩家；`internal/engineapi/movement.go` 在 Actor 锁内二次确认 HP，再提交 World 坐标，并在位置同步成功后才广播 AOI 变化，避免死亡与移动并发产生状态竞态。
- `AttackPlayerResult.Killed` 标记致死反击。致命广播固定为 HP=0 → 伤害飘字 → 玩家实体消失；死亡玩家保留在 World 中供定时复活，但 AOI 对存活观察者隐藏。
- `internal/engineapi/combat.go` 为死亡玩家登记 `ReviveAfter` 截止时间并清除所有指向该玩家的怪物仇恨。默认延迟为 3 秒，可通过 `CombatConfig.ReviveAfter` 配置；这是测试/开发约定，不代表原版数值配置。
- 到期复活使用 `World.RevivePlayer`，保持场景、实体句柄和坐标不变，HP/MaxHP 恢复为满值。先同步 Actor HP，再向本人及可见存活观察者广播完整 `OtherEntity`；广播失败不回滚已生效的复活状态。
- 断线或重新登录会移除过期的复活计时，不会把旧会话的复活任务带入新会话；登录仍按现有约定重置 HP 为 100。未新增数据表、数据库字段、死亡弹窗或复活客户端命令。

### 下行协议

| 命令 | 负载（不含 `skey/sysid/cmdid`） | 语义 |
| --- | --- | --- |
| `[00][09]` HP 变化 | `handle:f64, currentHP:f64` | 致死时发送 HP=0 |
| `[00][20]` 飘字 | `attacker:f64, target:f64, damage:f64, damageType:u32` | 致死伤害 |
| `[00][10]` 实体消失 | `handle:f64` | 死亡包序最后一项 |
| `[00][04]` | 完整 `OtherEntity` | 同句柄、同场景、同坐标的满血复活实体 |

### 隔离真实浏览器验收

- 复用 `setup_m5e_db.go` 创建并最终删除隔离库 `actor_s1_m5e_20261125_f1`，仅克隆既有表；使用现有账号 `haohaiyao` / Actor `65537`，初始为 51 级、0 经验、0 金币、场景 2。现网 `actor_s1` 未修改，也没有新增业务表。
- Go gateway `127.0.0.1:19002` → gameworld `127.0.0.1:16001`，门户仅临时指向 `19002`。真实 Edge 经公告关闭和开始按钮进入 `GameSceneView`，只有一个游戏 WebSocket 和一条登录帧。
- 开发怪物配置位于场景 2 `(7,6)` 格，玩家从 `(256,384)` 实时移动到 `(384,384)` 后进入攻击范围。客户端先记录玩家 100/100 HP，再收到怪物 100 点致死伤害。
- 实收死亡包序 `[00][09] handle=65537,HP=0` → `[00][20] attacker=1073741824,target=65537,damage=100` → `[00][10] handle=65537`；客户端记录死亡音效回调并从实体表与舞台移除玩家。
- 死亡窗口内注入移动请求，服务端记录“玩家已死亡，不能移动”，没有位置同步或后续伤害包。约 `2999.932ms` 后重发 `[00][04]`，句柄仍为 `65537`，坐标为死亡前 `(384,384)`，HP/MaxHP 为 `100/100`；随后自动战斗恢复。
- `death_browser_proof.json` 的 17 项断言全部为 `true`，共记录 40 个 WebSocket 帧；复活后的战斗允许玩家再次死亡，因此满血校验使用复活瞬间快照而非最终快照。经验和金币保持不变，无客户端脚本错误。
- 证据目录：`ltcq-go/reference/captures/browser_death_20261125/`。`death_initial_full_hp.png`、`death_before_lethal_hit.png`、`death_player_removed.png`、`death_player_revived.png` 保存关键时刻；`verify_death.mjs`、`death_browser_proof.json`、`verifier.log`、`gameworld.log`、`gateway.log` 和 `monster_spawns.json` 保留可复现素材。

### 边界、回归与恢复

- 本阶段只实现同场景、同坐标、满 HP 的安全复活，不含回城复活、付费复活、完整职业属性、MP、技能、掉落/拾取、副本结算或完整玩法持久化。
- 单元测试覆盖致死包序、死亡移动/攻击拒绝、并发致死只生效一次、复活后 AOI 可见、重复复活拒绝、死亡断线不重复广播、广播失败不回滚、断线重登清理旧计时、自定义复活延迟和复活后恢复战斗。
- `go test ./...`、`go vet ./...`、`gofmt -l cmd internal`、`git diff --check` 和浏览器验证脚本语法检查通过。`go test -race` 仍受当前 Windows 环境 `CGO_ENABLED=0` 且无 C 编译器限制。
- 验收后门户恢复 `9002`，停止 Go gateway/gameworld 与测试 Edge，删除隔离库及 `.tmp_m5f` 临时文件；复查 `16001/19002/9223` 关闭，C++ `9002/6001/5001/7001` 与门户 `80` 正常监听，门户返回 `9002`。

## 十六、怪物最小主动索敌、追击与脱战归位（2026-11-25）

### 已实现

- 怪物生成配置新增 `aggroRange`、`chaseStep`、`leashRange`，均为像素。主动 AI 必须同时提供攻击伤害、攻击范围和攻击间隔，且 `leashRange >= aggroRange`；加载前原子校验，非法配置不会产生部分怪物。三个字段全为 0 时保持原有受击仇恨反击行为。
- `internal/scene/world.go` 的 `MoveMonster` 提供服务端权威坐标更新，拒绝不存在、非怪物、已死亡或越界实体；可见玩家接收 `[00][0d]`，离开/进入 AOI 分别复用实体消失和完整实体包，不新增客户端命令。
- `internal/engineapi/combat.go` 每个 AI tick 选择同场景、攻击配置有效、索敌范围内最近的存活玩家；超出攻击范围时按 `chaseStep` 追击，进入范围后按 `attackIntervalSeconds` 攻击。
- 玩家死亡、断线、离开场景、目标失效或怪物越过 `leashRange` 时清除仇恨。脱战怪物进入 `returning` 状态，归位完成前禁止重新索敌，避免在 leash 边界反复横跳；到达出生点后的下一 tick 清除归位标记。
- 怪物死亡和重生清理攻击冷却、仇恨与归位状态。未配置主动 AI 的怪物不会巡逻、索敌或追击。本阶段不含技能、寻路、群体仇恨、巡逻、掉落和完整职业数值。

### 隔离真实浏览器验收

- 复用 `setup_m5e_db.go` 创建并最终删除隔离库 `actor_s1_m5e_20261125_g1`，仅克隆既有表；使用现有账号 `haohaiyao` / Actor `65537`，初始为 51 级、0 经验、0 金币、场景 2。现网 `actor_s1` 未修改，也没有新增业务表或字段。
- Go gateway `127.0.0.1:19002` → gameworld `127.0.0.1:16001`，门户仅临时指向 `19002`。真实 Edge 进入 `GameSceneView`，只有一个游戏 WebSocket 和一条登录帧。
- 测试怪物出生点为场景 2 `(448,384)`，HP 2000；玩家从 `(256,384)` 进入 256 像素索敌范围。实收首个怪物移动包 `[00][0d] handle=1073741824, mode=1, x=384, y=384, duration=16`，先移动后攻击，玩家实体 HP 100→90。
- 玩家实时移动到 `(1200,384)` 后，怪物追击到 `x=1024`，超过出生点 448 加 leash 512 的边界 960，随后转向归位。怪物离开 AOI 后不再发送移动包，因此验收让玩家移动到 `(1000,384)` 重新观察，通过 `[00][04]` 实体重入包确认怪物已回到 `(448,384)`，HP 1900/2000。
- 归位后稳定观察 1.6 秒，没有怪物移动或攻击包；经验和金币保持 0。`monster_ai_browser_proof.json` 的 15 项断言全部为 `true`，共记录 51 个 WebSocket 帧，无客户端脚本错误。
- 证据目录：`ltcq-go/reference/captures/browser_monster_ai_20261125/`。`monster_ai_initial.png`、`monster_ai_chased.png`、`monster_ai_leash.png`、`monster_ai_returned.png` 保存关键时刻；`verify_monster_ai.mjs`、`monster_ai_browser_proof.json`、`verifier.log`、`gameworld.log`、`gateway.log` 和 `monster_spawns.json` 保留可复现素材。

### 回归与恢复

- 单元测试覆盖主动追击与攻击包序、范围外/死亡玩家忽略、最近目标选择、断线归位、leash 归位期间目标仍在索敌半径内但不重新锁定、归位完成清状态，以及非法 AI 配置拒绝；场景测试覆盖怪物移动 AOI 的视野内、离开、重入和非法状态。
- `go test ./...`、`go vet ./...`、`gofmt -l cmd internal`、`git diff --check` 和浏览器脚本语法检查通过。`go test -race` 仍待支持 CGO 的环境补做。
- 验收后门户恢复 `9002`，停止 Go gateway/gameworld 与测试 Edge，删除隔离库及 `.tmp_m5g`；复查 `16001/19002/9223` 关闭，C++ `9002/6001/5001/7001` 与门户 `80` 正常监听，门户实际返回 `9002`。

## 十七、登录进度条弹框 `fzRedShow` 读 `undefined.Lv`（2026-11-25）

### 现象（排查入口，复发时先对号入座）
- 登录进度条跑到**大半**、进游戏前弹出 egret 原生 alert 框（localhost 显示），原文：
  - `错误信息: Uncaught TypeError: Cannot read properties of undefined (reading 'Lv')`
  - `出错位置：93行15883列`　`出错函数：e.fzRedShow`
  - `调用栈：main.min.js:93:15357 → 68:6564 → 68:6343 → 1:6171 → 1:5961 → egret.min.js:4:11822 → egret.web.min.js:2:15183`
- 关键定性：这是**客户端 JS 自己抛的异常对话框**，不是服务端 alert、不是账号/密码错误、不是 WS 链路断开。`state.errors` 常为空、WebSocket 帧数正常，容易误判成"已经进场景"。

### 根因（一句话）
`RoleData`（`CMD_Base=0, sBaseCmd_RoleData=2`）包尾的**法阵（DemonCircle）数据段**子阵数量写成 `0`，客户端 `SubRoles.parser` 据此构造出空 `matrixInfo=[]`，随后红点刷新 `fzRedShow → getMatrixInfo → i.mbMatrixInfo[e-1].Lv` 读到 `undefined.Lv` 抛异常。

定位链路（从弹框反查，逐层收敛）：
1. `data/h5game/main.min.js` 第 93 行 `e.fzRedShow` → `var i=this.getMatrixInfo(t,e); if(0==i.Lv)`。
2. `getMatrixInfo` → `i=SubRoles.ins().getSubRoleByIndex(t); return i.mbMatrixInfo[e-1]` → 数组为空返回 `undefined`。
3. `mbMatrixInfo` 的**唯一初始化点**在角色数据解析（`SubRole.parser`，main.min.js 约 699200 处）：`var u=t.readShort()`（子阵数量）→ `for(c=0;u>c;c++){h=new MatrixInfo; h.index=t.readShort(); h.initParser(t); ...}`。它来自 `RoleData` 包，**不是** `CMD=79` 的 `PackageID.Matrix` 系统包（那个只做运行时增量更新 `parseUptateMatrix`）。
4. Go 侧 `internal/engineapi/postlogin.go` 的 `buildRoleData` 末尾原写 `[2B huanhuaId][2B 子阵数=0]` → `u=0` → 空数组。

### 修复（权威依据 + 代码位置）
- 权威：`data/server/s1/Debug/gameworld/data/functions/systems/rolesystem/demoncircle/demoncircle.lua:1160` 的 `_G.packDemonCircleData`（C++ 组角色包时回调它）＋ `config/demoncir/demoncirbase.config`（6 子阵、每阵 5 潜能）、`demoncirskill.config`（4 特质）。
- 改动：`postlogin.go` 的 `buildRoleData` 末尾改为调用 `appendDemonCircleData(p)`；新号全部填 0（未激活）。布局（小端）：
  `[2B huanhuaId][2B 子阵数=6] ×6{[2B subId][2B Lv][2B WYLv][4B WYEXP][2B 特质数=4]×4{[2B id][2B lv]}[2B 领悟次数][2B 潜能数=5]×5{[2B id][2B 激活][4B 额外值]}}`
  子阵段共 `4 + 6×72 = 436` B；`RoleData` 总长 `3677 → 4109`。
- 单测：`postlogin_test.go` 的 `TestBuildRoleData_DemonCircleSegment` 逐字段校验子阵数=6、Lv=0、特质/潜能数量与 id；`TestBuildRoleData_MinimalClientLayout` 长度断言同步为 `3677-4+436`。

### 验证证据
- 真实验证：`node reference/captures/browser_bag_20261125/verify_bag.mjs --relogin` → `reference/captures/browser_bag_20261125/bag_relogin_browser_proof.json` 的 9 项断言全为 `true`（`sceneEntered`、`noClientErrors`、`bagTypesInitialized`、`persistedInitItem` 等），进度条弹框消失并成功进场景（map=2、51 级、背包 200013×1），共 29 个 WS 帧。修复前的失败快照 `bag_relogin_failure.json` 保留作对照（当时 `errors` 为空但卡在进度条）。
- `go test ./...`、`go vet ./...`、`gofmt -l cmd internal`、`git diff --check` 全绿。

### 复发速查清单（以后遇到"进游戏前弹框"按此顺序排）
1. 先看弹框**函数名**：`fzRedShow` / `getMatrixInfo` / `ReadingState` → 基本都是某个 `RoleData`/系统包字段段空或长度不对，不是链路问题。
2. 在 `main.min.js` 搜函数名，找到它读的字段（如 `.Lv`、`.mbMatrixInfo`），再搜该字段的**初始化点**（`xxx=t.readShort()` 循环）确定来自哪个包的哪一段。
3. 对照服务端组包函数（本例 `demoncircle.lua` 的 `packDemonCircleData`）确认字段序与数量；Go 侧补同布局，用单测锁字节。
4. 区分"系统包"与"角色内嵌段"：后者随 `RoleData` 一次下发，前者是独立 CMD（本例 79），不要改错地方。
5. 服务端日志/帧数正常但客户端 `state.errors` 为空时，仍要查 JS 异常对话框——egret 的 alert 不走 CDP `Runtime.exceptionThrown`。
6. 验证脚本卡 `loginLoading:true` 且网关 `10:53 ERROR gate 回向结束` → 多半是重启 gameworld 把网关连接打断导致网关退出，重启网关（参数只用 `-listen/-backend/-gatename`，**别传** `-data/-ostype`）再跑。

### 法阵/红点深度排查（2026-11-25 同日追加，未发现新复发）
用 agent-browser 真机登录 Go 隔离实例后逐项核对：
- `MatrixInfoModel.getMatrixInfo(0, 1..6)` 全部返回 `{Lv:0, WYLv:0, qnLingwuCount:0, TZData:4项, QNData:42/52/62/72/82/92项}`——证明 `appendDemonCircleData` 填的潜能 id（1/11/21/31/41…）让客户端 `QNData[qnIndex]` 正确建索引；`totalRedShow(0)=false` 不抛异常。
- 客户端含 `RedShow` 原型方法的类只有 `MatrixInfoModel` 与 `Ladder.checkRedShow`，后者各参数调用均返回 0，安全。
- 用 `ViewManager.ins().open(Win类)` 走**正规 UI 流程**打开 `MatrixWin / RoleWin / BagWin / MatrixPotentialPanel / SkillWin / ForgeWin / RoleInfoPanel`，全部 OK；法阵面板截图显示 Lv0、6 阵眼、属性对比与"激活"按钮正常，`Commander errors` 无异常。
- 排查陷阱：直接用 `new MatrixWin().open()` 或 `new RoleWin().open()` 会抛 `reading 'length'` / `reading 'index'` / `reading 'qianneng'`——这是**测试方式错误**（缺 stage/roleSelect 初始化与 open 参数），不是服务端数据缺失。判断面板是否真崩必须走 `ViewManager.open()`，不要直接 new。

结论：法阵段修复后无同类空数据弹框；本轮无需再改服务端组包代码。

## 十八、补齐 ClientConfig 客户端配置协议（2026-11-25）

### 缺口来源
IDE 预览实测时 gameworld 日志出现大量未响应上行帧，其中 `sysid=0 cmdid=19`（`cBaseCmd_ClientConfig`）是客户端配置更新请求。排查发现 Go 侧：
- 登录时不下发 `sBaseCmd_ClientConfig(19)`（原版由 `clientconfig.lua:50` 在 `aeUserLogin` 事件里发）；
- netroute 收到 cmdid=19 无特判，走 `OnNetMsg` 但业务系统 Lua 未加载 → 静默丢弃。

### 实现（依据 clientconfig.lua）
- `actorstore.go`：`Actor.ClientConfig map[int]int` + `ClientConfigSnapshot()`（按 key 升序稳定发包）+ `SetClientConfig(key,val)`（标脏待持久化）。
- `postlogin.go`：`buildClientConfig(actor)` 组包 `[2B 数量][每项 4B key+4B val]`，登录包序列插入在 EnterScene/OtherEntity 之后、背包初始化之前（索引 4）；包级函数 `handleClientConfig` 处理 `cBaseCmd_ClientConfig(19)` 请求读 `[4B key][4B val]` 更新静态数据。
- `netroute.go`：新增 `clientConfig` handler 字段 + `SetClientConfigHandler` + RouteUpstream 对 `[00][13]` 特判；`bootstrap.go` 安装 `newClientConfigHandler(store)`（A4 后由原 `handleClientConfig` 包装落库副作用）。
- 单测：`TestBuildClientConfig`（字节布局）、`TestHandleClientConfig`（更新/标脏/错误输入）；因登录包序列 +1，同步修正 `login_test.go` / `postlogin_test.go` / `aoi_test.go` 的回包数与包头断言。

### 验证
- `go test ./...` / `go vet ./...` / `gofmt -l cmd internal` / `git diff --check` 全绿。
- 重建 gameworld 隔离实例后 IDE 预览 + 独立浏览器实测：成功进场景（map=2、51 级），无 JS 异常/TYPEError（仅 headless autoplay 警告，无害），客户端正常消费 `00 13` 包。

### 后续可选（未做，需 golden 依据）
日志中还有 `sysid=10`(WorldBoss)、`76`(拍卖)、`17`(特戒)、`18`(排行)、`37`(公会)、`47`(好友) 等查询请求未响应。这些是 C++/Lua 业务系统，回包协议复杂且无 golden 参照，**未盲目实现**——若用户实测发现某功能"点了没反应"，再针对性抓 C++ golden 逆向后补。

## 十九、A4 持久化最小闭环：ClientConfig 重登保留（2026-11-25）

### 根因
`Actor.Dirty`（含 ClientConfig、Static）之前只是内存标记，没有任何 Flush；`login.go` 仅保存 `level/exp/gold`，背包另存 `actorvariable.variable` 的 `LTGOBAG1` magic-tail。结果是客户端 `[00][13]` 更新 ClientConfig 后，只要重登就回到空值。

### 实现
- 新增 `persist.go`：
  - `actorPersistData{Version, ClientConfig}` 以 JSON 存档，写入 `actorvariable.cs_variable`；
  - 前缀 `LTGOPERS1` 标识 Go A4 数据，旧 blob/其他用途数据不匹配前缀时按空存档处理，避免误解析导致登录失败；
  - `MySQLAccountStore.LoadActorPersist/SaveActorPersist` 负责读写，`INSERT ... ON DUPLICATE KEY UPDATE` 兼容无行角色；
  - `restoreActorPersist()` 在 `postlogin.go` 组装 `sBaseCmd_ClientConfig(19)` 包之前还原，包序仍为 `ActorBaseData/RoleData/EnterScene/OtherEntity/ClientConfig/BagInit×3`；
  - 初始 `flushDirty()` 只清 `ClientConfigDirty`；Static 序列化已在二十节补齐并改为统一 Flush，本节保留阶段记录。
- `actorstore.go`：新增 `ClientConfigDirty`，`SetClientConfig()` 同时标脏；`bootstrap.go` 安装 `newClientConfigHandler(store)`（A4-2 后签名为 `newClientConfigHandler(store,codec)`），在 `[00][13]` 更新后立即落库。
- 选列依据：`actorvariable.variable` 已被背包占用，`storedata/together_hit_equip` 语义属于商店/合击装备；`cs_variable` 是 staticVar 类 blob，Go 隔离库由本服务独占，不与背包 magic-tail 冲突。

### 验证
- 新增 `persist_test.go`：`TestFlushDirtyPersistsClientConfig`（落库/去重）、`TestClientConfigHandlerFlushesDirty`（路由即落库）、`TestPostLoginRestoresPersistedClientConfig`（重登还原且 ClientConfig 包携带旧值）。
- `gofmt -w internal/engineapi` 后 `go test ./...`、`go vet ./...`、`git diff --check` 全绿。
- 本轮按用户约定未开启 IDE 预览/隔离服务，因此未做浏览器重登实测；下一步若起隔离验证，重点观察 `[00][13]` 更新后重登仍保留同一 key/val。

## 二十、A4-2：Static/法阵数据持久化与发包接入（2026-11-25）

### 证据边界
- 原版 `LActor.getStaticVar` 在 s1 有 169+ 处调用，是通用玩家持久化表；仓库没有 `docs/persistence-format.md`，ADR-006 明确不做 C++ 私有 blob 兼容。因此采用 **Go 自有 JSON 存档**，不声称兼容旧 C++ staticVar。
- 法阵字段树与线格式有明确证据：`demoncircle.lua:5-32` 定义 `demoncircledata[roleId].subdemoncircleinfo[subId]`，`:1160-1195` 定义 436B 发包顺序；`demoncirqianneng.config` 给出潜能 `count`；激活规则见 `:382-393`（`arrayeyelv>0 && studycount>=count`）。

### 实现
- `luahost/tablecodec.go`：`TableNode/TablePair` 把 Lua table 编成可 JSON 化的宿主无关节点；保留字符串/整数键、整数/浮点/布尔/字符串/嵌套表；function/userdata 按 nil 落库；循环引用报错；键顺序 JSON 排序保证存档稳定。新增 `Field/Index/AsInt` 供业务读取。
- `persist.go`：`actorPersistData` 增加 `Static *luahost.TableNode`；`restoreActorPersist()` 登录时先解码 Static 再组装回包；`flushDirty()` 一次保存 ClientConfig+Static 当前快照，`ClientConfigDirty/StaticDirty/Dirty` 成功后统一清除；`bootstrap.go` 在登录事件后、角色离场时刷新快照，避免 Lua 表字段写入无法被 Go setter 拦截。
- `postlogin.go`：新增 `buildRoleDataWithDemon()`；`loadDemonCircleData()` 从 `staticVar.demoncircledata[0]` 读取 `huanhuaid/arrayeyelv/nutritionlv/nutritionexp/studycount/tezhiinfo/potentialpoint.extravalue`；`appendDemonCircleData()` 按原版 436B 布局写回 RoleData。无存档时仍全 0，保持此前客户端红点修复不回退。
- 潜能激活不是独立存档字段：按原版从 `arrayeyelv` 与 `studycount` 派生，`demonQiannengStudyRequirements` 来自 `demoncirqianneng.config`。

### 验证
- 新增 `luahost/tablecodec_test.go`：覆盖字符串/数字键、嵌套表、数组、浮点、循环引用、function 跳过。
- 扩展 `persist_test.go`：`TestStaticVarPersistsAcrossRelogin` 验证登录事件写 `demoncircledata` → 落库 → 新 Lua VM 重登读回。
- 扩展 `postlogin_test.go`：`TestBuildRoleDataUsesStaticDemonCircleData` 校验法阵段写入 huanhuaId、阵眼/蕴养/特质/领悟次数/潜能激活/额外值。
- `gofmt -w internal/engineapi internal/luahost` 后 `go test ./...`、`go vet ./...`、`git diff --check` 全绿。仍未开启 IDE 预览；浏览器实测留待下次短周期隔离验证。

## 二十一、M5-H：单种可叠加物品背包闭环（2026-11-26）

### 实现边界
- 使用隔离库 `actor_s1_m5e_m5h_20261125`，克隆既有账号 `haohaiyao` / Actor `65537`；未修改现网 `actor_s1`，未新增表或字段。
- `Actor.Items` 仅保存 `itemID -> count`，所有读写受 `Actor.mu` 保护；先持久化成功再提交内存并通知客户端。当前只有一种可叠加物品 `200013`，不含装备、品质或随机属性。
- 背包写入 `actorvariable.variable`：保留 trailer 前的 C++ staticVar 前缀，末尾追加 `LTGOBAG1`（magic、payload 长度、按 itemID 排序的物品数据、CRC32）；magic 存在但 trailer 损坏时拒绝加载。
- 登录发送普通/装备/寻宝三类 `[03][01]`；击杀奖励接入 `[03][04]` 发放与 `[03][05]` 叠加；使用复用客户端既有 `[03][06]`，下行结果 `0` 成功、`5` 数量不足。ItemData 固定 144B，客户端 handle 在发放、叠加、使用间保持稳定。

### 真实浏览器验收
- 隔离服务：Go gateway `127.0.0.1:19002` → Go gameworld `127.0.0.1:16001`；门户仅临时指向 `19002`。真实 Edge 进入 `GameSceneView`、map=2，无控制台错误。
- mutate：`bag_mutate_browser_proof.json` 的 15 项断言全部通过，127 个 WS 帧；实证 `[03][04] count=1` → `[03][05] count=2` → 客户端 `sendUseItem` 发 `[03][06]` → 成功回包与 `[03][05] count=1`，最终背包 `200013×1`。
- relogin：`bag_relogin_browser_proof.json` 的 9 项断言全部通过，30 个 WS 帧；重登恢复 `200013×1`，无额外背包变更包。
- restart：停止并重新启动 Go gameworld/gateway 后，`bag_restart_browser_proof.json` 的 9 项断言全部通过，30 个 WS 帧；进程重启后仍恢复 `200013×1`。
- 截图：`bag_mutate_initial.png`、`bag_mutate_stacked.png`、`bag_mutate_used.png`、`bag_relogin_initial.png`、`bag_restart_initial.png`。证据目录：`ltcq-go/reference/captures/browser_bag_20261125/`。
- 验收脚本移动断言读取场景实体坐标，不读取静态 `Actor.x/y`；使用物品调用兼容 iframe 与直载两种页面形态。上述修改均在独立验收脚本中，未修改游戏前端 JS、HTML 或图片资源。
- `go test ./...`、`go vet ./...`、`git diff --check`、`node --check verify_bag.mjs` 全绿；`go test -race` 仍待支持 CGO 的环境补做。

### C++ 基线恢复
- 隔离 Go gateway/gameworld、Edge、临时数据库及 `tcpcap` 代理均已停止或清理；门户恢复为 `9002`，`15001/19002/16001/9223` 均未监听。
- `GameWorld.txt` 的 DbServer 已从诊断端口 `15001` 恢复为 `5001`；直连日志显示 `04:34:37 game client ... registe success (gameworld_s1)`，gateway 日志显示 `04:34:48 gateway client ... registe success (gateway_s1)`。
- `wsprobe -addr 127.0.0.1:9002 -account 帅哥` 实测登录回包为 7B `skey + ff 01 00`，C++ 登录链路恢复；5001/6001/7001/9002/80 均正常监听。
- 本次恢复只调整服务进程与临时诊断配置，未修改游戏前端 JS、HTML 或图片资源。

## 二十二、A3 最小技能树与单目标释放（2026-11-26）

### 实现边界
- `skill.go` 从原版 `skills/skillsdesc/skillopen/skillsupgrade.config` 加载三职业五槽技能；校验 1–260 级共 3900 个角色技能配置存在且冷却有效，怪物技能不按角色冷却规则拒绝。
- 五槽开启等级为 `1/5/15/25/35`。首次达到开启等级免费激活 1 级；已学习技能扣除 `SkillsUpgradeConfig[currentLevel].cost`，有效上限为 `min(actor.Level,80)+ZhuanSheng×10`，最高 260。
- 技能存档复用 `actorvariable.cs_variable` 的 `LTGOPERS1` JSON，不新增表或字段。升级通过 MySQL 事务同时提交 `actors.gold` 与技能快照，提交成功后才改内存；事务失败不扣费、不升级。
- 释放仅支持 `DamageRatio>0`、`FlatDamage>0`、`TargetType=2`、`CalcType=1`、`CastType=0`、`AffectCount<=1`、`Teleport=0` 的单目标直接伤害。AOE、buff、治疗、传送不在本轮范围。
- 伤害为 `round(CombatConfig.Damage×DamageRatio+FlatDamage)` 的最小实现，不声称等价于 C++ 完整公式。冷却 key 为 `[actorID, baseSkillID]`，玩家之间隔离，升级不重置冷却。
- `RoleData` 技能段是 5×int32 **等级**，不是配置 ID；客户端 `Role.parser` 用 `SkillData.getSkillByJob(job,slot,level)` 还原配置。首次真实浏览器检查曾把配置 ID 误作等级，导致客户端显示 `22001/24001/...`，已按解析器修正并固化测试。

### 回归测试
- `skill_test.go` 覆盖配置加载、五槽初始化、开启等级、免费激活、费用、金币不足、事务失败无副作用、转生上限、一键升级索引与费用、登录/重登还原、RoleData 等级布局、伤害、冷却、玩家隔离、范围/场景/存活/目标类型校验和 32B 结果包。
- `TestNetRouterDispatchesSkillWithoutLua` 验证 `[02][01/04/05]` 显式进入 Go handler，不依赖 Lua dispatcher。
- `go test ./...`、`go vet ./...`、`git diff --check` 已通过；本机 `CGO_ENABLED=0`，`go test -race` 仍待支持 CGO 的环境补做。

### 隔离浏览器验收
- 使用克隆库 `actor_s1_m5e_skill_20261126`、账号 `haohaiyao`、Actor `65537`，Go gateway/gameworld 分别监听 `19002/16001`；未修改现网 `actor_s1`，未修改游戏前端 JS/HTML/图片。
- mutate 15 项断言全部通过：RoleData 完整 WS 帧 4113B，客户端正确解析五槽 `11001/12001/13001/14001/15001`；单技能升级扣 5000 金币，一键升级后首槽 13 级、金币 7000；技能结果帧 36B，两次伤害均为 250，怪物 HP `2000→1750→1200`，冷却期内没有重复结果，客户端无异常。
- relogin 10 项、gameworld 进程 restart 10 项断言全部通过：重登后首槽仍为 13 级、金币仍为 7000，且没有额外技能变更包。证据见 `ltcq-go/reference/captures/browser_skill_20261126/skill_mutate_browser_proof.json`、`skill_relogin_browser_proof.json`、`skill_restart_browser_proof.json` 和 `skill_db_status.txt`。
- 验收后已停止 Go gateway/gameworld/Edge，删除隔离库、临时二进制和 Edge profile；门户恢复 `9002`。`wsprobe` 实测 C++ 登录回包仍为 7B `skey + ff 01 00`，`5001/6001/7001/9002/80` 正常监听，`16001/19002/9223` 未监听。

## 二十三、A5-1：定时器生产链路与 Actor 查询 API（2026-11-26）

### 定时器修复
- 原实现只有 `Due()`，生产主循环从未调用，Lua 投递的定时器不会执行。现由 `TimerDriver.Start()` 启动 10ms 到点扫描，并在有登录管理器的 Engine 启动时启用；`Engine.Close()` 先停定时器，再关闭数据库与 Lua VM。
- L1 `OnGameTimer` 为分钟节拍，L2 `OnGamePrecisionTimer` 为 5 秒节拍；底层回调参数恢复为原版 `(actor, eid, ...)`。`System.getGlobalNpc` 提供原版 Lua 包装的 nil entity 回退值 0。
- 取消会移除待执行任务；回调执行中取消会阻止周期任务重新入队。登出仍由原版 `postscripttimer.lua` 的 Actor 清理逻辑负责。

### Actor 查询
- `LActor.getZhuanShengLevel` 读取在线 Actor 的 `actors.zhuansheng_lv`，`LActor.getVipLevel` 读取 `actors.vip_level`；两者离线或空参数返回 0，并使用读锁。
- `LActor.getJob/getSex/getSceneId/getExp` 同样读取登录时已加载的 `actors.job/sex/sceneid/exp`，离线或空参数返回 0，并使用读锁。
- `LActor.getServerId/getAccountName/getAccountId/getActorPower` 读取登录会话 ServerID 与 `actors.accountname/accountid/totalpower`；账号名空/离线返回空串，其余返回 0。战力是登录快照，不声称属性变化后实时重算。
- `LActor.getActorLevel/getActorZhuanShengLevel` 是已有等级/转生查询的原版别名。
- `LActor.getActorById` 只返回在线 Actor 句柄，离线或空参数返回 `nil`，不创建伪实体；`LActor.getActorId` 空参数返回 0。

### 奖励 API 核查结论
- 原版 `actorawards.lua` 会把 `LActor.giveAward/giveAwards/canGiveAwards` 覆盖为 Lua 实现，奖励仅 Numeric/Item，`canGiveAwards` 只按装备占用格数检查 `getEquipBagSpace`。
- Go 当前只有单种可叠加快照，没有装备背包实体与占用格模型；背包 `actorvariable.variable` 与 `actors.gold` 又是两条独立写入路径。直接批量调用现有 API 会产生部分发放，不能声称整批成功或原子回滚，因此本轮不实现奖励 API。

### 验证与边界
- 真实 Lua 包装回归覆盖一次性任务、Actor/业务参数、周期任务、普通取消、Actor 清理、回调内自取消及 `Start/Close` 生产驱动。
- Actor 回归覆盖转生/VIP 已加载值、在线/离线/空参数边界，并确认离线查询不会创建 Actor。
- `go test ./...`、`go vet ./...`、`git diff --check` 通过；`go test -race` 仍待 CGO 环境。
- 本轮为源码与自动化回归验收，未启动隔离 Go 服务、未改前端、未新增表或字段、未修改共享数据库。奖励类 API 尚未实现，不将注册成功的 stub 计入覆盖门禁。

## 二十四、A5-3：背包总数量与经验/VIP 查询别名（2026-11-26）

### 实现依据
- `LActor.getBagItemCount` 在原版 `test_npc_trade.lua`、`test_lottery.lua` 中用于交易/抽奖前后的背包数量变化断言，不是装备容量或占用格数。Go 返回在线 Actor `Items` 中各物品数量之和，空背包返回 0。
- `LActor.getActorCurExp` 在 `actorlogout.lua` 中作为当前角色经验记录，直接复用 `getExp`；`LActor.getVIPLevel` 在 `test_lottery.lua` 中读取 VIP 等级，直接复用 `getVipLevel`。
- 三个 API 均使用在线 Actor 查询和读锁；空参数或离线返回 0，不创建伪 Actor。

### 写 API 核查结论
- `actorexp.lua:confirmExp` 连续调用 `setLevel`、`setExp` 后由 Lua 发送 `UpdateExp`。Go 当前没有 `setLevel`，单独实现 `setExp` 不能形成完整经验升级链。
- `actorvip.lua` 的 `setVipLevel` 后会调用 `updateVipAttr` 并发送 VIP 更新包。现有 `flushDirty` 只保存 ClientConfig/Skills/Static，不保存 `actors.vip_level`；仅改内存会在重登时丢失。
- 因此本轮不注册内存假写实现。后续需分别补齐经验链的持久化顺序与 VIP 字段原子存档，并增加存档失败时不改内存的测试。

### 验证与边界
- `TestActor_QueryBagItemCountAndAliases` 覆盖多种物品数量求和、空背包、空参数、离线返回 0 和查询不创建 Actor。
- 聚焦 Actor 查询测试、`go test ./...`、`go vet ./...`、`git diff --check` 通过；`go test -race` 仍待 CGO 环境。
- 本轮只改 Go API/测试与本项目文档，未启动隔离服务，未修改前端、数据库 schema、共享数据库或 C++ 基线服务。

## 二十五、A5-4/A5-5：登录元信息查询与 VIP 原子存档（2026-11-26）

### 元信息查询
- `LActor.getRecharge/getCreateTime/getAppid/getPf/getPfId` 读取在线 Actor 的登录快照，分别映射现有 `actors.recharge/createtime/appid/pfid` 字段；`getPfId` 与 `getPf` 共用平台字段实现。
- `actors.createtime` 是 `datetime`，查询列使用 `COALESCE(UNIX_TIMESTAMP(createtime),0)` 直接扫描为 Unix 秒，不依赖 DSN 的 `parseTime` 设置。
- 查询只使用 `ActorManager.Get` 和读锁；空参数或离线 Actor 返回 0/空串，不创建伪 Actor。未新增表或字段。

### VIP 写入顺序
- `LActor.setVipLevel` 通过独立可选接口 `VIPLevelStore` 调用 `MySQLAccountStore.SaveVIPLevel`，没有扩张通用 `AccountStore` 接口。
- 写入持有 Actor 锁，先执行单字段 `UPDATE actors SET vip_level=?, updatetime=NOW()`；数据库成功后才更新内存。同等级调用幂等跳过，负等级、非整数等级、离线 Actor 和缺少存档器均返回错误。
- `RowsAffected()==0` 时继续查询 Actor 是否存在，以区分“值未变化”和“角色不存在”；存档失败保持在线 VIP 不变。
- 登录数据装配后、客户端回包和 `aeUserLogin` 前绑定存档器。`actorvip.lua` 登录事件中的首次 VIP 设置因此走真实落库路径；VIP 属性重算和协议包仍由原版 Lua 负责。

### 验证与边界
- `TestActor_QueryLoginMetadata` 覆盖五个 API 的在线值、空参、离线默认值和查询不创建 Actor。
- `TestActor_SetVipLevelPersistsBeforeMemoryUpdate` 覆盖成功同步、存档失败不改内存、幂等写入、缺少存档器及非法参数。
- `TestPostLoginSendsBaseRoleSceneAndBagPacketsBeforeLoginEvent` 增加登录元信息快照断言，并在 Lua 登录事件内实际调用 `setVipLevel`，验证事件触发前已绑定存档器。
- `go test ./... -count=1`、`go vet ./...`、`git diff --check` 通过；`go test -race` 仍待 CGO 环境。
- 端口复核：C++ 基线 `80/5001/6001/7001/9002` 均正常监听，隔离端口 `19002/16001` 关闭。本轮未启动隔离服务、未修改前端、schema 或共享数据库。
- 仍暂缓：奖励 API 缺装备背包容量和跨表原子性；`setExp` 缺 `setLevel` 与完整经验升级链；`getLastLoginIp` 的整数 IP 编码尚未证实。

## 二十六、A5-6：货币、玩法元信息与在线按名查询（2026-11-26）

### 实现依据
- `LActor.getMoneyCount` 与原版 `getCurrency` 共用在线货币查询。`_G.lua` 已确认 mtBindCoin=0、mtCoin=1、mtBindYuanbao=2、mtYuanbao=3；登录只把现有 `actors.gold/yuanbao` 装入类型 1/3，绑定货币和未知类型返回 0。
- `getGuildId/getGuildName/getChapterLevel/getMonthCard/getReincarnateLv` 复用现有 `actors.guildid/guild_name/chapterlevel/monthcard/reincarnate_lv` 列，登录查询和扫描顺序同步扩展，未新增表或字段。
- `isActor/getActorByName/getActorIdByName` 仅查询当前在线 Actor。按名查询遇到重名返回 nil/0，避免 map 遍历顺序造成不确定结果。
- 所有新增查询使用 `ActorManager.Get` 和 Actor 读锁，空参或离线返回 0/空串/false/nil，不创建伪 Actor。`getCurrency/changeCurrency` 的货币表读写同时增加 Actor 锁。

### 写入边界
- A5-6 当批 `changeCurrency` 仍只修改在线内存；A5-8 已把金币/元宝接入现有字段持久化，详见第二十八节。
- 奖励 API 继续暂缓：当前可叠加物品包与 `actors` 分表存储，且无装备背包容量模型，无法证明物品、货币和背包空间检查的整体原子性。

### 验证与边界
- `TestActor_QueryMoneyCount` 覆盖金币/元宝、绑定与未知类型、空参、离线、查询不创建 Actor，以及 `changeCurrency` 后的可见性。
- `TestActor_QueryExistingActorMetadata` 覆盖公会、章节、月卡、轮回字段和离线默认值；`TestActor_OnlineNameLookupAndIsActor` 覆盖在线命中、空名、非字符串、离线和重名失败关闭。
- `TestActor_CurrencyConcurrentAccess` 以 4 写 4 读、每路 250 次验证金币最终值为 1000；普通并发测试不等同于 `go test -race`。
- `TestPostLoginSendsBaseRoleSceneAndBagPacketsBeforeLoginEvent` 验证五个登录快照字段在进场景回包和 Lua 登录事件前装配完成。
- `go test ./... -count=1`、`go vet ./...`、`git diff --check` 通过；`go test -race` 仍待 CGO 环境。
- 端口复核：C++ 基线 `80/5001/6001/7001/9002` 均正常监听，隔离端口 `19002/16001` 关闭。本轮未启动隔离服务、未修改前端、schema 或共享数据库。

## 二十七、A5-7：职业性别别名与在线账号查询（2026-11-26）

### 实现依据
- `peakracesystem.lua:195-196` 在跨服 16 强名单中按 Actor 句柄读取职业、性别，确认 `getActorJob/getActorSex` 分别为现有 `getJob/getSex` 的别名。
- `txvipaward.lua:543,558` 用 `getActorByAccountName` 判断账号是否在线，离线时转入全局队列；`gmhandler.lua:239` 用 `getActorIdByAccountName` 取得在线角色 ID。二者只复用当前在线 Actor 的登录账号快照。
- 账号查询精确匹配；空账号、非字符串、未命中或同一账号多个在线 Actor 时，句柄查询返回 nil、ID 查询返回 0，避免 map 遍历顺序造成不确定结果。
- 所有新增查询不创建 Actor，不访问离线数据库，也不扩张 `actors` 表字段。

### 暂缓项
- `getRealLevel` 的 22 个调用点只证明它参与等级门槛判断，尚未证明与 `getLevel` 完全同义，不能直接替换。
- `getRealServerId` 仅出现在跨服路由，且无源码证明它与登录会话 `ServerID` 同义；跨服模型未接通前不猜测实现。

### 验证与边界
- `TestActor_QueryJobSexSceneAndExp` 扩展覆盖 `getActorJob/getActorSex` 的在线值、空参、离线和查询不创建 Actor。
- `TestActor_OnlineAccountLookup` 覆盖两个账号查询的在线命中、空参、空账号、非字符串、未命中、账号重名失败关闭及不创建离线 Actor。
- `go test ./... -count=1`、`go vet ./...`、`git diff --check` 通过；`go test -race` 仍待 CGO 环境。
- 端口复核：C++ 基线 `80/5001/6001/7001/9002` 均正常监听，隔离端口 `19002/16001` 关闭。本轮未启动隔离服务、未修改前端、schema 或共享数据库。

## 二十八、A5-8：金币/元宝写入持久化（2026-11-26）

### 实现依据与边界
- 新增独立可选接口 `CurrencyStore`，没有扩张通用 `AccountStore`。MySQL 仅白名单更新现有 `actors.gold/yuanbao` 单列及 `updatetime`，未新增表或字段。
- `LActor.changeCurrency` 对 `type=1/3` 在 Actor 锁内校验严格整数、当前余额、负余额和整数溢出；先调用存档器，数据库成功后才更新内存。存档失败不改内存，离线 Actor 不创建伪实体。
- `delta=0` 幂等成功，不要求存档器也不写库；绑定币种 `type=0/2` 和其他数值类型仍只改内存，不伪造持久化能力。
- 登录数据装配后、场景回包和 `aeUserLogin` 前绑定或清空存档器；登录事件内的金币/元宝写入因此可落库。
- 修复 `luahost` 数值转换：Lua 整数且可安全表示时仍转 Go `int`，小数、NaN/Inf 和超出宿主整数范围的值保留为 `float64`，避免在 Go 校验前被截断。

### 验证与边界
- `TestActor_ChangeCurrencyPersistsGoldAndYuanBao` 覆盖金币/元宝正负变更后的内存与 mock 存档同步，以及零变更不写库。
- `TestActor_ChangeCurrencyRejectsInvalidAndInsufficient`、`TestActor_ChangeCurrencyRejectsOverflow`、`TestActor_ChangeCurrencyRequiresStoreAndRollsBackSaveFailure` 覆盖缺参、非整数、字符串、nil、非法/离线 Actor、余额不足、溢出、缺存档器和存档失败不改内存。
- `TestActor_ChangeCurrencyMemoryOnlyTypesSkipStore` 覆盖绑定币种及未知数值类型不写库；`TestMySQLAccountStore_SaveCurrencyRejectsUnsupportedInput` 覆盖存档接口边界；`TestActor_CurrencyConcurrentAccess` 保留并发最终值校验。
- `TestPostLoginSendsBaseRoleSceneAndBagPacketsBeforeLoginEvent` 在 Lua 登录事件内实际调用金币变更，验证事件前已绑定存档器；`TestGopherLua_NumberConversionPreservesFractionAndRange` 守住 Lua 数值转换边界。
- `go test ./... -count=1`、`go vet ./...`、`git diff --check` 通过；`go test -race` 仍待 CGO 环境。
- 端口复核：C++ 基线 `80/5001/6001/7001/9002` 均正常监听，隔离端口 `19002/16001` 关闭。本轮未启动隔离服务、未修改前端、schema 或共享数据库。
- 奖励 API 仍暂缓：单币种持久化不等于跨物品、多币种和背包容量的整体原子事务；普通并发测试也不能替代 race 检测。

## 二十九、A5-9：在线玩家解析与单主角色查询（2026-11-26）

### 实现依据与边界
- `worldboss.lua:939-946` 用 `LActor.getActor(attacker)` 把攻击者实体解析为玩家并设置首领归属；Go 当前玩家实体句柄即 actorID，只允许解析在线 Actor，怪物或未知句柄返回 nil。
- `togetherhit.lua:123-127`、`actorrole.lua:75-79` 按 `0..getRoleCount(actor)-1` 遍历角色。Go 当前只有主角色，没有副角色或 `player.Roles[]` 模型，因此在线 `getRoleCount` 固定返回 1，`getRole(actor,0)` 返回同一 actorID，其他索引返回 nil。
- 三个 API 都使用严格正整数句柄；`getRole` 的索引也必须是整数。空参、nil、字符串、小数、NaN/Inf、越界整数和离线 Actor 返回类型默认值，不创建 Actor。
- `integerArgument` 由货币参数校验改为通用严格整数解析，`changeCurrency` 行为不变；没有新增表、字段、协议包或前端改动。

### 验证与边界
- `TestActor_GetActorAndRoleQueries` 覆盖在线玩家解析、整数与整数浮点句柄、单主角色 count/role、空参、非法句柄、怪物/离线句柄、非整数和越界索引，以及查询不创建 Actor。
- `go test ./... -count=1`、`go vet ./...`、`git diff --check` 通过；`go test -race` 仍待 CGO 环境。
- 端口复核：C++ 基线 `80/5001/6001/7001/9002` 均正常监听，隔离端口 `19002/16001` 关闭。本轮未启动隔离服务、未修改前端、schema 或共享数据库。
- 多角色仍明确未实现：不能把 count=1 当作副角色、合击多角色属性或角色切换能力已经接通。

## 三十、A5-10：技能等级与职业只读查询（2026-11-26）

### 实现依据与边界
- `skill.lua:14-24` 使用 `getRoleSkillLevel(actor, roleid, index)` 在技能升级前读取当前等级。Go 仅支持 `roleid=0` 与 `index=0..4`，通过 `skillBaseID(job, index+1)` 读取在线 Actor 已保存的五槽技能等级。
- `zhanlingsystem.lua:318-323` 使用 `GetRoleByJob(actor, job)` 获取指定职业角色以挂被动技能。Go 仅支持职业 1..3，当前主角色职业匹配时返回 actorID，否则返回 nil。
- 两个 API 都使用严格整数解析与 Actor 读锁；空参、小数、NaN/Inf、越界整数、非法槽位/职业、未学习和离线输入返回类型默认值，不创建 Actor。
- 没有新增表、字段、协议包、实体接口或前端改动。`changeGold` 仍暂缓：现有 Lua 写入证据不足以确定其客户端金额通知语义。

### 验证与边界
- `TestActor_GetRoleSkillLevel` 覆盖五槽等级、未学习槽位、整数浮点、roleid/index 边界、非法职业、非法句柄、离线查询与不创建 Actor。
- `TestActor_GetRoleByJob` 覆盖职业命中、整数浮点、职业不匹配、非法职业、非法句柄、空参与离线查询。
- `go test ./... -count=1`、`go vet ./...` 通过；`go test -race` 仍待 CGO 环境。
- 端口复核：C++ 基线 `80/5001/6001/7001/9002` 均正常监听，隔离端口 `19002/16001` 关闭。本轮未启动隔离服务、未修改前端、schema 或共享数据库。

## 三十一、A5-11：场景实体解析查询（2026-11-26）

### 实现依据与边界
- `citysystem.lua:285-290` 使用 `getEntity(killerHdl)` 解析击杀者实体，再用 `getActorByEt(getAITarget(et))` 判断 AI 目标玩家；该链路也出现在 `worldboss.lua`、`campbattlefb.lua`、`crossarenafb.lua` 等系统。
- Go 当前实体句柄用数字 ID 表示。`getEntity` 只读查询进程共享 `scene.World`，玩家和怪物均可解析；`getActorByEt` 额外要求 `EntityPlayer` 且 `DefaultActorManager` 中存在对应在线 Actor。
- World 在 `bootstrapWithLogin` 启动时通过 `atomic.Pointer` 绑定，避免多网关连接重复写入共享指针产生数据竞争；未绑定 World 时两个 API 均返回 nil。
- 两个 API 都使用严格正整数句柄。空参、nil、字符串、小数、NaN/Inf、越界整数、未知实体、怪物转 Actor 和只有 Actor 没有场景实体均返回 nil，不创建 Actor。
- 没有新增表、字段、协议包、场景接口或前端改动。`isPet` 暂缓：当前 scene 只有 Player/Monster，不能把恒 false 占位当作宠物能力已实现。

### 验证与边界
- `TestActor_EntityResolutionQueries` 覆盖玩家/怪物实体命中、整数浮点句柄、玩家实体转在线 Actor、怪物拒绝、无场景 Actor、未知句柄、非法参数、空参、未绑定 World 和查询不创建 Actor。
- `go test ./... -count=1`、`go vet ./...` 通过；`go test -race` 仍待 CGO 环境。
- 端口复核：C++ 基线 `80/5001/6001/7001/9002` 均正常监听，隔离端口 `19002/16001` 关闭。本轮未启动隔离服务、未修改前端、schema 或共享数据库。

## 三十二、A5-12：GM、镜像与副本状态失败关闭查询（2026-11-26）

### 实现依据与边界
- `gmsystem.lua:21` 要求 `getGmLevel(actor)==0` 时拒绝 GM 命令；当前没有 GM 权限模型，固定返回 0，避免普通玩家绕过权限检查。
- `chapter.lua:490` 用 `isImage` 排除镜像 Actor；当前只有真实在线 Actor，固定返回 false。
- `citysystem.lua:733`、`miscfunc.lua:202`、`campbattlefb.lua:627` 分别依赖副本句柄、副本 ID 与副本状态；当前没有副本实例模型，`getFubenHandle/getFubenId` 固定返回 0，`isInFuben` 固定返回 false。
- 修正 `getFubenHandle` 原 `stubInt1` 实现：它会把 actorID 当副本句柄返回，可能让 `actorID == fbHandle` 的偶然比较成立。本批改为显式失败关闭函数。
- 五个 API 对空参、在线/离线 Actor、非法句柄、小数、字符串和越界整数均返回相同默认值，不创建 Actor，不写数据库。该批只保证安全默认，不代表 GM、镜像或副本能力已经实现。

### 验证与边界
- `TestActor_FailClosedStatusQueries` 覆盖五个 API 的空参、在线/离线、非法句柄、小数、字符串和越界整数，并验证 `getFubenHandle(7)` 返回 0 而不是 7。
- `go test ./... -count=1`、`go vet ./...` 通过；`go test -race` 仍待 CGO 环境。
- 端口复核：C++ 基线 `80/5001/6001/7001/9002` 均正常监听，隔离端口 `19002/16001` 关闭。本轮未启动隔离服务、未修改前端、schema 或共享数据库。

## 三十三、A5-13：实体句柄、坐标与属性只读查询（2026-11-26）

### 实现依据与边界
- `postscripttimer.lua:20` 使用 `getHandle(entity)` 作为实体级 Lua 表键。Go 只做严格正整数句柄身份提取，合法句柄原样返回；空参或非法参数返回 0，不查询 World、不创建 Actor。
- `test_gathersystem.lua:47` 使用 `local x,y = getEntityPosition(actor)`；`rabbit.lua:242-246` 使用 `getIntProperty(monster, P_POS_X/P_POS_Y)`。坐标读取共享 `scene.World` 的实体快照，未命中或未绑定 World 返回 `(0,0)`。
- 为保留 Lua 双返回值，`engineapi.Entry` 增加 `FnMulti`，`Interpreter`/`GopherLua` 增加 `RegisterMulti`；原单返回值 `Register` 语义保持不变。
- `getIntProperty` 只映射 `_G.lua:1011-1124` 与现有数据对应的白名单：实体 ID/坐标/HP/MaxHP，以及 Actor 等级、性别、职业、经验、公会、四类 mt 货币、战力、充值、VIP。World 实体优先；`P_ID` 对玩家返回 actorID、对怪物返回 ConfigID；怪物 Actor 专有属性、未知属性和尚未证实的 `P_ROOT_EXP=86` 返回 0。
- 查询使用严格整数参数和 Actor 读锁，不创建 Actor、不写数据库，没有新增表、字段、协议包或前端改动。

### 验证与边界
- `TestGopherLua_RegisterMulti` 验证 Go 多返回值经真实 gopher-lua 同时返回两个值。
- `TestActor_EntityHandlePositionAndProperties` 覆盖玩家/怪物句柄、移动后的实时坐标、实体与 Actor 属性白名单、怪物受伤后的实时 HP、无场景 Actor、未绑定 World、非法/越界参数和查询不创建 Actor。
- `go test ./... -count=1`、`go vet ./...`、目标文件 `git diff --check` 通过；`go test -race` 仍待 CGO 环境。
- 用户要求关闭 Go 服务后，已停止 `19002/16001` 隔离进程，将门户恢复 C++ `9002`，删除隔离库 `actor_s1_m5e_live_20261126_a` 并清理 `.tmp_live`。端口复核：C++ `80/5001/6001/7001/9002` 正常监听，`19002/16001` 已关闭；本轮不把浏览器场景进入记为已验收。

## 三十四、A5-14：血量、位置、死亡状态与实体 ID（2026-11-26）

### 实现依据与边界
- `ai_action.lua:88-92` 用 `getHp/getHpMax` 计算怪物血量百分比；`newworldboss.lua:541-542` 用 `getPosition` 获取复活坐标；`togetherhit.lua:121-127` 用 `isDeath` 筛选未死亡角色。
- `bossinfo.lua:135-146` 用 `getId` 作为 boss 配置表键，`guardgodweaponfb.lua:186-202` 以 `MonstersConfig[monId]` 访问该 ID；玩家侧 `onRoleDie` 也使用 `getId(role)` 作为角色键。Go 对玩家返回 actorID、对怪物返回 ConfigID。
- `getHp/getHpMax/getPosition/isDeath` 优先读取共享 `scene.World` 实体快照，World 未命中时回退在线 Actor 的 `HP/MaxHP/X/Y`；Actor 回退使用读锁。`getPosition` 经 `FnMulti` 保留 Lua `(x,y)` 双返回值，`isDeath` 按 HP<=0 判断。
- 据 `monsteractionimpl.txt:92-94` 的 `P_ID` 模板匹配用法，修正 A5-13：怪物 `P_ID` 返回 ConfigID，不再返回运行时实体句柄。
- 严格校验正整数句柄；空参、小数、NaN/Inf、字符串、越界整数和未知句柄返回 `0/0/(0,0)/false/0`。不创建 Actor、不写数据库，没有新增表、字段、协议包或前端改动。

### 验证与边界
- `TestActor_HealthPositionDeathAndEntityID` 覆盖玩家/怪物血量与最大血量、World 对 Actor 快照的优先级、移动后的坐标、怪物受伤、玩家死亡、Actor 坐标/HP 回退、玩家/怪物 ID、非法/离线句柄和查询不创建 Actor。
- `TestActor_EntityHandlePositionAndProperties` 同步断言怪物 `P_ID=ConfigID`。
- `go test ./... -count=1`、`go vet ./...`、目标文件 `git diff --check` 通过；`go test -race` 仍待 CGO 环境。
- 本轮隔离服务保持关闭，门户继续使用 C++ `9002`；未修改游戏前端、schema 或共享数据库。

## 三十五、A5-15：游戏渠道派生查询（2026-11-26）

### 实现依据与边界
- `utils/actorfunc.lua:103-112` 对 `LActor.getPf` 做 Lua 包装：字符串以 `union` 开头且包含 `*` 时，取首个 `*` 后的子串，否则原样返回。
- Go 注册 `LActor.getGamePf`，在 Actor 读锁下读取 `PfID` 并执行同一派生规则；普通渠道、union 前缀、多个 `*`、大小写差异、无星号和空串均按原版行为处理。
- 严格校验正整数 Actor 句柄；空参、小数、NaN/Inf、字符串、越界整数或离线 Actor 返回空字符串。查询不创建 Actor、不写数据库，没有新增表、字段、协议包或前端改动。
- `getEntityType` 继续暂缓：虽然 `actorfunc.lua:93`、`actorrole.lua:98`、`test_gathersystem.lua:56`、`instance.lua:452-460` 有调用点，但当前仓库未找到 `EntityType_*` 数值定义，不能把 Go 内部 `EntityPlayer/EntityMonster` 枚举猜作原版常量。

### 验证与边界
- `TestActor_GetGamePf` 覆盖普通渠道、union 派生、首个星号、尾随星号、大小写、非 union 含星号、空串、整数浮点句柄、非法/离线输入、查询不创建 Actor 和并发只读。
- `go test ./... -count=1`、`go vet ./...`、目标文件 `git diff --check` 通过；`go test -race` 仍待 CGO 环境。
- Go 隔离服务保持关闭，门户继续使用 C++ `9002`；未修改游戏前端、schema 或共享数据库。

## 三十六、定时器真实节拍链告警修复（2026-11-26）

### 根因与修复
- 隔离运行曾出现 `attempt to call a non-function object`：5 秒链依赖的 `System.getNowTime/getStaticVar/isSameDay` 和分钟链依赖的 `System.getNowTime/timeDecode` 未注册；supertimer 在任务触发及打印时间时直接调用这些 API。
- `TimerDriver.Due` 原先传入 `(actorID, pid, args...)`，但 `postscripttimer.lua:108` 注册底层任务时已经把 Lua `eid` 作为额外参数；Go 再传自己的 `pid` 会挤占第一个业务参数。现在只传 `(actorID, args...)`，由 Lua 包装恢复 `onTimeEvent(actor, eid, ...)`。
- 底层回调失败现在记录 `pid/callback/err`，不再静默吞错；任务清理、已触发次数扣减和周期重排仍继续，避免单个 Lua 错误遗留队列或阻止后续任务。
- `System.getNowTime` 返回 Unix 秒；`timeDecode` 使用本地时区返回六个时间字段，非法输入返回六个 `0`；`isSameDay` 比较本地年份与年中日，非法输入返回 `false`。
- `System.getStaticVar` 返回每个 Lua VM 安装时创建的唯一表，用于 `engineevent` 跨节拍保存状态；当前仅进程内共享，VM 关闭即释放，不声称持久化。它不同于按 Actor 存储的 `LActor.getStaticVar`。

### 验证与运行边界
- `TestTimer_RealLuaTickChain` 加载真实 engineevent/supertimer：5 秒链首次登记 NewDay、同日第二次不重复，分钟节拍执行零延迟 supertimer 任务。
- `TestSystem_TimeAndStaticVar` 覆盖当前时间、同日/不同日、六返回值、非法时间输入和静态表引用稳定性；`TestTimer_PostScriptLiteWrapper` 先消耗一个底层任务编号，使 Lua `eid` 与 Go `pid` 不同，防止编号巧合掩盖参数错位。
- 修复前定向测试曾分别出现 `newDayCount` 未更新、`System.getNowTime` 未定义和 Lite 回调未收到 `4242`；修复后定向测试及 `go test ./... -count=1`、`go vet ./...`、目标文件 `git diff --check` 全部通过。
- 本轮未启动 Go 服务，门户继续使用 C++ `9002`；未修改游戏前端、schema、共享数据库或现网账号数据，也未提交或推送。

## 三十七、A5-16：元宝变更别名（2026-11-26）

### 实现依据与边界
- 原版有 64 处 `LActor.changeYuanBao` 调用，参数为 `(actor, delta, reason, ...)`；`actorcost.lua:56`、`campbattlefb.lua:539`、`auctionsystem.lua:587/927` 均按元宝数量增减使用，拍卖处额外传入 `true`。
- Go 将 `changeYuanBao` 固定映射为 `NumericType_YuanBao=3`，复用 `changeCurrency` 的 Actor 锁、严格整数、余额与溢出校验以及可选 `CurrencyStore`。保存现有 `actors.yuanbao` 成功后才更新内存，失败不改变余额。
- `delta=0` 幂等成功且不写库；离线 Actor、非法输入、余额不足、溢出或缺少存档器返回错误。第四个及之后的业务参数不改变金额语义；不新增表、字段或协议包。

### 验证与运行边界
- 覆盖元宝增减与金币隔离、额外 `true` 参数、空参/小数/字符串/NaN/Inf/非法句柄、余额不足、整数溢出、缺存档器、存档失败回滚、离线写入不创建 Actor 和 8 路并发递增。
- `go test ./... -count=1`、`go vet ./...`、目标文件 `git diff --check` 通过；`go test -race` 仍待 CGO 环境。
- Go 隔离服务保持关闭，门户继续使用 C++ `9002`；未修改游戏前端、schema、共享数据库或现网账号数据，也未提交或推送。

## 三十八、A5-17：玩家业务结构化日志（2026-11-26）

### 实现依据与边界
- 原版 `LActor.log(actor, ...)` 有 91 处调用，覆盖好友、副本、BOSS、商城等业务事件；当前 Go 注册项此前是空操作，事件会被静默丢弃。
- 本轮改为本地 `slog.Info("LActor.log")`，输出 `actorID` 与字符串数组 `args`。Actor 句柄仅做严格正整数解析：非法输入记录为 0，离线正整数保留原 ID；不要求 Actor 在线，也不查询或创建 Actor。
- 日志调用始终成功，不修改角色、货币、背包或数据库状态。`nil` 记录为 `nil`，布尔与数值按字符串保存；Lua 表仅按宿主默认字符串形式记录，不声称完整序列化。
- 本轮没有实现 7001 协议转发或日志库写入，不能把本地 slog 输出当作原版 loggerserver 已联通。

### 验证与运行边界
- `TestActor_LogStructuredAndStateless` 验证 JSON 日志的消息、级别、Actor ID、标量与 nil 参数，以及不创建 Actor；`TestActor_LogHandlesInvalidAndOfflineActors` 覆盖空参、0、负数、小数、字符串、NaN、Inf 与离线正整数。
- `go test ./... -count=1`、`go vet ./...`、目标文件 `git diff --check` 通过；`go test -race` 仍待 CGO 环境。
- Go 隔离服务保持关闭，门户继续使用 C++ `9002`；未修改游戏前端、schema、共享数据库或现网账号数据，也未提交或推送。

## 三十九、A5-18：共享战斗 AI 控制（2026-11-26）

### 实现依据与边界
- 原版 `actorrole.lua:89-110` 用 `setAITarget/setAIAttackMonster/stopAI` 控制切换目标、恢复自动打怪与停战；`citysystem.lua:282-293` 读取击杀者目标，`citysystem.lua:455` 调 `setAITargetNull` 清目标；`ai_action.lua:108-118`、`worldboss.lua:946` 允许怪物指定玩家。
- 玩家停战标记与玩家显式目标由进程共享 `CombatManager` 锁保护，怪物仇恨复用 `aggro`。`installCombatAPI` 按每个 Lua VM 注册捕获各自 Combat 的闭包，删除包级全局 Combat，避免多连接状态串线。
- `stopAI` 只停止在线存活玩家的自动战斗并保留目标；`setAITarget` 恢复玩家战斗并优先攻击显式目标，目标超出范围时不回退最近怪；`setAIAttackMonster` 清显式目标、解除停战并恢复最近怪索敌；`setAITargetNull` 只清玩家显式目标或怪物仇恨，保留停战状态。
- 当前仅支持玩家→怪物、怪物→玩家且同场景存活的目标。PvP、怪物→怪物、跨场景、死亡/离场、非法句柄、镜像、副本和副角色均失败关闭；不创建 Actor、不新增表/字段/协议。
- `getAITarget` 对失效目标返回 `nil` 并清理引用；击杀立即清理显式目标，断线与登录事件失败回滚均清理玩家 AI 和怪物仇恨。路由生命周期锁串行化旧连接离场与同号 `BindActor`；索敌重新读取 World 的实时 HP、位置与场景，跳过同 Tick 已死亡或已离场目标。`setAIPassivity/reCalcAttr/getGuildPtr` 仍因交互语义、属性管线或公会对象模型缺失而暂缓。

### 验证与运行边界
- 覆盖真实 gopher-lua 调用、显式目标优先、停战保留目标、清目标保留停战、恢复索敌、越界不回退、怪物指定玩家、死亡/离场/跨场景清理、PvP/非法配对、空参/小数/字符串/NaN/Inf/离线、未绑定 Combat、双 VM Combat 隔离、同号重连、登录回滚、同 Tick 死亡目标跳过与 8 路并发。
- `go test ./... -count=1`、`go vet ./...`、目标文件空白检查和 `gofmt` 检查通过；`go test ./internal/engineapi -shuffle=on -count=3` 与全量 `go test ./... -shuffle=on -count=1` 通过，未发现共享全局状态顺序泄漏；`go test -race` 仍待 CGO 环境。
- Go 隔离端口 `16001/19002` 未监听，C++ 基线 `9002/6001/5001/7001/80` 均在监听；未修改游戏前端、schema、共享数据库或现网账号数据，也未提交或推送。

## 四十、A5-19：setIntProperty 白名单写入（2026-11-26）

### 实现依据与边界
- `_G.lua:1011-1124` 的 property 枚举全集已确认；A5-13 已实现同白名单的 `getIntProperty` 只读映射，本批补齐对称写入侧。
- 等级、性别、职业、经验、公会 ID、战力、累计充值直接写 Actor 现有字段并标 `Dirty`，不声称已接入 `reCalcAttr` 或完整升级链；`setExp/setLevel` 仍按证据不足暂缓。
- 金币/元宝/绑定币按**绝对数量**写入：金币/元宝先经现有 `CurrencyStore` 写 `actors.gold/yuanbao` 再改内存，绑定币仅改内存，与 `changeCurrency` 落库边界一致；VIP 复用 `setVipLevel` 的先落库后改内存链路。
- `P_ID`、坐标、`P_HP/P_MAXHP` 读取以 World 实体为准，单独写 Actor 会造成读回不一致，继续失败关闭；未知属性、非法/离线句柄、非整数参数、负数金额/等级同样不改状态、不抛 Lua 错误。

### 验证与运行边界
- `setintproperty_test.go` 覆盖白名单字段 Lua 写入后 `getIntProperty` 回读、金币/元宝存档调用与内存值、绑定币不写 actors 表、VIP 存档、存档失败回滚、非法参数与离线句柄。
- 新增 `setCurrencyAmount` 仅用于绝对数量写入，不复用 `changeCurrency` 的增量/溢出语义；零变更不写库。
- 未新增数据库表/字段、未改协议或前端资源，也未触碰共享数据库；现有源码与文档已分批提交，抓包产物与 `.codebuddy` 计划文件未纳入提交。

## 四十一、A5-20：阵营标签 setCamp/getCamp（2026-11-26）

### 实现依据与边界
- `setCamp` 51 处、`getCamp` 11 处。实参包括自身 actorID、guildID、`CampType_Player`、`WorldBossCampType_*`、副本分配值（100/200）等；`getCamp` 用于阵营比较、排行榜暂存和 `crosswujifb.lua:331` 的发包数据。
- `CampType_*` 全局枚举数值在 `_G.lua` 与原版 Lua 资产中只有引用没有定义；已确认的仅是 `leadfuben.lua:3-7` 文件内 local 表 0/1/2 与 `crosswujifbmgr.lua:4-7` 的 100/200。因此 Go 只校验整数性，不做 0..2 范围校验。
- 只给在线 Actor 增加进程内 `Camp` 标签：不写库、不标脏、不发协议、不接 `CombatManager` 敌对判定；怪物/机器人句柄、`getCampId`、`Fuben.setMonsterCamp` 不在本批范围。

### 验证与运行边界
- `camp_test.go` 覆盖 0/1/2/100/200/大整数/负数的 Lua 写入回读、float64 整数值接受、空参/非整数/离线/不存在句柄的失败关闭，以及写入后 `Dirty` 仍为 false。
- `P_CAMP=94` 仅见原版 DC 日志读取，未见 `setIntProperty(P_CAMP, ...)`；本批不把阵营并入 `setIntProperty` 白名单。
- 未新增数据库表/字段、未改协议或前端资源，也未触碰共享数据库或现网服务。

## 四十二、新前端接入本机 C++ 环境试玩（2026-11-26）

### 目的
让用户提供的一套“本来就能玩”的前端包（`E:/雷霆传奇Linux版/雷霆传奇_Linux版前端_game.yyzzu.com/game.yyzzu.com`）通过 `http://demo.test/` 直连本机 C++ 服务端，先做一次原版前端 + 原版 C++ 的全链路试玩验证。该任务是**原版基线验收**，不是 Go 重写工作；验收标准是能出现登录框、点“开始游戏”后真实进入游戏场景。

### 手段（不改前端业务代码）
- `hosts` 已有 `127.0.0.1 demo.test`；EServer nginx 站点 `D:/tools/EServer-data/etc/nginx/conf/vhosts/demo.test_80.conf` 原本已把 `demo.test` 指向该前端目录，本次只在该 vhost 内新增 `sub_filter`，在 HTTP 响应层替换三处本地化值，不落盘修改 JS 文件：
  - `window.loginType = 1` → `0`（关闭渠道 SDK 模式，显示 egret 登录框）
  - 区服列表 URL → `http://demo.test/web/server_list/Serverlist.php/`
  - `window['userData'] = null;` → 默认 `uid/user_id/openId/sdkloginmodel/channelExt/game_appid/pid` 字段（缺字段会导致登录包被 gameworld 拒绝）
- 只改前端目录下两个 **PHP 配置文件**：
  - `web/server_list/config.php`：白名单加 `127.0.0.1`，区服 IP `182.61.136.28` → `127.0.0.1`，端口 `9001` → `9002`，登录 URL → `http://demo.test/web/s10001/Login.php/`
  - 根目录 `config.php`：`$url` → `http://demo.test/`
- 未改 `index.js`、`login.min.js`、`main.min.js`、HTML 或任何游戏资源；工作区 `www/` 的 git 状态为空。

### C++ 启动要点
- 启动顺序：`dbserver → gameworld → gateway → loggerserver`。
- 实测有效方式：
  - `dbserver.exe`：工作目录设为 `data/server/s1/Debug/dbserver`，**不做 stdout/stderr 重定向**；重定向启动时曾出现 gameworld 持续 `wait for db client` / `connect server time out`。
  - `gameworld.exe`：工作目录设为 `data/server/s1/Debug/gameworld`，可用输出重定向捕获日志，等待 `all game config load complete!`。
  - `gateway.exe` / `loggerserver.exe`：工作目录分别设为各自目录。
- 验收时端口：`80`（nginx）、`3306`（MySQL）、`5001/6001/7001/9002`（C++ 四件套）均监听；链路为 `gameworld→dbserver:5001`、`gateway→gameworld:6001`、`gameworld→loggerserver:7001`。
- 验收后 C++ 服务保持运行，便于用户继续试玩。

### 验证结果
- `http://demo.test/` 打开后显示登录框；输入账号 `haohaiyao`（角色“追风尘世”，82 级）点击“开始游戏”。
- 浏览器控制台输出 `进入游戏 {"actorid":65537,"actorname":"追风尘世","userlevel":82,...}`；截图确认已渲染游戏场景、怪物与 UI。
- `agent-browser network requests` 确认区服列表、`Login.php` 均请求 `demo.test`；系统层面确认客户端与 `9002` 建立 ESTABLISHED 连接。
- 验证截图与日志留在 `.tmp_cpp_logs/`（安全删除拒绝清理，可手动删除）。

### 回滚
- nginx 配置原始备份：`nginx_demo.test_80.conf.bak`（工作区根目录）。恢复时把该文件覆盖回 `D:/tools/EServer-data/etc/nginx/conf/vhosts/demo.test_80.conf`，再执行 `nginx.exe -p <EServer nginx 目录> -s reload`。
- 前端两个 PHP 配置按上文逆向改回原值即可；JS/HTML/资源从未改动，无需还原。

## 四十三、A5-21：DataPack 读取补齐与 changeMoney 别名（2026-11-26）

### 实现依据与边界
- 从 `engine-api-matrix.tsv` 前 120 名重新盘点未注册 API，选出三项有明确写侧/调用点证据的高频缺口：`LDataPack.readInt64`(12)、`LDataPack.readData`(26)、`LActor.changeMoney`(39)。
- `readInt64` 与 `writeInt64` 对称，按小端读取 8 字节并返回 64 位整数；`readData(pack,n,dt1,dt2,...)` 按 `_G.lua:407-419` 的 dt 常量与 `writeData` 对称读取变参，覆盖 byte/char/word/short/int/uint/int64/uint64/string。
- `changeMoney(actor, mtType, delta, 1, true, tips, ...)` 桥接到 `changeCurrency`：前三个参数同构，第 6 参日志 tip 作为 reason；`phylum/classField/family/genus` 等扩展参数不参与扣款。三参短调用同样支持。
- 越界、未知 dt 类型、非法字段数、离线/非法参数均失败关闭，不创建 Actor，不新增协议或数据库字段。

### 验证与运行边界
- 新增 `datapack_read_test.go`：覆盖 writeData→readData 往返（含 int64/string）、readInt64 大整数、越界/未知类型/非法字段数。
- 新增 `changemoney_test.go`：覆盖元宝扩展参数调用的三参桥接、金币三参短调用、余额与存档器调用、离线/非法参数/余额不足失败关闭。
- 未改前端、schema、共享数据库或现网服务；C++ 基线试玩服务保持运行，Go 隔离服务未启动。

## 四十四、A5-22：背包 checkItemNum/consumeItem 窄语义（2026-11-26）

### 实现依据与边界
- 原版 `actorcost.lua:6-65` 定义了 `checkItemNum/consumeItem`，本质是 Lua 包装：`useYuanBao=true` 时按 `storecommon.getYuanBaoPrice(itemId)` 用元宝补差价。商城价表与元宝替代价没有可安全复用的证据，且 `actorzhuanseng.lua:70/73/82/85` 会由客户端包打开该分支，因此本批**只做窄语义**。
- `checkItemNum(actor,itemId,count[,useYuanBao])`：仅当 `useYuanBao` 为 false/nil、itemId>0、count>0、Actor 在线时读 `Actor.Items` 返回是否够数；`useYuanBao=true`、非法参数、离线一律 false。
- `consumeItem(actor,itemId,count[,useYuanBao,log])`：同样条件下直接复用 `Actor.CostItem`，复用其先落库后改内存与背包协议通知；不查商城价、不扣元宝，第 5 参日志忽略。库存不足、非法参数、离线、`useYuanBao=true` 一律 false 且不改状态。
- 与 `costItem` 的分工：`costItem` 是底层原子扣减，`consumeItem` 是原版 Lua 包装名的窄兼容入口；两者都不处理元宝替代。
- `LActor.relive` 经复核有 35 处调用，但原版常同时做坐标、地图/副本、AI、Buff 与复活超时处理；`World.RevivePlayer` 仅复位 HP 并发 `EventEnter`，不足以安全实现完整语义，继续暂缓。

### 验证与运行边界
- 新增 `bagapi_test.go`：覆盖 `checkItemNum` 的足够/不足/精确/`false`/`nil` 标志、元宝标志失败关闭、非法参数与离线；覆盖 `consumeItem` 成功扣减、库存不足、元宝标志、非法参数、离线及失败路径不改背包。
- 新增 `truthy` 仅用于 Lua 布尔开关判定，不参与物品数量或货币计算。
- 未改前端、schema、共享数据库或现网服务；C++ 基线试玩服务保持运行，Go 隔离服务未启动。

## 四十五、A5-23：addItem/getStoreItemCount（2026-11-26）

### 实现依据与边界
- `LActor.addItem` 31 处调用，参数布局稳定为 `(actor, itemId, quality, strong, count, bind, logstr, logId)`，代表见 `txvipaward.lua:204/249/294/339/383/420/453`、`cdkey.lua:61`、`microclient.lua:91/120`。Go 只取 `args[1]=itemId` 与 `args[4]=count` 复用 `Actor.AddItem`；quality/strong/bind/日志参数无背包模型，明确忽略且不检查装备背包容量。
- `LActor.getStoreItemCount` 11 处调用，`_G.lua:210-215` 已确认 `ipBag=0`、`ipDepot=1`、`ipLottery=2`、`ipEntrust=3`。仅 `ipBag=0` 复用背包物品总数；depot/lottery/entrust/babel 等其他仓库类型一律返回 0，不伪造仓库。
- 两者均只操作在线 Actor 的 `Items` map；离线/非法句柄、非整数、`itemId<=0`、`count<=0` 一律失败关闭，不改状态。
- `LActor.AddSkill` 虽有 7 处调用，但实参既出现 `THCfg.skill_id` 也出现 `k*1000+v`，与现有 `Skills[skillBaseID(job,slot)]` 的映射关系未证实，继续暂缓；`LDataPack.writePacket` 缺长度前缀 golden，也暂缓。

### 验证与运行边界
- `bagapi_test.go` 扩展覆盖 `addItem` 参数位置（第 5 参为 count）、背包增量、非法参数失败关闭；覆盖 `getStoreItemCount(ipBag)` 总数与 1/2/3/5/99 仓库类型失败关闭。
- 未改前端、schema、共享数据库或现网服务；C++ 基线试玩服务保持运行，Go 隔离服务未启动。

## 四十六、B1 起点：Go 隔离全栈登录进场景（2026-11-26）

### 目的与边界
在不触碰 C++ 基线（`80/5001/6001/7001/9002`）与共享库 `actor_s1` 的前提下，用隔离库、隔离端口和隔离客户端入口验证 Go 服务端能完成真实客户端的登录鉴权、角色加载与进入游戏场景。

### 隔离环境
- 隔离库：`actor_s1_m5e_b1_20261126`，从 `actor_s1` 克隆 `haohaiyao`/actor `65537` 的 `globaluser/actors/roles/actorvariable/actorbinarydata`；创建命令：
  `M5E_DSN='root:123456@tcp(127.0.0.1:3306)/actor_s1?charset=utf8' go run setup_m5e_db.go -mode setup -database actor_s1_m5e_b1_20261126 -account haohaiyao -actor 65537`
  （脚本位于 `ltcq-go/reference/captures/browser_retaliation_20261125/`）
- Go 二进制：`ltcq-go/.tmp_b1/gameworld.exe`、`gateway.exe`，由 `go build` 生成。
- nginx 0080 入口：`D:/tools/EServer-data/etc/nginx/conf/vhosts/demo.test_8080.conf`，`listen 8080`、`server_name demo.test`，根目录与 80 站点相同；不改前端 PHP 配置，在响应层替换：
  - `window.loginType = 1` → `0`
  - 区服列表 URL → `http://demo.test:8080/web/server_list/Serverlist.php/`
  - `window['userData'] = null;` → 默认登录字段
  - `"port":"9002"` → `"port":"19002"`

### 启动步骤
在 `ltcq-go` 目录执行：
```bash
go build -o .tmp_b1/gameworld.exe ./cmd/gameworld
go build -o .tmp_b1/gateway.exe ./cmd/gateway
.tmp_b1/gameworld.exe -listen 127.0.0.1:16001 \
  -data ../data/server/s1/Debug/gameworld/data -ostype WIN \
  -dsn 'root:123456@tcp(127.0.0.1:3306)/actor_s1_m5e_b1_20261126?charset=utf8' \
  -monster-spawns config/monster_spawns.example.json
.tmp_b1/gateway.exe -listen 127.0.0.1:19002 -backend 127.0.0.1:16001 -gatename gateway_s1
```
验收时 `16001/19002` 均监听，`gateway` 日志出现 `gameworld 已连接` 与 `gate 注册已发送`。

### 验证结果
- 浏览器打开 `http://demo.test:8080/`，登录框出现，账号 `haohaiyao`，区服 `双线1区`（响应中端口已替换为 `19002`）。
- 点击“开始游戏”后客户端资源加载/解压成功，进入游戏场景；截图见 `ltcq-go/.tmp_b1/go_after_start.png`。
- gameworld 日志出现 `账号鉴权通过 sid=552 account=haohaiyao accountID=121103`，随后大量 `sysid/cmdid` 上行帧，说明角色加载与场景包交互正常。
- C++ 基线 `80/5001/6001/7001/9002` 与 MySQL `3306` 全程在线；未改前端文件、schema 或共享库。

### 保留与回滚
- 隔离环境当前**保持运行**，便于继续 B1 剩余链路（打怪/背包/存盘重登）。
- 停止顺序：浏览器 → Go gateway（PID 见运行日志）→ Go gameworld；不要停止 C++ gateway/gameworld。
- 若需撤掉 8080 入口：`nginx.exe -p D:/tools/EServer-data/childApp/server/nginx -s reload` 前删除 `demo.test_8080.conf`；80 站点 `demo.test` 不受影响。
- 若需删除隔离库：在 `ltcq-go/reference/captures/browser_retaliation_20261125/` 执行 `setup_m5e_db.go -mode drop -database actor_s1_m5e_b1_20261126`（`M5E_DSN` 同上）。

## 四十七、B1 剩余链路：打怪/背包/存盘重登（2026-11-26）

### 打怪与奖励
- 在同一隔离环境重新进入 `http://demo.test:8080/`，角色“追风尘世”51 级进入场景；场景配置有 1 只怪物（`gameworld.err.log:23:01:55 怪物生成完成 count=1`）。
- 点击怪物后怪物从画面消失，画面顶部金币数字增长；以隔离库为准，`actors` 行由 `exp=1000/gold=10000` 变为 `exp=1300/gold=13000`，证明击杀奖励已通过 Go 服务端写入 DB。
- `gameworld.err.log` 记录鉴权通过、角色加载帧以及持续上行帧，未见错误；战斗细节未逐帧记录，奖励落库以 DB 数值变化为准。

### 背包一致性
- 点击底部“背包”图标未展开面板，未产生新的上行帧；因此**不把背包 UI 交互记为通过**。
- 改用存档层验证：`actorvariable.variable` 背包 blob 长度 `17329`，重登前后尾字节均为 `1c6de73351fea9000000000000f03f01438d27a5fa41548a0000000000000000`，说明背包数据未被战斗或重登破坏。
- 背包协议初始化路径仍由 `postlogin.go:257-261` 的 `LoadBagBlob/decodeBagBlob` 承接；本批未新增或修改背包代码。

### 存盘重登
- 击杀后重新打开 `http://demo.test:8080/` 并再次点击“开始游戏”，角色仍为 51 级，怪物按配置重新刷新。
- 重登后隔离库数值：`level=51 exp=1300 gold=13000 yuanbao=5140348`；与重登前击杀后的 DB 数值一致，经验/金币未回退。
- 画面顶部金币数字在首次进入时曾与 DB 字段不同步，重登后画面与 DB 一致；验收以 DB 为准。

### 边界与后续
- C++ 基线 `80/5001/6001/7001/9002` 全程在线；未改前端文件、schema、共享库或 C++ 服务。
- 隔离环境继续保持运行；B1 标记为已验收，下一阶段为 **B2 稳定性**（72h soak、同号重连、登录失败回滚）。

## 四十八、B2 短周期稳定性：同号重连/失败回滚/soak（2026-11-26）

### 探针修复
- `cmd/wsprobe/main.go` 原把登录帧账号长度写死为 `0x0006`，只适用于默认账号“帅哥”（6 字节 UTF-8）；用 `haohaiyao` 或 `notexist_b2` 时 gameworld 报 `登录帧账号: UTF-8 字符串格式非法: len=6`，探针测试无效。
- 已改为写入账号实际 UTF-8 字节数（小端 u16），`gofmt`/`go vet ./...`/`go test ./... -count=1` 全部通过。

### 登录失败回滚
- `wsprobe -addr 127.0.0.1:19002 -account notexist_b2`：登录回包 `ff 01 02`（失败），gameworld 未创建 Actor。
- 隔离库 `globaluser=1 actors=1 roles=2 actorvariable=1 actorbinarydata=1`，角色 `level=51 exp=1400 gold=14000` 与背包 blob 长度/尾字节均不变。
- 修复前用旧探针得到的 `ff 01 01` 是网关层回显，不能作为登录成功判据；以后以 gameworld 的 `账号鉴权通过` 日志与 DB 状态为准。

### 同号并发登录
- 两个 `wsprobe -account haohaiyao` 相隔 800ms 并发登录，均回 `ff 01 00`，gameworld 日志出现两次 `账号鉴权通过 sid=561/562 account=haohaiyao accountID=121103`。
- DB 仍只有 1 个 actor，角色数值与背包 blob 不变，无重复建角或数据污染。

### 短周期 soak
- 连续执行 10 轮 `wsprobe -addr 127.0.0.1:19002 -account haohaiyao`，每轮观察 5s，约 50s。
- gameworld 工作集 `31,100,928 → 31,088,640` B，gateway `13,594,624 → 14,528,512` B；无持续增长趋势。
- gameworld 日志新增 21 行，均为常规帧记录；`WARN/ERROR/失败/panic` 过滤无命中。
- DB 各表行数、角色等级/经验/金币/元宝、背包 blob 长度与尾字节在 soak 前后完全一致。

### 边界与后续
- 72h soak 尚未执行，本批只完成短周期代理验证；长时间内存趋势、连接泄漏与跨天 DB 一致性仍需单独安排。
- 隔离环境 `16001/19002`、nginx `8080` 入口、隔离库继续保持运行；C++ 基线 `80/5001/6001/7001/9002` 未受影响，未改前端文件或共享库。

## 四十九、B2 72h soak 启动与 M6 批量探针准备（2026-11-26）

### 72h 后台 soak
- 脚本：`ltcq-go/.tmp_b1/soak_72h.ps1`；日志：`ltcq-go/.tmp_b1/soak_72h.log`。
- 启动命令：后台 `pwsh -NoProfile -ExecutionPolicy Bypass -File soak_72h.ps1`，PID `19184`，开始 `2026-11-26 23:15:59`，计划结束 `2026-11-29 23:15:59`。
- 每分钟记录一次：gameworld/gateway 工作集、`dbcheck.php` 角色与背包状态、`wsprobe -account haohaiyao` 的登录回包与观察结束。
- 首轮日志确认 `SOAK_START`，登录回包 `ff 01 00`；后续在 23:17:04 记录 `level=52 exp=925 gold=28000`，说明 soak 登录会触发服务端自动战斗奖励，DB 数值随周期增长属于预期，不能当作“DB 不变”的证据。

### wsprobe 批量并发能力
- `cmd/wsprobe/main.go` 新增 `-count/-concurrency/-observe/-quiet`：
  - 单探针保持原逐步输出；
  - 批量模式用 worker pool 并发调用 `probe()`，`-quiet` 只输出 summary；
  - 账号长度仍按实际 UTF-8 字节数写小端 u16。
- 回归：`gofmt -l cmd/wsprobe/main.go` 无输出，`go vet ./...`、`go test ./... -count=1` 全通过，Lint 无新增问题。

### 批量 burst 结果
- `100` 次 / `20` 并发 / `observe=500ms`：`ok=100 fail=0 elapsed=2.511s`。
- `500` 次 / `50` 并发 / `observe=300ms`：`ok=500 fail=0 elapsed=3.027s`；gameworld 日志新增 1000 行常规帧记录，`WARN/ERROR/失败/panic` 过滤无命中。
- 内存：gameworld `31.25MB → 31.30MB`，gateway `17.57MB → 26.08MB`；30 秒后 gameworld `31.37MB`、gateway `26.08MB`，gateway 工作集未回落，记为后续内存泄漏观察项（可能是 Go runtime 保留堆，不直接等同于泄漏）。
- DB：各表行数稳定为 `globaluser=1 actors=1 roles=2 actorvariable=1 actorbinarydata=1`，背包 blob 长度与尾字节不变；角色经验/金币因 72h soak 周期登录持续增加，不作为 burst 数据污染。

### 边界与后续
- 5000 机器人全量压测尚未执行；当前只完成 100/500 路探针 burst，作为 M6 前置能力验证。
- 72h soak 结束后需判读 `soak_72h.log` 的内存趋势、登录失败率和 DB 一致性；若 gateway 工作集持续不回落，需追加连接泄漏专项检查。
- 隔离环境与 C++ 基线继续并存，未改前端、schema 或共享库。

## 五十、M6 5000 路机器人 burst 与下一步规划（2026-11-26）

### 5000 路 burst
- 命令：`wsprobe -addr 127.0.0.1:19002 -account haohaiyao -count 5000 -concurrency 100 -observe 100ms -quiet`。
- 结果：`total=5000 ok=5000 fail=0 concurrency=100 elapsed=5.064s`；gameworld 日志新增约 1 万行常规帧记录，`WARN/ERROR/失败/panic` 过滤无命中。
- 内存：gameworld 工作集 `31,510,528 → 35,373,056` B，gateway `17,166,336 → 34,738,176` B；冷却 60s 后 gameworld `35,454,976` B、gateway `34,738,176` B，均未回落。
- 连接：19002 上 `ESTABLISHED=2`、`TIME_WAIT=5002`，符合 5000 次短连接 burst 后的内核状态；服务仍监听，后续 soak 登录回包正常。
- DB：各表行数仍为 `globaluser=1 actors=1 roles=2 actorvariable=1 actorbinarydata=1`，背包 blob 长度与尾字节不变；角色经验/金币随 72h soak 周期正常增长，不把 burst 记为数据污染。

### 下一步规划
1. **等 72h soak 结束**（计划 2026-11-29 23:15:59）后判读 `soak_72h.log` 的内存趋势、登录失败率、DB 一致性；gateway 工作集不回落列为专项观察点。
2. **A2 掉落/拾取 golden**：需用户确认在 C++ 试玩服做一次“打怪掉落→拾取”，用 `tcpcap` 抓 C++ 下行 golden；拿到包序前继续暂缓 Go 侧实现。
3. **A5 暂缓项**：`relive/AddSkill/writePacket/giveAwards/setExp/reCalcAttr/getEntityType` 等只在枚举、容量模型、协议 golden 或状态副作用证据补全后恢复，不靠猜测推进。
4. **M6 数值对齐**：5000 路 burst 已通过，后续重点是伤害/经验/掉落等数值与 C++ golden 对齐，而不是继续堆并发。

### 边界
- 72h soak PID 19184 仍在运行；C++ 基线 `80/5001/6001/7001/9002` 未受影响，未改前端、schema 或共享库。

## 五十一、暂缓项证据复核与 A2 golden 捕获准备（2026-11-26）

### A2 掉落/拾取：仍无 golden
- `ltcq-go/reference/captures/` 下无 drop/pick 相关 raw/json；`cpp_golden_20261124/*.raw` 仅握手/登录；`bag_cpp_golden.json` 只验证背包初始化；`bag_mutate_browser_proof.json` 的 `firstDropPacket` 指背包 Add，不是地面掉落。
- 已验证击杀包序是 `[00][10] → [00][05] → [00][07]`（实体消失、金币、经验），没有地面掉落物出现/拾取包。
- 原版 `sBaseCmd_GetItem=6`、`enDropBag=6`、`aeDropItem=90` 等常量存在，但未绑定到掉落/拾取协议；不可拿背包 `[03][04]/[03][05]` 或实体 `[00][04]/[00][10]` 代推。
- 仍缺：击杀掉落下行 raw、拾取上行 raw、拾取后掉落物消失/背包 Add/提示的下行包序与状态快照。

### AddSkill：继续暂缓
- `THCfg.skill_id` 来自 `togetherhit.config:6`，样例 `{71001,72001,73001}`；`godweaponbase.lua:312` 传入 `k*1000+v`，样例 `16001..16010/26001..27001/27001..27010`。二者都属于 `SkillsConfig` 的 configID 空间。
- Go `Skills` 键是 `skillBaseID(job,slot)=job*10000+slot*1000+1`，值为等级；`getRoleSkillLevel` 按 `index=0..4` 读 5 个槽。把 `Skills[71001]=1` 或 `Skills[16001]=1` 会形成孤儿状态，不会被升级、施放或登录下发读取。
- 调用对象还混有 actor、role、monster（`flamestamp.lua:352`），无法统一映射到当前 `Actor.Skills`。结论：不实现，不写 `Skills`，不发包，不落库。

### LDataPack.writePacket：继续暂缓
- Ghidra `functions.tsv:5602` 只确认 `LDataPack::writePacket` arity=3；Lua 调用有省略第三参和显式 `false`，全树未见 `true`。
- 没有 byte-level golden 能区分“裸写子包”与“u16 小端长度前缀 + 子包”；跨服 handler 立即 `readInt/readByte` 的流控只是间接证据。
- 缺失最小 golden：一次已知 `src` 字节、`dst` before/after、第三参取值/缺省的完整 flush 或网络帧。结论：不注册、不猜测实现，避免部分字节写入。

### M6 数值对齐缺口
- Go `monster_spawns.example.json` 的 `exp=100/gold=1000` 是开发示例值；RUNBOOK 十已记录它是示例配置，不是 C++ 对照。
- C++ `drop.lua:15-59` 消费 droptable，`actorawards.lua:9/14` 把 `type=0` 的 `id` 传给 `LActor.changeCurrency`；`_G.lua:171-176` 的 `mtBindCoin=0/mtCoin=1/mtBindYuanbao=2/mtYuanbao=3` 表明 `type=0,id=0/1/2` 是绑定币/金币/绑定元宝，经验走独立链路。
- `DropTableConfig[130002]` 虽有金币 51686/绑定币 400/绑定元宝 60 等数值，但 `worldboss.config:453-470` 的 `joinReward=130002` 实际解析到 `DropGroupConfig[130002]` 的物品表，不能据此给 monster 31001 定经验/金币。
- 结论：真实击杀奖励数值仍需 C++ golden，不能靠现有文本配置直接对齐。

### A2 golden 捕获入口已就绪
- 新增 nginx vhost：`D:/tools/EServer-data/etc/nginx/conf/vhosts/demo.test_8081.conf`，`listen 8081`、`server_name demo.test`，前端根目录与 80/8080 相同。
- 响应层替换：区服列表 URL → `http://demo.test:8081/web/server_list/Serverlist.php/`、端口 `9002` → `19003`、`loginType=0` 与默认 `userData`。
- 已 `nginx -t` 与 reload 验证：`http://demo.test:8081/` 返回 200，区服 JSON 端口为 `19003`。
- 待用户确认后执行：在 `ltcq-go` 目录运行 `go run ./cmd/tcpcap -listen 127.0.0.1:19003 -to 127.0.0.1:9002 -out reference/captures/a2_drop_pickup_20261126.raw`，打开 `http://demo.test:8081/`，在 C++ 试玩服做“击杀怪物→掉落物出现→拾取”，停止 tcpcap 后解析 raw。未确认前不启动抓包、不改 C++ 服务。

## 五十二、A2 首次 C++ golden 捕获：部分成功（2026-11-26）

### 操作
- 用户以“继续”确认后，启动 `tcpcap.exe -listen 127.0.0.1:19003 -to 127.0.0.1:9002 -out reference/captures/a2_drop_pickup_20261126.raw`（PID 18252）。
- 浏览器打开 `http://demo.test:8081/`，关闭“离线收益”弹窗后进入 C++ 场景，自动战斗产生多次击杀与候选掉落。
- 抓包结束后关闭浏览器并 `Stop-Process` 停止 tcpcap；复查 19003 已关闭，C++ `9002/6001/5001/7001` 仍监听，72h soak PID 19184 仍运行。

### 初步解析
- raw 文件：`ltcq-go/reference/captures/a2_drop_pickup_20261126.raw`，128,554B。
- 临时解析器 `.tmp_b1/parse_raw.go` 输出 `.tmp_b1/a2_frames.txt`。
- 发现 3 帧候选掉落实体下行：
  - `WS 11 S ... 4e21ae404e01020000000300000048030000712900000100000092ba000001000000`
  - `WS 42 S ... 4e21ae404e0102000000030000000c030000712900000100000092ba000001000000`
  - `WS 109 S ... 4e21ae404e010200000003000000d0020000712900000100000092ba000001000000`
- 三帧差异集中在第 5 个 4B 字段（`48030000=840`、`0c030000=780`、`d0020000=720`），疑似掉落物实体/位置或序号；其余 `71290000=10545`、`92ba0000=47762`、`01000000` 在三次中一致，尚未确认语义。
- 客户端帧出现 3 次 `4e21ae40 05 000000 0a 0a 01 000000`，但无法确定是拾取请求还是其他系统包；点击画面候选位置后未出现明确的拾取后下行包序。

### 阻塞与下一步
- `haohaiyao` 在 C++ 库 `actor_s1.globaluser` 中 `gmlevel=0`，不能用 `@dropbags 1` 在脚下稳定生成掉落物。
- 未取得完整拾取 golden：缺“拾取上行请求 → 掉落物消失 → 背包 Add/Update → 提示”的连续包序，不能实现 A2 或对齐真实奖励数值。
- 需用户二选一：
  1. 授权临时把 `haohaiyao` 的 `gmlevel` 设为 1，抓完 `@dropbags` 拾取后立即恢复为 0；
  2. 不做 GM 修改，由用户指认画面上的掉落物位置，再用 8081/tcpcap 抓一次明确拾取。
- raw 与解析输出保留在 `.tmp_b1/` 与 `reference/captures/`，按既有约定抓包产物不纳入 git 提交。

## 五十三、A2 临时 GM 抓包与包序复核：仍缺拾取 golden（2026-11-27）

### GM 权限处置
- 用户选择方案：临时把 `actor_s1.globaluser` 中 `haohaiyao` 的 `gmlevel` 设为 1，用 `@dropbags` 造掉落，抓完拾取后立即恢复为 0。
- 复核发现：续接会话开始时 `gmlevel` **仍为 1**（上轮抓包后未恢复），已用 `.tmp_b1/a2_gm_toggle.php restore` 立即恢复，并二次 `status` 核对：`userid=121103 account=haohaiyao gmlevel=0 closed=0`。该脚本带 `enable` 要求 `gmlevel=0/closed=0`、`restore` 要求 `gmlevel=1` 的前置守卫，避免误改。
- 结论：GM 权限已回到原值 0，未遗留提权。

### 抓包产物与解析
- 第二轮 GM 抓包已完成（日志 `a2_gm_tcpcap.err.log`：`23:48:41` 起 `19003→9002`，`00:02:20` 链路结束），raw：`reference/captures/a2_drop_pickup_gm_20261126.raw`。
- 解析输出 `.tmp_b1/a2_gm_live_frames.txt`（约 1155 行 WS 帧）。抓包产物按既有约定不纳入 git。

### 关键帧统计（下行 sysid/cmdid 取 payload byte4-5，上行取 byte8-9）
- 掉落实体下行 `4e01`（`CMD_Base`/候选 drop entity）共 **8 帧**，负载固定形态 `4e01 02000000 03000000 <entity> 71290000 01000000 92ba0000 01000000`，entity 依次：`d0020000=720`、`94020000=660`、`58020000=600`、`1c020000=540`、`e0010000=480`、`a4010000=420`、`68010000=360`、`2c010000=300`——每次 -60，符合 `@dropbags` 连续造掉落，与上一轮 `480/780/840` 同族。
- 拾取/奖励相关下行：`0006` = `sBaseCmd_GetItem` **0 帧**；`000a` = `sBaseCmd_EntityDisapear` **0 帧**。即**没有**“物品进包提示 + 掉落物消失”的下行证据。
- 背包下行 `0304` = `sBagCmd_AddItem` 共 9 帧，但均成组出现在登录/换线初始化（`0005 0100` 金额同步之后，如 WS58/145/221/281 带 `a38acb1f/fd8acb1f/328bcb1f/6b8bcb1f` 时间戳与 `8a8a0100/728e0100/...` 多个物品），是 **InitData 批量**，不是拾取单件增量。
- 上行 cmd 分布：`0101`×105、`00ff`×11（心跳）、`050a`×8、`0a0a`×7、`4c01`×2，其余各 1。`050a`（`85541164 <seq> 050a02000000`）内容固定、无拾取目标字段，前后无 GetItem/Disapear/AddItem 配对；`0a0a` 为登录期系统包族。协议表 `protocol.lua` 中 base 上行只有 `cBaseCmd_GmCmd=0/CreateRole=2/ReqTime=14/GetActorInfo=16/ClientConfig=19/ChangeName=22/SwitchTarget=25/.../HeartBeat=255`，无“地面拾取”专用 cmd；`cBagCmd_DepotPickUp=4` 是仓库取物，非地面掉落拾取。

### 结论与边界
- 本轮 GM 抓包**成功造出掉落**（8 帧 `4e01`），但**仍未捕获完整拾取 golden**：缺“拾取上行请求 → `000a` 掉落物消失 → `0003/0304` 背包增量 → `0006` 获得物品提示”的连续包序。
- 因此 A2 继续暂缓，不猜协议；M6 真实击杀/奖励数值对齐同样仍需 golden。
- 项目无 C++ 源码，协议权威仅来自 `protocol.lua` 常量 + golden 抓包；`85541164` 前缀在源码零匹配（属抓包帧头，非业务 cmd）。
- 下一步：如需继续 A2，应先解决“浏览器点击如何真正命中脚下掉落物并产生拾取上行”，再抓一次明确拾取会话；在此之前不改 Go 侧拾取实现。

## 五十四、72h soak 提前终止与环境状态核查（2026-11-27）

### soak 终止
- 续接会话核查：soak PID 19184 **已不在进程表**，`soak_72h.log` 共 447 行，最后有效记录到 `2026-11-27 00:02:39`，未到计划的 `2026-11-29 23:15:59`。
- 终止原因：日志 `00:02:42` 起 `dbcheck.php` 报 `PDOException SQLSTATE[HY000] [2002] 由于目标计算机积极拒绝，无法连接`（MySQL 127.0.0.1:3306 短暂拒绝）；soak 脚本的 DB 检查步骤在连接失败后中断，进程随后退出。这不是被主动判定为失败，而是环境重启导致的意外终止。
- 有效区间判读（2026-11-26 23:15:59 → 2026-11-27 00:02:39，44 个 MEM 样本，约 47 分钟）：
  - gameworld 工作集 29.78–33.81MB、gateway 13.81–33.13MB，与 M6 burst 后记录同量级，未见持续单调增长；样本数不足以支撑 72h 结论。
  - 末次 DB：actorid=65537、level=56、exp=225、gold=121000、yuanbao=5140348、bag_blob_len=17329、bag_tail 尾字节稳定；WS 登录回包 `ff 01 00/01` 交替正常。
  - 结论：约 47 分钟短周期数据稳定，但 72h 长周期目标**未达成**，需环境稳定后重新发起。

### 当前环境状态（核查时点）
- MySQL 3306 已恢复监听（PID 8720，进程 StartTime `2026-11-27 22:14:09`，说明 11-27 晚间发生过重启，soak 的 `00:02` 连接拒绝属该次重启前后的窗口）。
- C++ 基线端口 `9002/6001/5001/7001` 与 Go 隔离端口 `16001/19002`、捕获口 `19003` **当前均不监听**——环境重启后服务栈尚未拉起。
- 因此本轮未启动任何新的服务端进程、未改共享库；A2 golden 补抓与 72h soak 重跑都需先把服务栈恢复后再执行。
- 文档与代码改动仅 `docs/NEXT-STEPS.md`、`docs/RUNBOOK.md`，抓包产物仍不纳入 git。

## 五十五、服务栈恢复 + A2 拾取 golden 突破 + 72h soak 重跑（2026-11-27）

### 服务栈恢复（用户确认"恢复完整服务栈"后执行）
- C++ 四件套按 `dbserver → gameworld → gateway → loggerserver` 顺序拉起：
  - dbserver PID 9868（5001）；gameworld PID 596（6001，管理口 10001）；gateway PID 6332（9002）；loggerserver PID 6760（7001）。
  - 关键排障：首次 gameworld 早于 loggerserver 启动，出现持续 `connect fail! logger server 10061`，且浏览器登录卡在"登录连接中"、tcpcap 只收到 630B 握手就 EOF。按正确顺序重启 loggerserver → gameworld 后 `gameworld→logger(7001) Established=1`，gateway 重连 6001，`wsprobe 9002` 登录出现 `ff 01 00` 业务响应，浏览器随即成功进场景。**结论：C++ 启动顺序里 loggerserver 必须先于 gameworld。**
- nginx 随环境重启已在运行（PID 17292），80/8081 均监听，8081 区服列表端口 `19003` 正常，vhost 未动。
- Go 隔离栈：重新 `go build` 后启动 gameworld PID 17076（16001，隔离库 `actor_s1_m5e_b1_20261126`）、gateway PID 7720（19002，注册 gateway_s1）；`wsprobe 19002/haohaiyao` 登录 `ok=1`。
- 恢复后端口全 UP：80/8081/5001/6001/7001/9002/16001/19002。

### A2 拾取 golden 成功捕获（关键突破）
- 根因纠正：前端 `main.min.js` 显示地面掉落物 `CharItem2.touchEnabled=false`，**不可点击**；拾取是 `DropHelp.findDrop()` 每 100ms 自动寻路靠近后本地飞入动画 + `UserFb.sendGetReward()`。之前两轮"点击画面候选位置"永远不可能触发拾取，并非点击坐标问题。
- 用独立 Edge（`--remote-debugging-port=9223`）+ CDP 脚本 `.tmp_b1/a2_pickup_golden.mjs`：patch `GameSocket` 抓 WebSocket 上下行，登录后用 `window.gm('@dropbag 10101 x y')` 造物品掉落（dropgroup 10101 → 100% 掉物品 2000001 聚灵奇石），角色自动寻路拾取。
- 捕获产物：`reference/captures/a2_pickup_golden_20261127.raw`（936,603B，tcpcap 19003→9002 全量上下行）+ `a2_pickup_golden_20261127.json`（CDP hook 解析，43 上行/265 下行帧）。
- 账号安全：`haohaiyao` GM 临时 enable（gmlevel 0→1）后已 restore 并二次核对 `gmlevel=0 closed=0`；测试 Edge 与 tcpcap 均已停止，CDP 9223/19003 已关闭。
- **确认的 golden 包序**（payload 上行 sysid/cmdid 取 byte8-9，下行取 byte4-5；前缀为每帧 skey）：
  - GM 造掉落上行 `[00][00]`：`… 0000 1a00 4064726f7062616720313031303120313035343520343737363200` = `@dropbag 10101 10545 47762`（skey+seq 后 `@dropbag ...` 字符串）。
  - 拾取上行 `[01][01]`（`CMD_Fuben/Guanqia=1`, cmd=1）：`… 0101 00`，payload 1B flag=0（普通地图 `fubenID=0`），多次发出，由 `DropHelp` 靠近掉落物后自动触发。
  - 拾取下行：`[00][07]` `sBaseCmd_UpdateExp`（经验，如 `… 0007 53000000 8b4f0300 20000000`）、`[00][05]` `sBaseCmd_UpdateMoney`（金币）、`[00][0a]` `sBaseCmd_EntityDisapear`（掉落物消失，3 帧 `… 000a ff/fe/fd 000000 …`，handle 低字节递减）。
  - 本次 `@dropbag 10101` 实际发包为 EXP+MONEY+Disapear，未见 `[00][06]` GetItem / `[03][04]` BagAdd；即本条路径奖励走货币/经验，不进背包物品。要验证"物品进包"链路需再抓一次确掉装备的 dropgroup，但**拾取上行与掉落物消失下行已拿到**。
- 实现边界：A2 不再"完全没有 golden"——拾取触发上行 `[01][01]`、下行 Disapear/EXP/MONEY 已有实测；但 `[01][01]` 同时是 `UserFb.sendGetReward`（通用领奖），语义并非"按 handle 拾取某个掉落物"，Go 侧不可据此凭空造"指定掉落物拾取"协议。A2 仍按"不猜协议"原则，只把这些 golden 作为后续对齐依据。

### 72h soak 重跑
- 改造 `.tmp_b1/soak_72h.ps1`：按端口动态查 PID（不再硬编码 17832/19400），DB/WS 检查加 try/catch 防止环境重启时再次中断；旧日志备份为 `soak_72h.log.prev_20261127`。
- 新 soak PID 5652，`SOAK_START end=2026-11-30 22:51:50`；首轮采样 MEM gameworld(pid=17076)/gateway(pid=7720)、DB actorid/level/exp/gold/bag_tail、WS `ff 01 00` 回包均正常。计划满 72h 后判读。

## 五十六、A2 物品进包 golden 与 Go BagAdd 布局差异（2026-11-27）

### 抓取
- 沿用五十五的服务栈/CDP 方案，临时 GM（enable→restore 已核对 `gmlevel=0 closed=0`），tcpcap 19003→9002，CDP 脚本 `.tmp_b1/a2_bag_golden.mjs`。
- 进场景后从 EntityManager 的 `CharRole` 取到真实坐标 `(1729.77, 510.23)`（此前用旧抓包兜底坐标 10545/47762 导致掉落不在脚下）。
- 先后发 `@additem 101001 1`（装备"雷霆战甲"类，101001）与 `@dropbag 1310001 1729 510`（100% 掉装备 dropgroup）。
- 产物：`reference/captures/a2_bag_golden_20261127.raw`（79,595B）、`a2_bag_golden_20261127.json`（183,635B，CDP hook 43 上行/数百下行帧解析）。抓包产物按约定不入库。测试 Edge/tcpcap 已停止，9223/19003 关闭。

### 决定性 golden：`@additem` → `[03][04]` BagAdd
- `@additem` 上行 `[00][00]`：`7b448037 12000000 0000 1100 406164646974656d203130313030312031 00` = `@additem 101001 1`。
- 77ms 后下行 `[03][04]`（去掉 4B skey 后 payload 148B）：`03 04 01 dc cf cc 1f 00 00 01 00 89 8a 01 00 01 00 00 00 04 ...`，其中 `898a0100`=itemID 101001、`01000000`=count 1，确认**物品进包走 `CMD_Bag(3)/sBagCmd_AddItem(4)`**。
- `[00][06]` `sBaseCmd_GetItem` 全程 0 帧；全仓 Lua 也无 `sBaseCmd_GetItem` 发包点（仅 `protocol.lua:108` 常量），故该常量不是 `giveItem` 的必发下行，不能据此做实现判据。
- `@dropbag 1310001` 后本次仍只见自动战斗击杀串扰的 EXP/MONEY，装备掉落拾取未稳定复现（drops 检测为 0），但"物品进包"下行包已由 `@additem` 对照拿到。

### C++ golden 与 Go `buildBagAddItem` 的布局差异（待校准，不改代码）
- C++ payload 148B 布局：`[0..1]=03 04`、`[2]=01`(bagTab)、`[3..10]=handle(8B, 浮点样式 dccfcc1f00000100)`、`[11..14]=itemID`、`[15..18]=count`、`[19/23/27/31]` 四个非 0 u32（如 04/01/02/01）、`[147]=01`。
- Go `bag.go:240-244 buildBagAddItem` = `{3,4,bagTypeOther=0}` + `appendBagItem`（`bagItemHandle=itemID<<32|actorID` 再按 u64 写、后接 itemID/count 与 16 组 8B 零）+ 尾 `0`：长度同为 148B，但 `[2]=0`（C++ 为 1）、handle 编码与末尾u32组/尾字节不同。
- 注意：M5-H 背包此前已用真实浏览器断言通过（客户端能消费 Go 包），本次差异只说明 **C++ golden 的 BagAdd 字节布局与 Go 现有实现不一致**，不能据此断言 Go 包错误；要对齐须另立 golden 校准项，本轮遵守"不猜协议/不改核心实现"边界，未改 `bag.go`。
- A2 边界更新：拾取上行 `[01][01]`、掉落物消失 `[00][0a]`、物品进包 `[03][04]` 三类 golden 均已实测取得；但 `[01][01]` 是通用 `sendGetReward`（非指定 handle 拾取），且 BagAdd 布局有差异，A2 保持暂缓、仅作为对齐依据。

## 五十七、BagAdd 字段语义查证与"不可安全落地"决策（2026-11-27）

### 客户端 `[03][04]` 权威布局（data/h5game/main.min.js:44 UserBag.doAddItem）
payload 148B = `u8 sysid(3)` + `u8 subcmd(4)` + `u8 bagType` + `ItemData(144B)` + `u8 tipFlag`：
- `ItemData.parser`（main.min.js:44:15129）：`handle=f64 LE`(8B) + `configID=i32`(4B) + `count=i32`(4B) + `att[0..7]`(8 对 i32 type/value=64B) + `extAtt[0..7]`(8 对 i32=64B) = 144B。
- `[2]=bagType`：`UserBag.BAG_TYPE_OTHTER=0 / BAG_TYPE_EQUIP=1 / BAG_TYPE_TREASUREHUNT=2`（main.min.js:45）。golden 的 `[2]=1` 是**装备背包**（因 `@additem 101001` 是装备），Go M5-H 的 `[2]=0` 是普通物品背包——两者场景不同，非简单错误。
- handle 8B：客户端 `readDouble()` 当作**不透明句柄**，仅用于 `doDeleteItem`/`doUpDataItem` 按 handle 匹配，不反解 itemID/actorID。Go `bagItemHandle=itemID<<32|actorID` 经 `Float64bits/Float64frombits` 位重解释写出，客户端已验证可正常匹配；**handle 编码差异无需对齐**。
- `[19/23/27/31]` 非 quality/star/bind，而是 `ItemData.att[0]={type:4=atAttack,value:1}`、`att[1]={type:2=atMaxHp,value:1}`（`AttributeType` 见 main.min.js:2）；C++ golden 装备带属性、Go 全零。
- `[147]=tipFlag`：`doAddItem` 读 `n`，非 0 时弹"获得…/好装备"提示。Go 恒 0（静默），M5-H 已验证物品正常入包但不弹提示。

### 决策：本轮不改 Go 代码（证据足够但实现链缺环）
- 要实现"装备进装备背包 + 带属性 + 提示"，需同时具备：① Go 侧 ItemConfig/equip.config loader 与 `isEquip` 判断（`main.min.js`/`item.lua:17-20` 依赖 `cfg.type`，Go 侧 0 匹配）；② `LActor.addItem` 完整 quality/strong/bind/属性参数（Go 未保留）；③ 存档模型由单背包扩为分背包；④ InitData/Add/Delete/Update/UseItem 全链路 bagType 联动；⑤ 装备属性 att[] 生成。缺任一环都会变成猜实现。
- 因此维持"不猜协议/不改核心实现"边界，**未改 `bag.go`**；上述缺口写入下一步规划，待逐项补齐证据后再动。
- M5-H 验收（`RUNBOOK.md:534-549`、`bag_mutate_browser_proof.json`）用物品 200013（非装备），15 项断言全 true，证明 Go 普通物品链路正确；装备链路从未被 Go 覆盖，这正是 A2 的完整待办。
## 五十八、M6 数值对齐边界查证：C++ 试玩服无法取得可信击杀 golden（2026-11-27）

### 配置链路结论
- C++ 主场景怪物（Go 示例用的 31001）`MonstersConfig[31001]`（data/server/s1/Debug/gameworld/data/config/monster/monsters.config:3-13）只有 id/name/type/hp/atk/def/res/skill/ai，**没有 exp/gold/dropId 字段**；击杀经验/金币数值固化在 C++ 二进制（Ghidra 符号 `Monster::OnKilledByEntity` / `Actor::AddExp` / `Actor::ChangeGold`），文本配置无对照值。
- 唯一有文本的数值是挂机关卡（chapters.config:3-28 chapter 1）：`waveExp=13`/`expTime=2s`（每 2 秒 13 exp，聚合非单杀）、`monsterDropId=1320001`/`dropTime=2s` → dropgroup 1320001 → droptable 120001 → `{rate=33,type=0,id=1,count=157}`（每 2 秒 33% 概率 157 金币，期望≈51.81/2s）。这些是"一波 3 只 + 定时器"聚合值，不是"杀一只怪得多少"，不能用于单怪数值对齐。
- `type=0` 货币枚举（_G.lua:171-176）：`mtBindCoin=0/mtCoin=1/mtBindYuanbao=2/mtYuanbao=3`；经验不在枚举内，走独立 `LActor.addExp`（actorexp.lua:41-70 → `[00][07]`）。金币走 `giveAwards → changeCurrency → Actor::ChangeGold → [00][05]`（actorawards.lua:5-20）。
- Go 侧对齐目标：`config/monster_spawns.example.json` 的 `exp=100/gold=1000` → `combat.go:477 progression.GrantKill(attacker, rule.Exp, rule.Gold)` → `[00][07]` added / `[00][05]` 金额；这是开发占位，RUNBOOK:303/NEXT-STEPS:29 已明确。

### 为何当前 C++ 试玩服拿不到可信"纯击杀 golden"
- 服务端权威自动战斗（RUNBOOK:256-261）：玩家进范围后 gameworld 每 500ms 选最近怪攻击，不伪造客户端攻击包；前端 `RoleAI.useSkill()` 只是本地演出，没有"发一次攻击上行让服务端执行"的可靠接口。
- 前端 `GameLogic.ins().stopAI()`（发 cBase 子命令 33）可停服务端 AI、`RoleAI.ins().clear()` 停客户端 AI、`PlayFun.ins().closeAuto()` 关自动挑战，但 **chapter.lua 的 waveExp/dropTime 是服务端章节挂机定时器，前端无法停止**，EXP/MONEY 串扰不可避免（main.min.js 中无对应开关）。
- 因此即使 CDP 抓到 `[00][07]` added 与 `[00][05]` 金额，也无法区分是击杀奖励还是挂机节拍，不能作为对齐依据；强行抓取会污染证据、违反"不猜协议/不伪造数据"原则。

### 决策与边界
- 本轮未启动 CDP/tcpcap（19003/9223 均空闲），未改 Go 占位配置，未动 C++ 服务；GM 仍为 0。
- M6 数值对齐的**新前置条件**：需要"可控击杀 + 无挂机串扰"的环境（如 dev 模式单场景单怪、或服务端提供停 chapter 定时器的 GM/协议），或 C++ 二进制反编译给出 31001 的 exp/gold 常量；在此之前 `exp=100/gold=1000` 保留为占位，不得当作原版值。
- P4 内存观察：soak PID 5652（16 样本，约 15 分钟）gameworld 62.02→62.04MB 稳定、gateway 44.15→45.2MB（+1MB，继续观察），登录失败 0；未见泄漏证据，待 72h 满期判读。
## 五十九、反编译/A5/soak 三项复核（2026-11-27）

### 1. 反编译查 31001 exp/gold 常量：不可行
- `ltcq-go/reference/ghidra/` 只有 2 个文件：`functions.tsv`（2.13MB，4 列：地址/符号/命名空间/参数个数）与 `loggerserver_structs.txt`（3KB DWARF 结构体）；**无任何反编译伪代码/函数体/常量表/字符串表**（`*.c`/`*.java` 0 命中）。
- `Monster::OnKilledByEntity`(0062ffa4)、`Actor::AddExp`(00605a68)、`Actor::ChangeGold`(00605b68)、`MonsterConfigPool::Load/Get` 等只有符号与 arity，无实现；二进制中搜不到 C++ 符号串，无法把 PE 地址直接映射到 27MB ELF 二进制。
- 结论：现有反编译产物拿不到可信 31001 exp/gold；M6 仍需可控单怪击杀 golden 或真正的 Ghidra 伪代码导出。本轮未改 Go 占位值。

### 2. A5 giveAwards/canGiveAwards 复核：新 golden 补协议认知，但仍不可安全实现
- 新 golden（BagAdd 148B 布局、EXP/MONEY/Disapear）确认了"发什么包"，但 `giveAwards` 的暂缓理由中**协议**一项之外的缺口仍在：
  - `AwardType_Numeric=0/AwardType_Item=1` 未注入 Go VM（`enums.go:41-78`），Lua 包装层在 Go 侧静默失效；
  - `giveAwards(actor, rewards, log)` 的 rewards 是 Lua 数组表（`ipairs`），engineapi 无 Lua table→奖励列表解码（只处理标量）；
  - 背包写 `actorvariable.variable` 与货币写 `actors.gold/yuanbao` 是两条独立存档路径，批量发放无原子性/回滚；
  - `canGiveAwards` 需 `getEquipBagSpace`（剩余格数）与 ItemConfig/isEquip，Go 只有 `Items map[int]int` 单背包无容量概念，两者均缺；
  - 无邮件补发链路（`mailsystem.sendMailById` 仅 Ghidra 符号），guarded 调用方走 else 分支时奖励无处可去；
  - `docs/engine-api.md:123-125` 写 awardTable={id,count[,bind]} 与真实 Lua {type,id,count,job} 漂移，不能照文档实现。
- 结论：`canGiveAwards` 保持 stubBool 恒 false（失败关闭）；`giveAwards` 不实现。即使 best-effort 窄语义也有 6 项前置条件，不满足则不落地，不把 stub 计入覆盖门禁。

### 3. soak 完整趋势判读（修正此前只取前段的不完整结论）
- 新 soak（PID 5652，`SOAK_START end=2026-11-30 22:51:50`）日志为追加模式，统计须从 `SOAK_START end=2026-11-30` 行起算（第 447 行），此前误把旧段计入。
- 23 个样本、PID 稳定（gameworld 17076/gateway 7720，未重启）：gameworld 62.02→22.97MB（min 22.93/max 62.04），gateway 44.15→12.19MB（min 10.95/max 45.33），登录失败 0。
- 判读：峰值在启动初期（加载配置/首次登录），随后稳定回落到 23/12MB，是 Go GC/工作集回收的**健康模式**，未见单调增长或泄漏；M6 burst 后 gateway 工作集未回落的旧观察项，在本轮长周期里已回落，可降级为"继续常规观察"。待 72h 满期做最终判读。
## 六十、A5 基础能力：注入 AwardType/NumericType 确证常量（2026-11-27）

### 证据（闭合推导，非猜测）
- `_G.lua` 全文无 `AwardType_*` / `NumericType_Gold/YuanBao`，是 C++ `lua_pushinteger` 注册的全局标量，不在快照里。
- `AwardType_Numeric=0`：`drop.lua:9` 用 `item.type == AwardType_Numeric and item.id == NumericType_Gold` 判定金币；`droptable.config:296` 等大量配置金币条目写 `type=0`，故 AwardType_Numeric 只能是 0。
- `AwardType_Item=1`：`actorawards.lua:16-17` 的 Item 分支走 `giveItem`；`vipgift.config:7/13/20/27` 等大量配置物品条目写 `type=1`（id=200156 等物品ID），故只能是 1。
- `NumericType_Gold=1` / `NumericType_YuanBao=3`：与 `_G.lua:171-176` 的 `mtCoin=1/mtYuanbao=3` 同空间，`postlogin.go:32-33` 的 `currencyGold/currencyYuanBao` 已用 1/3；`storesystem.lua:143` 等大量调用把 NumericType_YuanBao 直接传给 getCurrency。

### 改动
- `ltcq-go/internal/luahost/enums.go`：新增 `cppGlobalTables` 条目 `AwardTypeAndCurrency`，用 `if X == nil then X = n end` 注入 4 个标量常量；保留现有 `NumericType` 占位表不动（其 Chips=1/GuildContrib=2/GuildFund=3 与 mt* 重号是未证实占位，标注 TODO，本轮不碰以免破坏加载期行为）。
- 未注入无法证实的 `NumericType_Achieve/Chips/GuildContrib/GuildFund/GodWeaponExp/ShenShouExp`（RUNBOOK 五十八/五十九：ghidra 无伪代码）。
- `luahost_test.go` 的 `TestGopherLua_InjectEnums` 增加 4 个常量断言 + 分支命中测试（`0==AwardType_Numeric`、`1==AwardType_Item`、`NumericType_Gold==mtCoin==1`、`NumericType_YuanBao==mtYuanbao==3`），直击"注入前奖励静默丢失"的根因。

### 验证与边界
- `go test ./...` 全通过、`go vet ./...` 通过、`git diff --check` 通过。
- 这是 A5 giveAwards 的**前置基础能力之一**，不是 giveAwards 本身；Lua table→奖励列表解码、跨表原子性、容量模型、邮件补发仍缺（RUNBOOK 五十九），giveAwards 继续不实现。
- 注入值只影响 Lua VM 常量可见性，不改变 Go 侧任何业务 API 或协议；无需重启服务栈（当次测试 VM 验证即可）。
