# Agent Runtime 累计示例

这是依据 Day 01–11 学习计划持续演进的可运行参考基线。后续课程在这里逐步扩展同一套 Runtime。它不代表学习者已完成对应练习。

共享仓库根目录的 `go.mod`（Go 1.22.2），无需新建 Module，也不导入根目录 `internal` 中的练习实现。所有命令在仓库根目录运行：

```sh
go run ./examples/cmd/demo
go run ./examples/cmd/knowledge
go run ./examples/cmd/access
go test ./examples/...
go test -race ./examples/...
```

Demo 不联网、不需要 API Key，输出 `计算结果为 2`。测试使用标准库；模型 HTTP 测试注入 RoundTripper，不连接真实 Provider 或监听端口。

```text
examples/
  cmd/knowledge/main.go            # Day 08 注册、检索→读取→模型往返
  cmd/demo/main.go                 # Model → Tool → Model 离线入口
  internal/model/                 # 消息、响应、Mock、Model/ToolBinder
    openai/                       # HTTP 适配、请求/响应转换、错误分类
  internal/tool/                  # Tool、DescribedTool、受控错误
    knowledge/                    # SearchKnowledge/GetDocument + FakeStore
    calculator/                   # 严格参数解析的最小业务示例
  internal/agent/                 # Loop、Options、同步事件
  CONTRACTS.md                    # 后续自动化必须读取的契约和迁移规则
```

| 章节 | 示例位置 | 累计能力 |
| --- | --- | --- |
| Day 01 | model、tool、agent、cmd/demo | Generate/Execute/Run、串行 Loop、Mock |
| Day 02 | model/openai | FinishReason、Usage、HTTP 错误和响应规范化 |
| Day 03 | model、tool、openai | 可选 ToolBinder/DescribedTool，不可变绑定 |
| Day 04 | agent/options.go、agent.go | 合作式取消、单工具超时、有限步数 |
| Day 05–07 | agent/event.go、agent_test.go | 同步事件、终止事件、共享 Sink 包装器 |
| Day 08 | tool/contract.go、registry.go、knowledge、cmd/knowledge | Spec/Meta/Invoke、注册与可见集合、Store 适配 |
| Day 09 | tool/calculator、agent.go | 参数边界、整批名称/ID 检查、结果关联 |
| Day 10 | tool/policy.go、cmd/knowledge | 整次超时、显式只读重试、结果预算 |
| Day 11 | tool/access.go、agent、cmd/access | 可信主体、批次执行授权、授权审计信号 |

Day 08 工具契约现已按线上修订版补齐。后续能力仍按路线推进：独立 ToolsNode 执行器、中间件、重试/幂等、权限与 MCP 留给 Day 10–14 按路线增量设计。Eino 是阅读对照，不作为运行依赖。

Provider 示例构造函数为 `openai.NewChatModel(*ChatModelConfig)`；`BaseURL` 传完整 Chat Completions endpoint，HTTPClient 可注入。真实调用由使用者自行配置 API Key 和超时；本次仅验证模拟 HTTP，不宣称真实 API 已验证。工具 Schema 与实际校验需同步维护；当前 JSON 解码采用 Go 标准库字段匹配规则，不额外拒绝重复键或大小写变体。

Day 08 的接口映射、旧接口兼容及边界见 [工具设计说明](internal/tool/design.md)。流式工具只作为 Eino 阅读对照，今天不提供未完成的 Stream 接口。

## Day 10：工具执行策略

新增 internal/tool/policy.go 和 policy_test.go；cmd/knowledge 使用 WithPolicy 统一配置整次逻辑调用的超时与结果字节预算。旧 Tool/DescribedTool、Registry.Tools、Agent.New/Run 和所有消息类型不变。

默认只执行一次；只有显式 ReadOnly、MaxAttempts>1 且提供 Retryable 分类器才能重试。受控 tool.Error 和取消/超时不重试；未知错误仅由可信后端分类器决定。示例不为写入自动重试，也不实现持久化幂等。取消、超时不能撤销已发生的外部操作。

本次环境未提供 Go 语言工具链，go test ./examples/... 和 go test -race ./examples/... 均未能运行（命令返回 Go: Unknown option: test）；不得据此视为测试通过。安装工具链的下载请求亦因代理连接失败受阻。请在可用 Go >=1.22.2 环境从仓库根重新运行两条命令。

## Day 11：可见性、执行授权与审计

- 新增 tool/access.go：Principal、WithPrincipal、Authorizer 与不可变 CapabilityAuthorizer。read/write 为应用配置的独立能力；未声明工具、未知或缺失主体均拒绝。
- Agent 增加可选 WithAuthorizer：在整批名称/ID 校验后、首个 Execute 前检查所有请求，拒绝或授权服务出错时终止 Run。旧调用不传选项时行为不变；显式 nil 配置被拒绝。对不可信模型开放工具时，宿主必须主动配置授权。
- 现有 WithVisibleTools 仍只过滤描述，不充当授权。Policy.ReadOnly 只控制重试资格，也不授予权限。
- Event 新增 tool_authorized/tool_denied 及 PrincipalID（可信、不含凭据的主体标识）。决策记录不含参数、正文或原始错误；authorized 不意味着执行已发生。同步 EventSink 没有持久化/交付保证。
- cmd/knowledge 以离线 knowledge-reader 主体启用读取授权，保留 Day10 策略包装；cmd/access 模拟模型请求隐藏写工具，输出“授权拒绝；写入次数: 0”。
- 此处为工具级能力快照，不是登录实现、资源/租户 ACL、审批或实时撤权。真实后端仍需资源级鉴权；WithPrincipal 仅由完成认证的宿主注入，不能采用模型提供的主体字段。

### 本日真实验收

使用官方 Go 1.22.2 linux/amd64，从仓库根执行累计示例验收：

- go test ./examples/...：通过。
- go test -race ./examples/...：通过。
- go run ./examples/cmd/knowledge：通过，读取两个可见工具的元信息后输出最终回答。
- go run ./examples/cmd/access：通过，授权拒绝且模拟写入次数为0。

这次验证覆盖累计 Day10 策略测试；上文 Day10 发布时未能运行的记录是历史状态，现已由本次累计验收补充。测试不证明真实认证/后端 ACL 或生产审计持久化已经完成。
