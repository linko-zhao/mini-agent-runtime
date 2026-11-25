# Agent Runtime 累计示例

这是依据 Day 01–26 学习计划持续演进的可运行参考基线。后续课程在这里逐步扩展同一套 Runtime。它不代表学习者已完成对应练习。

共享仓库根目录的 `go.mod`（Go 1.22.2），无需新建 Module，也不导入根目录 `internal` 中的练习实现。所有命令在仓库根目录运行：

```sh
go run ./examples/cmd/demo
go run ./examples/cmd/knowledge
go run ./examples/cmd/access
go run ./examples/cmd/mcp
go run ./examples/cmd/mcpagent
go run ./examples/cmd/state
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
| Day 15 | agent/state.go、RunWithState、cmd/state | Run/调用状态、终态快照、部分结果与深拷贝 |

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

## Day15：Run状态与观测快照

新增 agent/state.go、state_test.go 与 cmd/state；修改agent.go和executor.go，将Step、Messages和调用记录集中在每次Run私有record中。原Run(ctx,input)(string,error)、New、工具/消息/事件形状保持，旧executeBatch私有入口仍委托同一实现；新增RunWithState(ctx,input)(RunResult,error)。

Run状态为created→running→completed/failed/cancelled；终态不可重启。调用状态planned→running→returned/errored/unconfirmed，预检查或未开始的后续调用为blocked。这些是Runtime观测，不代表远端事务提交/回滚。

RunResult包含Output与独立RunSnapshot；Snapshot.Clone复制消息、ToolCalls参数及CallRecord.Result指针。Messages只描述Loop已接纳的历史；部分批次失败时，已返回工具结果保留在Calls.Result中，即使它们未进入下一轮模型。取消后的已开始调用为unconfirmed，尚未开始为blocked，不能据此自动重放。

本日没有运行中状态查询、存储/Checkpoint或恢复入口。每次执行新建随机RunID；重跑同一输入是新的执行，不是恢复。快照包含用户输入、参数和结果，不作为公开审计事件；RunID也不作为业务幂等键。

官方Go1.22.2下，状态目标测试、go test ./examples/...、go test -race ./examples/... 均通过；go run ./examples/cmd/state实际输出：
- doc-1: run=completed call=returned messages=4
- missing: run=completed call=errored messages=4

参考基线继续覆盖Day14的7场景矩阵。并发状态测试使用无状态模型和工具，不表示可并发共享带游标的Mock或不安全后端。

## Day 09–15：图解、骨架与完整测试

[课程首页](../index.html) 已更新 Day 09–15；练习附件、隔离准备与可重跑验证见 [交付说明](../docs/learning/exercises/README.md)。练习测试指向学习者 internal，参考仍在 examples，两套 model 类型不混用。

Day 09 的批次授权段提取为私有 authorizeBatch 插槽，Day 11 使用同一位置练习，公开 API 不变。Day 14 新增 internal/assembly 的宿主 Build 与七场景验收；原 integration/week02_test.go 保留。Day 15 新增 internal/runstate 的纯状态转换和快照测试；既有 Agent.Run/RunWithState 及观测快照继续保留，也没有新增持久化恢复能力。

## Day16：运行上下文与状态更新

新增 `examples/internal/runstate/run.go` 与 `run_test.go`。NewRun(ctx,runID,input,maxSteps) 创建每次运行独立的状态所有者；Context 原样返回 ctx，Snapshot 深拷贝当前状态，Apply 复用 Day15 Advance 校验并提交单个事件。没有更换旧模型、工具或消息类型，没有新建 module。

非终态的 Apply 观察到 ctx.Err() 时，提交 Cancel 并返回原 context 错误；拒收当前迟到事件。已是终态则返回 ErrTransition，保持结果。Snapshot 不推进状态；显式 Cancel 事件不会调用宿主 cancel。单事件串行不等于整个 I/O 链互斥，宿主仍需按顺序驱动，在 Apply 外执行 I/O。

本课没有把纯状态机接入 agent.RunWithState；旧观测 API 与 Step 含义全部保留。参考与练习仅映射 examples/internal 到 internal，不混用包类型。页面先提供可编译 TODO 和完整行为测试，答案在最后折叠。

Go1.22.2 下原样骨架编译通过、目标在 TODO 处按预期失败，同一套测试对参考实现通过含 race。当前累计 `go test ./examples/...` 和 `go test -race ./examples/...` 均通过；学习者原实现、真实网络及持久恢复未验证。原始材料验证记录见 [Day16](../docs/learning/exercises/day16/verification.json)。

## Day17：Checkpoint边界与快照

新增 `examples/internal/runstate/checkpoint.go`、`checkpoint_test.go`。保留Day15 State/Event/Advance与Day16 Run：Checkpoint()只在Ready、ToolsPending或终态返回带标签的独立候选；ModelRunning/ToolsRunning返回零值及ErrCheckpointBoundary。Clone复用既有深拷贝，分类和拷贝在同一次Run锁内读取。

捕获只读，不采样ctx、不增加轮次、不暂停驱动、不写入存储。候选产生后工具仍可能被宿主执行，旧候选不是未执行证明或恢复许可。终态归档保留Pending，不自动重开；Ready也不意味着模型预算尚余。没有修改旧Agent.RunWithState或其Step含义。

Day17练习支持与页面全文包含可编译TODO、完整行为测试和折叠答案；prepare/verify日期范围扩展至17，固定baseline不变。所有类型共享原go.mod，不修改学习者internal/cmd。参见 [Day17页面](../docs/learning/agent-runtime-day-17.html)。

本次Go1.22.2验证：骨架编译通过、目标在TODO处按预期失败；原样测试对隔离internal兼容参考通过含race，累计兼容副本与当前examples普通/race回归通过。原始记录见 [Day17验证](../docs/learning/exercises/day17/verification.json)。未验证真实模型、外部后端或持久恢复。

## Day18：Checkpoint存储与版本控制

新增 examples/internal/runstate/store.go、store_test.go：CheckpointStore.Load/Save、StoredCheckpoint、MemoryCheckpointStore。复用Day17 Checkpoint/Clone，不改State、Run、消息协议或旧Agent接口。

每个精确RunID单独计数；expected=0只创建，其他值须匹配当前Version；成功递增，即使候选正文相同。冲突不覆盖；Load/Save返回独立深拷贝。零值内存Store可用，不可复制已使用对象；新实例为空，只有最新记录，无持久性或跨进程共享。

输入校验只覆盖可信候选的基本信封，不是完整历史/真实性校验。nilctx/空键/非法边界和预算/版本溢出被拒绝；观察到取消或截止时间保留context错误。存储ctx与Runctx独立，可归档取消终态。CAS不是执行租约、模型轮次、格式版本或幂等键；没有Restore/Resume或工具自动重放。

原样TODO骨架可编译，目标在TODO处失败；相同测试在隔离internal参考副本通过含race，兼容副本累计回归通过；当前仓库根examples普通/race均通过（Go1.22.2）。[Day18页面](../docs/learning/agent-runtime-day-18.html)、[验证记录](../docs/learning/exercises/day18/verification.json)。学习者原实现、真实持久后端/网络、崩溃恢复未验证。

## Day19：恢复位置与待确认调用

新增examples/internal/runstate/recovery.go与recovery_test.go；PlanRecovery(ctx,CheckpointStore,runID)仅Load一次、局部检查、分类并返回独立RecoveryPlan{Position,Record}。不Save、不Apply、不执行模型/工具，不创建恢复Run。

位置为recorded_ready、review_pending、terminal、budget_exhausted。保留完整StoredCheckpoint及观察到的Version；ToolsPending的原调用需要外部确认，终态Pending保留供诊断，均不授权自动重放。Ready也仅描述记录，不证明它仍新鲜或当前拥有执行权。

检查身份/版本/Day18信封、首条user、Ready无Pending/末消息无调用、Pending与最后assistant请求按ID/名称/参数/顺序匹配、Completed结果一致。它不是完整历史重放、可信反序列化或外部效果验证。预取消不Load，Load后观察到取消则拒收迟到结果；其他Load错误保持原因链，无重试。

共享根go.mod，复用Day15–18所有接口；不修改学习者internal/cmd。原样骨架可编译、TODO红灯符合预期、相同测试对隔离internal参考通过含race；当前examples普通/race与兼容副本累计回归通过（Go1.22.2）。[Day19页面](../docs/learning/agent-runtime-day-19.html)、[验证记录](../docs/learning/exercises/day19/verification.json)。没有验证真实持久存储、真实模型/网络、租约或恢复执行。

## Day20：重放与幂等边界

新增examples/internal/runstate/replay.go与replay_test.go。PlanReplay接收Day19 ReviewPending计划及可信宿主证据，纯决策生成逐项Hold/Invoke/Reuse与整批Blocked；不Load/Save/Apply，不执行工具或恢复Run。

依据为Unknown、NotStarted、ResultKnown、BackendDedup。Unknown使整批Blocked；已确认结果复用；肯定未开始证据或外部核验的后端去重契约才产生Invoke建议。单有键、缺少结果、MCP注解或readOnly不构成证明。未阻塞也没有授予执行归属或权限；现有Tool接口尚无通用键传递适配，不可丢弃键后执行。

证据绑定RunID、Version与原Pending的顺序/ID/名称/参数；附加Result/键须匹配依据，重复后端键拒绝。计划Record、Steps和输入深拷贝隔离；取消保留原context错误。所有旧接口、消息和根go.mod保留，不修改学习者internal/cmd。

Go1.22.2最终原样骨架编译通过、TODO红灯符合预期；同一套测试对隔离internal参考通过含race，兼容副本累计回归通过；仓库根examples普通/race通过。[Day20页面](../docs/learning/agent-runtime-day-20.html)、[原始验证](../docs/learning/exercises/day20/verification.json)。未验证真实后端去重、幂等键传输、持久性、执行租约或自动恢复。

## Day21：第三周集成复盘

新增examples/internal/assembly/week03.go与week03_test.go。ReviewWeek03(ctx,store,source,runID)把Day19 PlanRecovery、按需Evidence与Day20 PlanReplay接成只读宿主链；原Build/Agent/Run/Store接口不变。

Ready、预算耗尽与终态只返回Recovery，Replay=nil且不查询Source；ReviewPending要求ReplayEvidenceSource，传原ctx和记录副本，取得一次证据后再决策。依赖错误保留原因链，所有失败返回零Week03Report；取消拒收迟到证据。审查不Save、Apply或执行工具。

8个矩阵子场景覆盖正常位置、终态Pending、未知阻塞、结果/去重与未开始建议；另测依赖故障、旧证据/非法记录、取消、端口输入副本及16个并发审查。Go1.22.2下原样骨架编译通过、TODO红灯符合预期；同一测试对隔离internal参考通过含race，兼容副本及当前examples普通/race累计回归通过。

[Day21页面](../docs/learning/agent-runtime-day-21.html)、[原始验证](../docs/learning/exercises/day21/verification.json)。没有验证真实持久存储、证据服务、后端去重/键传输、执行租约或自动恢复；学习者原internal/cmd不读不改。Day22按第四周路线继续中断/审批。

## Day22：内存暂停门禁

新增 internal/runstate/pause.go、pause_test.go。PauseGate 仅在 ToolsPending 建立暂停；所有推进必须经同一门禁，宿主只在 StartTools 成功后执行工具。原 ctx 取消经 Apply 观察并清除请求，不新增 Phase，也没有审批/继续/持久化。详见 CONTRACTS.md 和 [Day22](../docs/learning/agent-runtime-day-22.html)。运行仓库根 go test ./examples/... 与 go test -race ./examples/...；发布材料原始结果见 exercises/day22/verification.json。

## Day23：审批决定与请求关联

新增 internal/runstate/approval.go、approval_test.go；pause.go 增加私有决定与终态清理。Decide只记录匹配RunID/RequestID的首份可信宿主决定，CurrentDecision只读。批准和拒绝都仍暂停，不执行工具、不写Store，不提供身份/权限证明或恢复。旧接口保留，完整迁移与错误顺序见CONTRACTS.md。[Day23页面](../docs/learning/agent-runtime-day-23.html)提供骨架和完整测试。

在仓库根验收 go test ./examples/... 与 go test -race ./examples/...；实际材料测试记录在 docs/learning/exercises/day23/verification.json。未验证学习者原实现、真实审批服务、持久恢复或真实工具后端。

## Day24：人工决定与继续路径

新增 internal/runstate/resolve.go、resolve_test.go，复用Day22/23门禁和决定。ResolveDecision消费目标匹配的已记录决定，批准推进ToolsRunning、拒绝推进Failed；I/O仍在宿主锁外执行，必须检查本次nil错误且Phase=ToolsRunning。没有新Phase/Event或旧签名迁移，不写Store。完整契约与调用方调整见CONTRACTS.md及[Day24页面](../docs/learning/agent-runtime-day-24.html)。

从仓库根运行 go test ./examples/... 与 go test -race ./examples/...；实际材料验证见 docs/learning/exercises/day24/verification.json。内存一次消费不保证持久恢复或真实工具exactly-once；学习者原实现未验证。

## Day25：中断状态与保存边界

新增internal/runstate/pause_snapshot.go、pause_snapshot_test.go。PauseSnapshot在同一门禁锁内捕获请求与可选决定；SchemaVersion=1，Clone与Gob编解码保持消息/参数所有权和参数字节内容。只交付候选数据，不调用Store或恢复执行；格式版本不等于CAS版本。完整规则与旧数据映射见CONTRACTS.md和[Day25页面](../docs/learning/agent-runtime-day-25.html)。

从仓库根运行go test ./examples/...与go test -race ./examples/...；材料真实记录见docs/learning/exercises/day25/verification.json。编解码仅用于受信宿主数据，不保证来源、新鲜度、保存确认或持久恢复；学习者原实现未验证。

## Day26：故障与外部执行确认

新增internal/runstate/outcome.go、outcome_test.go。InspectToolOutcomes接收ToolsRunning观察，逐项只读Lookup并核对RunID/Step/Call原字节身份；复用ReplayEvidence，只有整批ResultKnown才返回完整Results。Unknown/NotStarted不重试，来源失败保留错误链，不Apply/Save/Invoke。旧门禁、快照、重放策略不改。完整契约见CONTRACTS.md和[Day26页面](../docs/learning/agent-runtime-day-26.html)。

在仓库根运行go test ./examples/...与go test -race ./examples/...；材料真实结果见docs/learning/exercises/day26/verification.json。来源为离线fake，未验证真实台账、模型/工具后端、真实性、新鲜度、持久恢复或学习者实现。
