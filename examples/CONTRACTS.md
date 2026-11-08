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
4. 未知工具、协议错误、模型失败、超出步数（`errors.Is(err, agent.ErrMaxSteps)`）、取消与超时终止 Run。默认没有自动重试或恢复；Day10 显式只读策略可启用有限重试，不能据此保证跨 Run 的 exactly-once。
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
- `agent.WithVisibleTools(names...)` 控制此 Agent 绑定给模型的描述集合，不删执行注册。不同 Run 若需不同可见集合，创建各自 Agent 并绑定共享的不可变适配器；不在共享 Agent 上修改可见状态。执行授权由 Day11 的 WithAuthorizer 单独配置。
- `knowledge.SearchKnowledge/GetDocument` 通过注入的 Store.Search/Get 执行；FakeStore 为离线只读数据。运行时传取消/超时，真实 Store 负责后端鉴权；Runtime 的结果预算和执行授权留待后续章节。

此前示例仅覆盖本地旧版 Day 08 的参数校验内容，缺少线上修订版的能力契约。本次补齐上述契约，保留旧接口适配，完整映射见 `examples/internal/tool/design.md`。后续自动化应先核对 main 上的当前页面，不能以落后的本地副本认定已覆盖章节。

## Day 10 增量：兼容执行策略包装

- 新增 WithPolicy(DescribedTool, Policy) (DescribedTool,error)，包装后 Name/Definition 不变，Execute 仍复用 json.RawMessage 与 context；Registry.Tools() 结果可逐项包装后传给 Agent.New。没有公开接口迁移、没有新的 Module。
- Policy.Timeout 限制整个逻辑调用，覆盖所有尝试和等待；父 context/Agent.WithToolTimeout 若更早到期则优先生效。仍要求工具合作取消，不用后台 goroutine 强杀。
- MaxAttempts=0 默认1；大于1必须 ReadOnly=true 且给出 Retryable。只对非受控且分类为暂时性的问题重试；tool.Error、context.Canceled、DeadlineExceeded 不重试。RetryDelay 等待响应取消；每次尝试收到独立参数副本。
- MaxResultBytes=0 不限；正值按 UTF-8 字节数限制成功结果。超出时返回受控 ExecutionError，模型收不到超限原文；不直接截断可能包含结构化 JSON 的结果。
- 同一逻辑调用的 ID/历史消息与工具事件仍由 Agent 管理；内部重试不会额外追加 tool messages，事件耗时覆盖整个逻辑调用。模型下一轮重新请求不是本次重试。
- ReadOnly 是应用装配时的可信断言，不信任模型自报；未来 Day11 授权与能力分类不能被此开关替代。没有跨 Run 去重、持久化幂等或 exactly-once 保证。写入如需重试，须另行设计服务端幂等键与确认协议。
- 新增 policy_test.go 验证默认不重试、资格拒绝、受控错误不重试、显式读取暂时失败恢复、取消/超时、取消等待和结果预算。当前环境缺 Go 工具链，验收命令已尝试但未能执行；尚未验证编译/运行。

## Day 11 增量：执行授权与决策审计

### 稳定接口与装配

- 新增 tool.Principal{ID string}、WithPrincipal(ctx,Principal)、PrincipalFromContext(ctx)。仅承载已认证宿主提供的请求主体，不能信任模型的参数/文本自报身份；不存令牌、密码或 Prompt。
- tool.Authorizer.Authorize(context.Context, Call) error 复用现有 Call（model.ToolCall 别名）。Agent 传入独立 Arguments 副本，授权器不能修改即将执行的参数缓冲区。
- NewCapabilityAuthorizer(map[string]Capability,map[string][]Capability) 复制规则与授权配置；ReadCapability/WriteCapability 是独立能力，没有隐式继承。未配置工具或未知/缺失主体返回 ErrForbidden，取消保留 context 错误。该实现构造后只读，可共享；自定义授权器也必须支持预期的并发 Run。
- 新增 agent.WithAuthorizer(tool.Authorizer)；Agent.New/Run 和既有 Tool/Meta/Invoke 形状不变。省略选项保留旧行为，不代表已经启用授权；显式传 nil 在 New 报错。
- 装配路径仍是 Registry.Tools → Day10 WithPolicy → Agent.New。WithVisibleTools 不删注册，Policy.ReadOnly 不授权；权限来自独立且可信的配置。

