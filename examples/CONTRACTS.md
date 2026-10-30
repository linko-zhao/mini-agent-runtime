# 累计示例契约与自动化约定

## 目录和稳定接口

Module：`github.com/linko-zhao/mini-agent-runtime`，示例包前缀：`github.com/linko-zhao/mini-agent-runtime/examples/internal/`。沿用 Day 01 的职责划分，只增加 examples 前缀以存放参考代码。没有每日独立 Module，不用类型别名连接学习者实现。

- `model.Model.Generate(context.Context, []Message) (*Response, error)`。
- `Message{Role, Content, ToolCalls, ToolCallID}`；`ToolCall{ID, Name, Arguments json.RawMessage}`。
- `Response{Message, FinishReason, Usage}`；保留未知 FinishReason，Usage 缺失为零。
- `model.ToolBinder.WithTools([]ToolDefinition) (Model, error)` 是可选能力，返回新模型，不修改共享适配器。
- `tool.Tool{Name() string; Execute(context.Context, json.RawMessage) (string, error)}`；`DescribedTool` 嵌入 Tool 并提供 `Definition() model.ToolDefinition`。
- `agent.New(model.Model, map[string]tool.Tool, maxSteps int, ...Option) (*Agent, error)`；`Run(context.Context, string) (string, error)`。
- `WithEventSink(EventSink)`、`WithToolTimeout(time.Duration)`；`EventSink.Emit(context.Context, Event)` 同步发布，不包含 Prompt、参数或工具原始输出。

接口形状以 Day 01 建立、Day 02/03 扩展的定义为依据。Day 01 的示例 module `github.com/linko/...` 按该页允许自定义仓库地址的说明归一为当前仓库地址。

## 数据流与错误边界

1. New 复制注册表，验证 map key、Name、Definition 名称一致；排序描述后绑定一次。旧 map 入口无法检测组装之前已覆盖的重复 key；Day 08 的 Registry.Register 在入口拒绝重名，Registry.Tools 提供旧 API 的适配快照。
2. Run 的消息历史、步数均为局部变量。模型调用前检查取消；返回 nil 响应或非 assistant 消息是协议错误。
3. Assistant 消息先入历史；整批预校验非空且批内唯一的 ToolCall ID 和已注册名称，再按顺序调用工具；结果 Role=tool，ToolCallID 引用原始 ID。
4. 未知工具、协议错误、模型失败、超出步数（`errors.Is(err, agent.ErrMaxSteps)`）、取消与超时终止 Run。没有自动重试或恢复，不能据此保证跨 Run 的 exactly-once。
5. 工具参数/业务错误以受控 `tool.Error` 回填，未知内部错误统一回填 `execution_failed: tool execution failed`，不泄漏原始错误文本。`tool.Error.Cause` 与 `model.ModelError.Err` 支持 Unwrap。
6. 超时是合作式取消，不启动隔离 goroutine，不强杀工具。零超时表示禁用，负值构造失败；父 Context 仍生效。
7. 每次 Run 发布一个 started 和一个 finished/failed；每次已开始的模型/工具调用发布 completed。Event 使用 Day 06 的 `EventRunStarted` 等常量与 Day 05 的 `Status string`，Step 从 1 开始。共享 Sink 用同一个 `NewSynchronizedSink` 包装器；下游不能重入包装器。
8. Agent 构造后不改注册与模型绑定；并发安全仍要求注入的 Model、Tool、Sink 支持并发。脚本 Mock 有游标，应每个 Run 创建一个，不支持共享并发。

## 与历史片段的对应和迁移

- 旧页面 `internal/...` → 可运行参考 `examples/internal/...`；旧 `cmd/demo` → `examples/cmd/demo`；调用方仅修改 import 前缀，不能混用两套 Message 类型。
- Day 08 中 `internal/tool/calculator.go` 的旧路径 → `examples/internal/tool/calculator/calculator.go`，Tool.Name 仍为 calculator。
- 早期页面对工具错误允许 fail-fast 或回填。本基线统一采用上面的终止/回填策略，调用方用 errors.Is/As 判断，不匹配原始字符串。取消绝不当普通工具结果回填。
- 早期文档中没有完整构造函数及注册算法；本基线补齐 New、可选配置、注册表复制与批次预校验。Day 08 新增 Registry，通过 Tools() 适配当前 New 调用；未来新增 Executor 若必须改签名，列明旧→新接口和所有调用方修改。

## 后续自动化的执行规则

生成前读取 README、本契约、前一天 HTML、相关基础章节，以及当天涉及的 examples 文件。根据固定 56 天路线推进，不能以根 internal 的实现状态选题或补课。

每页遵守 `docs/learning/agent-runtime-56-day-schedule.md` 的“教学目标与引导规范”：先说明具体问题、完整 Agent 数据流与设计取舍，每项练习写明“为什么做 → 操作与观察 → 完成证据”，分别验收搭建思路和实作结果。

每页必须给出“承接前序设计”“本日增量”，明确新增/修改/复用的 examples 文件、接口、数据流、错误策略、验收与次日衔接。新增功能复用现有类型，不能每天重建工程、无说明更名或只留下无法组合的伪代码。确需破坏性调整时先提供完整迁移步骤并同步本文件。

当天新增可运行示例应同步更新 examples、相关测试和本契约，连同当天 HTML 和根 index.html 一起提交；根 internal/cmd 是学习者练习，自动化不修改。测试命令从仓库根执行 `go test ./examples/...`、`go test -race ./examples/...`，只记录真实运行结果。伪代码必须标明，缺少运行环境时注明未验证。GitHub 写入前检查最新 main 和涉及路径，基于最新父提交更新，冲突先回读，禁止覆盖无关文件。

## Day 08 修订版工具契约（后续以此增量扩展）

- `Spec{Name, Description, InputSchema}` 与 `Described.Meta(context.Context) (Spec, error)` 分离元信息和执行；`Invokable` 嵌入 Described 并提供 `Invoke(context.Context, json.RawMessage) (string, error)`。描述不访问业务 Store，也不含凭据。
- `Call` 是既有 `model.ToolCall` 的别名，沿用 ID/Name/Arguments，复用消息协议。
- `NewRegistry()`、`Register(ctx, Invokable)`、`Lookup(name)`、`Specs(visible []string)`。nil 枚举全部，非 nil 空集合枚举零个；注册拒绝空名/重复名/非法 schema，Specs 稳定排序并深拷贝元信息，不执行工具。
- `Registry.Tools()` 将完整注册集合适配为旧 `map[string]Tool`，`Adapt(ctx, Invokable)` 可单独适配。旧 Tool/DescribedTool 保持不变；无需批量改动已有 Calculator 调用方。
- `agent.WithVisibleTools(names...)` 控制此 Agent 绑定给模型的描述集合，不删执行注册。不同 Run 若需不同可见集合，创建各自 Agent 并绑定共享的不可变适配器；不在共享 Agent 上修改可见状态。执行授权另由 Day 11 增加。
- `knowledge.SearchKnowledge/GetDocument` 通过注入的 Store.Search/Get 执行；FakeStore 为离线只读数据。运行时传取消/超时，真实 Store 负责后端鉴权；Runtime 的结果预算和执行授权留待后续章节。

此前示例仅覆盖本地旧版 Day 08 的参数校验内容，缺少线上修订版的能力契约。本次补齐上述契约，保留旧接口适配，完整映射见 `examples/internal/tool/design.md`。后续自动化应先核对 main 上的当前页面，不能以落后的本地副本认定已覆盖章节。
