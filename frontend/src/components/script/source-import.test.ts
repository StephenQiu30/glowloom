import { expect, it } from "vitest";
import { parseSourceImport } from "./source-import";
it("明确旧章节导入保留原HTML字节、源UUID/位置/状态，不继承旧计数或身份", () => {
  const old = [
    {
      id: "11111111-1111-4111-8111-111111111111",
      projectId: "22222222-2222-4222-8222-222222222222",
      kind: "episode",
      title: "标题😀",
      sourceText: '<p style="text-align:right">原稿&nbsp;😀</p>',
      wordCount: 999,
      status: "ready",
      position: 17,
      createdAt: "2026-10-01T00:00:00Z",
      updatedAt: "2026-10-01T00:00:00Z",
    },
  ];
  const parsed = parseSourceImport(JSON.stringify(old), "beeftv");
  expect(parsed).toEqual([
    {
      source_kind: "episode",
      title: "标题😀",
      status: "ready",
      original_html: old[0].sourceText,
      provenance: { external_id: old[0].id, external_position: 17 },
    },
  ]);
  expect(() =>
    parseSourceImport(
      JSON.stringify([{ ...old[0], kind: "future-kind" }]),
      "beeftv",
    ),
  ).toThrow(/第 1/);
  expect(() =>
    parseSourceImport(
      JSON.stringify([{ ...old[0], extraFormat: "unknown" }]),
      "beeftv",
    ),
  ).toThrow();
});
it("批次1..2500全部预校验，格式问题给行位置并保留原JSON草稿", () => {
  const text = JSON.stringify([
    {
      source_kind: "chapter",
      title: "一",
      status: "draft",
      document: { type: "doc" },
      provenance: {},
    },
    {
      source_kind: "chapter",
      title: "二",
      status: "draft",
      document: { type: "doc", content: [{ type: "image" }] },
      provenance: {},
    },
  ]);
  expect(() => parseSourceImport(text, "typed")).toThrow(/第 2/);
  expect(() => parseSourceImport("[]", "typed")).toThrow(/1 至 2500/);
  expect(() =>
    parseSourceImport(
      JSON.stringify(Array.from({ length: 2501 }, () => ({}))),
      "typed",
    ),
  ).toThrow(/1 至 2500/);
  expect(text).toContain('"image"');
  expect(() =>
    parseSourceImport('[{"title":"第一份","title":"第二份"}]', "typed"),
  ).toThrow(/重复/);
});
