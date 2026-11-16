# Mini Agent Runtime

一个用 Go 学习和实现 Agent Runtime 的项目，从最小的「模型 → 工具 → 模型」循环出发，逐步加入工具注册、执行策略、授权、MCP 和运行状态。

仓库同时提供可运行代码、累计参考示例，以及配套的中文图解与 TDD 练习。学习路线共 8 周、56 天，目前课程页面已覆盖 Day 01–16；后续主题以路线文档为准。

## 核心能力

- **Agent Loop**：统一模型消息与工具调用，按顺序执行工具，将结果关联到原始调用 ID，并限制最大模型轮次。
- **模型适配**：提供脚本式 Mock 和兼容 OpenAI Chat Completions 的非流式 HTTP 适配器，支持工具描述绑定。
- **工具体系**：支持工具元信息、注册表、可见集合，以及计算器、知识检索和文档读取示例。
- **执行控制**：支持上下文取消、单工具超时、执行事件，以及工具策略包装中的超时、结果字节预算和显式只读重试。
- **执行授权**：通过可信主体和能力快照检查整批工具调用，并记录授权决策事件。
- **MCP 教学实现**：包含初始化、工具发现、调用及 Runtime 工具适配，示例使用内存 JSON-RPC 传输。

根目录的 `internal/`、`cmd/` 用于学习者累计实现；`examples/` 是独立的累计参考基线，两者共享根目录 `go.mod`。参考基线还包含 `RunWithState` 和运行快照示例，不能据此认为根目录实现已具备相同能力。两套实现中的 `model` 等类型也不能混用。

## 快速开始

### 环境要求

- Go **1.22.2 或更高版本**。
- Python **3.9 或更高版本**：仅在准备隔离练习副本时需要。
- 离线示例无需 API Key；首次下载 Go 依赖通常需要联网。

以下命令均从仓库根目录执行：

```sh
go mod download

# 强制使用 Mock 模型运行根目录 demo
OPENAI_API_KEY= go run ./cmd/demo
```

预期输出：

```text
计算结果为 2
```

该示例先由 Mock 模型请求计算 `1 + 1`，Runtime 执行计算器后，将结果送回模型取得最终回答。

### 运行其他示例

| 命令 | 演示内容 |
| --- | --- |
| `go run ./cmd/knowledge` | 工具注册、知识检索与文档读取，附带执行策略和读取授权 |
| `go run ./cmd/access` | 拒绝读取主体发起的写工具调用，输出写入次数为 `0` |
| `go run ./cmd/mcp` | 内存 MCP 会话的初始化、工具发现与调用 |
| `go run ./cmd/mcpagent` | 将 MCP 工具适配为本地别名，完成 Agent 工具往返 |

上述四个示例均使用离线夹具，不连接真实模型或 MCP 网络服务。

若希望先体验累计参考实现，可执行：

```sh
go run ./examples/cmd/demo
go run ./examples/cmd/knowledge
go run ./examples/cmd/access
go run ./examples/cmd/mcp
go run ./examples/cmd/mcpagent
go run ./examples/cmd/state
```

其中 `state` 示例展示成功调用与受控工具错误对应的运行状态和快照。完整说明见 [累计示例文档](examples/README.md)。

## 接入真实模型

根目录 `cmd/demo` 根据 `OPENAI_API_KEY` 是否为空选择模型：为空时使用 Mock；非空时使用 HTTP 适配器，此时下面三个变量都必须配置。

```sh
export OPENAI_API_KEY='你的 API Key'
export OPENAI_BASE_URL='https://api.openai.com/v1/chat/completions'
export OPENAI_MODEL='你的模型名称'

go run ./cmd/demo
```

`OPENAI_BASE_URL` 必须是**完整的 Chat Completions 请求地址**，适配器不会自动拼接路径。模型与服务需要支持相应的消息和工具调用格式。当前 demo 的用户输入固定为 `Hi`，真实模型输出取决于服务响应，不保证与 Mock 的固定回答一致。

