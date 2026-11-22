# 运维手册（RUNBOOK）

## 一、启动顺序（与原版 run.sh 一致，顺序不可乱）

```
dbserver → gameworld → gateway → loggerserver
（gameworld 启动后等 "load global function!!!!" 与 "all game config load complete" 日志）
```

- C++ 版：`data/server/s1/Debug/<svc>/<svc>.exe`（Linux 用 `s1/run.sh`；Linux 部署记得 chmod +x——历史事故：nohup.out 记录过"权限不够"）
- Go 版：`ltcq-go` 构建产物，配置沿用同名 .txt
- 停止：先 gateway（断新连接）→ gameworld（存盘）→ dbserver/loggerserver

## 二、端口表

| 端口 | 服务 | 说明 |
|---|---|---|
| 9001 | gateway | 客户端 WS（MaxSession 8192） |
| 6001 | gameworld | 网关后端 |
| 5001 | dbserver | 存取 |
| 7001 | loggerserver | 日志 |
| 10001 | gameworld 内部 HTTP | 管理口（127.0.0.1） |
| 3306 | MySQL | 三个库：actor_s1/globaldata/log_s1（+center） |
| 80 | nginx | PHP 门户（EServer） |

## 三、健康检查（Go 版）

每个 Go 服务暴露 `http://127.0.0.1:<port+10000>/healthz`：返回进程名/版本/在线数/最后心跳。
`/metrics`：Prometheus 格式（在线数、包 p99、Lua VM 内存、DB 连接池、错误率）。
告警基线：单区在线 >70% 容量、包 p99 >10ms、错误率突增。

## 四、备份与恢复

```
备份：每日 mysqldump --databases actor_s1 globaldata center log_s1 | 保留 7 天
     + binlog 实时（恢复点 = 最近 dump + binlog 重放）
恢复：停服 → 导入最近 dump → 重放 binlog 至目标时刻 → 起服
Go 版数据格式变更：走 migrations/NNN_*.sql 顺序执行
```

## 五、常见故障表（本工程已实际踩过）

| 现象 | 原因 | 处置 |
|---|---|---|
| 客户端无登录框 | login.min.js 里 `window.loginType` 被硬编码 1（渠道SDK模式） | 确保为 0（已修复）；浏览器 Ctrl+F5 清缓存（js 有 1h 缓存头） |
| nginx reload 报 Access denied | nginx 以管理员启动，普通权限发不出信号 | 统一用普通权限启动；`nginx.exe -t && nginx.exe -s reload` |
| gameworld 卡 "wait for db client" | dbserver 未先启动 | 按 §一顺序重启 |
| gameworld 报 envir/quest 缺失 | 旧版路径死引用（已确认死代码区），引擎容错跳过 | 无需处理；不要去补造旧目录 |
| 启动大量 ItemConfig/EquipConfig ERR | 配置校验警告，引擎容错 | 正常现象，对照原版基线日志 |
| loggerserver "connect to db server succ" 但表不存在 | log_s1 未建表 | 从 loggerserver/data/main.lua 提取建表 SQL（提取脚本见会话记录，占位符 %d%02d→年月） |
| git push 卡死 | GitHub 直连不稳 | 用 SSH remote；分批提交 |
| Python 假命令（exit 49） | Microsoft Store 占位符 | 用 `D:\tools\python\python.exe` |

## 六、发布与回滚

- 版本兼容矩阵见 compatibility.md；发布前跑对应列回归
- 金丝雀：新区先上新版本 → 稳定后全区滚动
- 回滚 = 回退二进制版本 + 数据按 §四恢复到发布前 dump
