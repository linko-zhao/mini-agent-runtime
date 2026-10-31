# Agent Runtime 累计示例

这是依据 Day 01–09 已发布学习计划整理的可运行参考基线。后续课程在这里逐步扩展同一套 Runtime。它不代表学习者已完成对应练习。

共享仓库根目录的 `go.mod`（Go 1.22.2），无需新建 Module，也不导入根目录 `internal` 中的练习实现。所有命令在仓库根目录运行：

```sh
go run ./examples/cmd/demo
go run ./examples/cmd/knowledge
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

Day 08 工具契约现已按线上修订版补齐。后续能力仍按路线推进：独立 ToolsNode 执行器、中间件、重试/幂等、权限与 MCP 留给 Day 10–14 按路线增量设计。Eino 是阅读对照，不作为运行依赖。

Provider 示例构造函数为 `openai.NewChatModel(*ChatModelConfig)`；`BaseURL` 传完整 Chat Completions endpoint，HTTPClient 可注入。真实调用由使用者自行配置 API Key 和超时；本次仅验证模拟 HTTP，不宣称真实 API 已验证。工具 Schema 与实际校验需同步维护；当前 JSON 解码采用 Go 标准库字段匹配规则，不额外拒绝重复键或大小写变体。

Day 08 的接口映射、旧接口兼容及边界见 [工具设计说明](internal/tool/design.md)。流式工具只作为 Eino 阅读对照，今天不提供未完成的 Stream 接口。

## Day 10：工具执行策略

新增 internal/tool/policy.go 和 policy_test.go；cmd/knowledge 使用 WithPolicy 统一配置整次逻辑调用的超时与结果字节预算。旧 Tool/DescribedTool、Registry.Tools、Agent.New/Run 和所有消息类型不变。

默认只执行一次；只有显式 ReadOnly、MaxAttempts>1 且提供 Retryable 分类器才能重试。受控 tool.Error 和取消/超时不重试；未知错误仅由可信后端分类器决定。示例不为写入自动重试，也不实现持久化幂等。取消、超时不能撤销已发生的外部操作。

本次环境未提供 Go 语言工具链，go test ./examples/... 和 go test -race ./examples/... 均未能运行（命令返回 Go: Unknown option: test）；不得据此视为测试通过。安装工具链的下载请求亦因代理连接失败受阻。请在可用 Go >=1.22.2 环境从仓库根重新运行两条命令。