自行装配模型时，可通过 `openai.ChatModelConfig.HTTPClient` 注入带超时的 HTTP 客户端，也可通过调用上下文设置截止时间。默认 HTTP 客户端没有配置整体超时。

## 目录结构

```text
.
├── cmd/                        # 根目录实现的可运行入口
│   ├── demo/                   # 最小模型与计算器闭环
│   ├── knowledge/              # 知识工具、策略与授权
│   ├── access/                 # 授权拒绝示例
│   ├── mcp/                    # MCP 协议示例
│   └── mcpagent/               # MCP 工具接入 Agent
├── internal/                   # 学习者累计实现
│   ├── agent/                  # Loop、批次执行、选项与事件
│   ├── model/                  # 模型接口、Mock 与 OpenAI 适配器
│   ├── tool/                   # 工具契约、注册表、策略与授权
│   └── mcp/                    # MCP 客户端与内存服务夹具
├── examples/                   # 独立的累计参考实现
│   ├── cmd/                    # 参考示例入口，包含 state
│   ├── internal/               # 参考实现与配套测试
│   ├── integration/            # 跨组件故障矩阵
│   └── CONTRACTS.md            # 参考实现契约与迁移规则
├── docs/learning/              # 学习路线与每日 HTML 课程
│   └── exercises/              # 练习骨架、测试、答案与准备脚本
├── index.html                  # 中文课程导航
└── go.mod                      # 全仓库共享的 Go Module
```

## 学习路径

用浏览器打开根目录的 [课程导航](index.html)，按章节阅读图解、行为契约、骨架和测试，再动手实现。课程正文和代码可离线阅读，外部源码链接需要联网。

| 章节 | 主题 |
| --- | --- |
| Day 01–07 | 最小 Loop、模型适配、工具绑定、取消与超时、事件和集成复盘 |
| Day 08–14 | 工具契约、批次执行、执行策略、授权、MCP 与跨组件验收 |
| Day 15 | 纯内存状态机、合法转换与快照 |
| Day 16 | 运行上下文、状态所有权、取消边界与并发副本 |
| Day 17–56（规划） | Checkpoint、恢复、记忆、调度、评估与部署 |

推荐通过准备脚本创建隔离练习副本，例如：

```sh
python3 docs/learning/exercises/prepare.py 09
```

脚本会打印新副本的绝对路径；进入该目录后，按当天页面运行目标测试并实现骨架。准备过程不会修改原仓库的 `internal/` 和 `cmd/`。更多用法见 [练习交付说明](docs/learning/exercises/README.md)，完整路线见 [56 天学习计划](docs/learning/agent-runtime-56-day-schedule.md)。

## 测试与验证

```sh
# 全仓库测试，覆盖根目录实现与 examples
go test ./...

# 累计参考实现及其并发检查
go test ./examples/...
go test -race ./examples/...

# 第二周跨组件集成故障矩阵
go test ./examples/integration -run TestWeek02EndToEndMatrix -v
```

竞态检查需要环境支持 Go race detector 及相应的 C 工具链。测试主要使用 Mock、FakeStore、注入的 HTTP 传输和内存 MCP 消息；通过这些测试不代表真实模型或真实 MCP 网络服务已验证。故障矩阵覆盖范围见 [集成验收说明](examples/integration/README.md)。

## 实现边界

- 这是逐步演进的学习项目。持久化 Checkpoint、跨进程恢复和部署等属于后续路线，当前运行快照不提供恢复能力。
- 工具可见集合仅控制模型描述；需要执行权限时，应显式配置 `WithAuthorizer`，并由可信宿主注入主体。真实业务后端仍需资源级鉴权。
- MCP 实现是固定协议版本 `2025-06-18` 的串行教学子集，尚未提供真实 stdio、Streamable HTTP 或 OAuth 传输。
- 取消和超时依赖模型、工具及传输配合，不能撤销已经发生的外部操作；只读重试策略也不提供持久化幂等保证。

扩展累计参考实现前，请先阅读 [参考实现契约](examples/CONTRACTS.md)，确认接口、错误语义和迁移约定。
