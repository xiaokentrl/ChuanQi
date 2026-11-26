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

### LActor.getActor(entityHandle) → actorHandle | nil
- `worldboss.lua:940` 在首领归属判定中把攻击者实体句柄解析为玩家 Actor；Go 当前玩家句柄即 actorID。
- 仅解析当前在线玩家；空参、非正整数、小数和未命中返回 nil，不查询怪物实体、不创建 Actor。

### LActor.getEntity(entityHandle) → entityHandle | nil / getActorByEt(entityHandle) → actorHandle | nil
- `citysystem.lua:285-290` 先把击杀者句柄解析为实体，再把 AI 目标实体解析为玩家 Actor；`worldboss.lua`、`campbattlefb.lua` 等也使用同一链路。
- Go 当前实体句柄用数字 ID 表示。`getEntity` 仅查询进程共享 `scene.World` 中存在的玩家或怪物，命中返回同一 ID；`getActorByEt` 额外要求实体类型为 Player 且在线 Actor 存在。
- World 在引擎启动时通过原子指针绑定。空参、非正整数、小数、越界整数、未知实体、怪物转 Actor 或未绑定 World 均返回 nil，不创建 Actor。

### LActor.getHandle(entityHandle) → number / getEntityPosition(entityHandle) → x, y / getIntProperty(entityHandle, property) → number
- `postscripttimer.lua:20` 用 `getHandle` 作为实体级 Lua 表键；`test_gathersystem.lua:47` 用 `getEntityPosition` 取采集物坐标；`rabbit.lua:242-246` 用 `getIntProperty` 读怪物坐标。
- Go 实体句柄是正整数数字 ID。`getHandle` 只做句柄身份提取，合法正整数原样返回，空参或非法参数返回 0；它不是存在性查询，不访问 World、不创建 Actor。
- `getEntityPosition` 只读查询进程共享 `scene.World`，命中玩家或怪物时返回实体快照的像素坐标 `(X,Y)`；未命中、非法参数或未绑定 World 返回 `(0,0)`。注册表通过 `FnMulti` 和 `Interpreter.RegisterMulti` 保留 Lua 双返回值。
- `getIntProperty` 只实现 `_G.lua` 属性枚举与现有数据能对应的白名单。World 实体优先返回 `P_ID=0/P_POS_X=1/P_POS_Y=2/P_HP=7/P_MAXHP=10`（`P_ID` 对玩家为 actorID、对怪物为 ConfigID）；在线 Actor 返回 `P_LEVEL=6/P_SEX=55/P_VOCATION=56/P_EXP=57/P_GUILD_ID=70/P_BIND_COIN=62/P_COIN=63/P_BIND_YB=64/P_YB=65/P_FIGHT_VALUE=100/P_RECHARGE=102/P_VIP_LEVEL=103`。
- 怪物没有在线 Actor 时，Actor 专有属性返回 0；未知属性（包括尚未证实语义的 `P_ROOT_EXP=86`）、非法/离线句柄、缺参或未绑定 World 返回 0。查询不创建 Actor、不写数据库。

### LActor.getHp(entityHandle) / getHpMax(entityHandle) / getPosition(entityHandle) → x, y / isDeath(entityHandle) → bool / getId(entityHandle) → number
- `ai_action.lua:88-92` 用 `getHp/getHpMax` 计算怪物血量百分比；`newworldboss.lua:541-542` 用 `getPosition` 原地复活；`togetherhit.lua:121-127` 用 `isDeath` 筛选未死亡角色；`bossinfo.lua:135-146` 与 `guardgodweaponfb.lua:186-202` 用 `getId` 区分实体句柄和怪物模板 ID。
- Go 对玩家和怪物统一使用正整数数字句柄。`getHp/getHpMax/getPosition/isDeath` 优先读取 `scene.World` 实体快照；World 未命中时回退在线 Actor 的 `HP/MaxHP/X/Y`。World 坐标是运行期权威值，Actor 坐标只是未绑定场景实体时的快照回退。
- `isDeath` 按当前 HP `<=0` 判断；`getPosition` 通过多返回值桥接返回 `(x,y)`。`getId` 与 `getIntProperty(P_ID)` 对玩家返回 actorID、对怪物返回 ConfigID，无场景实体时回退在线 Actor.ID。
- 空参、非正整数、小数、NaN/Inf、越界整数或未知句柄分别返回 `0/0/(0,0)/false/0`；查询不创建 Actor、不写数据库。

