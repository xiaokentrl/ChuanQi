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
| 4.1 luahost | Interpreter 接口 + gopher-lua 实现 + #include 预处理器（once/#ifndef/#if OSTYPE）+ _G.lua 枚举注入 |
| 4.2 加载链冒烟 | **726 文件零错误——第 13 周前必须达成（最大风险关口）** |
| 4.3 engineapi | registry 表驱动实现 L* API（T0 优先），含 Lua 调用超时看门狗（500ms，原版缺失） |
| 4.4 五大注册表 | netmsgdispatcher/actorevent/monevent/csmsg/dbret 回调接通（csmsg 为 stub） |
| 4.5 网络路由 | 6001、OnNetMsg、LDataPack 收发 |
| 4.6 场景实体 | AOI/移动广播/怪物 AI（Go 实现——ADR-004 红线） |
| 4.7 定时器四层 | OnGameTimer / 5 秒精度 / supertimer / postscripttimer |

验收：原客户端连 Go 全栈，登录→进场景→打怪→拾取→存盘重登不丢。

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
| M1-M6 | ⏳ 未开始 | — |
