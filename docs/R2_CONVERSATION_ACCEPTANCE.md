# R2-01 Conversation → ResearchRun 验收（人工 opt-in）

## 当前固定证据

- #346 固定 head `2699b2ae767169d2e080fc3d177e489e395c0056`：Go CI #38717246469、Web CI #38717246458 成功；merge `e03cd00d15bfc10996533785f8feb5c0f4c297fc`。
- #347 固定 head `1c0968d3468aef385667db72da21fe2ad63ecff9`：Go CI #38835298685 成功；merge `11a33ce8d2d535fa05472208fe7aecfe173e5c1d`；main CI #38837045112 成功。
- #343 Eino tool loop、#344 真实 RunManager/FileRunStore、#345 未授权请求无副作用均有自动化覆盖。
- 以上**不代表**已完成对话入口的真实 Hosted Provider E2E，也不代表 CI Browser fixture 产生了真实研究成果。

## 前置条件及费用边界

本检查仅在操作人员**明确批准一次付费真实研究**后执行；不要在普通 CI 或文档校验中自动运行。使用回环接口和现有开发配置，参考 [LIVE_RESEARCH_ACCEPTANCE.md](LIVE_RESEARCH_ACCEPTANCE.md)；不要提交凭证、研究敏感产物或运行日志明文。建议将并发限制为一，先核实第三方配额，再提交一次请求，失败不自动重试；不涉及任何真实交易。

1. 启动 `go run ./cmd/aegis-web -dev -portfolio-review=false`，再执行 `cd web && npm run dev:live`，使用本地浏览器打开工作台。
2. 设置并确认真实 Provider 的服务端凭证有效，登录工作台；在「研究对话」输入 `请研究 600519.SH`，填入 `600519.SH`，显式勾选确认后**仅点击一次**发送。留空股票代码的普通问答不会启动研究。
3. 记录 HTTP `POST /api/workbench/conversation` 的成功响应中的 `run_id`、`symbol`、`state`，应为真正生成的 32 位十六进制 Run ID、`600519.SH` 和初始状态。只记录非敏感字段与请求时间，不记录密钥或 Authorization。
4. 使用既有工作台 Run 列表及 `GET /api/v1/research/runs/{run_id}` 检查同一 ID 的状态；必要时查看已持久化事件 / 只读 Artifact。确认进入终态：`COMPLETED` 时报告有可读的同 ID typed Artifact；`FAILED` 时错误与阶段可观察，且不得伪称研究成功。不要将 `PENDING` / `RUNNING` 当成成功。
5. 查询「这次研究的空头依据？」时，应只读取既有 Run 的多空产物，不再启动新的付费研究；无辩论产物或 Run 失败时明确说明不可用，不编造观点。
6. 复核 Run ID、创建时间、事件、报告中的标的/截至时间一致；刷新页面后用历史列表重读同一任务。记录真实操作结果、最终状态和必要的脱敏证据。

## 验收判定

- **PASS**：显式确认只产生一个新 Run；HTTP 返回可查询 Run ID；Go/Web 对同一 Run 读到一致状态；真实 Provider 完整执行并产生来源可核验的报告；追问不触发第二次付费研究。
- **FAIL**：点击确认后没有 Run ID、重复创建、HTTP 找不到新建 Run、错误标的、失败冒充完成、跨 Run 引用混淆、权限绕过。记录失败阶段和固定 SHA，按真实缺陷新建 Issue。
- **NOT RUN**：未获得费用授权、凭证不齐或 Hosted 未实际执行。**不得**将模拟执行、浏览器 fixture 或普通 CI 绿灯升级为 Hosted PASS。

R2-01 的 #202 在 PASS 证据和验收范围确认前保持 Open；通过后更新 [DEVELOPMENT_ROADMAP.md](DEVELOPMENT_ROADMAP.md) 中 R2-01 的勾选项，再进入 R2-02。
