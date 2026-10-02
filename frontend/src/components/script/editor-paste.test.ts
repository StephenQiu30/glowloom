import { expect, it } from "vitest";
import { assertRichPaste } from "./editor-paste";
it("受支持的剪贴板格式保留，未知CSS/节点/危险href明确阻止富文本粘贴", () => {
  expect(() =>
    assertRichPaste(
      '<h2 style="text-align:right">章</h2><p><strong>甲</strong><span style="color:#1234">乙</span><mark data-color="rgba(1,2,3,0.5)">丙</mark><a href="/chapter">链接</a></p><ol start="0"><li><p>零</p></li></ol>',
    ),
  ).not.toThrow();
  for (const html of [
    "<table><tr><td>表</td></tr></table>",
    '<p style="font-size:24px">字</p>',
    '<span style="background-color:yellow">未映射高亮</span>',
    '<a href="javascript:alert(1)">脚本</a>',
    '<p><img src="https://example.test/private.png"></p>',
    '<ol type="I"><li>罗马序号</li></ol>',
    '<a href="/part" title="语义标题">标题</a>',
    "<script>doSomething()</script>",
  ])
    expect(() => assertRichPaste(html)).toThrow(/粘贴/);
});
