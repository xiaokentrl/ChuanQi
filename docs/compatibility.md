# 版本兼容矩阵（compatibility）

> 原则：客户端 × Lua 脚本 × 服务端 三方版本必须落在同列；升级任一方先在新区金丝雀验证。

## 当前基线（来自 s1/version.txt，2020-06 开发版快照）

| 组件 | 版本 |
|---|---|
| egret 客户端（main.min.js 编译产物） | 2021 线上构建（不可重建，源码不在包内） |
| Lua 脚本 | 5165 |
| Config | 5164 |
| gameworld (C++) | 5168 |
| dbserver / gateway / loggerserver (C++) | 4510 / 4478 / 4478 |
| PHP 门户 | 2021 线上版（loginType 修复 + localhost 化为本仓修改） |
| Go 服务 | 未发布（M1 起） |

## 兼容规则

| 变更 | 兼容性 | 要求 |
|---|---|---|
| Lua 脚本小改（文案/数值） | ✅ 任意组合 | 无 |
| Lua 脚本新增协议子命令 | ✅ 需服务端支持该 CMD 路由 | netmsgdispatcher 注册后服务端 regScriptNetMsg 生效 |
| Go 服务升级（字节层不变） | ✅ 与 C++ 版互换 | golden 回归通过 |
| Go 服务帧格式变更 | ❌ 跨版本禁用 | 全量对拍 + 客户端无需重发（帧不变是红线） |
| 客户端 main.min.js 更换 | ⚠️ 黑盒 | 见 client-boundary.md，需全量回归 |

## 发布记录

| 日期 | 组件 | 版本 | 验证 |
|---|---|---|---|
| 2026-11-22 | 门户 | localhost 化 + loginType 修复 | 全链路实测通过 |