### 执行顺序与失败

Run 保存 assistant 后先校验整批调用 ID/名称，再做整批授权；任何拒绝/授权服务错误/取消均停止 Run，不调用本批工具，也不再调用模型。ErrForbidden 经包装保留 errors.Is；自定义授权器错误保留原始错误链，仅返回受信宿主，不进入模型历史。已有工具参数/业务错误回填语义不变。

整批授权只是调用开始前的判断，不构成事务、执行承诺或跨系统锁。允许决策后仍可能被取消或被后端资源级 ACL 拒绝；前面的操作不能自动撤销。当前授权是静态工具级能力快照，不解析资源/租户权限，也不在内部每次重试时重新授权。实时撤权须由宿主/后端执行层另外设计与校验。

### 审计信号与兼容迁移

- Event 新增 EventToolAuthorized/EventToolDenied，增加 PrincipalID；其余字段继续使用 Step、ToolName、ToolCallID、At、Status。允许为 ok；明确拒绝为 denied；授权器故障为 error，取消为 cancelled。
- 决策事件在执行前发出；前项允许、后项拒绝时，会有“允许”记录但本批执行次数仍为0。调用开始/完成仍由原事件表示；Run 保持一个终态。
- 事件不含 Arguments、结果正文、令牌或原始错误；主体使用可信不透明标识。Emit 仍同步且无返回值，不保证审计持久化、交付或抗篡改，也没有“落盘成功后才执行”的事务保证。共享 Sink 沿用同一个 SynchronizedSink 包装器。
- 基线采用命名字段 Event 构造，无需修改；历史外部消费者如按位置构造 Event，须改为命名字段。事件 switch 应接受新类型。未开启授权时不增加决策事件，旧测试行为保持。
- 调用方只需从可信会话生成 WithPrincipal，并给 New 增加 WithAuthorizer；不更换模型、消息、工具适配或 module。

### 验证结果

新增 access_test.go 与 authorization_test.go 覆盖配置快照、默认拒绝、并发读取、可见性与授权分离、整批零执行、错误链、参数副本、取消和显式 nil 配置。官方 Go1.22.2 下 go test ./examples/... 与 go test -race ./examples/... 均通过；knowledge 与 access 两个离线入口实际运行成功。此前 Day10 “未验证”条目保留为当时历史状态，本次累计测试已覆盖这些文件。

授权守卫只覆盖启用了 WithAuthorizer 的 Agent.Run 路径；宿主若直接调用 Execute/Invoke，必须自行施加同等授权。审计事件仅在配置 WithEventSink 时交给消费者；示例授权测试使用收集器验证决策，两个演示入口主要验证执行路径，不声称写入审计存储。

## Day 12 增量：MCP 协议层（固定2025-06-18）

### 复用与新增接口

- 新增 examples/internal/mcp：ProtocolVersion、Transport.Send(context.Context,[]byte)([]byte,error)、NewClient、Initialize、ListTools(ctx,cursor)、CallTool(ctx,name,json.RawMessage)。旧 model.Message/ToolCall、Tool.Execute、Meta/Invoke、Agent.New/Run 不变，无新 module 或依赖。
- Descriptor、ToolsPage、ToolResult 是带 JSON 标签的线协议数据，并非第二套 Runtime Tool/Call/Message。Day13 将 Descriptor 映射到已有 tool.Spec、将客户端调用适配到 Invoke；今天尚未注册远端工具到 Agent。
- DemoServer 仅作单会话内存夹具，读取既有 Invokable.Meta 并调用 Invoke；cmd/mcp 复用 knowledge.GetDocument/FakeStore。它没有生产服务器认证，也不假定客户端身份自动成为服务器权限。

### 生命周期、能力和错误

