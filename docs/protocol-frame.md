# 协议帧格式（protocol-frame v0.1）

> 状态：WS 层与 C++ 数据包类**已确认**；端到端长度前缀为**高置信推断**，待抓包终验。
> 证据来源：gateway 二进制字符串、Ghidra 函数地图（DataPacket 命名空间 53 函数）、Lua 侧调用模式。

## 一、WS 层（gateway ↔ egret 客户端）

**标准 RFC6455 WebSocket**，证据（gateway 二进制内嵌字符串）：
- 握手响应模板：`HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: %s\r\n\r\n`
- 类：`WebSocketHead`（GetLen/GetLenNeedByte/reset）、`WebSocketOpcode`
- 结论：gateway 完整实现 RFC6455 服务端（掩码解码/帧组装），**Go 侧用 gobwas/ws 对齐**（ADR-001）

## 二、业务包内层结构（WS payload / 进程间共用）

### 已确认（证据充分）

| 项 | 格式 | 证据 |
|---|---|---|
| CMD + 子命令 | 各 1 字节（0-255） | netmsgdispatcher.lua:10 `sysid/cmd 上限256`；csmsgdispatcher "包头为两个 byte"；LDataPack.allocPacket(actor, CMD, subcmd) |
| 字符串 | **2 字节无符号长度前缀 + 字节流** | C++ `DataPacket::rawWriteStringLen<unsigned short>` |
| 整数族 | int=4B / short=2B / char=1B / int64=8B / uint 同宽 / double=8B | C++ `operator<<<int/short_int/char/long_long_int/double>`、`writeAtom<T>` |
| 结构体直写 | ItemData/EntityHandle/NetId/CrossUserData 等类型化 operator<< | C++ 模板实例清单 |
| 字节序 | **小端**（x86-64 原生，无转换函数痕迹） | 架构推断，抓包终验 |

### 高置信推断（待抓包终验 ⚠）

| 项 | 推断 | 验证方法 |
|---|---|---|
| 长度前缀 | 包头 `2 字节或 4 字节总长` 位于 CMD 前 | DevTools WS 面板抓帧；或 gateway `WebSocketHead::GetLen`/`GetLenNeedByte` 反编译 |
| 压缩/加密 | 疑似无（无特征字符串） | 同上 |

### 抓包终验步骤（M2 前完成）

1. 浏览器 DevTools → Network → WS → 选游戏连接 → Messages 页导出二进制帧
2. 对照登录回包（CMD_Base/ServerOpenDay，actorlogin.lua:7-16 已知字段序：4×int + 1×byte）
3. 核对长度字段宽度与 CMD 位置 → 回填本表

## 三、进程间协议

| 链路 | 形态 | 证据 |
|---|---|---|
| gateway ↔ gameworld | 与客户端同构转发（透明） | gateway 定位为转发器；对拍零差异即可证 |
| gameworld ↔ dbserver | DbCmd 结构化命令（45 条，非明文 SQL） | dbprotocol.lua 全量 |
| gameworld ↔ loggerserver | 日志事件包 | loggerserver/data/main.lua 消费端 |
| 跨服 | CrossSrvCmd(11)+SubCmd(~60)，**首版 stub**（ADR-005） | csproto.lua |

## 四、协议常量（权威引用）

- CMD 全表：`functions/protocol.lua`（84 CMD + 983 子命令，c/s 前缀分向）
- DB 命令：`functions/dbprotocol.lua`（45 条）
- 提示类型位掩码：engine-api.md §五（tt* 11 值已确认）
- 货币枚举：mt*/qat* 双体系已确认；NumericType_* 待 Ghidra 注册点提取

## 五、登录→进场景链路（2026-11-24 实现，11-25 真实客户端实测终验通过）

### 已确认常量（protocol.lua + _G.lua，单一事实源）

| 符号 | 值 | 出处 |
|---|---|---|
| `CMD_Base` | 0 | protocol.lua:10 |
| `CMD_Login` | 255 | protocol.lua:96 |
| `sBaseCmd_ActorBaseData` | 1 | protocol.lua:103 |
| `sBaseCmd_RoleData` | 2 | protocol.lua:104 |
| `sBaseCmd_EnterScene` | 3 | protocol.lua:105 |
| `sBaseCmd_SyncTime` | 14 | protocol.lua:116 |
| `sBaseCmd_ServerOpenDay` | 23 | protocol.lua:126 |
| `aeUserLogin` | 13 | _G.lua:677 |
| `mtCoin`（金钱/gold） | 1 | _G.lua:174 |
| `mtYuanbao`（元宝） | 3 | _G.lua:171 |

