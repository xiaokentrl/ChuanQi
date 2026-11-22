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