- Client 串行化单会话请求并支持排队取消；初始化前不能发工具请求。Initialize 校验版本、ServerInfo 和 Capabilities，发无ID的 initialized 通知且不得收到响应，完成后才 ready。
- 只支持固定协议版本；协商结果不同则拒绝，不声称自动兼容最新版本。tools 能力缺失时阻止 list/call；不是工具不存在或用户无权执行的同义词。
- JSON-RPC整型请求ID由Client独立分配，不复用模型ToolCallID；响应检查jsonrpc=2.0、ID匹配及result/error互斥。该子集的方法result必须为object，列表tools与调用content必须为array；不做完整JSON Schema、多模态或所有MCP扩展校验。
- ListTools 返回一页和不透明 nextCursor，不自动翻页；后续调用方需限制分页次数和检查跨页名称冲突。保留InputSchema、Annotations，不能把远端readOnlyHint当作授权/安全重试证明。
- ToolResult 保留所有原始内容块和structuredContent；isError=true是一次有效工具执行结果，Go error为nil。JSON-RPC错误返回可errors.As识别的RPCError，Error()只给代码，原始Message/Data仅供受控诊断；传输/取消错误支持errors.Is。Day13再定义面向模型的安全映射，不能直接泄露内部错误。
- 有效RPC错误不自动失效会话。传输或协议不确定后标记failed，禁止自动重放initialize或tools/call；排队尚未发送时取消不改变其他调用状态。恢复应创建新的连接/会话；不能仅对已经初始化的同一传输重发initialize。
- 固定响应预算1MiB在Transport返回之后检查，仅限制本客户端后续解析，不保证实际网络传输层的内存预算。真实传输仍需自己的帧大小、流处理和资源清理策略。

### 明确不在本日实现的内容

没有生产stdio/Streamable HTTP、服务端主动请求/通知、OAuth/会话头、完整并发多路复用或持久会话恢复。Transport是单请求JSON教学端口；真实工程应使用成熟SDK。MCP结束连接通过传输资源关闭，不存在本课自造的shutdown RPC。Day11本地授权与后端资源权限继续成立，但今天的直接协议演示不声称已走Agent授权链。

### 实际验证

新增client_test覆盖初始化与notification顺序、缺失能力、版本拒绝、消息ID、畸形响应、大小限制、错误层次、分页、原始内容保留、取消与禁止重放。官方Go1.22.2下累计 go test ./examples/...、go test -race ./examples/... 均通过；cmd/mcp演示实际运行成功。旧Day10未验证说明保留为历史记录，本日累计验收继续覆盖其策略代码。

## Day 13 增量：MCP到既有工具契约的适配

### 新增与复用

- 新增 examples/internal/tool/mcptool；Caller仅声明Day12的ListTools/CallTool方法，由已初始化的*mcp.Client实现。宿主负责会话初始化、连接清理与恢复；外部SDK客户端需实现此协议端口，不是直接假定类型兼容。
- New(Caller,prefix,mcp.Descriptor)(tool.Invokable,error)、Discover(ctx,Caller,prefix,maxPages)([]tool.Invokable,error)。返回对象实现原Meta/Invoke；Registry.Tools沿原路径适配旧Execute。没有第二套Runtime Message/ToolCall，也不改Agent.New/Run。
- 元信息在构造时复制InputSchema，Meta每次返回副本，不执行远端调用。原本地knowledge工具及其输出语义不变。

### 名称与发现

本地名为 prefix + "__" + 原远端名。前缀仅接受^[A-Za-z][A-Za-z0-9-]*$，不含下划线，以保持分隔边界唯一；远端名限定^[A-Za-z0-9_-]+$；组合最长64字节。这是面向当前模型工具名称的适配约束，不代表全部MCP名称都限于此；不兼容时明确报ErrDiscovery，不静默清洗名称。宿主可在未来以显式映射扩展，并更新契约。

发现分页必须maxPages>0，原样往返不透明游标；检测重复游标、跨页工具重名、基本Schema与名称错误。任何失败返回nil。Discover不修改Registry，宿主应先收集完整候选，再新建Registry装配；Registry.Register继续拒绝重名，不覆盖原工具。不同服务器使用宿主分配的不同前缀，前缀隔离不能代替服务器认证或目录信任。

### 结果与错误映射

