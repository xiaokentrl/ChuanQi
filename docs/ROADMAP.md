# 雷霆传奇 Go 重写工程规划（ROADMAP v1.3）

> 项目性质声明：本项目为已有源码包的学习研究性重写工程。传奇类 IP 存在历史诉讼风险，
> 源码包源自他人商业运营产物——**若涉及对外商业运营，须先完成法律评估**。
>
> 版本：v1.3 ｜ 2026-11-22 ｜ 修订：v1.0 基础路线 → v1.1 可维护性 → v1.2 容量/安全/健壮 → v1.3 边界与触发式决策

## 一、目标与范围

**目标**：用 Go 重写四个 C++ 进程（dbserver / gameworld / gateway / loggerserver），Lua 业务层、
配置、PHP 门户、egret 客户端零改动，原客户端无感游玩。

**非目标（钉死）**：
- ❌ 不重写 Lua 玩法层（414 文件 / 9.9 万行是资产）
- ❌ 不重写 PHP 门户（安全债另列清单分期偿还）
- ❌ 不兼容 2021 旧玩家数据（dump 为空结构，无迁移负担）
- ❌ **首版跨服降级为 stub**（csproto 60 条命令不做完整实现，触发条件见 §10；ADR-005）

**验收总标准**：原 egret 客户端 → 登录 → 创角 → 进场景 → 打怪 → 充值发货，全程无感知后端是 Go。

## 二、关键技术决策（ADR 索引）

| 决策 | 结论 | ADR |
|---|---|---|
| 不用重量级框架，标准库+精选库（≤8 直接依赖） | 自建 netframe 薄层 | 001 |
| Lua 宿主：gopher-lua 先行，Interpreter 接口隔离 | 升级 LuaJIT 预留路径 | 002 |
| 服务配置 .txt 用 gopher-lua 解析 | 配置本身是合法 Lua 表 | 003 |
| **热路径（移动/AOI/攻击）留 Go，不进 Lua** | 容量支柱，Go/Lua 边界清单在 engine-api.md | 004 |
| 首版跨服 stub | 单区跑通是主线 | 005 |
| 全新 schema，不做旧数据迁移 | 无历史包袱 | 006 |
| 差分对拍 + golden 字节为质量锚点 | 每服务先挂原版跑 | 007 |
| 可观测性：slog + /healthz + /metrics | 原版零监控，补齐 | 008 |

## 三、阶段分解

### 阶段 0 · 逆向与规格（1-2 周）
| 项 | 产出 | 验收 |
|---|---|---|
| 0.1 Ghidra Baseline 工程 | gameworld（28MB，DWARF 未剥离）71 systems 模块函数地图 | 能定位 msg_mgr/reCalcAttr/actor blob 序列化 |
| 0.2 包帧格式逆向 | docs/protocol-frame.md | DevTools 抓真实 WS 流验证吻合 |
| 0.3 API 规格书 | docs/engine-api.md + engine-api-matrix.tsv | **v0.1 调用矩阵已完成**：568 函数/12666 调用/T0 前 50 占 81%；T0 语义规格逐个补齐 |
| 0.4 staticVar/actorblob 格式 | docs/persistence-format.md | 空库+人工造数交叉验证 |

### 阶段 1 · Go loggerserver（0.5 周）
收包 7001 → 月表写 MySQL；对拍：原版 gameworld 同时发两套 loggerserver 比对入库。
验收：10 分钟游戏双库一致。

### 阶段 2 · Go gateway（1-2 周）
WS 握手复刻 + 8192 会话池 + 6001 透明转发（不解包），双向字节记录存 reference/captures/。
验收：原客户端经 Go 网关进游戏，转发字节零差异。

### 阶段 3 · Go dbserver（2-4 周）
45 个 DbCmd → SQL（dbprotocol.lua 已列全）+ 连接池 + 缓存；Go dbserver 挂原版 gameworld 对拍。
验收：原版 gameworld 全功能正常，数据一致。

