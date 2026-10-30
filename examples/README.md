# Agent Runtime 累计示例

这是依据 Day 01–09 已发布学习计划整理的可运行参考基线。后续课程在这里逐步扩展同一套 Runtime。它不代表学习者已完成对应练习。

共享仓库根目录的 `go.mod`（Go 1.22.2），无需新建 Module，也不导入根目录 `internal` 中的练习实现。所有命令在仓库根目录运行：

```sh
go run ./examples/cmd/demo
go test ./examples/...
go test -race ./examples/...
```

Demo 不联网、不需要 API Key，输出 `计算结果为 2`。测试使用标准库；模型 HTTP 测试注入 RoundTripper，不连接真实 Provider 或监听端口。

```text
examples/
  cmd/demo/main.go                 # Model → Tool → Model 离线入口
  internal/model/                 # 消息、响应、Mock、Model/ToolBinder
    openai/                       # HTTP 适配、请求/响应转换、错误分类
  internal/tool/                  # Tool、DescribedTool、受控错误
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
| Day 08–09 | tool/calculator、agent.go | 参数边界、整批名称/ID 检查、结果关联 |

示例覆盖已发布页面的核心契约，不将近期调整后的第 2 周路线误称为已实现：独立 ToolsNode 执行器、中间件、重试/幂等、权限与 MCP 留给 Day 10–14 按路线增量设计。Eino 是阅读对照，不作为运行依赖。

Provider 示例构造函数为 `openai.NewChatModel(*ChatModelConfig)`；`BaseURL` 传完整 Chat Completions endpoint，HTTPClient 可注入。真实调用由使用者自行配置 API Key 和超时；本次仅验证模拟 HTTP，不宣称真实 API 已验证。工具 Schema 与实际校验需同步维护；当前 JSON 解码采用 Go 标准库字段匹配规则，不额外拒绝重复键或大小写变体。
