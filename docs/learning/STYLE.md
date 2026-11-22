# 学习页面固定样式

所有学习页共用 [assets/learning.css](assets/learning.css)，页面中的样式规则统一维护在这一个文件里。页面内容可以采用各自需要的 HTML 结构；共享样式固定颜色、字号、行高、内容宽度、卡片、代码块、响应式布局和底部导航。

新建学习页时，从 [page-template.html](page-template.html) 开始，并在 `<head>` 中引用共享样式：

```html
<link rel="stylesheet" href="assets/learning.css">
```

不要在学习页中加入内嵌 `<style>` 或重复定义共享规则。需要新的通用视觉组件时，扩展共享样式表；正文只使用已有组件类名。

页面底部使用统一导航类，并按日期填写相邻页面链接。创建 Day N 时，同时把 Day N-1 的“下一项”指向新页面，并把新页面的“下一项”设为 Day N+1，即使该文件尚未创建。不要仅因下一页文件暂不存在就禁用链接；只有整个计划的最后一天才禁用“下一项”。第一天没有上一项时，才将“上一项”换成灰色禁用项，例如：

```html
<nav class="page-navigation" aria-label="页面导航">
  <a class="page-nav-button" href="上一项.html">上一项</a>
  <a class="page-nav-button" href="../../index.html">返回主页</a>
  <a class="page-nav-button" href="下一项.html">下一项</a>
</nav>
```

```html
<span class="page-nav-button is-disabled" aria-disabled="true">上一项</span>
```

首页位于仓库根目录，直接引用 `docs/learning/assets/learning.css`；首页本身不显示前后项导航。