### LActor.getGamePf(actor) → string
- `utils/actorfunc.lua:103-112` 在原版 Lua 中包装 `LActor.getPf`：仅当字符串以 `union` 开头且包含 `*` 时，返回首个 `*` 后的子串；否则原样返回。
- Go 从在线 Actor 的 `PfID` 只读获取该值。严格校验正整数句柄；空参、非整数、NaN/Inf、越界整数或离线 Actor 返回空字符串。查询不创建 Actor、不写数据库。
- `getEntityType` 继续暂缓：已有 Lua 调用点，但尚未取得 `EntityType_*` 的原版枚举数值，不能直接把 Go 内部实体枚举当作协议常量。

### LActor.getGmLevel(actor) / isImage(actor) / getFubenHandle(actor) / getFubenId(actor) / isInFuben(actor)
- `gmsystem.lua:21` 用 GM 等级拒绝普通玩家执行 GM 命令；`chapter.lua:490` 用 `isImage` 排除镜像 Actor；`citysystem.lua:733`、`miscfunc.lua:202`、`campbattlefb.lua:627` 用副本句柄/ID/状态判断当前实例。
- Go 当前没有 GM 权限、镜像 Actor 或副本实例模型，因此分别固定返回 `0/false/0/0/false`，所有入参（含空参、非法句柄、离线 Actor）均失败关闭。
- `getFubenHandle` 已移除错误的 `stubInt1` 回显 actorID 行为；这些默认值只防止绕过权限或误判副本状态，不代表 GM、镜像或副本能力已经实现。

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

### LActor.getCurrency(actor, type) / getMoneyCount(actor, type) / changeCurrency(actor, type, delta, reason)
- 调用 107/36/69 处。已确认使用 **mt*** 枚举：mtBindCoin=0、mtCoin=1、mtBindYuanbao=2、mtYuanbao=3。
- `getCurrency` 与 `getMoneyCount` 共用在线 Actor 查询实现：`actors.gold/yuanbao` 登录时装入货币表 1/3；绑定货币 0/2 及未知类型当前返回 0。空参、缺类型或离线返回 0，不创建 Actor。
- `changeCurrency` 对金币 `type=1` 和元宝 `type=3` 使用可选 `CurrencyStore` 单字段写入现有 `actors.gold/yuanbao`；Actor 锁内完成严格整数、余额和溢出校验，数据库成功后才更新内存，失败保持原余额。`delta=0` 幂等成功且不写库，离线 Actor、非法参数和缺少存档器均返回错误。绑定币种 `type=0/2` 与其他数值类型暂保持纯内存语义；仍不能充当跨物品、多币种奖励事务的原子入口。
- `changeYuanBao(actor, delta, reason, ...)` 是元宝专用别名，固定走 `type=3` 的同一存档路径；第四个及以后的业务参数不改变金额语义，也不影响余额校验。原版调用见 `actorcost.lua:56`、`campbattlefb.lua:539`、`auctionsystem.lua:587/927`。
- `changeMoney(actor, mtType, delta, 1, true, tips, ...)`（39 处）桥接到 `changeCurrency`：前三个参数同构，第 6 参日志 tip 作为 reason；`phylum/classField/family/genus` 等扩展参数不参与扣款，也不作为余额条件。与已暂缓的 `changeGold` 不同，这里有明确 `mt*` 类型证据。三参短调用同样支持。

