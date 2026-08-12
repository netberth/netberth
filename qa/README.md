# NetBerth QA — Devil's Test Harness（隔离模拟环境）

开源的“魔鬼测试”套件。**每个套件各自起一个全新实例**（独立端口 + 独立数据目录），
结果可复现、防爆破/限流状态不会在套件之间泄漏，也永远不会碰到正在运行的演示实例。

| 目录 | 工具 | 覆盖 |
|------|------|------|
| `security/` | curl + shell + python | 认证/吊销/RBAC/JWT 篡改/SQLi/超大请求体/防爆破/全局限流/安全头（44 项） |
| `chaos/` | shell + python + ab | 优雅停机、kill -9、端口冲突、迁移备份、损坏 DB、只读目录、slowloris、SIGSTOP/SIGCONT、突发（14 项） |
| `load/` | k6 | 公共/鉴权/登录压力/用户 churn/WebSocket：QPS、延迟、失败率 |
| `boundary/` | curl + raw TCP | HTTP 边界/模糊：畸形请求行、头部注入、超大头部、chunked/CL+TE、路径穿越、非法 UTF-8、方法滥用、slowloris |
| `smoke/` | curl + python | 正常使用全旅程：登录/刷新/吊销、仪表盘、用户/转发/代理 CRUD、Webhook 端到端投递、WS、登出（22 项） |
| `stress/` | ab + python + shell | 连接洪泛、并发 CRUD、refresh 竞态、WS 洪泛、慢消费者背压、fd=256 极限、goroutine/RSS 增长观测 |
| `sim/` | shell + python + ab | 模拟部署形态：TLS 面板+HSTS、真实反代上游、规则重启持久化、资源受限主机、doctor、多实例共存 |
| `e2e/` | Playwright | 真实 Chromium：登录、页面导航、深链刷新、规则/用户 CRUD（18 项） |
| `soak/` | k6 | 长时间混合负载（默认 10 分钟，`NB_QA_SOAK_SECONDS` 可调）：斜坡/平台/尖峰/排空 + 常驻 WS |

## 用法

```bash
# 一键全跑（security → load → e2e → chaos → boundary → smoke → stress → sim，可选 soak）。
# 未设置 NB_QA_PASS 时自动生成随机管理员密码，脚本内不硬编码任何密码。
./qa/run-all.sh
# 多轮全跑（默认 3 轮）
./qa/rounds.sh

# 单项
./qa/security/security.sh http://127.0.0.1:18444 admin <password>
./qa/chaos/chaos.sh /tmp/netberth-qa 19443
k6 run qa/load/load.js --env BASE=http://127.0.0.1:18445 --env USER=admin --env PASS="$NB_QA_PASS"
./qa/boundary/boundary.sh http://127.0.0.1:18544 admin <password>
./qa/smoke/smoke.sh /tmp/netberth-qa
./qa/stress/stress.sh /tmp/netberth-qa
./qa/sim/sim.sh /tmp/netberth-qa
k6 run qa/soak/soak.js --env BASE=http://127.0.0.1:18545 --env SOAK_SECONDS=600
NB_QA_BASE=http://127.0.0.1:18446 NB_QA_PASS="$NB_QA_PASS" \
  NODE_PATH="$(npm root -g)" playwright test --config qa/e2e/playwright.config.js
```

## 环境变量

- `NB_QA_BIN`：指定现成二进制（默认 `go build ./cmd/netberth` 到 `/tmp/netberth-qa`）
- `NB_QA_USER` / `NB_QA_PASS`：load/e2e 实例会先做首登改密，归一到该密码；`run-all.sh` 未设置时会自动生成随机密码并导出给所有套件
- `NB_QA_SEC_PORT` / `NB_QA_LOAD_PORT` / `NB_QA_E2E_PORT` / `NB_QA_CHAOS_PORT`：默认 18444/18445/18446/19443
- `NB_QA_BND_PORT` / `NB_QA_SOAK_PORT`：默认 18544/18545
- `NB_QA_SOAK=1`：run-all 追加 10 分钟 soak（`NB_QA_SOAK_SECONDS` 可调）
- `NB_QA_KEEP=1`：保留各套件数据目录便于事后排查
- `NB_QA_ROUNDS`：`rounds.sh` 的轮数（默认 3）
- `NB_QA_SOAK=1`：启用长时间 soak（默认关；`NB_QA_SOAK_SECONDS` 控制时长，默认 600 秒）
- 所有套件均不硬编码管理员密码；独立运行时请显式传入或设置 `NB_QA_PASS`
- 压测实例通过 `NB_RATE_LIMIT_RATE=100000 NB_RATE_LIMIT_BURST=200000` 放大全局限流，
  以测量原始吞吐；限流本身由 security/chaos 套件验证

## 2026-08-12 实测结果（负责人最终复跑，全部全绿）

| 套件 | 结果 |
|------|------|
| security | 44/44 PASS，0 WARN |
| chaos | 14/14 PASS |
| k6 load | 56,709 checks 100%、56,670 请求、0 失败、WS 40/40、p95≈42ms |
| e2e | 18/18 PASS（真实 Chromium） |
| boundary | 26/26 PASS（含 slowloris 5.0s 切断、431、CL+TE 拒绝） |
| smoke | 22/22 PASS |
| stress | 15/15 PASS（3k 洪水 0 连接错误、fd=256 存活恢复、goroutine/RSS 无增长） |
| sim | 11/11 PASS（TLS+HSTS+TLS1.3、反代上游、持久化、受限实例） |
| soak | 10min 原始跑：857,206 请求 / p95≈0.7ms / WS 2,408 全通；修复共享 refresh 竞争后的 5min 复跑：442,841 checks / 441,634 请求 / 0 失败 / p95≈0.86ms / WS 1,208 全通 |

所有脚本只对测试实例操作，不影响正式环境；`run-all.sh` 一次跑齐 security→load→e2e→chaos→boundary→smoke→stress→sim。
