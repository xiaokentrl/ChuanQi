# gateway↔gameworld 协议（gateway-protocol v0.1）

> 来源：tcpcap 拦截链实测（原版 gateway 9002→tcpcap 6001→gameworld 6002 全链路录制，1516B 逐段解码）。
> 状态：gateway→gameworld 方向 100% 解码；gameworld→gateway 回向在本次抓包中为零（卡点见 §三）。

## 一、连接与注册（80B，复用 srvlib 统一握手）

```
[4B] ca 66 aa 12        魔数（与 loggerserver 握手同款）
[4B] 01 00 00 00        类型=1（gate；loggerserver 版本里此字段=6）
[8B] 0                  idx/保留
[64B] "gateway_s1"      服务名，零填充
```
gameworld 侧日志：`new gate connect,idx=0` + `gateway client (...) registe success (gateway_s1)`。

## 二、帧格式（注册后全部流量）

```
[4B] ee cc 00 00        帧魔数 0xCCEE
[4B] payloadLen         含 2B cmd
[4B] cd cd cd cd        分隔常量
[2B] cmd
[payload-2B]
```

| cmd | payloadLen | 时机 | payload 内容 |
|---|---|---|---|
| 0 | 4 | 每 10 秒 | 心跳（2B 观测 00 00） |
| 1 | 46 | 客户端 WS 连入 | sessionId(2B, 观测 0x228=552) + 8B 零 + 2B(cd cd) + 客户端 IP 串("127.0.0.1\0") |
| 2 | 14 | 客户端断开 | sessionId + 10B |
| 5 | 68 | 客户端登录帧 | sessionId + 10B + **登录明文**：4B+2B+4B(actorid?) + 6B("帅哥" UTF-8) + 2B + 32B(md5 密码 e10adc…) + … |
| 9 | 10 | （在 loggerserver 侧为 online） | — |

**注**：cmd=5 的 payload 里直接看到账号+密码哈希——gateway 把客户端 WS 帧（解掩码后）加 sessionId 前缀重新封装转发。

## 三、卡点：gameworld 无回向数据

- 实测：1516B 全为 gateway→gameworld 单向；客户端登录帧（cmd=5）发出后 gameworld 零响应 → 客户端 20 秒超时
- gameworld 日志显示注册成功（registe success），但登录转发无处理迹象
- gameworld 侧 gate 会话处理类：**GameGate**（boot.log 有 `GameGate::OnDisconnected` 引用）
- 下一步：Ghidra 反编译 gameworld 的 GameGate 类（注册确认流程、cmd=1/2/5 的处理逻辑），确认：
  1. 注册后 gameworld 是否回确认帧（抓包未见——可能注册确认在 gameworld→gateway 的另一连接，或确认是纯内部状态）
  2. cmd=5 转发的登录处理前置条件（gate 会话状态机）

## 四、Go gateway 实现清单（基于本协议）

```
1. 连接 gameworld:6001 → 发送 80B 注册（[12AA66CA][01][0][0][gateway_s1]）
2. 每 10s 发心跳帧（cmd=0）
3. 客户端 WS 接入 → 发 cmd=1 会话建立（sessionId 分配 + 客户端 IP）
4. 客户端 WS 帧 → 解掩码 → 发 cmd=5（sessionId + payload）
5. 客户端断开 → 发 cmd=2
6. 接收 gameworld 回向帧 → 按 sessionId 路由到对应客户端 WS（回向解析待 §三 定案后补）
```

## 五、★ M2 实测结论（2026-11-23）：Go gateway 协议正确，卡点在客户端参数流

### gateway 侧已验证（探针 + 逐字节对照）
- 注册/心跳/cmd=1/cmd=5 帧**与 golden 逐字节同构**（gate_upstream.raw 解析证明）
- gameworld 端实证：registe success ✓ / openuser ✓ / **password ok ✓**（buildUserData 修正后）
- **重要更正**：golden cmd=5 的 `00 00 cd cd` 是**客户端登录数据自带字段**（勿在 gateway 加前缀——双重添加曾致静默失败，已回退）