- 成功返回ResultEnvelope的JSON字符串：text为所有文本块的字符串数组（可为空），structuredContent如存在须为对象并保留。不是原Document JSON的直接副本，调用方应按新远端工具的Envelope解析；不改变旧工具输出。
- 非文本块返回受控ExecutionError，Cause为ErrUnsupportedContent；不返回前面部分文本。畸形结果使用ErrInvalidResult。当前Model.Message.Content仍为字符串，真正多模态支持需要后续明确扩展，不能把base64图片当作已理解。
- MCP IsError使用受控摘要和ErrRemoteExecution；RPC/传输错误使用固定remote tool request failed摘要，Cause保留可信诊断。取消/DeadlineExceeded保持errors.Is并终止Run。原始RPC Message/Data、错误结果内容不进入模型错误文本。
- Day10 Policy位于适配Invoke外侧，应用结果预算覆盖最终投影JSON。受控tool.Error不自动重试，远端annotations即使声明readOnlyHint也不会改变此行为；会话failed时不自动重连。模型仍可收到受控失败并结束，或按既有有限步数继续；失败会话继续拒绝网络发送。
- 原模型ToolCallID仍由Agent组装tool消息保留；JSON-RPC整数ID独立分配。适配器不把模型ID或Principal加进工具arguments。

### 授权和演示边界

装配路径为 Initialize → Discover → 新Registry → Registry.Tools → WithPolicy → Agent.New(WithVisibleTools,WithAuthorizer)。本地描述和授权表使用别名，执行时才路由原远端名。发现目录可能已发送initialize/list请求；授权拒绝的零执行专指后续tools/call不发出，不能说整个流程没有协议流量。权限与只读分类由可信宿主配置，不从远端annotations自动生成；真实后端仍检查资源/租户权限。

内存Server在同进程接收context，不能据此推断真实网络会自动传递Principal。主机身份、网络认证和服务器资源授权是不同边界。本日不实现生产HTTP/stdio、OAuth或动态目录刷新。

### 实际验证

新增测试覆盖元信息快照、名称边界、分页预算/循环/重名、原子发现失败、结果投影与拒绝非文本、受控错误及不重试、取消、Registry不覆盖冲突。真实内存JSON-RPC端到端测试验证：描述绑定本地别名、远端请求原名、模型ID与RPC ID分离、结果回到模型历史、拒绝后tools/call计数为0。官方Go1.22.2下累计examples普通/race测试通过，mcpagent演示实际成功。Mock验证协议和搭建路径，不证明真实模型会正确选择工具或生产认证已验证。

## Day14 增量：跨组件集成基线

新增 examples/integration/week02_test.go 和 integration/README.md，只增加测试夹具、断言和设计记录；不新增Runtime公共接口，不更换任何消息、工具、Policy、Authorizer或MCP类型。共享根go.mod，根internal/cmd保持不动。

集成场景使用同批两个已知ID的独立读取：本地get_document与远端knowledge__get_document。保留本地Document JSON、远端ResultEnvelope两种既有输出，通过各自契约验收，不为测试统一格式而静默改旧工具。脚本模型只验证流程和历史，不保证真实回答质量。

7个矩阵场景覆盖正常、未批准远端能力、重复模型CallID、远端结果预算、远端业务失败、远端已执行后协议响应损坏、传输等待超时。计数分别为本地Invoke、tools/call传输尝试、远端Invoke、模型轮次；事件和下一轮消息另行断言，不能只看最终字符串。

失败层次保持原约定：模型调用ID协议错误/授权拒绝/取消超时终止Run；MCP响应损坏会令Client.failed，适配器仍可回填受控ExecutionError，因此Run可能以脚本文本结束。两种协议错误属于不同边界，不用一个“失败”标志抹平。failed客户端的后续探针不得发送请求，现有有限步数与禁止自动重放保持。

授权拒绝矩阵缺少远端能力的宿主规则，而两个描述仍对模型可见，验证默认拒绝。初始化/list属于装配流量，零执行断言只针对业务tools/call。审计允许决策不代表执行承诺。超时夹具在转发前等待，远端Invoke=0仅是该注入点的观测；损坏响应夹具先调用Server，表明协议失败无法回滚调用。本课没有外部写入事务。

