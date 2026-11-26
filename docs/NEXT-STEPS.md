# 雷霆传奇 Go 重写 · 整体后续规划（2026-11-25 制定）

本文件是「从实际代码/文档核查得出的下一步路线图」，非愿景文档。所有状态依据 `docs/ROADMAP.md`、`docs/RUNBOOK.md` 与 `ltcq-go/internal/` 源码，核查日期 2026-11-25。每完成一项就回填状态与证据，避免"做了却记成没做/没做却记成做了"。

## 一、实际状态（核查基线）

| 项 | 状态 | 依据 |
|---|---|---|
| M1 loggerserver | ✅ 完成 | ROADMAP.md `loggerserver-spec.md:146` |
| M2 网关+登录鉴权 | ✅ 字节对齐 | ROADMAP.md:195、RUNBOOK 七.8 |
| 4.1 luahost / 4.2 加载链 / 4.4 注册表 / 4.5 网络 / 4.7 定时器 | ✅ 完成 | ROADMAP.md:59-65；446 个 lua 编译 444/446 |
| M4 登录后业务链 | ✅ 实测通过 | postlogin.go 全链 + RUNBOOK 八/十七/十八；ROADMAP.md/protocol-frame.md 已回填实测终验 |
| M5-D~G（奖励重生/反击HP/死亡复活/怪物AI） | ✅ 隔离验收 | ROADMAP.md:198-201 |
| M5-H 背包 | ✅ 隔离验收 | bag.go 单种可叠加物品；mutate 15 项、relogin/restart 各 9 项真实浏览器断言通过 |
| 4.3 engineapi | 🔶 T0 约 60 API，7 个接内存实体 | ROADMAP.md:61 |
| 4.6 场景实体 | 🔶 技能/掉落拾取待做；ClientConfig/Static/法阵持久化已闭环 | ROADMAP.md:64；RUNBOOK 十九/二十 |
| M0 逆向规格 | 🔄 Ghidra 工程导入中 | ROADMAP.md:194 |

### 重要偏差（先修正认知）
- **M4 的 "golden 终验" 实际已完成**：真实浏览器已验证 ActorBaseData/RoleData/EnterScene 被客户端正确消费、法阵段 436B 布局正确、ClientConfig 正常。ROADMAP.md 与 protocol-frame.md 已回填实测终验。
- **背包不是待办**：`bag.go` 已实现 `cmdBag=3` 的 InitData(1)/Delete(3)/Add(4)/Update(5)/UseItem(6)、MySQL `actorvariable.variable` blob 存取（含 CRC）、`Actor.AddItem/CostItem`，客户端真实验收通过。掉落/拾取直接复用 `Actor.AddItem`。

## 二、路线图（2026-11-25 按需求价值重排）

优先级判据：**玩家可见的数据正确性 > 玩法功能 > 架构长尾 > 需 golden 才能安全做的项**。

### 阶段 A：M4/M5 收敛（当前）
目标：把 🔶 转 ✅，修数据正确性，再补玩法。

| 优先级 | 任务 | 内容 | 依据/验收 |
|---|---|---|---|
| P0 ✅ | A1 回填文档 | M4 golden 实测通过、法阵/ClientConfig 状态 | 已完成 |
| P0 ✅ | A4-1 ClientConfig 持久化 | `persist.go` 把 ClientConfig 写入 `actorvariable.cs_variable`（`LTGOPERS1` magic），登录组装 `00 13` 包前还原；`[00][13]` 更新后同步 Flush | 已完成；`persist_test.go` + 全量 go test/vet/diff-check 通过，见 RUNBOOK 十九 |
| P0 ✅ | A4-2 Static/法阵持久化 | `luahost/tablecodec.go` 将 staticVar Lua table 编为 Go 自有 JSON 节点；登录还原、登录事件/离场 Flush；`buildRoleDataWithDemon` 从 `staticVar.demoncircledata[0]` 读法阵字段并按原版 436B 布局发包 | 已完成；`tablecodec_test.go`/`persist_test.go`/`postlogin_test.go` + 全量 go test/vet/diff-check 通过，见 RUNBOOK 二十 |
| P1 ✅ | A3 技能树 | 服务端技能数据与释放协议（当前只有普攻） | 最小实现完成；真实浏览器 mutate 15 项、重登/重启各 10 项断言通过，C++ 基线已恢复 |
| P2 🔶 | A5 engineapi 长尾 | API 覆盖度补齐到 ≥80% 门禁（只计真实实现） | A5-1/A5-3/A5-4/A5-5 已完成：定时器生产链路、角色/账号/服务器/战力/背包查询、登录元信息查询与 VIP 原子存档；本批补金币/元宝、公会/章节/月卡/轮回快照、在线角色名/账号查询、职业/性别别名及货币锁，见 RUNBOOK 二十三至二十七；奖励与经验写 API 仍待容量或完整依赖链 |
| **P3 暂缓** | A2 掉落/拾取 | 协议布局已逆向但 golden 缺失、6 项不确定（见第四节） | 按"禁止无 golden 猜协议"原则暂缓；需先抓 C++ golden |

