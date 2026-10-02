import { expect, it } from "vitest";
import {
  parseStructureDocument,
  validateReviewedBoundaries,
  scalarSnippet,
  readEpisodeView,
  readVersionPage,
} from "./review-model";
const a = "11111111-1111-4111-8111-111111111111";
const b = "22222222-2222-4222-8222-222222222222";
const c = "33333333-3333-4333-8333-333333333333";
it("历史角色版本pin完整保留，拒绝无角色的孤立pin与行动伪pin", () => {
  const item = {
    type: "line",
    kind: "dialogue",
    line_key: b,
    content: "台词",
    span_start: 0,
    span_end: 2,
    character_id: a,
    character_version_id: c,
  };
  const scene = {
    scene_key: a,
    seq_no: 1,
    heading: "",
    location_text: "",
    time_of_day: "",
    span_start: 0,
    span_end: 2,
    items: [item],
  };
  const document = { scenes: [scene], unassigned_lines: [] };
  expect(parseStructureDocument(document, 0, 2)).toEqual(document);
  expect(() =>
    parseStructureDocument(
      {
        scenes: [{ ...scene, items: [{ ...item, character_id: undefined }] }],
        unassigned_lines: [],
      },
      0,
      2,
    ),
  ).toThrow();
  expect(() =>
    parseStructureDocument(
      {
        scenes: [
          {
            ...scene,
            items: [
              {
                type: "action",
                line_key: b,
                content: "台词",
                span_start: 0,
                span_end: 2,
                character_version_id: c,
              },
            ],
          },
        ],
        unassigned_lines: [],
      },
      0,
      2,
    ),
  ).toThrow();
});
it("完整分集严格Unicode坐标、preface和无遗漏边界，拒假episode0/间隙/重叠", () => {
  const boundaries = [
    { seq_no: 1, title: "集😀", span_start: 2, span_end: 4 },
    { seq_no: 2, title: "第二集", span_start: 4, span_end: 9 },
  ];
  expect(
    validateReviewedBoundaries(boundaries, { start: 0, end: 2 }, 9),
  ).toEqual(boundaries);
  for (const change of [
    { ...boundaries[0], seq_no: 0 },
    { ...boundaries[0], span_start: 3 },
    { ...boundaries[0], span_end: 5 },
    { ...boundaries[0], span_end: 2 },
  ])
    expect(() =>
      validateReviewedBoundaries(
        [change, boundaries[1]],
        { start: 0, end: 2 },
        9,
      ),
    ).toThrow();
  expect(scalarSnippet("甲😀é\n乙", 1, 4)).toBe("😀é");
  expect(() => scalarSnippet("😀", 0, 2)).toThrow();
});
it("手工结构保留行/场景键、对话行动顺序，拒重复/错位/action伪角色与未知属性", () => {
  const scene = {
    scene_key: a,
    seq_no: 1,
    heading: "内景",
    location_text: "室内",
    time_of_day: "夜",
    span_start: 5,
    span_end: 11,
    items: [
      {
        type: "line",
        line_key: b,
        kind: "dialogue",
        speaker_text: "甲",
        content: "台词😀",
        span_start: 5,
        span_end: 8,
      },
      {
        type: "action",
        line_key: c,
        content: "站起",
        span_start: 8,
        span_end: 11,
      },
    ],
  };
  const document = { scenes: [scene], unassigned_lines: [] };
  expect(parseStructureDocument(document, 5, 11)).toEqual(document);
  expect(() =>
    parseStructureDocument({ ...document, raw: true }, 5, 11),
  ).toThrow();
  expect(() =>
    parseStructureDocument(
      {
        scenes: [
          {
            ...scene,
            items: [{ ...scene.items[1], line_key: b, speaker_text: "假的" }],
          },
        ],
        unassigned_lines: [],
      },
      5,
      11,
    ),
  ).toThrow();
  expect(() =>
    parseStructureDocument(
      {
        ...document,
        unassigned_lines: [
          { line_key: c, content: "重复", span_start: 6, span_end: 7 },
        ],
      },
      5,
      11,
    ),
  ).toThrow();
});
it("版本分页不接受倒退游标、重复身份或把三种SHA混为一项", () => {
  const item = {
    id: a,
    version_no: 7,
    content_hash: "a".repeat(64),
    document_sha256: "b".repeat(64),
    source_manifest_sha256: "c".repeat(64),
    char_count: 9,
    source_count: 2,
    created_at: "2026-10-02T00:00:00Z",
  };
  expect(
    readVersionPage({ items: [item], next_version_no: 7 }, 0, 25).items[0],
  ).toEqual(item);
  expect(() =>
    readVersionPage({ items: [item], next_version_no: 9 }, 0, 25),
  ).toThrow();
  expect(() => readVersionPage({ items: [item, item] }, 0, 25)).toThrow();
  expect(() => readVersionPage({ items: [item] }, 7, 25)).toThrow();
});
it("候选读取严格绑定项目/组织/不可变版本，绝不借候选宣布正式分集", () => {
  const scope = { projectId: a, orgId: b };
  const value = {
    version_id: c,
    head: { version_id: c, split_revision: 0, candidate_split_set_id: a },
    candidate: {
      id: a,
      version_id: c,
      org_id: b,
      project_id: a,
      kind: "candidate",
      origin: "sources",
      boundaries: [{ seq_no: 1, title: "集", span_start: 0, span_end: 9 }],
      created_at: "2026-10-02T00:00:00Z",
    },
    episodes: [],
  };
  expect(readEpisodeView(value, scope, c, 9).episodes).toEqual([]);
  expect(() =>
    readEpisodeView(
      { ...value, candidate: { ...value.candidate, org_id: c } },
      scope,
      c,
      9,
    ),
  ).toThrow();
  expect(() =>
    readEpisodeView(
      { ...value, head: { ...value.head, candidate_split_set_id: b } },
      scope,
      c,
      9,
    ),
  ).toThrow();
});
