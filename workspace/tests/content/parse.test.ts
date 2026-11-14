import { describe, expect, it } from "vitest";
import { parseMarkdown } from "../../src/content/parse";

describe("Markdown reading contract", () => {
  it("preserves Chinese duplicate headings and explicit source anchors", async () => {
    const doc = await parseMarkdown(
      '# 项目\n\n<a id="src-ws-07"></a>\n\n## 方案\n\n内容 FR-WS-01。\n\n## 方案\n',
    );
    expect(doc.title).toBe("项目");
    expect(doc.headings.map((h) => h.id)).toEqual(["项目", "方案", "方案-1"]);
    expect(doc.html).toContain('id="src-ws-07"');
    expect(doc.blocks.some((b) => b.text === "内容 FR-WS-01。")).toBe(true);
  });
  it("keeps GFM tables and links but removes executable HTML and unsafe URLs", async () => {
    const doc = await parseMarkdown(
      '# 安全\n\n| 字段 | 值 |\n| --- | --- |\n| a | 1 |\n\n<script>window.bad=1</script>\n<img src="x" onerror="bad()">\n\n[恶意](javascript:alert%281%29)\n\n[来源](../design/43.md#src)',
    );
    expect(doc.html).toContain("<table>");
    expect(doc.html).not.toMatch(/<script|onerror|javascript:/);
    expect(doc.links).toContain("../design/43.md#src");
  });
  it("identifies Mermaid as a diagram and does not execute MDX", async () => {
    const doc = await parseMarkdown(
      "# 流程\n\n```mermaid\nflowchart LR\n A-->B\n```\n\n{process.exit(1)}",
    );
    expect(doc.diagrams).toHaveLength(1);
    expect(doc.diagrams[0].source).toContain("A-->B");
    expect(doc.html).toContain("{process.exit(1)}");
  });
});