### 阶段 B：M5 Go 全栈端到端（2~3 天）
目标：从"隔离验收"升级为"真实客户端全程无感"。

- B1 打通 登录→创角→进场景→打怪→拾取→存盘重登→充值发货 全链路
- B2 72h soak 前置验证

### 阶段 C：工程质量（穿插）
- C1 `go test -race`（Windows 无 CGO，需换有 C 编译器环境）
- C2 luahost 冒烟进 CI（MySQL service 容器）
- C3 `gameworld -dev` 单进程模式 ADR 立项

### 阶段 D：M6 数值校准（最后）
- D1 reCalcAttr、技能伤害对齐
- D2 5000 机器人压测报告
- 建立在 B 全链路打通之后，否则校准无锚点

### 按需触发（不盲目做）
- 日志中 Boss(10)/拍卖(76)/特戒(17)/排行(18)/公会(37)/好友(47) 查询未响应：等实测发现某功能"点了没反应"，再抓 C++ golden 逆向补对应系统。**禁止无 golden 依据地猜协议**。
- M0 Ghidra 常量提取（NumericType 等新注册枚举）：并行依赖，不阻塞 A；property ID 全集与 mt/qat 货币枚举已从 `_G.lua` 和原版调用确认。
- PHP 安全加固 8 项、跨服（ADR-005 延迟）、AOI 空间网格优化：阶段 3 后或触发式。

## 三、当前执行点

- **当前执行点**：A1/A4-1/A4-2/M5-H/A3/A5-1/A5-3/A5-4/A5-5/A5-6 至 A5-19 已完成 → 继续 **A5 engineapi 长尾**。A5-19 将 `setIntProperty` 接入白名单写入：内存字段标脏，金币/元宝/绑定币按绝对数量写入（金币/元宝先落库），VIP 复用 `setVipLevel` 原子存档；`P_ID`、坐标、HP/MaxHP 因读侧 World 优先继续失败关闭。下一批只做有现有字段或在线状态支撑的 API，不再把证据不足的写接口当作可用。奖励入口 `giveAwards/canGiveAwards` 已核定仍缺装备背包容量和跨表原子性，继续暂缓；`sendTipmsg/reCalcAttr` 仍无完整证据，不猜协议。`setExp` 需连同缺失的 `setLevel` 规划完整经验链；`getLastLoginIp` 需先证实整数 IP 到原字符串的编码；`getEntityType` 需先取得原版 `EntityType_*` 枚举数值，均不做猜测实现。RoleData 技能段已确认是 5×int32 等级而非配置 ID。
- **背包的位置**：已完成（M5-H），不是待办；掉落/拾取阶段会调用它，不重复做。
- **A2 掉落暂缓**：golden 缺失，见第四节。
- **现场约定**：写代码期间 IDE 预览不再长期占用；实测采用"起隔离 Go 服务 → 验证 → 停服务 → 恢复 C++ 现网 9002"的短周期，避免 Go 服务长期挂在隔离端口。

## 四、A2 掉落/拾取逆向结论（2026-11-25，暂缓实现）

已做 very thorough 逆向（结论详细见本轮 subagent 输出），关键发现：

- **掉落实体**：`EntityType.DropItem=6`，通过 `sBaseCmd_OtherEntity(0,4)` 独立实体下发。布局：`[u16 count][type=6][f64 handle][u32 configID][f64 masterHandle][u32 x][u32 y][RewardData{type:u32,id:u32,count:u32}]`。证据：`main.min.js` L11/L2/L21/L5/L66。
- **拾取路径**：客户端自动寻路靠近 → 本地 `postFlyItem` 飞入 + `sendGetReward(CMD_Base=0, cmd=1, [u8 flag])`（`main.min.js` L5/L67）；BOSS 掉落另走 `CMD_Fuben=1, cmd=29`。
- **原版**：`dropsys.createDropById`（C++）生成地面实体，Lua 只有调用点（`monsteractionimpl.txt:1060` 等）。
- **Go 现状**：`combat.go:320` 击杀直接 `actor.AddItem` + 金币/经验包，**绕过了地面掉落/拾取链路**；无 `sBaseCmdGetItem=6` 常量、无 DropItem 实体类型。

