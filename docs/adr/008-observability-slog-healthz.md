# ADR-008：可观测性 = slog + /healthz + /metrics（不做全链路追踪）

状态：已采纳 ｜ 日期：2026-11-22

## 背景
原版零监控（只有本地日志与 memory.txt 手工快照）。Go 版需要基本运维可见性。候选：zap+自研、OpenTelemetry 全家桶、stdlib slog+Prometheus。

## 决策
- 日志：`log/slog` 结构化 JSON（对齐原版 [ERR]/[TIP] 语义分级）
- 每服务 `/healthz`（进程名/版本/在线数/最后心跳）+ `/metrics`（Prometheus 文本）
- 指标集：在线数、包 p99、Lua 调用时延/超时数、DB 连接池、错误率
- 不做 OpenTelemetry 分布式追踪——单区几千人规模，四进程日志关联即可

## 理由
1. stdlib 零依赖，符合 ADR-001 依赖预算
2. 指标选择对齐容量告警基线（70% 水位、p99>10ms）
3. 原版运维者可按熟悉的关键字过渡

## 后果
+ 运维可见性从零到够用
- 日志量需控制（slog 分级 + gameworld 高频路径采样）
