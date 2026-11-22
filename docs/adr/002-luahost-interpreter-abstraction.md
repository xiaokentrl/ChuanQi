# ADR-002：Lua 宿主经 Interpreter 接口隔离，gopher-lua 先行

状态：已采纳 ｜ 日期：2026-11-22

## 背景
Lua 业务层重度依赖 5.1 语义（module()/package.seeall/setfenv）。候选：gopher-lua（纯 Go）、LuaJIT cgo 绑定、Lua 5.4+compat shim。

## 决策
`internal/luahost` 定义 `Interpreter` 接口（Load/Call/Register/PCall/SetTimeout），首个实现 gopher-lua。性能不足时按 ROADMAP §10 触发条件切换 LuaJIT 实现，上层零改动。

## 理由
1. 纯 Go：部署零 cgo、调试方便
2. 接口隔离把"换宿主"从架构手术降级为新增实现类
3. 第 13 周 M4 冒烟（726 文件加载）提前暴露兼容性风险

## 后果
+ 可升级性预留完成
- gopher-lua 性能约为 C-Lua 的 1/10~1/30——容量靠 ADR-004 热路径分流兜底
- 接口需覆盖 Lua 调用超时看门狗（原版缺失的健壮性升级点）