### 字符串编码（两种，勿混用）

- **C++ `rawWriteStringLen<unsigned short>`**：2B 无符号长度前缀 + 字节流（无 NUL）——`ActorBaseData` 的 `actorname` 等 C++ 引擎组包字段用此。
- **Lua `LDataPack.writeString`**：裸字节 + NUL 结尾（无长度前缀）——登录账号、Lua 业务包用此。

### 登录成功后下行包顺序（Go `postlogin.go` 实现序）

```
登录回包 [ff][01][00]（login.go，先发）
  → ActorBaseData [00][01][body] / RoleData [00][02] / EnterScene [00][03] / OtherEntity [00][04]
  → 背包初始化 [03][01]×3 / ClientConfig [00][13]（Go 组包）
  → OnActorEvent(actor, aeUserLogin=13)（Lua 事件链，actorlogin.lua 发 ServerOpenDay 等）
```

> ✅ **golden 终验结论（2026-11-25）**：早期 `g2c` 抓包为空导致布局只能推断；11-24~25 通过真实客户端实测完成终验——
> `ActorBaseData`/`RoleData`/`EnterScene` 布局被客户端正确消费并成功进入场景；`RoleData` 尾部法阵段 436B（6 子阵×4 特质×5 潜能，见 `demoncircle.lua:1160`）修复 `fzRedShow` 弹框；`ClientConfig`(19) 补齐。
> 证据见 RUNBOOK §八/§十七/§十八。旧的"骨架推断待终验"表述已作废。

## 六、战斗奖励与怪物重生（2026-11-25 真实浏览器验证）

| 命令 | 负载（不含 `skey/sysid/cmdid`） | 语义 |
|---|---|---|
| `[00][05]` | `moneyType:i16, absoluteGold:u64` | 货币绝对量；普通金币类型为 1，不是增量 |
| `[00][07]` | `level:i32, exp:i32, addedExp:i32` | 当前等级、当前经验、本次增加经验 |
| `[00][04]` | 完整 `OtherEntity` | 怪物重生复用原句柄并重发满血实体 |
| `[00][10]` | `handle:f64` | 死亡后实体消失 |

致死攻击按 HP=0 → `[00][20]` 伤害 → `[00][10]` 消失 → `[00][05]` 金币 → `[00][07]` 经验广播。奖励只发送给击杀者；存档使用 level/exp/gold 绝对快照，失败重试不重复累加。证据见 `ltcq-go/reference/captures/browser_reward_20261125/reward_browser_proof.json`。

## 七、怪物反击与玩家 HP（2026-11-25 真实浏览器验证）

| 命令 | 负载（不含 `skey/sysid/cmdid`） | 语义 |
| --- | --- | --- |
| `[00][09]` HP 变化 | `handle:f64, currentHP:f64` | 同时用于玩家与怪物；当前 HP 最低为 0 |
| `[00][20]` 飘字 | `attacker:f64, target:f64, damage:f64, damageType:u32` | 怪物反击复用玩家伤害包，`damageType=0` |

怪物被成功攻击后记录玩家仇恨，只对仍在同场景、攻击范围内且存活的当前目标反击。每次反击先向受害者显式发送 `[00][09]`，再发送 `[00][20]`；其他可见玩家接收相同战斗广播。开发示例伤害为 10、范围为 128、间隔为 2 秒，真实浏览器实收玩家 HP `90 → 80 → 70 → 60 → 50`。玩家 HP 归零后仍保留实体，死亡/复活不在本阶段范围内。证据见 `ltcq-go/reference/captures/browser_retaliation_20261125/retaliation_browser_proof.json`。

## 八、玩家死亡与安全复活（2026-11-25 真实浏览器验证）