### LActor.getGuildId/getGuildName/getChapterLevel/getMonthCard/getReincarnateLv(actor)
- 读取登录快照中的现有 `actors.guildid/guild_name/chapterlevel/monthcard/reincarnate_lv` 字段，不新增表或列。
- 数值查询空参或离线返回 0，字符串查询返回空串；均使用 `ActorManager.Get` 和读锁，不创建 Actor。公会名是登录时快照，不保证反映运行期公会改名。

### LActor.getActorJob/getActorSex(actor)
- `peakracesystem.lua:195-196` 按角色句柄读取职业、性别用于跨服 16 强名单；二者分别是 `getJob/getSex` 的别名。
- 复用在线 Actor 的 `Job/Sex` 登录快照；空参或离线返回 0，不创建 Actor。

### LActor.isActor(handle) / getActorByName(name) / getActorIdByName(name) / getActorByAccountName(account) / getActorIdByAccountName(account)
- `isActor` 仅对当前在线 Actor 返回 true；空参、0、离线句柄均为 false。
- 两个按角色名查询仅搜索在线 Actor：句柄查询命中返回 actorId，未命中、空名、非字符串或重名返回 nil；ID 查询对应返回 0。
- 两个按账号查询仅搜索在线 Actor 的登录账号快照：句柄查询未命中、空账号、非字符串或账号重名返回 nil；ID 查询对应返回 0。账号名精确匹配，不查离线数据库。
- 重名失败关闭，不依赖 map 遍历顺序；查询不创建离线 Actor。

### LActor.getRoleCount(actor) → n / getRole(actor, index) → roleHandle
- 调用 120/75 处。**合击多角色系统**：index 0 为主角色，1+ 为副角色（开启角色功能页）。
- Go 当前只有主角色模型，没有 `player.Roles[]`：在线 Actor 的 `getRoleCount` 返回 1，`getRole(actor,0)` 返回同一 actorID，其他索引返回 nil。空参、非整数索引或离线 Actor 不创建实体。
- `togetherhit.lua:123-127` 与 `actorrole.lua:75-79` 均按 `0..count-1` 遍历角色；副角色属性和装备 API 仍未实现，不能把计数 1 当作多角色能力已接通。

### LActor.getRoleSkillLevel(actor, roleid, index) → level
- `skill.lua:14-24` 按主角色 `roleid=0`、技能槽 `index=0..4` 读取当前等级，用于升级前校验；Go 用 `Actor.Job` 计算 `job*10000+(index+1)*1000+1`，读取 `Actor.Skills` 中对应基础技能等级。
- 仅支持当前单主角色模型。空参、非法整数、其他 roleid、越界槽位、离线 Actor 或职业不在 1..3 时返回 0；未学习槽位同样为 0，不创建 Actor。

### LActor.GetRoleByJob(actor, job) → roleHandle | nil
- `zhanlingsystem.lua:318-323` 在战灵皮肤被动技能中按职业取得角色句柄；Go 仅支持职业 1..3，当前在线主角色职业匹配时返回同一 actorID。
- 职业不匹配、空参、非法/离线 Actor 返回 nil，不创建 Actor；副角色查询仍不支持。

### LActor.reCalcAttr(actor, force?)
- 调用 46 处。属性重算收口：遍历各系统加成（Lua 提供 get*Attr 族）→ C++ 合成 → setIntProperty。
- Go：pipeline 重算（各系统注册的加成提供器按序取值）。force=false 时增量重算。

### LActor.setIntProperty(actor, property, value)
- 调用 66 处；property 枚举全集 `_G.lua:1011-1124` 已确认，A5-13 已先接通同白名单的只读查询。
- Go 仅写已证实且 Actor 有对应字段的白名单：等级、性别、职业、经验、公会 ID、战力、累计充值；金币/元宝/绑定币按绝对数量写入内存，其中金币/元宝先经现有 `actors.gold/yuanbao` 存档器落库，VIP 复用 `setVipLevel` 的原子存档链。
- `P_ID`、坐标、`P_HP/P_MAXHP` 读侧以 World 实体为准，单写 Actor 会造成状态不一致，继续失败关闭；未证实属性、非法/离线句柄、非整数参数、负数金额/等级等同样不修改任何状态、不抛 Lua 错误。
- 内存字段写入只标 `Actor.Dirty`，不声称已下发协议或接入 `reCalcAttr` 属性管线；`setExp/setLevel` 仍需完整经验链，不能用本 API 代替。

