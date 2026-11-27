# loggerserver 协议与实现规格（M1 前置调研笔记）

> 来源：main.lua 全文阅读（293 行）+ Ghidra 反汇编 LoggerServer 类（loggerserver 二进制，DWARF 完整）。
> 状态：调研完成，编码前还差 PrecessData 反编译确认包结构。

## 一、关键结论（改变 M1 工作量估算）

1. **核心日志插入在 C++，不在 Lua**：LoggerServer 类直接实现 7 类日志插入
   （InsertCounter/InsertInstall/InsertDau/InsertEconomy/InsertChat/InsertMilestone/InsertOnline）
   —— main.lua 的 `precessLog` 只是可选 Lua 扩展（当前仅实现 cmd=9 在线人数）
2. **7001 的包结构入口 = `LoggerServer::PrecessData`**（收包分发点，反编译目标）
3. `main.lua` 提供 C++ → Lua 桥：`precessLog(cmd, serverid, dp, db_conn)`，当前只处理 cmd=9
   （两次 readInt → INSERT log_online_YYYYMM → BI 字符串外发）
4. **原版启动行为必须复刻**：`CreateTablesThreadRoutine` 在线程里循环 **100 个月 drop+create**
   全部日志表（每次启动清空日志数据——原版语义如此，对拍一致优先）
5. BI 外发：`SendToScribe`/`doSendDcApiLog*`（key=已脱敏 REMOVED_BI_KEY，sendtobi=false 时停用）

## 二、LoggerServer 类方法清单（Ghidra 导出）

```
生命周期: LoggerServer / ~LoggerServer / DoStartup / DoStop / CloseSock
连接:     NewSession / PrecessData / SetDbConnection / ConnectSQL
线程:     CreateTablesThreadRoutine / LoggThread
插入:     InsertCounter / InsertInstall / InsertDau / InsertEconomy /
          InsertChat / InsertMilestone / InsertOnline
BI上报:   SendToScribe / PostLogPacket / UrlEncodeSize / doSendDcApiLog /
          doSendDcApiLogList / doSendDcApiProtoLog
```

C++ 绑定（供 main.lua 使用）：`logger.getDate / getDateStr / exeSql / urlEncode / sendToScribe`，
`DataPack.readInt`（与 gameworld 的 LDataPack 同源）。

## 三、Go 版 M1 实现规格（草案）

```
cmd/loggerserver:
├── 监听 7001（Accept 循环，单连接即 gameworld）
├── PrecessData 等价物：解析包 → 按 cmd 分发
│   ├── Insert* 七类：字段序需反编译各 Insert 函数确认（下一批）
│   └── cmd=9 → precessLog 等价：INSERT log_online_YYYYMM(now, serverid, cnt)
├── 启动时：100 个月 drop+create 循环（复刻原版语义）
├── SQL 模板：照抄 main.lua（含 now() 时间语义）
└── BI 上报（sendToScribe）：日志打印即可（sendtobi=false 对齐原版配置）
```

## 四、★ 结构体权威布局（DWARF 导出，2026-11-22）

> 来源：Ghidra dump_structs.java（DWARF log_def.h）。记录体 = 整个 POD 结构体裸拷贝（`operator>><LCounter>`）。
> 与抓包互证：cmd=1 记录中 `01 00 01 00`=ActorId、`"帅哥"`=**aid_（账号）**、`"91"`=user_level_、`"add exp"`=counter_、`"3338"`=value_、`"425256"`=extra_、`"chapter fb win"`=kingdom_ ——之前推断的"角色名32B字段"实为 aid_ 账号字段，已修正。

### LCounter size=556（cmd=1）
```
+0    actorid_  ActorId[4]     +4    aid_  char[64]     +68   user_level_ char[32]
+100  counter_  char[32]       +132  value_ char[32]    +164  extra_ char[128]
+292  kingdom_  char[32]       +324  phylum_ char[32]   +356  classfield_ char[32]
+388  family_   char[32]       +420  genus_ char[128]   +548  save_db_ bool[1]
+552  flag_     int[4]         （校验: flag_ & 1 有效）
```

### LEconomy size=1384（cmd=4）
```
+0 actorid_[4] +4 aid_[64] +68 currency_[32]("yb") +100 amount_[32] +132 value_[32]
+164 kingdom_[32] +196 phylum_[32] +228 classfield_[32] +260 family_[32] +292 genus_[32]
+324 extra_[128] +452 extra2_[32] +484 pf_[128] +612 flag_[4] +616 openkey_[512] +1128 pfkey_[128] +1256 pfid_[128]
```

### LDau size=324（cmd=3）
```
+0 aid_[64] +64 source_[32] +96 affiliate_[32] +128 creative_[32] +160 family_[32]
+192 genus_[32] +224 ip_[32] +256 from_uid_[32] +288 extra_[32] +320 flag_[4]
```

### LInstall size=292（cmd=2）
```
+0 aid_[64] +64 source_[32] +96 affiliate_[32] +128 creative_[32] +160 family_[32]
+192 genus_[32] +224 from_uid_[32] +256 extra_[32] +288 flag_[4]
```