### 客户端参数流（egret 登录包构造链）
```
login.min.js 开始游戏 → HTTP Login.php（建号验证 ✓ nginx log 实证）
→ window.start(param) → index.js:544 window['paraUrl'] = param
→ main.min.js 读 paraUrl 构造 WS 登录包 → 发送
```
- **实测差异**：当前 egret 登录包缺 `cd cd 00 00 00 00` 字段段（52B vs golden 58B）
- 根因方向：param 由 Serverlist.php 响应构造（srvname/ip/port/url 字段），
  Login.php 响应内容可能参与登录包字段填充——下一步在 login.min.js 中定位登录包构造代码
  （grep login.min.js 的帧组装逻辑），对比 Serverlist 响应字段
- **gateway 侧无问题**：Go gateway 转发与原版逐字节同构已实证

### M2 验收状态
- 协议破解 100% ｜ Go 实现完成 ｜ gameworld 密码验证实证通过
- 待办：客户端参数流修复后（egret 发完整登录包）→ 客户端经 Go gateway 进游戏 = M2 验收

## 六、★ M2 卡点最终破译（2026-11-23）：SysVarMgr 加载死锁

**现象链**：gameworld 启动卡 `StartEngine` 的 `while(!isInitComponentFinished)` 循环（"wait for data"刷屏）→ `started_=false` → gate 的 cmd=5（登录转发）被静默忽略（cmd=1/注册在循环内 ProcessSysMsg 正常处理——openuser/registe success 实证）

**反编译定案（SysVarMgr，initcomp/sysvar_decomp.c）**：
```c
isLoad: return sysVarCount_ == curSysVarCount_;
Load:   sysVarCount_ = -1; curSysVarCount_ = 0;
        → 向 dbserver 发命令 0x209（system_var_mgr.cc:0x19）
        → 等 dbserver 返回 sysvar 表数据（OnDbReturnData 逐条 createSysVar）
```
**卡死条件**：dbserver 对 0x209 的响应未到达 → sysVarCount_ 保持 -1 → isLoad 永远 false。
- actor_s1.sysvar 表存在但 0 行（dump 空结构）
- dbserver boot.log 无 sysvar 相关日志（处理静默）
- 07:42 时代 password ok 曾触发 = 当时 dbserver 对 0x209 有响应（状态完好窗口）

**修复方向**：①反编译 dbserver 的 0x209 处理函数（dbserver 已导入 Ghidra）；②或抓 5001 流量看 0x209 请求/响应；③或 sysvar 表插入种子行测试

**教训**：曾误判 golden cmd=5 的 `00 00 cd cd` 为 gateway 前缀——实际是客户端登录数据自带字段，gateway 双重添加导致额外失败（已回退）

## 七、★ 混合路径实测（2026-11-28 09:19-09:20）

**拓扑（用户重组后）**：探针 → C++ gateway(9002) → **Go gameworld(6001)** → 响应回探针
- 探针登录帧（skey 前缀 + golden 同构数据）→ C++ gateway 转发 → Go gameworld 处理
- **探针收到响应帧 `ff 01 00`（7B）** = 登录响应格式（与 B2 时代成功登录一致）
- gameworld 端 openuser 三次（09:19:15/09:19:57/09:20:43）与探针/重连次数吻合
- **结论：Go gameworld 经 C++ gateway 的登录处理+响应链路实测通过（M2+M4 混合形态）**

**环境注意事项**：
- s1/Debug/gameworld/boot.log 由一个竞争的僵尸 C++ gameworld 实例写入（wait-for-db 卡住、反复重启）——非当前栈成员（当前 Go gameworld=596/6001，C++ 基线=17076/16001），清理时注意区分
- EServer AutoStartAndRestartService 会自动拉起被杀的服务——实验杀进程前先确认 EServer 的自动重启行为
- soak 断点：11-28 00:21:50 为上一段最后采样（机器休眠停滞），判读口径改为累计运行时长