### LActor.setCamp(actor, camp) / getCamp(actor)
- 调用 51/11 处。实参来源包括 actorID、guildID、`CampType_Player`、`WorldBossCampType_*`、副本分配值（100/200）等；`getCamp` 用于阵营比较、排行榜暂存与副本发包数据。
- `CampType_*` 全局枚举数值在资产中只见引用未见定义，已确认的只有文件内 local 表（`leadfuben.lua:3-7` 为 0/1/2）与 `crosswujifbmgr.lua:4-7` 的 100/200，因此 Go 不按 0..2 范围校验，只接受整数。
- Go 在 `Actor` 上新增进程内 `Camp` 标签，`setCamp` 写在线 Actor，`getCamp` 读在线 Actor；离线、非法句柄、非整数参数失败关闭，`getCamp` 返回 0。
- 不落库、不标脏、不发协议、不参与 `CombatManager` 敌对判定；怪物/机器人句柄、`getCampId`、`Fuben.setMonsterCamp` 均不在此 API 覆盖范围内。

### LActor.stopAI(actor) / setAITarget(source, target) / getAITarget(entity) / setAIAttackMonster(actor) / setAITargetNull(entity)
- `actorrole.lua:89-110` 用目标句柄为 0 的切换请求恢复自动打怪，用 `stopAI` 停止 AI；`citysystem.lua:282-293` 读取击杀者当前目标，`citysystem.lua:455` 调 `setAITargetNull` 清目标；`ai_action.lua:108-118` 与 `worldboss.lua:946` 允许怪物显式指定玩家目标。
- Go 在进程共享 `CombatManager` 中保存玩家停战标记、玩家→怪物显式目标与怪物→玩家仇恨目标。`installCombatAPI` 为每个 Lua VM 注册捕获各自 Combat 的闭包，运行时不再依赖包级全局 Combat。
- `stopAI` 仅停止当前在线、存活且在场景中的玩家自动战斗，保留已有目标，不等于无敌或清除目标；`setAITarget` 会重新启用玩家自动战斗。显式目标存活且同场景时优先于最近怪；超出攻击范围时不回退攻击其他怪。
- `setAITarget` 当前仅支持玩家→怪物、怪物→玩家。来源或目标不存在、死亡、跨场景、PvP、怪物→怪物、镜像、副本及副角色均失败关闭，不创建 Actor、不写数据库、不下发新协议。
- `getAITarget` 返回共享状态中的有效目标句柄；无目标、非法输入或目标死亡/离场/跨场景时返回 `nil` 并清理失效引用。`setAIAttackMonster` 清除玩家显式目标、解除停战并恢复现有最近怪选择。`setAITargetNull` 只清除玩家显式目标或怪物仇恨，不解除 `stopAI`，也不等同于恢复自动索敌。
- 玩家断线时清理其停战、显式目标与复活状态，并清除怪物对该玩家的仇恨；登录事件失败回滚时同样清理 AI 后移除 World 实体。旧连接离场清理与同号 `BindActor` 由路由生命周期锁串行化；索敌每次重新读取 World 中的 HP 与场景，避免同 Tick 旧快照指向已死亡或已离场目标。
- `setAIPassivity` 虽已确认 `true=被动、false=主动`，但与 `stopAI` 的交互、配套查询和多角色状态依据不足，继续暂缓。

### LActor.enterFuBen(actor, fbId, ...) / exitFuben(actor) / getFubenHandle
- 调用 43/45/54 处。副本进出（instance 系统对接点）。
- Go：实例管理器；Lua 侧生命周期回调经 instanceevent。

### LActor.sendTipmsg(actor, msg, typeMask?)
- 调用 310 处。typeMask=ttScreenCenter/ttTipmsgWindow 等位或（枚举见 _G.lua）。
- Go：组包 CMD_Notice 系列子命令，写 msg + type。

