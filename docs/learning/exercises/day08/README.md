# Day 08 图解与 TDD 样板附件

先读 [Day 08 学习页面](../../agent-runtime-day-08.html)。页面已内嵌下面三个文件的完整内容，可离线阅读和复制。

| 附件 | 练习目标路径 | 内容 |
| --- | --- | --- |
| contract.go.txt | internal/tool/contract.go | 类型、接口与兼容适配骨架 |
| registry.go.txt | internal/tool/registry.go | 注册表及查询骨架 |
| day08_contract_test.go.txt | internal/tool/day08_contract_test.go | 8 个完整行为测试及所有夹具 |

附件用 `.txt` 后缀保存，避免尚未实现的骨架混入仓库的 `go test ./...`。应用时使用右侧真实 `.go` 文件名。不要把新定义追加到同名旧定义之后；已有代码先保留，页面提供独立工作树练习步骤。

共用仓库根 go.mod，不建立每日独立 module。复用 internal/model/model.go 与 internal/tool/tool.go，测试只使用标准库，指向学习者 internal/tool。参考答案继续位于 examples/internal/tool；页面内的完整答案将 `/examples/internal/` import 前缀映射为 `/internal/`，不混用两套 model 类型。

## 发布验证记录

日期：2026-11-09（Asia/Shanghai）。Go 1.22.2 linux/amd64。

验证在临时仓库副本中完成，没有修改当前仓库 internal 或 cmd。保留副本中所有前序/后续文件，仅替换本日两个实现文件并加入完整测试。

- **骨架编译**：`go test ./internal/tool -run '^$'` 退出码 0。
- **预期红灯**：`go test ./internal/tool -run '^TestDay08' -count=1` 退出码 1，TestDay08RegisterLookup 停在 `TODO: NewRegistry`；不是编译失败。
- **参考绿灯**：将 examples/internal/tool 的 contract.go 和 registry.go 复制到验证副本的练习路径，仅映射 import 前缀，原样运行同一测试；`go test -race ./internal/tool -run '^TestDay08' -count=1` 退出码 0。
- **分步依赖**：按页面五步逐步替换参考方法体，其余方法维持 TODO；五组目标测试分别退出码 0，确认早期测试不依赖后续实现。
- **累计回归**：兼容参考实现的临时副本中 `go test ./...` 退出码 0；当前仓库 `go test ./examples/...`、`go test -race ./examples/...` 均退出码 0。
- **离线演示**：`go run ./examples/cmd/knowledge` 退出码 0，打印可见工具数量 2 和预设结束文本。
- **页面检查**：Chromium，1280px / 390px；断网模式以本地响应注入载入页面，未请求外网。正文无横向溢出；两幅 CSS 图正常；完整测试与附件逐字一致；一键复制、键盘展开/收起、清单刷新后保存均通过；无 JavaScript 错误。

分步测试采用页面五组精确正则；临时副本只补上截至该步应完成的方法，其余保留 panic。并发测试使用独立 fake；注册表锁并不让任意业务工具自动具有并发安全性。

以上证明教学材料相互兼容及参考基线通过。没有验证或代写学习者当前实现，也没有验证真实模型、网络资料库或完整 JSON Schema 校验。