**暂缓原因（关键）**：
1. 已有 C++ 抓包 `gw6001_full_session.bin`(644KB)/`gw9001_full_session.bin`(570KB)/`gate_upstream.raw` 搜 `00 04 01 00 06 00`（DropItem 实体包）**全部 0 命中**——无 golden 可对照。
2. 6 项不确定未解：`sBaseCmd_GetItem(0,6)` payload 与 handler、拾取上行 cmd 准确值、`dropsys` 组包细节/handle 规则/超时消失包、DropItem configID 语义、多实体 count>1 布局。
3. 按本规划"禁止无 golden 依据猜协议"原则，不硬写。需先抓 C++ 现网掉落 golden（有现网操作风险，需用户确认）或转做其他有依据任务。

## 五、变更记录

- 2026-11-25 制定：基于 ROADMAP/RUNBOOK/源码核查，确认背包已完成、M4 golden 实际已通过；确定起点为 A1+A2。
- 2026-11-25 A1 完成：ROADMAP/protocol-frame 的 M4 状态回填为"实测通过"，golden 终验结论落定。
- 2026-11-25 A2 逆向完成但暂缓：掉落协议 golden 缺失，待用户决定抓包或换任务。
- 2026-11-25 A4-1 完成：ClientConfig 写入 `actorvariable.cs_variable` 并在登录还原；新增独立 Dirty 标记与 Flush；全量验证通过。
- 2026-11-25 A4-2 完成：Lua staticVar 以 Go 自有 JSON 节点持久化；登录/离场快照落库；RoleData 法阵段从 `demoncircledata[0]` 读取；全量验证通过。
- 2026-11-26 M5-H 隔离验收：真实浏览器 mutate 15 项、重登 9 项、进程重启后 9 项断言全部通过；最终 `200013×1` 重登/重启后保持；未修改游戏前端资源，也未新增数据库表。
- 2026-11-26 A5-1：修复定时器生产驱动与 `(actor, eid, ...)` 回调签名，补齐 Lua Lite/Ex/取消/自取消/登出清理回归；转生、VIP、职业、性别、场景、经验、在线 Actor、服务器、账号、战力查询及等级/转生别名接已有字段，离线 Actor 返回 `nil`。全量 go test/vet/diff-check 通过，未启动隔离服务或修改数据库。
- 2026-11-26 A5-2 核查：`actorawards.lua` 才是原版奖励入口，仅 Numeric/Item，容量只检查装备背包；Go 无装备背包且背包与 actors 分表，暂不实现跨表非原子发放，也不把 `canGiveAwards=true` 占位当作可用。
- 2026-11-26 A5-3：新增 `getBagItemCount`（在线背包物品数量总和）及 `getActorCurExp/getVIPLevel` 原版别名；复用已有字段并覆盖空背包、空参和离线边界。核查 `setExp/setVipLevel` 后延后写实现，全量 go test/vet/diff-check 通过，未改前端、schema 或共享数据。
- 2026-11-26 A5-4/A5-5：接入 `getRecharge/getCreateTime/getAppid/getPf/getPfId`，创建时间由 SQL 转 Unix 秒；`setVipLevel` 通过独立可选存档接口先更新 `actors.vip_level` 再改内存，失败不改变在线值。覆盖登录快照、登录事件内写入、空参/离线、存档失败和幂等写入；全量 go test/vet/diff-check 通过，C++ 基线五端口正常，隔离端口关闭。
- 2026-11-26 A5-6：接入 `getMoneyCount`、公会/章节/月卡/轮回登录快照与在线按名查询；`getGuildId` 错误占位改为真实字段，重名按失败关闭处理，货币读写增加 Actor 锁。覆盖登录装配、空参/离线、绑定/未知货币、重名及并发增减；全量 go test/vet/diff-check 通过，C++ 基线五端口正常，隔离端口关闭。
- 2026-11-26 A5-7：补齐 `getActorJob/getActorSex` 别名与 `getActorByAccountName/getActorIdByAccountName` 在线账号查询；账号精确匹配、重名失败关闭，不查离线数据库。覆盖别名、在线命中、空参/非字符串/离线和账号重名；全量 go test/vet/diff-check 通过，C++ 基线五端口正常，隔离端口关闭。`getRealLevel/getRealServerId` 语义差异未证实，继续暂缓。
- 2026-11-26 A5-8：`changeCurrency` 的金币/元宝经可选 `CurrencyStore` 先写现有 `actors.gold/yuanbao` 再改内存；严格校验整数、余额和溢出，零变更幂等，失败不改内存，绑定/其他类型仍仅内存。登录事件前绑定存档器；修复 Lua 数值转 Go 时小数被截断的问题。覆盖正负变更、非法参数、余额不足、溢出、缺存档器、存档失败、纯内存类型、登录事件写入及并发；全量 go test/vet/diff-check 通过，C++ 基线五端口正常，隔离端口关闭。
- 2026-11-26 A5-9：替换 `getActor/getRoleCount/getRole` 占位；仅查在线玩家，当前单主角色模型返回 count=1、index=0 返回同一 actorID，其他索引或非法/离线输入返回 nil/0 且不创建 Actor。覆盖在线命中、整数小数边界、空参、越界索引、怪物/离线句柄和查询不创建实体；全量 go test/vet/diff-check 通过，C++ 基线五端口正常，隔离端口关闭。
- 2026-11-26 A5-10：实现 `getRoleSkillLevel(actor, roleid, index)` 与 `GetRoleByJob(actor, job)` 两个只读 API；前者按主角色职业映射五槽基础技能等级，后者仅对职业 1..3 的在线主角色返回 actorID。严格校验整数、roleid=0、槽位 0..4、职业范围与在线状态，默认返回 0/nil 且不创建 Actor。覆盖五槽已学习/未学习、非法角色/职业、离线与数值边界；全量 go test/vet 通过，C++ 基线端口正常。`changeGold` 的客户端金额通知语义未证实，继续暂缓。
- 2026-11-26 A5-11：实现 `getEntity/getActorByEt`，复用进程共享 `scene.World`；前者解析玩家或怪物实体句柄，后者仅把玩家实体转为在线 Actor。引擎启动时原子绑定 World，严格校验正整数句柄，未知实体、怪物、非法参数或未绑定 World 返回 nil 且不创建 Actor。覆盖玩家/怪物命中、在线 Actor、无场景 Actor、非法/离线句柄和未绑定 World；全量 go test/vet 通过。`isPet` 因当前无 Pet 实体模型继续暂缓。
- 2026-11-26 A5-12：注册 `getGmLevel/isImage/getFubenHandle/getFubenId/isInFuben`，在当前无 GM、镜像或副本实例模型时分别固定返回 `0/false/0/0/false`；修正 `getFubenHandle` 原 `stubInt1` 错误回显 actorID 的问题。覆盖空参、在线/离线、非法句柄、小数、字符串和越界整数；全量 go test/vet 通过。该批只保证失败关闭，不代表 GM、镜像或副本能力已实现。
- 2026-11-26 A5-13：实现 `getHandle/getEntityPosition/getIntProperty`。句柄只做严格正整数身份提取；坐标读取共享 World 实体快照并通过新增多返回值桥接保留 Lua `(x,y)`；属性仅映射 `_G.lua` 枚举中已有 World/Actor 字段支撑的白名单，未知属性、`P_ROOT_EXP` 和非法输入返回 0。覆盖移动后实时坐标、玩家/怪物属性、怪物受伤后的实时 HP、Actor 属性、无 World/无场景 Actor、非法参数与不创建 Actor；全量 go test/vet/diff-check 通过，未改前端、schema 或共享数据库。
- 2026-11-26 A5-14：实现 `getHp/getHpMax/getPosition/isDeath/getId`，World 实体快照优先、在线 Actor 字段回退；坐标保留 Lua 双返回值，死亡按 HP<=0 判断，玩家 ID 返回 actorID、怪物 ID 返回 ConfigID。据 `monsteractionimpl.txt:92-94` 修正 A5-13 怪物 `P_ID` 语义。覆盖 World/Actor 优先级、移动、怪物受伤、玩家死亡、无场景 Actor、非法参数与不创建 Actor；全量 go test/vet/diff-check 通过。Go 隔离服务已按用户要求关闭，门户恢复 C++ 9002，隔离库与 `.tmp_live` 已清理，C++ 五端口正常。
- 2026-11-26 A5-15：实现 `getGamePf`，按 `actorfunc.lua:103-112` 从在线 Actor 的 `PfID` 派生游戏渠道；仅对 `union` 前缀且含 `*` 的值取首个星号后子串，其余原样返回。严格校验正整数句柄，空参、非法/越界和离线返回空字符串。覆盖普通渠道、union 解析、多个星号、大小写、空串、非法/离线和并发只读；全量 go test/vet/diff-check 通过，未改前端、schema 或共享数据库。`getEntityType` 因原版枚举数值证据不足继续暂缓。
- 2026-11-26 定时器运行链修复：补齐 `System.getNowTime/timeDecode/isSameDay/getStaticVar`，解决真实 5 秒与分钟链的 `attempt to call a non-function object`；`TimerDriver.Due` 不再多传 Go `pid`，由原版 `postscripttimer` 携带 Lua `eid`，回调失败记录日志但继续清理与周期重排。真实 engineevent/supertimer 节拍、时间 API 边界、VM 静态变量引用及 `eid≠pid` Lite 回调已回归；全量 go test/vet/diff-check 通过。`getStaticVar` 当前仅为 VM 内共享表，不声称持久化；Go 隔离服务保持关闭。
- 2026-11-26 A5-16：`LActor.changeYuanBao` 从空操作改为固定 `NumericType_YuanBao=3` 的 `changeCurrency` 别名，复用现有 `actors.yuanbao` 可选存档器；Actor 锁内严格校验整数、余额和溢出，先落库后改内存，零变更不写库。覆盖元宝增减且金币不变、额外业务参数、非法/离线输入、余额不足、溢出、缺存档器、存档失败回滚和 8 路并发；全量 go test/vet/diff-check 通过。未新增表/字段、未改前端或共享数据，Go 隔离服务保持关闭。
- 2026-11-26 A5-17：`LActor.log` 从空操作改为本地 `slog.Info` 结构化日志，保留严格正整数 Actor ID 与字符串化可变参数；非法句柄按 0 记录，离线正整数按原 ID 记录，不查询/创建 Actor 或修改业务状态。覆盖消息字段、标量/nil 参数、空参、零/负/小数/字符串/NaN/Inf/离线句柄；全量 go test/vet/diff-check 通过。当前未接 7001 或日志库，Lua 表不声称完整序列化；Go 隔离服务保持关闭。
- 2026-11-26 A5-18：`stopAI/setAITarget/getAITarget/setAIAttackMonster` 接入进程共享 `CombatManager`；新增玩家停战与显式目标状态，支持玩家→怪物、怪物→玩家同场景存活目标，停战保留目标、设置目标恢复战斗、恢复自动打怪清显式目标。死亡/离场/跨场景/非法类型失败关闭，断线清理 AI 状态；覆盖 Lua 桥接、目标优先、越界不回退、怪物指定玩家、失效清理、非法参数、未绑定管理器与 8 路并发；全量 go test/vet/diff-check 通过。未新增表/字段、协议或前端修改，Go 隔离服务保持关闭。`setAIPassivity/reCalcAttr/getGuildPtr` 继续暂缓。
- 2026-11-26 A5-18 复核加固：删除包级全局 Combat，改为按 Lua VM 闭包绑定各自 CombatManager；登录事件失败回滚先清 AI 再移出 World；路由生命周期锁串行化旧连接离场与同号重登；索敌实时重读 World 的 HP/位置/场景，跳过同 Tick 已死亡或离场目标。据 `citysystem.lua:455` 新增 `setAITargetNull`，只清显式目标或怪物仇恨并保留停战，区别于恢复索敌的 `setAIAttackMonster`。覆盖双 VM 隔离、同号重连、登录回滚、同 Tick 死亡目标与清目标边界；全量 go test/vet/diff-check 与 shuffle 回归通过，Go 隔离服务保持关闭。
- 2026-11-26 A5-19：`LActor.setIntProperty` 从占位改为白名单写入。等级/性别/职业/经验/公会/战力/充值写内存并标脏；金币/元宝/绑定币按绝对数量写入，金币/元宝先经现有 `CurrencyStore` 落库，VIP 复用 `setVipLevel` 原子存档；`P_ID`、坐标、HP/MaxHP 因读侧 World 优先继续失败关闭。覆盖白名单回读、金币/元宝/绑定币/VIP 写入、存档失败回滚、非法参数与离线边界；新增 `setintproperty_test.go` 单测通过，未改前端、schema 或共享数据库。