官方Go1.22.2下目标矩阵7个子场景与累计 go test ./examples/...、go test -race ./examples/... 均通过。后续Day15起按既定状态机/Checkpoint路线增量设计，复用此矩阵；当前Run局部消息不能提供跨进程恢复，保存状态也不自动解决不确定外部调用或exactly-once。

## Day15 增量：Run状态机与最终内存快照

### 兼容接口与状态所有权

- 新增agent.RunStatus、CallStatus、CallRecord、RunSnapshot、RunResult、CanTransition以及Agent.RunWithState(ctx,input)(RunResult,error)。
- 原Run(ctx,input)(string,error)委托同一执行路径；New、Model、Message、ToolCall、Tool/Meta/Invoke和Event保持不变。只需状态的调用方选择新API，不要替换旧消息类型。
- agent.go中的Step/Messages移入每次调用私有runRecord；executor.go保留executeBatch旧私有签名，委托executeBatchWithState。记录不放进共享Agent字段，不增加另一套执行逻辑。
- 新Run由crypto/rand生成不透明RunID。构造失败时尚未发出RunStarted，返回零值Result和错误；成功启动后正常/错误返回均提供终态快照。ID不代表跨系统幂等承诺，事件接口本日未增加RunID关联。

### 迁移与状态不变量

- created只允许到running；running允许到completed/failed/cancelled；终态不可自循环或重启。unfinished调用不能被当作成功完成，内部非法迁移产生ErrInvalidTransition。
- completed表示Run以文本正常返回，不表示业务目标已达成；工具受控错误可与RunCompleted并存。context取消/超时归为RunCancelled，FailureClass分别为cancelled/deadline_exceeded；步数耗尽保持errors.Is(ErrMaxSteps)，FailureClass=max_steps；其他Run错误只存受控run_error类别。
- Snapshot.Step为当前进入的Loop轮次，不是成功模型调用计数；例如预取消可能进入第1轮后停止。实际调用次数仍通过事件/测试探针验证。
- CallRecord用Step+Index定位，Index在同批从0开始，保留原CallID/ToolName；不能用CallID作唯一map键，否则会丢失非法重复ID的诊断记录。
- planned可到running，之后为returned/errored/unconfirmed；批次检查失败或终止后的未开始项为blocked。returned/errored只描述接口返回，均不证明外部事务状态；unconfirmed突出已开始调用在取消后无已接受结果，不授权自动重试。
- Calls.Result是已接受的tool消息（含受控错误摘要）；Context取消后不保存迟到结果。批次中前项已返回、后项取消时，原executeBatch仍不回填部分历史，Snapshot.Messages只保留已接纳的历史，而Calls.Result保留前项已知结果。不能把“未进入下一轮模型”当“没执行”。
- Go错误仍从第二返回值保留原errors.Is/As契约，不把原始错误文本塞入FailureClass。快照包含Prompt/参数/工具正文，不同于脱敏事件，消费/持久化需由宿主自行施加访问和数据策略。

### 副本与范围

Snapshot.Clone复制Messages、嵌套ToolCalls参数与Calls.Result指针；调用方可修改自己的副本，不影响另一个Run或模型原响应。字符串沿Go不可变值语义。旧Run共用状态记录路径，有记录/复制分配开销，不宣称零成本。

新API只返回最终内存副本，无运行中读接口、写盘、版本化存储、恢复或重放入口。不能通过修改Snapshot.Status重新启动Run，也不能把Go对象可序列化理解成可安全恢复。MCP会话/连接状态不包含在快照中，权限和后端副作用仍是独立边界。

### 真实验收

state_test.go覆盖迁移守卫、受控工具错误与Run完成并存、协议/授权/模型/取消/步数终止、部分批次结果保留、快照深拷贝和同一Agent的8个并发Run隔离。官方Go1.22.2下目标测试与累计普通/race测试通过；cmd/state实际运行成功。所有旧7场景集成测试保持通过。真实模型、持久Checkpoint、跨进程恢复及exactly-once不在本日验证范围。