### 阶段 4 · Go gameworld 宿主（6-10 周，核心）
| 子项 | 内容 |
|---|---|
| 4.1 luahost | ✅ 已实现：Interpreter 接口 + gopher-lua 宿主 + `--#` 预处理器（include/once/pragma/ifndef/ifdef/if/define/endif/else）+ 500ms PCall 看门狗 + require loader（dataRoot/functions 解析）+ `_G.lua` 枚举注入（1214 常量 + SystemId/Protocol 占位，真实值待 Ghidra） |
| 4.2 加载链冒烟 | **726 文件零错误——第 13 周前必须达成（最大风险关口）**；✅ 2026-11-23 已提前预研：446 个 .lua 编译 444/446（99.6%），见下方"冒烟预研结论" |
| 4.3 engineapi | 🔶 A5 进行中：registry 表驱动（声明式 Entry + CI 可校验）；LDataPack 包组（write*/read*/writeData/flush，golden 字节测试）；A5-1/A5-3/A5-4/A5-5 已接通转生/VIP/职业/性别/场景/经验/在线 Actor/服务器/账号/战力/背包总数量/充值/创建时间/渠道元信息查询及原版别名，VIP 与金币/元宝单字段按先落库后改内存写入；新增金币/元宝查询、公会/章节/月卡/轮回登录快照、在线角色名/账号查询及货币锁，职业/性别别名已补齐；A5-9 接通在线玩家解析与单主角色 count/role 查询；A5-10 接通五槽技能等级只读查询与按职业返回主角色句柄；A5-11 接通场景实体解析与玩家实体到在线 Actor 的严格转换；A5-12 补齐 GM/镜像/副本状态查询并修正 getFubenHandle 错误回显 actorID 的失败关闭语义；A5-13 接通句柄提取、实体坐标双返回值与 World/Actor 属性白名单只读查询；A5-14 接通血量、最大血量、坐标、死亡状态与实体模板 ID 查询，并修正怪物 P_ID 应返回 ConfigID 的语义；A5-15 接通 getGamePf 渠道派生查询，getEntityType 因原版枚举值证据不足继续暂缓；A5-16 将 changeYuanBao 从空操作接入现有元宝单字段存档链路，覆盖增减、非法输入、余额/溢出、存档失败与并发；A5-17 将 LActor.log 从空操作接入本地 slog 结构化日志，明确尚未接 7001；A5-18 接通 stopAI/setAITarget/getAITarget/setAIAttackMonster/setAITargetNull，使用进程共享 CombatManager 的停战、显式目标与失效清理状态；AI API 按 Lua VM 闭包绑定 Combat，重连离场串行化，登录回滚清 AI，索敌实时重读 World 状态；A5-19 将 setIntProperty 从占位改为白名单写入：等级/性别/职业/经验/公会/战力/充值写内存并标脏，金币/元宝/绑定币按绝对数量写入且金币/元宝走现有存档器，VIP 复用 setVipLevel 原子存档；P_ID、坐标、HP/MaxHP 因读侧 World 优先继续失败关闭；A5-20 将 setCamp/getCamp 接为在线 Actor 进程内阵营标签，仅校验整数不校验范围，不落库/不发包/不参与战斗判定；CampType 全局枚举数值无定义证据，怪物句柄继续失败关闭；A5-21 补齐 LDataPack.readInt64/readData（与 write* 对称的变参读取）和 LActor.changeMoney（changeCurrency 兼容别名）；A5-22 补齐 LActor.checkItemNum/consumeItem 窄语义（仅查/扣 Actor.Items，useYuanBao=true 失败关闭）；奖励与经验写 API 因容量或依赖链不足暂缓，其余 T1/T2 长尾待补，不把 stub 计入覆盖门禁 |
| 4.4 五大注册表 | ✅ 已实现：加载原版 5 个 Lua module + System stub + Go 触发入口（Fire/FirePCall），csmsg 按 ADR-005 stub；4 测试通过（真实 OnNetMsg/OnActorEvent 回调、超时） |
| 4.5 网络路由 | ✅ 已实现：`netframe` 网关帧编解码下沉 + `NetRouter`（sid↔actor 映射、cmd=5 上行→OnNetMsg、LDataPack.flush→cmd=5 回包帧）+ `cmd/gameworld` 6001 骨架（80B 注册握手 + 帧 pump）；4 测试通过（帧→OnNetMsg→readInt→回包帧全链路、会话映射、未绑定 actor、短帧拒绝） |
| 4.6 场景实体 | 🔶 进行中：进程级共享 World、矩形 AOI、登录/移动/断线生命周期、静态怪物、最小自动战斗与怪物主动 AI 已接通；真实浏览器已验证伤害/死亡/消失、经验/金币奖励、满血重生、存档重登、索敌/追击/脱战归位。技能、掉落/拾取与完整玩法持久化仍待做 |
| 4.7 定时器四层 | ✅ 生产链路已接通：Go 10ms 调度器 + L1 分钟/L2 5 秒节拍 + 原版 supertimer/postscripttimer Lua 包装；Engine 启动/关闭驱动生命周期，Go 仅传 `(actor, ...)`，由 Lua 包装携带自身 `eid`，支持取消、周期与回调内自取消；2026-11-26 补齐 System 时间/VM 静态变量依赖、真实 Lua 节拍链和 `eid≠pid` 回归，回调错误记录日志且仍执行清理/重排 |

