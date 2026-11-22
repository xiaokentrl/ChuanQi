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

## 四、★ 实证抓包解码结果（2026-11-22，capture 工具实测）

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

以 4 字节分隔符 `cd cd cd cd` 锚定的记录结构（171 条/98KB 实测）：

```
[4B] cd cd cd cd            记录分隔符
[2B] cmd                    ← 1=InsertCounter 4=InsertEconomy 9→Lua …（与 PrecessData switch 完全对应）
[2B] seq                    序号（观测恒 01 00）
[4B] 00 00 00 00            pad
[32B] 角色名                UTF-8 零填充定长（"帅哥" 实测 206 次）
[NB] 按 cmd 的字段流        变长（counter/economy 字段可见 "add exp"/"chapter fb win"/"yb" 等内容）
[4B?] 记录间还有 ee cc 00 00 + 变长 4B（疑似上一记录尾的 logid/timestamp 字段）
```

实测样本：cmd=1（counter）记录含 `91`、`add exp`、`3338`、`425256`、`chapter fb win`
——与 main.lua 的 logCounter INSERT 列序（user_level/counter/value/…/kingdom）吻合；
cmd=4（economy）记录含 `yb`（元宝缩写）——logEconomy 的 currency 列。

### 遗留歧义（M1 编码中用对拍闭环）

- 记录间 `ee cc 00 00 + 4B` 的归属（尾字段 or 头字段）：两种解读差 74 字节，对拍 diff 一轮即可定案
- `cdcdcdcd` 是否真的恒定（若为随机 pad 则换用 cmd+seq 锚定）——171 次全部命中，可信度高

## 五、编码前待办（更新）

1. **反编译 `PrecessData`**：确认包帧（长度前缀/CMD 位置/serverid 来源）
2. **反编译 7 个 Insert\***：各表字段序与类型（对照 main.lua 的 INSERT 模板验证）
3. 反编译 `PostLogPacket`：确认它是"gameworld→loggerserver 的包接收"还是"scribe 发送"
