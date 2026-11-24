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
