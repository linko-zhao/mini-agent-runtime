# Day 08 · 工具契约与 Eino 对照

| 课程接口 | Eino 阅读符号 | 职责 |
| --- | --- | --- |
| Spec / Described.Meta | schema.ToolInfo / BaseTool.Info | 描述能力，供模型选择，不执行外部操作 |
| Invokable.Invoke | InvokableTool.InvokableRun | context + JSON 参数进入本地实现 |
| 后续流式能力 | StreamableTool.StreamableRun | 本节仅阅读，未来增加独立能力接口并说明关闭责任 |
| Registry / Specs | ToolsNodeConfig.Tools 的配置边界 | 注册能力与模型可见描述分开 |
| model.ToolCall / tool.Call | assistant ToolCalls | 调用 ID 属于 Runtime 协议，不是工具输入 schema 的字段 |

阅读入口：[components/tool/interface.go](https://github.com/cloudwego/eino/blob/main/components/tool/interface.go)、[compose/tool_node.go](https://github.com/cloudwego/eino/blob/main/compose/tool_node.go)。这是一组设计对应，不宣称 API 兼容，也不引入 Eino 依赖。

## 连续演进与接入

Day 01 的 `Tool.Execute` 和 Day 03 的 `DescribedTool.Definition` 继续供现有 Loop 使用。Day 08 新工具实现 `Meta/Invoke`；`Registry.Register` 快照元信息并拒绝重复，`Registry.Tools()` 把新能力适配给原来的 `agent.New`。也可使用 `Adapt(ctx, t)` 单独适配。无需更换消息类型或重建 Module。

数据流：Store → SearchKnowledge/GetDocument → Registry.Register → Specs(本轮可见名称) → ToolDefinition → Model.WithTools → assistant ToolCall → Runtime 查完整注册表 → Invoke → ToolCallID 匹配的 tool 消息 → Model。离线示例见 `examples/cmd/knowledge/main.go`，脚本模型依次检索、读取、返回文本，不声称进行了真实模型推理。

`Meta` 的 context 只用于元信息发现，不能保留为后续调用的执行 context。适配层把每次 Execute 收到的 context 原样传给 Invoke；Store 也收到同一个 context。Specs/Definition 返回 schema 副本；注册后修改元信息不会偷偷改变模型契约，需要显式重建注册表。

## 责任边界

- Store 适配层持有真实后端访问与鉴权逻辑，凭据不放进 Spec。FakeStore 仅供演示，不提供生产鉴权。
- Runtime 管理超时、取消和调用关联；工具/Store 必须合作式遵守 context。结果预算留给 Day 10；当前示例不承诺生产结果大小限制。
- 模型可见集合用于选择能力，不授予或撤销执行权限。隐藏能力仍在 Registry 中；Day 11 将新增执行授权检查，不能把过滤描述当作权限边界。
- 目前 Registry 使用同步调用，不提供流式结果；不为尚未实现的能力放置空壳接口。