### LActor.getItemCount(actor, itemId) → n / addItem / giveItem / canAddItem
- 背包查询/发放族。giveItem(42) 直接发放；addItem(31) 带位置语义；getEquipBagSpace(12) 装备包空间。

### LActor.log(actor, ...)
- 调用 91 处。玩家级业务日志，原版最终走 loggerserver 通道。
- Go 当前实现本地 `slog.Info("LActor.log")`，字段为严格正整数 `actorID` 与字符串化 `args`；非法句柄按 `actorID=0` 记录，离线正整数按原 ID 记录。日志不查 Actor、不创建实体、不修改业务状态，也不再静默丢弃。
- 当前未接 7001 协议、未写日志库；Lua 表参数仅按宿主默认字符串形式记录，不声称已实现完整序列化。

### LDataPack.readInt64(12 处) / readData(26 处)
- `readInt64` 与 `writeInt64` 对称，按小端读取 8 字节并返回 64 位整数，避免 32 位截断；越界返回错误。
- `readData(pack, n, dt1, dt2, ...)` 按 `_G.lua:407-419` 的 `dt*` 常量返回 n 个值，类型覆盖 byte/char/word/short/int/uint/int64/uint64/string；与 `writeData` 变参布局对称。
- 字段数不足、未知 dt 类型或读取越界一律返回错误，不推进游标到不可知位置；不发送协议、不修改业务状态。

### LDataPack.test_allocPack(72 处) 【待确认】
- 名称带 test 前缀但调用 72 次——疑似调试期遗留的高频用法，Ghidra 确认与 allocPacket 的差异后再定实现策略。

### 待提取项（依赖 _G.lua 快照/Ghidra）
- ~~tt* 提示类型位掩码全集~~ ✅ 已确认（_G.lua）：ttTipmsgWindow=1 ttScreenCenter=2 ttDialog=4 ttHearsay=8 ttMessage=16 ttWarmTip=32 ttGmTip=64 ttChatWindow=128 ttScreenMarquee=256 ttScreenCenterDown=512 ttInvalidTmType=-1
- ~~货币枚举~~ ✅ 双体系确认：**mt***（mtBindCoin=0, mtBindYuanbao=2, mtYuanbao=3, mtMoneyTypeCount=4）与 **qat***（qatMoney=6, qatBindMoney=5, qatBindYuanBao=7, qatYuanbao=15）；`changeCurrency` 族已按原版 Lua 调用确认使用 **mt***。
- **NumericType_***（Chips/GuildContrib/GuildFund/GodWeaponExp/ShenShouExp/Achieve 等）：**不在 _G.lua 快照**（比导出版本新的 C++ 注册枚举）→ 从 Ghidra 定位 lua_setglobal("NumericType_...") 注册点提取值（脚本已定位模式）
- ~~property 属性 ID 全集~~ ✅ `_G.lua:1011-1124` 已确认；A5-13 仅实现与 World/Actor 现有字段对应的白名单，其余属性继续暂缓。

### System 时间与节拍依赖（已实现）

| API | Go 语义 |
|---|---|
| `System.getNowTime()` | 返回 Unix 秒，供 supertimer/engineevent 比较触发时间 |
| `System.timeDecode(timestamp)` | 按本地时区返回 `year, month, day, hour, minute, second`；空参或非法时间返回六个 `0` |
| `System.isSameDay(a, b)` | 比较两个 Unix 时间戳的本地年份与年中日；参数不足或非法返回 `false` |
| `System.getStaticVar()` | 返回当前 Lua VM 安装时创建的唯一表；重复调用共享引用，仅进程内状态，不声称持久化 |

`timeDecode` 与 `isSameDay` 复用严格整数校验，不截断小数或字符串数字。`getStaticVar` 不同于按 Actor 存储的 `LActor.getStaticVar`。

## 六、变更流程

新增/修改 API：① registry 加行 → ② 本文档加节（CI 漂移校验）→ ③ 按影响分级跑测试（ONBOARDING 表）。