## 八、A2 BagAdd golden 提取方法（下会话开局）

a2_bag_golden_20261127.raw 含 tcpcap 段标记文本（"@time [C] len=N"行），需先剥离元数据重组字节流再搜 CCEE 帧：
perl 剥离法参考 frame_analyze2.pl（正则提取 hex 列还原原始字节），重组后按 [CCEE][len][CDCDCDCD][cmd=3][sub=4][body 144B] 遍历，
目标帧候选 @71757 附近（`03 04 01 dc cf cc...`），对照 RUNBOOK 五十七字段语义（[2]=bagTab/att 对/[147]=tipFlag/148B）校准 buildBagAddItem。

## 九、★ golden BagAdd 帧已锁定（2026-11-28）

WS 帧遍历器（客户端↔gateway 腿，跳过 HTTP 头按 RFC6455 遍历）在 a2_bag_golden S 流
锁定 BagAdd 帧：**@70128, op=2, len=152**（= 4B 会话前缀 `7b 44 80 37` + 148B BagAdd 帧），
payload: `7b448037 | 0304 | 01dccfcc 1f000001 00898a01 00010000 0004...`
同族帧（len 140-162）：@97 InitData、@30847、@32666、@74577（`14 01 01 00 80 00 7c 43 "C:0x16b2ff&T:` 广播）。

**下一步**：以 @70128 帧（152B）为 golden 逐字节对照校准 buildBagAddItem
（对照脚本=本轮 WS 遍历器，a2_s_ws.bin 已存 Temp）。校准后 A2 掉落拾取实现即无协议障碍。

## 十、★ BagAdd 148B 构造点破译（2026-11-28 深夜）

**ItemBag::NotifyAdd(item, showTip) 反编译（equipbag_decomp.c）**：
```c
msg.bag_type = GetType(this);          // [2] bagTab = 容器类型（ItemBag/EquipBag 各异）
Item::WriteData(item, &msg.item);      // [3-146] = Item 结构序列化（148B 主体）
msg.show_tip = showTip;                // [147] tipFlag = AddItem 第三参
Actor::SendData<BagAddItemProto>(...)
```
- **三个遗留差异定案**：①bagTab=容器类型 ✓ Go 已路由 ②att 区域=Item::WriteData 序列化（非 EquipConfig 直映）③tipFlag=showTip 参数
- **最后目标**：反编译 `Item::WriteData` → 精确字段序（Item 结构：handle/strong/level/att 等）→ buildBagAddItem 校准完成
- equipbag_decomp.c（1682 行，EquipBag 18 方法 + ItemBag 全方法）已入库 reference/ghidra/

## 十一、★ ItemData 权威布局（2026-11-28，DWARF item_data.h）

```c
ItemData (144B = 0x90, memcpy 整体拷贝):
+0   int64_t       uid           [8]    ← 不透明句柄（Go: NetId f64 对应）
+8   int32_t       id            [4]    ← itemID
+12  int32_t       count         [4]
+16  ItemAttrData  attr_data     [64]   ← 属性对 ×8（每组 8B = [4B key][4B value]）
+80  ItemAttrData  ex_attr_data  [64]   ← 扩展属性对 ×8
```
- **我的 buildBagAddItem 布局与 C++ 一一对应**（NetId f64↔uid、16 组 att 对↔attr_data+ex_attr_data）
- golden 实证：uid=不透明、id/count 一致、attr_data 非零对 [4,1][2,1]、ex_attr 全零
- **A2 校准剩余 = 字段值填充**：装备物品 AddItem 时按 EquipConfig 填 attr_data 属性对（key 语义待 C++ CreateItem 确认）+ tipFlag=showTip
- **布局本身已 100% 对齐——无结构差异**
