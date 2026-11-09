# Agent Runtime 累计示例

这是依据 Day 01–14 学习计划持续演进的可运行参考基线。后续课程在这里逐步扩展同一套 Runtime。它不代表学习者已完成对应练习。

共享仓库根目录的 `go.mod`（Go 1.22.2），无需新建 Module，也不导入根目录 `internal` 中的练习实现。所有命令在仓库根目录运行：

```sh
go run ./examples/cmd/demo
go run ./examples/cmd/knowledge
go run ./examples/cmd/access
go run ./examples/cmd/mcp
go run ./examples/cmd/mcpagent
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
| Day 12 | internal/mcp、cmd/mcp | MCP 初始化、能力协商、JSON-RPC 工具发现与调用 |
| Day 13 | tool/mcptool、cmd/mcpagent | MCP到Meta/Invoke适配、名称隔离、授权后的Agent往返 |
| Day 14 | integration/week02_test.go | 本地/远端混合批次、故障矩阵、历史/事件/计数联合验收 |

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

## Day 12：MCP 协议练习

新增 examples/internal/mcp/{types,client,fake,client_test}.go 与 examples/cmd/mcp/main.go。仍共享根 go.mod，不新增 SDK 依赖，也不替换 Tool/Meta/Invoke 或 Agent 消息。MCP Client 是 Host 访问 Server 的协议层；Day13 再做 Runtime Tool 适配。

- 固定协议版本2025-06-18，依次执行 initialize、无ID的 notifications/initialized，再 tools/list、tools/call。
- Transport.Send 收发完整 JSON-RPC JSON；DemoServer 在内存中复用既有 knowledge.GetDocument 和 FakeStore，不启动端口或进程。
- 这是串行请求的教学子集，不是完整 SDK：无真实 stdio/Streamable HTTP、服务器主动请求/通知、OAuth、完整多模态校验或传输关闭管理。只有内存资源，不发明 MCP shutdown 方法。
- capabilities.tools 只表示协议能力，不授权；远端 annotations 也不能自动决定只读重试。直接调用 DemoServer 没有套用 Day11 Agent 授权，这是可信离线夹具，不是可公开的业务服务器。
- JSON-RPC 请求ID与模型 ToolCallID 属于不同域。nextCursor 按不透明字符串原样往返，调用者负责分页；工具结果 isError 与 RPC error、传输错误分开。原始内容块/structuredContent 保留，Day13 再决定如何映射模型输入。
- 传输/协议失败后会话失效，避免自动重放可能已发生的远端操作。排队中取消不发请求；传输本身仍须合作取消。真实连接的重连和清理由成熟 SDK/宿主另行实现。

本日验收：官方Go1.22.2下 go test ./examples/... 与 go test -race ./examples/... 通过；go run ./examples/cmd/mcp 实际输出协议版本2025-06-18、工具数量1与 doc-1 的文档JSON。验证覆盖内存JSON消息，不代表真实MCP网络连接或真实模型集成通过。

## Day 13：MCP能力接回Agent

新增 examples/internal/tool/mcptool/{adapter,discovery,adapter_test,integration_test}.go 与 examples/cmd/mcpagent/main.go。仍共享根go.mod；原Tool/Meta/Invoke、Registry、Agent、MCP Client均不修改。

- 初始化后调用 mcptool.Discover(ctx,client,"knowledge",4)，完整发现后在新Registry中注册；Meta只读描述快照，不发网络请求或执行工具。
- 本地名 knowledge__get_document 路由到远端原名 get_document。前缀只能用字母开头的字母/数字/连字符，远端名限字母/数字/下划线/连字符，组合最长64字节；不兼容名称明确拒绝，不暗中重命名。
- 发现有页数上限、游标循环检测和跨页重名检测，失败返回nil而非半套能力；Discover不改变运行中的注册表。主机应装配新Registry成功后再发布，不能把注册逐项写入活动Agent。
- 成功结果统一为JSON字符串 ResultEnvelope{text:[],structuredContent?:object}；仅支持文本块，图片/资源等类型明确拒绝，不丢掉非文本块后伪装成功。旧本地knowledge工具输出保持原样。
- 非取消的远端错误、isError和不支持的内容变为受控tool.Error，原因链留给可信宿主；取消/超时继续终止Run。默认不重放不确定请求，受控错误也不会触发Day10 Policy自动重试。
- RPC请求ID由Client管理，模型ToolCallID由Agent保留，适配器不把调用ID或Principal塞进工具参数；本地可见集合与执行授权使用别名。远端annotations不授予权限或配置安全重试。
- 内存DemoServer与Caller在同进程传context，仅作协议夹具；这不等于身份会跨真实网络传递。真实服务仍需认证/资源权限，主机也必须信任绑定的服务器和能力分类。

本日验证（官方Go1.22.2）：go test ./examples/...、go test -race ./examples/... 均通过；go run ./examples/cmd/mcpagent 输出本地别名、原模型CallID、真实夹具文档JSON和脚本模型结束结果。端到端测试验证允许路径的历史关联和拒绝路径远端tools/call计数为0。未连接真实MCP网络服务或真实LLM。

## Day14：第二周综合集成与复盘

新增 [跨组件集成验收说明](integration/README.md) 与 integration/week02_test.go。没有新Runtime公共接口、业务算法或Module；复用Day08–13的装配链和既有类型。

从仓库根运行 `go test ./examples/integration -run TestWeek02EndToEndMatrix -v`，先预测计数，再观察正常、授权拒绝、模型ID冲突、结果预算、远端业务失败、执行后响应损坏和超时7个场景。分别检查实际工具调用、模型第二轮历史、错误摘要和唯一Run终态；脚本模型最后返回文本不等于用户任务已正确完成。

官方Go1.22.2下目标矩阵7个子场景、累计examples普通测试和race均通过。所有协议交互使用内存传输与FakeStore，不宣称真实网络/模型或恢复能力通过。下一周才开始Run状态与Checkpoint设计。

## Day 09–15：图解、骨架与完整测试

[课程首页](../index.html) 已更新 Day 09–15；练习附件、隔离准备与可重跑验证见 [交付说明](../docs/learning/exercises/README.md)。练习测试指向学习者 internal，参考仍在 examples，两套 model 类型不混用。

Day 09 的批次授权段提取为私有 authorizeBatch 插槽，Day 11 使用同一位置练习，公开 API 不变。Day 14 新增 internal/assembly 的宿主 Build 与七场景验收；原 integration/week02_test.go 保留。Day 15 新增 internal/runstate 的纯状态转换和快照测试；未修改旧 Agent.Run，也没有新增持久化恢复能力。
