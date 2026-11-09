# 共享教学依赖快照

基于 01c9ca6 及本批 Day 09 授权插槽、Day 14 装配、Day 15 纯状态增量；用于隔离练习，不替换 main 的实际代码。固定 New/Run 与批次接口，后续章节共享同一份，不建立新 Module。

所有 .go.txt 已映射到学习者 internal 路径。prepare.py 只把它们写入新建的仓库外副本，再应用当天 TODO 和完整测试。当前 main 的 RunWithState/RunSnapshot 观测实现保留在 examples；与纯转换模型的区别见上级 README。