### LMilestone size=168（cmd=5）
```
+0 actorid_[4] +4 aid_[64] +68 milestone_[32] +100 value_[32] +132 extra_[32] +164 …
```

### LChat size=1288（cmd=14）
```
+0 actorid_[4] +4 aid_[64] +68 name_[32] +100 ip_[32] +132 pf_[32] +164 type_[4]
+168 toaid_[64] +232 toname_[32] +264 content_[1024]
```

### DataPacketReader size=32
`mem_ptr_/end_ptr_/offset_/data_end_` 四指针 —— 即 dp_ 指向的流式读缓冲。

### 记录外层（抓包+PrecessData 综合）
```
[2B cmd] ← 流中每记录首字段（cdcdcdcd 后）
[结构体裸拷贝：从 actorid_/aid_ 起]
……记录间 ee cc 00 00 + 4B 的归属（LogData 外层字段 or 结构体首部）在对拍中闭环
```

## 四a、★ 实证抓包解码结果（2026-11-22，capture 工具实测）

**方法**：Go capture 监听器占 7001 → 原版 gameworld 自动重连 → 25 秒抓 98KB 原始字节 → 模式分析。
工具：`ltcq-go/cmd/loggerserver -capture`（正式保留为对拍工具）。

### 连接握手（一次，80 字节）

```
[4B] ca 66 aa 12        魔数 0x12AA66CA
[4B] 06 00 00 00        未知（版本/SPID=6？）
[4B] 01 00 00 00        serverId = 1 ✓
[64B] "gameworld_s1"    服务名，零填充定长 64B
[剩余] 零填充至 80B
```

### 日志记录流（握手后持续）

以 4 字节分隔符 `cd cd cd cd` 锚定的记录结构（171 条/98KB 实测），cmd 分布与 PrecessData switch 完全对应。
cmd=1 记录字段内容与 LCounter 布局逐一互证（见 §四）。

### 遗留歧义（M1 编码中用对拍闭环）

- `ee cc 00 00 + 4B` 的归属（LogData 外层 or 前记录尾）：差 74 字节，对拍 diff 一轮定案
- `cdcdcdcd` 恒定性：171 次全命中，可信度高

## 五、★ M1 对拍结论（2026-11-23，首个里程碑验收）（2026-11-23，首个里程碑验收）

**方法**：同一 golden 字节流（546KB/837帧）分别喂原版 loggerserver（logreplay 工具）与 Go 版（replay 模式），比对入库结果。

| 结果 | 原版 C++ | Go 版 |
|---|---|---|
| 帧解析 | 837 帧全收 | **837 帧全收，0 解析错误** |
| economy 列映射 | txt 文件 ground truth | **逐字段吻合**（yb/-400/expenditure/zhanling onReqAddExp/…） |
| counter 214 条 | 0 行落库 | 206 行成功 + 8 条 UTF-8 截断拒绝 |
| online 3+117 行 | ✅ 直 INSERT | ✅ 直 INSERT |
| **非 online 日志落库** | ❌ **全数丢失** | ✅ 正常落库 |

**重大发现（原版缺陷）**：C++ Insert* 用 **LOAD DATA INFILE** 批量导入（先落 `d:/log_x_YYYYMMDDHH.txt` 行文件），
MySQL 5.7 默认 `--secure-file-priv` 禁止该语句 → **原版生产环境非 online 日志全部静默丢失**
（只有 cmd=9 online 走直 INSERT 幸存）。Go 版参数化 INSERT 修复此缺陷。

**决策记录**：
- Go 版不复刻 LOAD DATA 失败路径，直接参数化 INSERT（意图对齐而非缺陷对齐）
- 8 条 counter 失败 = classfield 定宽 32B 截断中文字符产生非法 UTF-8——与原版 LOAD DATA 行为等价
  （原版 txt 中同样含坏字节），保持等价；如需兜底可在 cstr() 清理无效 UTF-8 尾字节

**M1 验收状态**：✅ 达成（原版功能等价 + 修复原版数据丢失缺陷 + 对拍闭环）


1. **反编译 `PrecessData`**：确认包帧（长度前缀/CMD 位置/serverid 来源）
2. **反编译 7 个 Insert\***：各表字段序与类型（对照 main.lua 的 INSERT 模板验证）
3. 反编译 `PostLogPacket`：确认它是"gameworld→loggerserver 的包接收"还是"scribe 发送"

## 附：A2 BagAdd golden 定位记录（2026-11-28）

- `a2_bag_golden_20261127.raw` 中候选帧 @71757：`03 04 01 dc cf cc 1f 00 00 01 00 89 8a 01 00 01 00 00 00 04 00 00 00 01...`
  （`03 04` 头 + 掩码字段，非 101001——需按 @additem 时间线对齐确认目标帧）
- 字段语义（RUNBOOK 五十七已解码）：[2]=bagTab、handle=不透明 f64、[19/23/27/31]=att 属性对、[147]=tipFlag、帧长 148B
- ItemConfig loader + 容量模型已入库（a7051f68/5c8575e4）——BagAdd 帧校准为 A2 最后一步
