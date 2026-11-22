# 引擎 API 规格书（engine-api v0.1）

> 本文是 Lua 层与 Go 宿主之间契约的**单一事实源**（ADR：registry 表驱动 + CI 漂移校验）。
> v0.1 = 调用矩阵与分层；语义规格（T0 逐函数）在阶段 0.3 持续补齐。
> 数据来源：`s1/Debug/gameworld/data/functions + script` 全部 .lua 的调用点提取（2026-11-22）。
> 完整矩阵见 [engine-api-matrix.tsv](engine-api-matrix.tsv)（函数级：调用次数排序）。

## 一、总量统计

| 命名空间 | 函数数 | 调用次数 | 说明 |
|---|---|---|---|
| LDataPack | 37 | 6266 | 包收发（allocPacket/write*/read*/flush）|
| LActor | 479 | 6175 | 玩家/实体操作（大头，长尾极长）|
| LGuild | 43 | 210 | 公会 |
| LActorMgr | 5 | 8 | 名字校验/随机名 |
| LAN | 4 | 7 | 语言包 |
| **合计** | **568** | **12666** | |

另有非 L 前缀引擎回调与 `System.*`（引擎事件入口，见 ROADMAP 4.4 五大注册表），随阶段 4.4 补录。

## 二、分层实施策略

| 层 | 定义 | 规模 | 策略 |
|---|---|---|---|
| **T0** | 调用次数前 50 | 占 **81%**（10302 次） | 逐个填语义规格 + 单测，最先实现 |
| **T1** | 前 51-100 | 累计 91%（11475 次） | 语义规格简版，实现+冒烟覆盖 |
| **T2** | 长尾 468 个（多数仅 1-2 次调用） | 9% | registry 机械注册，用到哪个填哪个 |

## 三、T0 Top-50 清单（实现顺序基准）

按调用频次降序（完整 50 行见 matrix TSV 前 50 行）：

```
LDataPack.writeInt 1405 ｜ LActor.getActorId 1402 ｜ LDataPack.allocPacket 804
LDataPack.writeShort 706 ｜ LDataPack.writeByte 677 ｜ LDataPack.flush 657
LDataPack.readInt 361 ｜ LActor.sendTipmsg 310 ｜ LDataPack.writeString 224
LDataPack.readShort 214 ｜ LDataPack.writeChar 200 ｜ LDataPack.setPosition 185
LActor.getStaticVar 163 ｜ LActor.postScriptEventLite 141 ｜ LActor.getItemCount 134
LActor.giveAwards 133 ｜ LActor.getActorById 128 ｜ LActor.costItem 126
LActor.getRoleCount 120 ｜ LDataPack.writeData 111 ｜ LActor.getCurrency 107
LDataPack.readString 106 ｜ LActor.getLevel 100 ｜ LActor.getName 99
LActor.getActorName 99 ｜ LDataPack.getPosition 98 ｜ LActor.getZhuanShengLevel 96
LActor.log 91 ｜ LActor.cancelScriptEvent 84 ｜ LActor.canGiveAwards 77
LActor.getRole 75 ｜ LDataPack.readByte 73 ｜ LDataPack.test_allocPack 72
LActor.stopAI 71 ｜ LActor.changeCurrency 69 ｜ LActor.setIntProperty 66
LActor.changeYuanBao 64 ｜ LActor.getGuildId 63 ｜ LDataPack.writeDouble 61
LActor.getFubenHandle 54 ｜ LActor.getActor 54 ｜ LActor.setCamp 51
LDataPack.readChar 49 ｜ LActor.getGuildPtr 49 ｜ LDataPack.writeUInt 48
LActor.getVipLevel 47 ｜ LActor.reCalcAttr 46 ｜ LActor.exitFuben 45
LDataPack.allocBroadcastPacket 44 ｜ LActor.enterFuBen 43 ｜ LActor.giveItem 42
```

T0 分组（实现批次）：
1. **包组**（LDataPack 全部 37 个）——净字节操作，golden 测试覆盖
2. **身份/属性组**（getActorId/getName/getLevel/getZhuanShengLevel/属性族）
3. **物品货币组**（giveAwards/costItem/getCurrency/changeYuanBao…）
4. **事件/变量组**（getStaticVar/postScriptEventLite/cancelScriptEvent…）
5. **场景副本组**（enterFuBen/exitFuben/getFubenHandle/stopAI…）

## 四、热路径边界清单（ADR-004，容量红线）

| 路径 | 归属 | 理由 |
|---|---|---|
| 移动/位置同步/AOI 广播 | **Go** | 每玩家 10-20 包/秒，进 Lua 则单区瓶颈 800 人 |
| 攻击/受击包路由与广播 | **Go** | 战斗高频 |
| 帧编解码（LDataPack 底层） | **Go** | 字节操作 |
| 定时器调度（四层的引擎节拍） | **Go** | 驱动层 |
| 业务事件处理（任务/奖励/活动/GM/商城） | **Lua** | 原版设计意图，热更层 |
| 数值合成最终计算（reCalcAttr 汇总） | **Go** 收口，Lua 各系统提供加成值 | 原版即此分工 |
| 新玩法逻辑 | **Lua** | 默认全部进 Lua，除非命中上表 |

> 判断规则：**每秒 ≥5 包/玩家的路径归 Go；交互型/低频路径归 Lua**。新增路径先归类再实现。

## 五、T0 语义规格（v0.2，按调用频次分批填写）

> 包组（LDataPack 全部 37 个）为净字节操作，规格自明，合并一条：
> **allocPacket(actor, cmd, subcmd) 分配回包（自动写入 CMD/subcmd 头）；write{Int,Short,Byte,Char,String,Data,Double,UInt,Word,Int64…} 顺序写入；read* 读取客户端上行；setPosition/getPosition 定位读写游标；flush 发出；allocBroadcastPacket 场景广播；rollBack 放弃未 flush 的写**。
> 测试基准：golden 字节（ADR-007）。以下仅列非自明函数。

