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
