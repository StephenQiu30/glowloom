import { expect, it } from "vitest";
import {
  readSourceDetail,
  readSourcePage,
  readSourceReceipt,
  readScriptWorkspace,
  sourceCommandSchema,
  sourceProvenanceSchema,
} from "./source-model";
const pid = "11111111-1111-4111-8111-111111111111";
const actor = "22222222-2222-4222-8222-222222222222";
const org = "33333333-3333-4333-8333-333333333333";
const version = "44444444-4444-4444-8444-444444444444";
const lineage = "55555555-5555-4555-8555-555555555555";
const old = "66666666-6666-4666-8666-666666666666";
const fresh = "77777777-7777-4777-8777-777777777777";
const summary = {
  id: old,
  source_lineage_id: lineage,
  source_revision: 2,
  source_kind: "chapter",
  title: "第一章😀",
  status: "ready",
  origin: "manual",
  position: 0,
  char_count: 2,
  content_hash: "a".repeat(64),
  rich_sha256: "b".repeat(64),
};
it("工作区身份闭集、空头无写入假事实和分页绑定immutable版本", () => {
  const workspace = {
    current_actor_id: actor,
    current_org_id: org,
    state: {
      project_id: pid,
      org_id: org,
      revision: 0,
      updated_at: "0001-01-01T00:00:00Z",
    },
  };
  expect(readScriptWorkspace(workspace, pid).state.revision).toBe(0);
  expect(() =>
    readScriptWorkspace(
      { ...workspace, state: { ...workspace.state, org_id: actor } },
      pid,
    ),
  ).toThrow();
  expect(() =>
    readScriptWorkspace({ ...workspace, object_key: "private" }, pid),
  ).toThrow();
  expect(
    readSourcePage(
      { version_id: version, items: [summary], next_position: 1 },
      { after: 0, limit: 25 },
    ),
  ).toMatchObject({ next_position: 1 });
  expect(() =>
    readSourcePage(
      { version_id: version, items: [summary, summary] },
      { after: 0, limit: 25 },
    ),
  ).toThrow();
  expect(() =>
    readSourcePage(
      { version_id: fresh, items: [{ ...summary, position: 25 }] },
      { versionId: version, after: 25, limit: 25 },
    ),
  ).toThrow();
  expect(() =>
    readSourcePage(
      { version_id: version, items: [summary], next_position: 0 },
      { after: 0, limit: 25 },
    ),
  ).toThrow();
});
it("选中私有正文必须匹配lineage/原source/span/Unicode正文事实", () => {
  const detail = {
    ...summary,
    document: {
      type: "doc",
      content: [
        { type: "paragraph", content: [{ type: "text", text: "中😀" }] },
      ],
    },
    plain_text: "中😀",
    source_span: {
      source_id: old,
      source_lineage_id: lineage,
      position: 0,
      start: 0,
      end: 4,
    },
    provenance: {},
  };
  expect(readSourceDetail(detail, lineage).plain_text).toBe("中😀");
  for (const invalid of [
    { ...detail, plain_text: "中" },
    { ...detail, source_span: { ...detail.source_span, source_id: fresh } },
    { ...detail, source_lineage_id: fresh },
    { ...detail, source_span: { ...detail.source_span, end: 1 } },
    { ...detail, source_span: { ...detail.source_span, end: 3 } },
  ])
    expect(() => readSourceDetail(invalid, lineage)).toThrow();
});
it("写入完整body与CAS闭集，明确版权，bulk两种输入互斥和预算", () => {
  const body = {
    expected_revision: 2,
    base_version_id: version,
    rights_confirmed: true,
    source_kind: "chapter",
    title: "😀".repeat(512),
    status: "draft",
    document: { type: "doc" },
    provenance: {},
  };
  expect(
    sourceCommandSchema.parse({ action: "create", body }).body,
  ).toMatchObject({ title: body.title });
  for (const invalid of [
    { ...body, expected_revision: undefined },
    { ...body, rights_confirmed: false },
    { ...body, title: "😀".repeat(513) },
    { ...body, original_html: "<p>不能同时存在</p>" },
    { ...body, provenance: { encoding: "utf8" } },
  ])
    expect(
      sourceCommandSchema.safeParse({ action: "create", body: invalid })
        .success,
    ).toBe(false);
  expect(
    sourceCommandSchema.safeParse({
      action: "reorder",
      body: {
        expected_revision: 2,
        base_version_id: version,
        source_lineage_ids: [lineage, lineage],
      },
    }).success,
  ).toBe(false);
});
it("旧永久回执只确认为该意图，拒绝跨lineage/重复ID/错误版本回执", () => {
  const command = {
    action: "update" as const,
    lineageId: lineage,
    sourceId: old,
    body: {
      expected_revision: 2,
      base_version_id: version,
      rights_confirmed: true as const,
      source_kind: "chapter" as const,
      title: "章",
      status: "draft" as const,
      document: { type: "doc" as const },
      provenance: {},
    },
  };
  const receipt = {
    script_revision: 3,
    project_revision: 9,
    version_id: fresh,
    split_set_id: actor,
    source_mappings: [
      { source_lineage_id: lineage, old_source_id: old, new_source_id: fresh },
    ],
    changed: true,
    duplicate: false,
  };
  expect(readSourceReceipt(command, receipt).script_revision).toBe(3);
  for (const invalid of [
    { ...receipt, script_revision: 2 },
    {
      ...receipt,
      source_mappings: [
        { ...receipt.source_mappings[0], source_lineage_id: org },
      ],
    },
    {
      ...receipt,
      source_mappings: [...receipt.source_mappings, ...receipt.source_mappings],
    },
    {
      ...receipt,
      source_mappings: [
        { ...receipt.source_mappings[0], old_source_id: fresh },
      ],
    },
  ])
    expect(() => readSourceReceipt(command, invalid)).toThrow();
});
it("提取警告是原件历史闭合事实，保留零段落并拒绝未知代码、重复和越界", () => {
  const warnings = [
    { code: "embedded_media_omitted", count: 2, paragraph: 0 },
    { code: "table_layout_flattened", count: 1 },
  ];
  expect(sourceProvenanceSchema.parse({ warnings })).toEqual({ warnings });
  for (const invalid of [
    [{ code: "unknown_format", count: 1 }],
    [warnings[0], warnings[0]],
    [{ code: "run_format_omitted", count: 0 }],
    [{ code: "run_format_omitted", count: 200001 }],
    [{ code: "run_format_omitted", count: 1, paragraph: 200000 }],
    [{ code: "run_format_omitted", count: 1, object_key: "private" }],
  ])
    expect(
      sourceProvenanceSchema.safeParse({ warnings: invalid }).success,
    ).toBe(false);
  expect(
    sourceCommandSchema.safeParse({
      action: "create",
      body: {
        expected_revision: 0,
        rights_confirmed: true,
        source_kind: "document",
        title: "原件",
        status: "draft",
        document: { type: "doc" },
        provenance: { warnings },
      },
    }).success,
  ).toBe(false);
});
