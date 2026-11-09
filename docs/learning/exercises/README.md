# Day 09–15：图解与 TDD 学习材料

每一页都按“具体问题 → 概念 → 两张图 → 设计理由 → 行为契约 → 完整骨架 → 完整测试 → 分步实现 → 复盘 → 折叠答案”组织。正文、图示、代码和测试可离线阅读；官方源码链接需要联网。

| 日期 | 练习模块 | 练习目标 |
| --- | --- | --- |
| [Day 09](../agent-runtime-day-09.html) | 批次执行 | 整批预检查、顺序调用、原 ID 回填与取消 |
| [Day 10](../agent-runtime-day-10.html) | 执行策略包装 | 总超时、显式只读重试、参数副本和结果预算 |
| [Day 11](../agent-runtime-day-11.html) | 工具级授权 | 可信主体、不可变能力快照、整批授权与审计 |
| [Day 12](../agent-runtime-day-12.html) | MCP Client | 握手、能力门禁、JSON-RPC、发现/调用与失败会话 |
| [Day 13](../agent-runtime-day-13.html) | MCP Tool 适配 | 别名、完整目录发现、结果投影与安全错误 |
| [Day 14](../agent-runtime-day-14.html) | 宿主 Build 装配 | 复盘各层职责，用七场景矩阵检查真正接通 |
| [Day 15](../agent-runtime-day-15.html) | 纯内存状态机 | 明确阶段、合法事件、消息快照与终态 |

## 怎样开始

在原仓库根运行，例如：

```sh
python3 docs/learning/exercises/prepare.py 09
```

脚本会打印新练习副本的绝对路径。进入该目录后，使用页面给出的路径编辑、运行目标测试。要求 Python 3.9+、Go ≥ 1.22.2；不需要 API Key。

脚本不读学习者实现来判断进度，也不修改原仓库 internal/cmd。它在仓库外创建完整副本，用配套 examples/internal 和 examples/cmd 提供同一累计模块的前置依赖及命令入口，再把当天目标替换成 TODO 骨架；测试 import 指向副本 internal。仍使用原 go.mod，没有新的每日 Module。

可用 `--output /一个尚不存在的新目录` 指定副本位置。脚本拒绝原仓库、仓库内部、父目录和任何已存在目录，不自动清理或覆盖旧副本。

如果自己的累计实现已经兼容，可以只应用页面完整测试，保留已完成的方法；不必为了制造红灯挖空重做。手工保存时，按真实目标 `.go` 路径命名，不在同一包重复定义旧类型/方法。

附件采用 `.go.txt`，避免未实现骨架被仓库普通回归自动编译。HTML 内嵌内容与附件逐字一致，均可下载；参考答案默认折叠。`manifest.json` 记录每一天的目标、依赖、测试和参考映射。

## 前后章节怎样组合

所有练习共享原 model.Message/ToolCall/Response、工具接口、Agent.New/Run。练习使用 internal，答案使用 examples/internal，不能跨两套 model 包混用类型。

Day 09 的执行器保留 authorizeBatch 插槽。Day 11 单独实现该私有方法及 access.go；它仍处于协议校验之后、Execute 之前。旧版本需要把授权循环移到插槽中，不能重复授权。其他公共接口没有破坏性迁移。

Day 14 的 Build 是宿主装配函数，不是新 Runtime 协议。Day 15 的 runstate 是纯状态转换，未接入旧 Agent.Run，也没有实现 Checkpoint 或自动恢复。每页都明确新增、修改和复用范围。

## 发布验证与限制

日期：2026-11-09（Asia/Shanghai）。使用官方 Go 1.22.2 linux/amd64；验证在临时副本中进行，未修改学习者实现。

每一天都已验证：

1. 原样骨架与完整测试可编译。
2. 目标测试确实执行，在当天 TODO 方法处按预期失败。
3. 同一套测试不改断言、不绕到 examples，换入兼容参考实现后通过，包括 race。
4. Chromium 断网模式下注入本地 HTML，1280px 与 390px 无正文横向溢出；全部代码块与附件逐字一致。
5. 一键复制、键盘展开/收起、清单刷新后保存正常，无脚本错误或外部资源请求。

原始输出见 [红绿测试记录](verification.json) 与 [浏览器记录](browser-verification.json)。当前参考示例的普通/race 累计回归也通过；兼容参考副本的累计回归记录在 verification.json 的 cumulative 字段。

重跑代码交付验证：

```sh
python3 docs/learning/exercises/verify.py --go /你的Go工具链/bin/go --report /tmp/learning-verification.json
```

`--go` 可省略，默认使用 PATH 中的 `go`。脚本只在临时副本编译测试；`--report` 写入指定报告，不写入原实现。报告原样保留每次编译、红灯和绿灯输出。

这些证据说明材料相互兼容，不证明学习者已完成练习、真实模型会选对工具、生产网络协议/认证已验证或跨进程恢复可靠。清单只保存自评进度，不能替代设计讲解与实际测试结果。
