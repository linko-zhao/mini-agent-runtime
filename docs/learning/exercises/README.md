# Day 01–15：图解与 TDD 学习材料

每一页都按“具体问题 → 概念 → 两张图 → 设计理由 → 行为契约 → 完整骨架 → 完整测试 → 分步实现 → 复盘 → 折叠答案”组织。正文、图示、代码和测试可离线阅读；官方源码链接需要联网。

| 日期 | 练习模块 | 练习目标 |
| --- | --- | --- |
| [Day 01](../agent-runtime-day-01.html) | 最小 Loop 与脚本模型 | 模型/工具职责、消息往返、步数及基础错误 |
| [Day 02](../agent-runtime-day-02.html) | HTTP 模型适配 | 请求/历史编码、响应规范化、错误分类与注入传输 |
| [Day 03](../agent-runtime-day-03.html) | 工具说明绑定 | 不可变绑定、schema、一次装配与稳定排序 |
| [Day 04](../agent-runtime-day-04.html) | 生命周期 | 取消前后边界、单工具超时与不回滚副作用 |
| [Day 05](../agent-runtime-day-05.html) | 同步事件 | 正常顺序、错误/取消/预算的唯一终态 |
| [Day 06](../agent-runtime-day-06.html) | 共享 Sink | 唯一包装器互斥、原样转发与取消后观察 |
| [Day 07](../agent-runtime-day-07.html) | 第一周装配 | BuildWeek01 与七场景集成矩阵 |
| [Day 08](../agent-runtime-day-08.html) | 工具契约与注册表 | Meta/Invoke、快照、可见集合与旧接口桥接 |
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

脚本不读学习者实现来判断进度，也不修改原仓库 internal/cmd。它在仓库外创建完整副本。Day01–07 按日序依次叠加前序 reference/support/tests，只准备已经介绍的 internal 依赖；Day09–15 继续用 baseline/ 中固定版本的同一累计模块提供前置依赖及配套命令入口，再把当天目标替换成 TODO 骨架；测试 import 指向副本 internal。仍使用原 go.mod，没有新的每日 Module。

可用 `--output /一个尚不存在的新目录` 指定副本位置。脚本拒绝原仓库、仓库内部、父目录和任何已存在目录，不自动清理或覆盖旧副本。

如果自己的累计实现已经兼容，可以只应用页面完整测试，保留已完成的方法；不必为了制造红灯挖空重做。手工保存时，按真实目标 `.go` 路径命名，不在同一包重复定义旧类型/方法。

附件采用 `.go.txt`，避免未实现骨架被仓库普通回归自动编译。HTML 内嵌内容与附件逐字一致，均可下载；参考答案默认折叠。`manifest.json` 记录每一天的目标、依赖、测试和参考映射。

## 前后章节怎样组合

所有练习共享原 model.Message/ToolCall/Response、工具接口、Agent.New/Run。练习使用 internal，答案使用 examples/internal，不能跨两套 model 包混用类型。

Day 09 的执行器保留 authorizeBatch 插槽。Day 11 单独实现该私有方法及 access.go；它仍处于协议校验之后、Execute 之前。旧版本需要把授权循环移到插槽中，不能重复授权。其他公共接口没有破坏性迁移。

Day 14 的 Build 是宿主装配函数，不是新 Runtime 协议。Day 15 的 runstate 是纯状态转换，未接入旧 Agent.Run，也没有实现 Checkpoint 或自动恢复。每页都明确新增、修改和复用范围。

## Day01–07 发布验证与阶段衔接

日期：2026-11-10（Asia/Shanghai）。原样骨架、完整测试与兼容参考在仓库外隔离副本验证，使用官方 Go 1.22.2。结果见 [第一周红绿与累计验证](week01-verification.json)、[离线浏览器验证](week01-browser-verification.json)。每页的状态区区分材料验证与学习者实现状态。

```sh
python3 docs/learning/exercises/prepare.py 01
python3 docs/learning/exercises/verify.py --go /你的Go工具链/bin/go --days 1 2 3 4 5 6 7 --report /tmp/week01-verification.json
```

Day01 建立核心 Message/ToolCall/Model/Tool/New/Run；Day02 扩展 Response，预先提供后续绑定类型声明；Day03 才实现绑定；Day04 给 New 增加兼容的可变选项并将 Run 移到 run.go；Day05 加同步事件；Day06 只加共享 Sink 包装器；Day07 加宿主装配。每个本日测试文件自带独立命名的 fake/fixture，不依赖另一天的测试辅助函数。HTML 全文与附件逐字一致。

第一周的参考附件按当日范围裁剪，与最新 examples 的公开接口连续；不提前引入授权、状态快照等算法。Day01–08 工具错误暂时终止 Run，Day09 才在 executor 边界迁移为受控错误回填并加整批 ID 校验，后续依赖快照已包含该增量。Day08 的工具注册与旧接口桥接可直接使用 Day07 的 model/tool 类型；Day08 练习与验证入口继续沿用该页独立说明，不属于 prepare.py 的日期范围。

本批额外重跑 Day09–15 红绿验证，确认 prepare.py 扩展未破坏原流程；最新 examples 普通/race 累计回归也独立执行。真实模型、真实网络后端与学习者原实现未验证。Eino 阅读路径 components/model/interface.go、components/tool/interface.go、adk/runner.go、adk/interface.go、callbacks/interface.go 已通过 GitHub 读取确认符号；当前 BaseChatModel、Runner、AgentEvent 和 Handler 包含兼容别名，框架不是本课程运行依赖。

## Day09–15 历史发布验证与限制

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

## 与最新 main 的衔接

main 在本批讲义准备期间新增了 agent.RunWithState、RunSnapshot 与调用状态记录，相关实现及测试全部保留。旧版 Day 15 的运行快照说明另存为 [观测快照参考](../agent-runtime-day-15-snapshot-reference.html)。新版 Day 15 先练习纯转换；该转换模型与实际运行观测属于不同层次，不互相替换，也不代表恢复协议。

`baseline/` 是全课程共用的教学依赖快照，基于 01c9ca6 及本批授权插槽、装配与纯状态模块增量；包路径均已映射为 internal。它固定早期练习的 New/Run/executeBatch 契约，防止后续 main 的可选观测接口让旧骨架无法编译。Day09–15 复用这份基线；Day01–07 使用同一模块按阶段裁剪的前序依赖，所有日期共享原 go.mod，没有每日独立 Module。升级教学基线时必须同时迁移骨架、调用点与完整测试，再重新执行红绿验证；不得用它覆盖真实项目。
