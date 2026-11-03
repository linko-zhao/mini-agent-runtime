# Day14 · 第二周跨组件集成验收

本目录是累计课程的集成练习，不定义新的Runtime公共API。入口为 week02_test.go / TestWeek02EndToEndMatrix；共享根go.mod，复用 examples/internal 下的模型、工具、授权、MCP会话和适配器。测试私有计数器/脚本模型只是夹具，不是另一套业务工具或推理引擎。

## 场景与装配

用户已经知道local-1和remote-1两个文档ID，需要比较两份正文，因此模型可在同一批请求独立读取；若ID来自前次检索，应放到后续模型轮次。

本地GetDocument由Registry直接适配；远端GetDocument经内存DemoServer、Client.Initialize、mcptool.Discover后注册为knowledge__get_document。两者再经过WithPolicy、WithVisibleTools、WithAuthorizer和同一Agent.Run。两个Store独立，全部离线，未启动真实网络服务。

模型第二轮必须收到原assistant.ToolCalls与两条正确ID的tool消息。本地结果继续是Document JSON，远端结果继续是Day13 ResultEnvelope；不为了集成静默改变旧接口或输出格式。

## 先预测，再运行

从仓库根执行：

```sh
go test ./examples/integration -run TestWeek02EndToEndMatrix -v
go test ./examples/...
go test -race ./examples/...
```

先盖住测试期望字段，手填计数，再与-v日志对照。tools/call是传输尝试计数，remoteInvoke是业务Invoke进入次数，不代表成功完成或外部副作用提交。

| 场景 | 本地Invoke | tools/call尝试 | 远端Invoke | 模型调用 | Run终态 |
| --- | ---: | ---: | ---: | ---: | --- |
| mixed-success | 1 | 1 | 1 | 2 | Finished |
| unapproved-remote | 0 | 0 | 0 | 1 | Failed |
| duplicate-model-ID | 0 | 0 | 0 | 1 | Failed |
| remote-result-budget | 1 | 1 | 1 | 2 | Finished，远端tool受控错误 |
| remote-business-failure | 1 | 1 | 1 | 2 | Finished，远端tool受控错误 |
| corrupt-response-after-execution | 1 | 1 | 1 | 2 | Finished，受控错误且MCP客户端failed |
| remote-timeout | 1 | 1 | 0 | 1 | Failed，DeadlineExceeded |

超时夹具故意在转发到Server前等待ctx.Done，所以该行远端Invoke为0；不能据此推断真实网络超时一定未执行。损坏响应夹具则先执行Server，再返回错误ID，以展示“收到错误不撤销已经发生的调用”。本场景都是读取，不宣称验证外部写入回滚或事务。

## 验收的不变量

1. 模型描述绑定本地原名与远端别名，远端协议仍请求原名。
2. 模型CallID与JSON-RPC整数ID分域，Principal不混进工具arguments。
3. 整批授权/模型ID检查发生在任何工具开始前；第一项允许的审计决策不意味着它已经执行。
4. 所有Run只有一个终态，已开始工具都有completed；错误/取消状态保持原契约。
5. 受控工具失败可反馈模型，RunFinished只表示本次Run返回文本；业务目标是否完成要另看结果证据。脚本最终文本不是质量证明。
6. 本地与远端结果分别按已有格式解析，错误摘要不含原始文档/私有报文。
7. MCP会话失败后不得隐式重放；当前Run状态和会话状态可以不同。

## 设计复盘

按“需求变化 → 改动层 → 保持的契约 → 新断言”写三条设计记录，例如：新增第二个MCP Server、按租户限制文档、要求远端失败就终止Run。不得用修改提示词代替执行边界的改动，也不假定Eino默认错误/并行策略等于本课程。

下一周从Run状态与恢复边界开始：目前messages是Run局部状态；若进程在外部请求后丢失响应，单纯重跑Loop可能重复操作。Checkpoint需描述可恢复状态和不确定调用，不是事务回滚或exactly-once开关。本日不新增状态机/Checkpoint代码。

## 本日真实结果

官方Go1.22.2 linux/amd64下，目标矩阵7个子场景全部通过；go test ./examples/... 与 go test -race ./examples/... 均退出码0。结果覆盖内存JSON-RPC、FakeStore与脚本模型，不代表生产HTTP/stdio、OAuth、真实LLM、持久审计或跨进程恢复已验证。
