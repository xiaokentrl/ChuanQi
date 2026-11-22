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

## 四、编码前待办

1. **反编译 `PrecessData`**：确认包帧（长度前缀/CMD 位置/serverid 来源）
2. **反编译 7 个 Insert\***：各表字段序与类型（对照 main.lua 的 INSERT 模板验证）
3. 反编译 `PostLogPacket`：确认它是"gameworld→loggerserver 的包接收"还是"scribe 发送"