验收：原客户端连 Go 全栈，登录→进场景→打怪→拾取→存盘重登不丢。

> **M2 实测修订（2026-11-23）**：隔离端口对照证明 Go 网关协议正确（握手/skey/注册/cmd=5 上行均实证），探针打现网 C++ 网关能拿到登录成功回包 `ff 01 00`。Go 全栈无回包的根因在 **Go gameworld 缺引擎级登录状态机**（登录 `CMD_Login=255` 由 C++ `GameGate`/`Actor::Onlogin` 处理，非 Lua 协议），诊断日志实证帧已到达（`sid=552 sysid=1 cmdid=0 len=62`）。
>
> **✅ 已修复（同日）**：`internal/engineapi/login.go` 实现登录状态机（解析→MD5 校验→openuser→BindActor→回包），`NetRouter` 按 `CMD_Login` 分流；`cmd/gateway` 回向补 skey 回显。隔离端口实测回包 `71 a0 97 68 ff 01 00`（len=7）与 C++ 现网同构，真实 MySQL 校验通过。**M2 网关侧 + 登录鉴权已达字节对齐**。详见 RUNBOOK §七.8。
>
> **✅ 登录后业务链已实测通过（2026-11-24 实现，11-25 回填状态）**：`internal/engineapi/postlogin.go` 编排登录成功后的业务链——① `AccountStore.LoadActorData` 从 actors 表加载角色数据并填充内存 Actor；② `AccountStore.EnsureActor` 幂等建号（等价存储过程 `createnewactor`，actorid 走 auto_increment）；③ 下发 `ActorBaseData`/`RoleData`/`EnterScene`/`OtherEntity`/背包初始化/ClientConfig；④ `FirePCall(OnActorEvent, aeUserLogin=13)` 触发 Lua 事件链。`cmd/gateway` 回向**所有业务包**补 skey。
>
> **golden 终验结论**：原"骨架推断、待抓包终验"已于 11-25 通过真实客户端实测关闭——`ActorBaseData`/`RoleData`/`EnterScene` 布局被客户端正确消费并成功进场景；`RoleData` 尾部法阵段 436B（6 子阵×4 特质×5 潜能，见 `demoncircle.lua:1160`）修复了 `fzRedShow` 读 `undefined.Lv` 弹框；`ClientConfig`(19) 协议补齐。详见 RUNBOOK §八/§十七/§十八 与 protocol-frame.md §五。

#### 冒烟预研结论（2026-11-23，提前验证 4.2 风险关口）

工具：`cmd/luasmoke`（`go run ./cmd/luasmoke -dir <gameworld/data> -mode compile|load`）。依赖 gopher-lua v1.1.0（Lua 5.1 实现）。

| 验证级别 | 范围 | 结果 |
|---|---|---|
| 语法编译（compile） | 446 个 `.lua` 业务脚本 | **444/446 通过（99.6%）** |
| 语法编译（compile） | 全部 1395 个资产（.lua/.config/.confg） | 1363/1395（97.7%） |
| 执行（load，带 stub） | 446 个 `.lua` | 75/446（16.8%）——失败均为依赖未就绪（require 模块 62、全局表 nil 307），非语法/引擎不兼容 |

结论：
- **项目核心策略可行**：gopher-lua 能编译 99.6% 的业务脚本，726 文件零错误的风险关口基本排除（无需切换到 LuaJIT，ADR-002 触发条件未满足）。
- 2 个 `.lua` 编译失败（`config/modules/moduleclass/cornucopia.lua`、`sample.lua`）为裸表表达式 `{...}`（非语句块），原版按表达式 eval，非 gopher-lua 不兼容。
- 30 个 `.config` 失败为原版自定义多表序列/编码格式（如 `language/lang/*.config` 末尾 `},`），需 luahost 预处理器处理（4.1），非阻塞。
- load 模式的失败正是阶段 4.3（engineapi 568 API）+ 4.4（五大注册表）+ 加载顺序要解决的问题——**属预期工作量，不是可行性风险**。

### 阶段 5 · 数值校准与长尾（2-4 周）
属性管线（reCalcAttr）、技能伤害对齐；差异单滚动管理。
验收：实测 2 小时无感知差异。

### 阶段 6 · 跨服实现（延迟，触发条件见 §10）

