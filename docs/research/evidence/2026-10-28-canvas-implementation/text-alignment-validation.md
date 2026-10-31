# 段落对齐实现与验证（2026-10-28）

来源：用户要求无限画布全量一致；即梦实页全屏格式菜单已确认左对齐、居中对齐、右对齐三项（父任务浏览器观察）。设计合同追加于设计10 §7.7.2，验收关联 PRD14 AC-CJC-04 / Plan23 CASE-CJC-04。此证据只覆盖对齐能力，不替代完整画布接受。

## 实现边界

- 全屏与内联文档工具栏在“引用”之后、“加粗”之前增加“段落对齐”；只列三项，当前值由编辑器选区派生。菜单关闭回到正文焦点，打开菜单后失权仍禁止写入。
- 浏览器复用项目已有的 Tiptap TextAlign 3.32.0；canvas-collab 增加同版本官方扩展依赖和锁文件，保证服务投影 schema 一致。
- 仅paragraph/heading允许textAlign，显式业务值为left/center/right；Go、前端导入和协作规范化器拒绝未知值/任意CSS/错误节点属性。
- Tiptap内部缺省null在持久化前省略，空attrs同时删除；旧文档仅打开不应引起内容或图哈希改变。编辑器预接受回调也传规范化正文，避免默认属性被错误提交。
- 对齐沿既有正文Yjs片段与Go正式接受链，无新API/存储/协议分叉；非编辑RichTextContent保持同一段落/标题对齐。
- Markdown导出仍使用既有规则；标准Markdown不表达对齐。竞品对齐后.md是否使用HTML尚需实页补证，不能把当前导出标为格式全保真；结构化JSON与画布正文保存对齐值。

## Red → Green

| 层 | 修复前 | 修复后 |
| --- | --- | --- |
| 协作schema | 左中右输入触发UNREGISTERED_TEXT_ATTRIBUTE；null默认同样失败 | 三值规范化、Yjs bytes重开、旧文档不变均通过 |
| Go边界 | 错节点/justify/CSS/数字/null均被接受 | 合法三值通过；上述非法值全部拒绝 |
| 前端 | 无“段落对齐”按钮；只读输出丢失对齐 | 菜单存在，修改回调保存，撤销重做、重开和只读渲染通过 |

## 自动验证

- `pnpm --dir canvas-collab test`：104项全部通过、0skip。扩展真实Tiptap双客户端测试：A设置居中，B编辑文字，A撤销只恢复对齐、不删除B的文字；重做及第三文档从接受bytes重开保留居中。
- 前端定向Vitest：4文件15项通过，含正文编辑器/只读内容/JSON导入校验和既有Markdown下载回归。
- `go test -race ./tests/canvas -run 'TestCanvas(TextAlignment|FormattedTextRoundTrip)' -count=1`：通过。Go合法/非法用例包含8个子场景。
- `go vet ./internal/canvas/domain`、定向goimports、golangci-lint（0 issues）、govulncheck（No vulnerabilities found）通过。
- 本次前端ESLint及Prettier、协作改动文件Prettier通过。
- 初次TypeScript检查发现unknown attrs使用in操作的收窄问题，已补显式object判断；修复后 `pnpm exec tsc --noEmit` 退出0通过，未把第一次写为通过。

## 集成接受条件

canvas-collaboration运行镜像没有源码挂载，必须重建该服务以加载新schema与依赖，并重启backend加载严格校验后再做真实浏览器保存回读。子任务未操作共享服务或浏览器，父任务统一进行运行验证，防止与生成器链路重启冲突。需在正式画布验证三种对齐、跨段选区、关闭全屏、刷新回读、只读和关闭菜单焦点；自动测试不替代这一步。

源码文件：canvas-collab/src/rich-text-schema.mjs、package.json/pnpm-lock.yaml；frontend/.../editor/rich-text/rich-text-editor.tsx、rich-text-content.tsx与lib/rich-text-document.ts；backend/internal/canvas/domain/rich_text.go。测试：canvas-collab/tests/text-alignment.test.mjs、tiptap.test.mjs；frontend对应两个rich-text测试；backend/tests/canvas/text_alignment_test.go。未提交或推送，保留此前文本动作和并行任务修改。

技术依据：[Tiptap TextAlign官方文档](https://tiptap.dev/docs/editor/extensions/functionality/textalign)及本地安装3.32.0源实现；React事件处理和客户端边界按Vercel规则与本地Next 16.3.6文档核对。Go变更遵循[Google Go规范](https://google.github.io/styleguide/go/guide)、[决策](https://google.github.io/styleguide/go/decisions)及[实践](https://google.github.io/styleguide/go/best-practices)。