| 命令 | 负载（不含 `skey/sysid/cmdid`） | 语义 |
|---|---|---|
| `[00][09]` HP 变化 | `handle:f64, currentHP:f64` | 致死反击先发送当前 HP 0 |
| `[00][20]` 飘字 | `attacker:f64, target:f64, damage:f64, damageType:u32` | 致死伤害随后广播 |
| `[00][10]` 实体消失 | `handle:f64` | 死亡包序最后一项，AOI 隐藏死亡玩家 |
| `[00][04]` | 完整 `OtherEntity` | 复活复用玩家原句柄和原坐标，重发满 HP 实体 |

致死包序固定为 `[00][09] HP=0` → `[00][20]` → `[00][10]`。死亡玩家不发送移动位置同步，不接受移动或继续受击；其怪物仇恨同时清除。开发默认复活延迟为 3 秒（`CombatConfig.ReviveAfter`），到期后玩家仍在原场景、原坐标，以 `HP/MaxHP=100/100` 重发实体并恢复战斗。复活不新增客户端命令、数据库字段或死亡弹窗协议。证据见 `ltcq-go/reference/captures/browser_death_20261125/death_browser_proof.json`。

## 九、怪物主动移动与 AOI 重入（2026-11-25 真实浏览器验证）

| 命令 | 负载（不含 `skey/sysid/cmdid`） | 语义 |
|---|---|---|
| `[00][0d]` 位置同步 | `handle:f64, mode:u16, x:u32, y:u32, duration:u32` | 怪物追击/归位复用玩家移动协议；64 像素步长使用 `mode=1,duration=16` |
| `[00][04]` | 完整 `OtherEntity` | 怪物离开后重新进入玩家 AOI 时重发实体 |
| `[00][10]` 实体消失 | `handle:f64` | 怪物离开玩家 AOI 时发送；离开后不再向该玩家广播其坐标 |

主动 AI 由服务端 World 权威更新怪物坐标。索敌、追击、攻击和归位都不新增客户端命令；只有 AOI 内可见玩家接收位置同步。真实浏览器实收怪物从 `x=1024` 越过 leash 后归位，并在玩家回到可观察距离后通过 `[00][04]` 重入确认出生点 `(448,384)`。归位状态完成前不会因目标仍处于索敌半径而重新锁定。证据见 `ltcq-go/reference/captures/browser_monster_ai_20261125/monster_ai_browser_proof.json`。

## 十、最小技能树与单目标释放（2026-11-26）

| 命令 | 负载（不含 `skey/sysid/cmdid`） | 语义 |
|---|---|---|
| `[02][01]` 上行 | `attackerHandle:f64, targetHandle:f64, skillConfigID:i32` | 释放技能；Go 仅接受已绑定玩家攻击同场景存活怪物 |
| `[02][01]` 下行 | `attackerHandle:f64, skillConfigID:i32, targetHandle:f64, damageCount:u16, appliedDamage:f64 × count` | 最小单目标伤害结果，count 固定 1，共 32B |
| `[02][04]` 上行 | `roleIndex:u16, skillIndex:u16` | 单技能激活/升级；当前只支持主角色 roleIndex=0 |
| `[02][04]` 下行 | `roleIndex:u16, skillIndex:u16, level:u16` | 更新一个技能槽等级 |
| `[02][05]` 上行 | `mode:u16, roleIndex:u16, selectedIndex:u16` | mode=0 升级选中已学习技能，mode=1 升级全部已学习技能 |
| `[02][05]` 下行 | `roleIndex:u16, level:u16 × 5` | 一键升级后的五槽等级 |

`RoleData [00][02]` 的 `power:f64` 后是 **5×int32 技能等级 + 5×int32 后置字段**，不是技能配置 ID。客户端 `Role.parser` 对每槽执行 `SkillData.getSkillByJob(job, slot, level)`；配置 ID 由 `job×10000 + slot×1000 + level` 得出。Go 持久化使用基础 ID `job×10000 + slot×1000 + 1 → level`，登录时只写等级。该布局已由真实浏览器 SubRoles 解析结果校正。

真实浏览器实收完整 WS 帧长度：`[02][01]` 为 36B（4B skey + 2B 命令 + 30B 负载），`[02][04]` 为 12B，`[02][05]` 为 18B；`RoleData [00][02]` 为 4113B。证据见 `ltcq-go/reference/captures/browser_skill_20261126/skill_mutate_browser_proof.json`。
