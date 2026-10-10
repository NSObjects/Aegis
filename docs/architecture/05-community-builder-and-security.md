# 05. 社区策略、Strategy Builder Agent 与安全治理

> **远期设计，不是当前开放的第三方代码安装能力。** 返回 [架构索引](README.md)。

## 1. 社区应该交付什么

社区主要贡献**独立研究策略包**（选股、事件、量化、仓位研究等），官方负责稳定 Core、每日批量数据采集及基础 Capability。社区策略可定义自己的 Graph/Agent/Tool/规则，声明所需数据能力、权限、参数、版本及 typed schema。

官方与第三方可共享策略执行协议，不需要共享编程语言。官方可信 Go builtin 可以进程内运行；第三方可采用声明式包或隔离 Worker。**不允许 Strategy A 依赖 Strategy B 的运行过程**，避免形成不受控策略调用 DAG；组合比较属于 Host/Web 显式地启动独立 Run 并比较结果。

未来社区发布对象至少需：
- 作者、版本、包内容摘要 digest、许可与依赖锁
- kind、Manifest、输入输出 Schema、配置、能力需求/权限
- 策略原理、适用环境、完整运行示例、数据/费用假设
- 测试报告、回测方法、风险与局限、变更和弃用记录

## 2. 安全威胁不能靠提示词解决

威胁包括：恶意 Go/Python 包、供应链依赖、MCP 越权、Skill 含恶意命令、提示注入、上传下载和秘密外泄、刷爆 API/LLM 成本、伪造研究证据与回测、污染其他策略的运行态。

建议最小执行等级：

| 运行模式 | 信任前提 | 运行办法 |
|---|---|---|
| 官方 builtin | 受代码审查、项目测试与发布控制 | 可先进程内，但仍要求预算/审计/最小副作用 |
| 第三方声明式 | 使用受限的白名单节点和能力 | 受限解释执行，无任意 shell/文件操作 |
| 第三方任意代码 | 默认不可信 | 受限进程/容器 Worker；明确 RPC、网络白名单、只读 FS、CPU/内存/时间配额、凭证代理 |

进程隔离不自动等于安全；必须验证网络出站、容器逃逸面、宿主目录、环境变量、数据库和密钥的隔离。没验证之前不要承诺能安全安装任意社区代码。**不建议将不可信 Go 插件直接加载为宿主进程的 `.so`。**

## 3. Capability 权限与外部 MCP/Skill

- Strategy Manifest 声明依赖，Host 在 Run 前核准可访问的最小 Capability 集合。
- 不直接向策略下发 TuShare Key、MCP 密钥、数据库连接串或管理员令牌；应走能力 Broker。
- Tool discovery 只能看到已授权目录；“可以搜索工具”绝不是权限边界。
- MCP/Skill/网页文本均是低信任输入；不能让其中的“忽略以前规则、安装脚本、上传配置”等指令获得高权限。
- 控制 egress、请求限流、超时、成本上限、日志脱敏与审计，安全执行拒绝与模型推理分离。
- 研究数据提供商可能限制存储/商业使用/再分发；允许插件访问不等于允许平台转售数据。

## 4. 对话开发策略（Builder Agent）

```text
Conversation with user
  -> Strategy requirements & acceptance tests
  -> Discover permitted official Capabilities / Tools
  -> Generate declarative workflow or isolated code package
  -> Static checks / schema / unit tests
  -> PIT historical replay + out-of-sample + cost/robustness tests
  -> Independent ValidationReport (not self-scored)
  -> User approval -> New immutable candidate version
  -> Install / Publish (subject to policy)
```

Builder 优先**组合已存在的官方能力**，而不是每次自己写一个实时数据插件或重新实现指标。生成能运行 ≠ 正确、可靠或有投资价值。对话生成的策略不应该在无人确认下修改运行中的正式版本、提升权限或自动发布。

## 5. Memory / Reflection 应服务持续研究

Research Memory 要记录判断、时间、所用证据、错判、事后观察、失败原因与策略修订建议，区别于市场原始数据库和聊天记录。用 `run_id` / `strategy_digest` / `as_of` / `evidence_refs` 关联。注意 PIT：历史回测不能读到未来才得出的“反思”。

建议循环：
`Research Run -> Outcome Observation -> Error Analysis -> Candidate Modification -> Independent Validation -> Approved New Version`。

策略研究与独立回测分开，不能由 Builder 为自己生成的策略宣布“已验证有效”。收益报告需声明数据集、期间、费用/滑点、样本内外、幸存者偏差和可见性局限。

## 6. 发布前必须验证

1. 未授权 news/search 的策略在 MCP/Tool Search 路径也拿不到它。
2. 不可信包无法获取主机密钥、任意网络、宿主目录或直接访问其他策略数据。
3. Skill 建议 `curl | sh` 或外传环境变量，不能自动执行。
4. 第三方 Worker 超时/崩溃，Host 将该 Run 置于安全终态且不影响其他 Run。
5. Builder 生成的策略 PIT/Schema/预算测试失败则无法发布，即便模型声称通过。
6. 升级/撤回策略不会修改历史固定 digest 和研究产物。
7. 所有研究回测宣称与真实交易结果严格区分；Aegis 不接入真实交易执行。