### LActor.getActorId(actor) → number
- 调用 1402 处。实体句柄 → 数字 actorId（DB 主键，actor_s1.actors.actorid）。Go：actor.ID()。边界：离线实体无效。

### LActor.getStaticVar(actor) → table
- 调用 163 处。**玩家持久化主通道**：各系统存 `var.系统名 = {...}`，跨登录持久（落 actorblob，格式见 persistence-format.md）。
- Go：返回惰性 LTable，写时标脏，登出/定时序列化。单 VM 串行无锁。

### LActor.postScriptEventLite(actor, delayMs, func, ...) / cancelScriptEvent(handle)
- 调用 141/84 处。玩家级闭包定时器（postscripttimer.lua 封装，C++ 回调点 `MY_PostScriptOnTimeEvent`）。
- Go：注册到 postscripttimer 调度器；**登出自动清理**（actorevent aeUserLogout 挂钩）。Lite=一次性，Ex=可重复。

### LActor.giveAwards(actor, awardTable, reason)
- 调用 133 处。awardTable = 配置奖励列表（如 `TaskCfg.awardList`，元素 {id,count[,bind]}），reason=日志标签字符串。
- Go：遍历发放 → 物品走背包、货币走 changeCurrency 族；失败整体回滚。canGiveAwards(77 处) 先验背包空间。

### LActor.costItem(actor, itemId, count, reason) → bool
- 调用 126 处。扣物品，reason 入日志。同族：consumeItem(19)/costItemByUid(6)/checkItemNum(13)。
- Go：背包扣减 + DB 脏标记；不足返回 false 不扣。

### LActor.getCurrency(actor, type) / changeCurrency(actor, type, delta, reason)
- 调用 107/69 处。type = NumericType_* 枚举（**C++ 全局注册，Lua 源无定义——提取自 _G.lua 快照，待 Ghidra 交叉确认**）。delta 负=扣。
- Go：货币表接口；变更入审计日志（对齐原版 log 参数习惯）。

### LActor.getRoleCount(actor) → n / getRole(actor, index) → roleHandle
- 调用 120/75 处。**合击多角色系统**：index 0 为主角色，1+ 为副角色（开启角色功能页）。
- Go：player.Roles[]；属性/装备 API 多数有 Role 变体（getRoleCount 的高频说明了多角色渗透深度）。

### LActor.reCalcAttr(actor, force?)
- 调用 46 处。属性重算收口：遍历各系统加成（Lua 提供 get*Attr 族）→ C++ 合成 → setIntProperty。
- Go：pipeline 重算（各系统注册的加成提供器按序取值）。force=false 时增量重算。

### LActor.stopAI(actor) / setAITarget / setAIPassivity / setAIAttackMonster
- 调用 71+ 处。AI 控制族：停 AI/设目标/设被动/指定打怪。
- Go：AI 状态机接口（scene 层），原版 ai_state 在 C++。

### LActor.enterFuBen(actor, fbId, ...) / exitFuben(actor) / getFubenHandle
- 调用 43/45/54 处。副本进出（instance 系统对接点）。
- Go：实例管理器；Lua 侧生命周期回调经 instanceevent。

### LActor.sendTipmsg(actor, msg, typeMask?)
- 调用 310 处。typeMask=ttScreenCenter/ttTipmsgWindow 等位或（枚举见 _G.lua）。
- Go：组包 CMD_Notice 系列子命令，写 msg + type。

### LActor.getItemCount(actor, itemId) → n / addItem / giveItem / canAddItem
- 背包查询/发放族。giveItem(42) 直接发放；addItem(31) 带位置语义；getEquipBagSpace(12) 装备包空间。

### LActor.log(actor, ...)
- 调用 91 处。玩家级日志（走 loggerserver 通道）。
- Go：结构化日志事件 → 7001 日志流（与阶段 1 联通）。

### LDataPack.test_allocPack(72 处) 【待确认】
- 名称带 test 前缀但调用 72 次——疑似调试期遗留的高频用法，Ghidra 确认与 allocPacket 的差异后再定实现策略。

### 待提取项（依赖 _G.lua 快照/Ghidra）
- ~~tt* 提示类型位掩码全集~~ ✅ 已确认（_G.lua）：ttTipmsgWindow=1 ttScreenCenter=2 ttDialog=4 ttHearsay=8 ttMessage=16 ttWarmTip=32 ttGmTip=64 ttChatWindow=128 ttScreenMarquee=256 ttScreenCenterDown=512 ttInvalidTmType=-1
- ~~货币枚举~~ ✅ 双体系确认：**mt***（mtBindCoin=0, mtBindYuanbao=2, mtYuanbao=3, mtMoneyTypeCount=4）与 **qat***（qatMoney=6, qatBindMoney=5, qatBindYuanBao=7, qatYuanbao=15）——changeCurrency 族用哪套待 Ghidra 确认
- **NumericType_***（Chips/GuildContrib/GuildFund/GodWeaponExp/ShenShouExp/Achieve 等）：**不在 _G.lua 快照**（比导出版本新的 C++ 注册枚举）→ 从 Ghidra 定位 lua_setglobal("NumericType_...") 注册点提取值（脚本已定位模式）
- property 属性 ID 全集（setIntProperty/getIntProperty 参数域）：同上路径提取

## 六、变更流程

新增/修改 API：① registry 加行 → ② 本文档加节（CI 漂移校验）→ ③ 按影响分级跑测试（ONBOARDING 表）。