## 四、里程碑时间线

```
第1-2周    M0 阶段0 逆向规格
第3周      M1 loggerserver 替代成功
第4-5周    M2 原客户端经 Go 网关进游戏
第6-9周    M3 持久化全部走 Go
第10-19周  阶段4 ─ M4(第13周): 726文件加载通过【风险关口】
                   ─ M5(第19周): Go 全栈端到端 + 72h 长稳
第20-23周  M6 数值校准 + 压测报告（5000 机器人）+ 无感替换
阶段6      跨服（延迟）
```

单人全职约 5.5 个月（含 v1.2/v1.3 增补）。

## 五、质量门禁（每个里程碑出栏条件）

```
通用：gofmt/goimports 零 diff ｜ golangci-lint 通过（errcheck+staticcheck+gocritic+depguard+funlen+gocognit+mnd）
      ｜ go vet 零告警 ｜ engineapi 覆盖率 ≥80% ｜ netframe 帧编解码 100% golden 覆盖
      ｜ 选型/格式/协议决策落 docs/adr/
专项：M2 起对拍回归一键化（netframe 对比模式）｜ M4 起 luahost 冒烟进 CI（MySQL 用 service 容器）
      ｜ M5 出栏含 72h soak ｜ M6 出栏含压测报告
```

## 六、可维护性机制（为半年后的人类维护者设计）

1. **分层依赖规则 lint 强制**：cmd → domain(scene/combat/storage) → engineapi → luahost → netframe，depguard 只许向下
2. **registry 表驱动**：568 个 API 声明式注册（单一事实源），CI 校验注册表 ↔ engine-api.md 不漂移
3. **变更影响分级**（见 ONBOARDING.md 首表）
4. **升级预案**：Interpreter 接口（换宿主）/ 双版本 CI 矩阵（升 Go）/ migrations/ 目录（换 schema）——见 ADR
5. **开发模式单进程**：`gameworld -dev` 内嵌 db/log，人工调试零门槛（ADR 待立项）
6. **AI 生成代码防线**：funlen≤80 行、gocognit≤25、mnd 禁魔法数字、每包 doc.go、语义注释格式
   `// 原版: <c++位置> | Lua调用: N处 | 规格: engine-api.md#xxx`

## 七、容量规划

| 维度 | 数值 |
|---|---|
| 单区同时在线 | **3000 设计目标**（gateway 上限 8192；原版商用同量级） |
| 单区理论上限 | ~8000（需 LuaJIT，触发条件见 §10） |
| 集群 | N 区 × 3000 线性扩展（s1/s2/s999 模式） |
| 注册用户 | 百万级（MySQL 非瓶颈） |
| PHP 门户 | ~500 req/s |

**瓶颈链**：网络 IO（Go netpoller 万级，非瓶颈）→ 会话内存（~10KB×8192，非瓶颈）→ MySQL（批量写，非瓶颈）
→ **Lua VM 单解释器 = 唯一真瓶颈** → 容量第一支柱 = ADR-004 热路径留 Go。
若纯 Lua 路径承载全部消息，瓶颈仅 ~800 在线——这条红线写进 engine-api.md 边界清单。

## 八、稳定性 / 安全性 / 健壮性

**稳定性**：进程守护（systemd/NSSM）｜ panic 三级隔离（会话 recover→Lua pcall→引擎级才重启）
｜ **Lua 500ms 超时看门狗（原版缺失，防脚本死循环卡全服）** ｜ 背压+包优先级丢帧（移动可丢业务不可丢，原版无）
｜ 优雅停机存盘 ｜ /healthz + /metrics 告警（单区 70% 水位）｜ **72h soak 为 M5 出栏条件**（原版有泄漏前科）

**安全五层**：
1. 网络：帧边界/CMD 白名单、畸形包 fuzz 进 golden、每连接限速（原版无）、可选 wss、内部端口 127.0.0.1
2. 账号：Login.php 密码 md5固定盐→bcrypt、防爆破限速、充值验签加时间戳+nonce
3. 业务：服务端权威（移动校验/货币物品仅服务端）、GM 鉴权延续、gm2021 已知 8 项漏洞分期重写
4. 数据：弃 root 建专用最小权限账号、**每日 mysqldump+binlog 保留 7 天（原版无备份策略）**、PHP 注入点修复
5. 供应链：govulncheck 进 CI、依赖 ≤8 周检、密钥不入库制度（已实践）

**健壮性**：MySQL 断连→拒登不崩溃+数据挂起追平 ｜ 网关断线重连会话保持 ｜ 单脚本异常隔离禁用
｜ kill -9 损失上限=快照间隔 ｜ 金丝雀开新区灰度（复用 mergesrv 开区机制）

## 九、反过度工程清单（不做，防止手痒）

微服务化 ｜ K8s/容器编排 ｜ OpenTelemetry 全链路 ｜ 多活容灾 ｜ ORM/代码生成 ｜ 客户端源码重建
｜ i18n ｜ 预抽象存储后端 ｜ 提前 LuaJIT/协程池/自研网络库

**"度"的判断标准**：新想法三问——①原版有对应物吗？②触发条件到了吗？③坏了谁修？两否即不做。

## 十、触发式决策表

| 事项 | 触发条件 | 动作 |
|---|---|---|
| gopher-lua → LuaJIT | 压测 <2000 在线 且 Lua CPU >60% | 启用 Interpreter 第二实现 |
| wss/TLS | 决定公网 https 部署 | gateway 配置项（胶水层已验证可改） |
| 跨服完整实现 | 单区全玩法验收（M6 后） | 阶段 6 独立里程碑 |
| 合服工具 | 真要合服时 | 补 Go 侧入口（mergesrv Lua 框架已有） |
| PHP 安全加固 | 阶段 3 后穿插窗口 | 8 项清单 |
| 性能基准进 CI | 出现性能回归事故后 | 加 benchmark job |

## 十一、风险登记表

| 风险 | 概率 | 对策 |
|---|---|---|
| gopher-lua module/setfenv 边角偏差 | 中 | 第 13 周 M4 冒烟提前暴露；备选 ADR-002 切换路径 |
| staticVar blob 逆向失败 | 中 | 兜底自定 schema（ADR-006 无历史包袱） |
| 战斗公式细节不准 | 高 | 配置驱动+实测校准，承认长尾 |
| 范围蔓延 | 中 | §1 非目标 + §9 反过度工程清单钉死 |
| GitHub 网络不稳 | 低 | 分批 push（已验证） |

## 十二、里程碑状态表

| 里程碑 | 状态 | 备注 |
|---|---|---|
| M0 阶段0 逆向规格 | 🔄 进行中 | 0.3 v0.1 矩阵完成；0.1 Ghidra 工程导入中 |
| M2 网关+登录鉴权 | ✅ 字节对齐 | Go 网关协议逐字节同构；登录状态机回包 `ff 01 00` + skey 实证 |
| M4 登录后业务链 | ✅ 实测通过 | postlogin.go 全链 + 背包 + 法阵段 + ClientConfig；真实浏览器进入场景、各面板无弹框；golden 以客户端消费终验 |
| M4 4.6 场景实体/AOI/打怪 | 🔶 最小闭环 | 共享场景/AOI/移动/静态怪物/自动战斗已实现；真实浏览器验证伤害→死亡→消失→经验/金币奖励→满血重生→存档重登 |
| M5-D 奖励与重生 | ✅ 隔离验收 | 致死攻击只结算一次；level/exp/gold 绝对值存档与失败重试；同句柄定时满血重生；17 项真实浏览器断言通过 |
| M5-E 怪物反击与玩家 HP | ✅ 隔离验收 | 受击仇恨、范围/间隔/伤害配置、HP 钳制与协议广播；真实浏览器 14 项断言通过，玩家 HP 90→80→70→60→50；C++ 基线已恢复 |
| M5-F 玩家死亡与安全复活 | ✅ 隔离验收 | 致死包序、死亡移动冻结、清除仇恨、同场景同坐标延迟满血复活与恢复战斗；真实浏览器 17 项断言通过，默认约 3 秒复活；C++ 基线已恢复 |
| M5-G 怪物最小主动 AI | ✅ 隔离验收 | 主动索敌、最近目标、追击、配置间隔攻击、leash 脱战与归位中禁重新索敌；真实浏览器 15 项断言通过，AOI 重入确认出生点；C++ 基线已恢复 |
| M5-H 单种物品背包闭环 | ✅ 隔离验收 | `200013` 叠加、`[03][04]` 发放、`[03][05]` 叠加、`[03][06]` 使用；`actorvariable.variable` blob 持久化；mutate 15 项、重登/重启各 9 项真实浏览器断言通过 |
| A3 最小技能树与释放 | ✅ 隔离验收 | 五槽激活升级、一键升级、原子存档、玩家冷却、单目标伤害与 `[02]` 路由已实现；真实浏览器 mutate 15 项、重登/重启各 10 项断言通过，C++ 基线已恢复 |
