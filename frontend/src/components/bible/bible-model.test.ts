import { expect, it } from "vitest";
import {
  characterInputSchema,
  characterContentSchema,
  voiceContentSchema,
  readBibleDetail,
  readBiblePage,
  readBibleVersion,
  readBibleReceipt,
  visuallyReady,
  versionInput,
} from "./bible-model";

const actor = "11111111-1111-4111-8111-111111111111",
  org = "22222222-2222-4222-8222-222222222222",
  project = "33333333-3333-4333-8333-333333333333",
  entry = "44444444-4444-4444-8444-444444444444",
  version = "55555555-5555-4555-8555-555555555555",
  look = "66666666-6666-4666-8666-666666666666",
  media = "77777777-7777-4777-8777-777777777777",
  rendition = "88888888-8888-4888-8888-888888888888";
const at = "2026-10-02T00:00:00Z",
  sha = "a".repeat(64);
const identity = {
  origin: "http://localhost:3000",
  actorId: actor,
  orgId: org,
  projectId: project,
};
const character = {
  name: "真实合成角色",
  definition: {},
  looks: [{ id: look, name: "默认造型", default: true }],
};
const head = {
  id: entry,
  org_id: org,
  project_id: project,
  kind: "character",
  revision: 2,
  current_version_id: version,
  deleted: false,
  created_at: at,
  updated_at: at,
};
const snapshot = {
  id: version,
  entry_id: entry,
  org_id: org,
  project_id: project,
  kind: "character",
  number: 1,
  actor_id: actor,
  created_at: at,
  origin: "manual",
  content_sha256: sha,
  character,
};
const image = {
  asset_id: media,
  revision: 1,
  sha256: sha,
  byte_size: 200,
  kind: "image",
  rendition_id: rendition,
  rendition_sha256: sha,
};

it("编辑正文仅转换正式手工字段，不把服务器冻结的造型和声音重发或删除", () => {
  const detail = readBibleDetail(
    { head, current: snapshot, resolved_id: entry },
    identity,
    "character",
    entry,
  );
  expect(versionInput(detail)).toEqual({
    name: character.name,
    definition: {},
  });
  expect(versionInput(detail)).not.toHaveProperty("looks");
  expect(detail.current).toEqual(snapshot);
});

it("完整手工字段闭合，名称按Unicode scalar计数，保留空格与原文而不静默丢字段", () => {
  const input = {
    name: "😀".repeat(512),
    aliases: ["  旧名称  "],
    description: " 原描述\n",
    definition: {
      role: "剧情定位",
      physique: "体型",
      multi_view_prompt: "三视图约束",
      voice_timbre: "音色描述",
    },
  };
  expect(characterInputSchema.parse(input)).toEqual(input);
  for (const bad of [
    { ...input, name: "😀".repeat(513) },
    { ...input, name: "\ud800" },
    { ...input, aliases: ["同名", "同名"] },
    { ...input, name: " \n" },
    { ...input, voice: {} },
    { ...input, definition: { unknown: true } },
  ])
    expect(characterInputSchema.safeParse(bad).success).toBe(false);
});
it("角色默认造型恰一个、ID不重复，六种用途唯一且每图须真实rendition", () => {
  expect(characterContentSchema.safeParse(character).success).toBe(true);
  for (const looks of [
    [],
    [{ ...character.looks[0], default: false }],
    [...character.looks, { id: media, name: "第二套", default: true }],
    [...character.looks, { ...character.looks[0], default: false }],
    [
      {
        ...character.looks[0],
        references: [
          { role: "front", media: image },
          { role: "front", media: image },
        ],
      },
    ],
    [
      {
        ...character.looks[0],
        references: [
          { role: "primary", media: { ...image, rendition_id: undefined } },
        ],
      },
    ],
  ])
    expect(
      characterContentSchema.safeParse({ ...character, looks }).success,
    ).toBe(false);
});
it("三视图就绪只能来自turnaround或front/side/back完整组合，primary不冒充", () => {
  const refs = (roles: string[]) =>
    roles.map((role) => ({ role, media: image }));
  expect(visuallyReady(refs(["primary"]))).toBe(false);
  expect(visuallyReady(refs(["front", "side"]))).toBe(false);
  expect(visuallyReady(refs(["front", "side", "back"]))).toBe(true);
  expect(visuallyReady(refs(["turnaround_sheet"]))).toBe(true);
});
it("catalog与样本严格互斥；音频保留真实原件身份，空uuid rendition不伪造图片", () => {
  const sample = {
    kind: "sample",
    sample: {
      name: "合成样本",
      media: {
        asset_id: media,
        revision: 1,
        sha256: sha,
        byte_size: 20,
        kind: "audio",
        rendition_id: "00000000-0000-0000-0000-000000000000",
      },
    },
  };
  expect(voiceContentSchema.parse(sample)).toEqual(sample);
  expect(voiceContentSchema.safeParse({ ...sample, catalog: {} }).success).toBe(
    false,
  );
  expect(
    voiceContentSchema.safeParse({
      ...sample,
      sample: {
        ...sample.sample,
        media: { ...sample.sample.media, rendition_id: rendition },
      },
    }).success,
  ).toBe(false);
  expect(
    voiceContentSchema.safeParse({
      kind: "catalog",
      catalog: {
        model_key: "actual",
        model_version_id: version,
        model_version: 1,
        voice_key: "actual",
        param_schema_sha256: sha,
        params: { speed: Infinity },
      },
    }).success,
  ).toBe(false);
});
it("详情与精确历史须匹配org/project/kind/稳定身份；AI来源不能缺实际output事实", () => {
  const detail = readBibleDetail(
    { head, current: snapshot, resolved_id: entry },
    identity,
    "character",
    entry,
  );
  expect(
    detail.current.kind === "character" ? detail.current.character : undefined,
  ).toEqual(character);
  expect(
    readBibleVersion(snapshot, identity, "character", entry, version).id,
  ).toBe(version);
  for (const bad of [
    { ...snapshot, org_id: media },
    { ...snapshot, entry_id: media },
    { ...snapshot, location: { name: "混合" } },
    { ...snapshot, origin: "ai" },
    { ...snapshot, previous_id: version },
  ])
    expect(() =>
      readBibleVersion(bad, identity, "character", entry, version),
    ).toThrow();
  expect(() =>
    readBibleDetail(
      {
        head: { ...head, current_version_id: media },
        current: snapshot,
        resolved_id: entry,
      },
      identity,
      "character",
      entry,
    ),
  ).toThrow();
});

it("合并后的原身份正文保持完整，resolved必须指向另一个真实身份且不能自环", () => {
  const redirected = {
    head: { ...head, redirect_id: media },
    current: snapshot,
    resolved_id: media,
  };
  const read = readBibleDetail(redirected, identity, "character", entry);
  expect(read.current).toEqual(snapshot);
  expect(read.resolved_id).toBe(media);
  expect(() =>
    readBibleDetail(
      { ...redirected, resolved_id: entry },
      identity,
      "character",
      entry,
    ),
  ).toThrow();
});
it("列表绑定fresh principal并拒跨域、重复ID和未知DTO；无写入种子", () => {
  const value = {
    entries: [{ head, name: character.name, content_sha256: sha }],
    current_actor_id: actor,
    current_org_id: org,
  };
  expect(
    readBiblePage(value, project, "character", identity).entries,
  ).toHaveLength(1);
  for (const bad of [
    { ...value, current_actor_id: media },
    { ...value, entries: [...value.entries, ...value.entries] },
    {
      ...value,
      entries: [{ ...value.entries[0], head: { ...head, project_id: media } }],
    },
    { ...value, secret: "unknown" },
  ])
    expect(() => readBiblePage(bad, project, "character", identity)).toThrow();
});
it("旧永久回执按原CAS而非当前GET校验，非法200、合并目标与拆分身份闭合", () => {
  const receipt = {
    entry_id: entry,
    kind: "character",
    revision: 3,
    version_id: version,
    version_number: 1,
    project_revision: 8,
    content_sha256: sha,
  };
  const command = {
    action: "confirm" as const,
    kind: "character" as const,
    id: entry,
    body: { expected_revision: 2 },
  };
  expect(
    readBibleReceipt({ ...receipt, confirmed_version_id: version }, command)
      .revision,
  ).toBe(3);
  for (const bad of [
    receipt,
    { ...receipt, confirmed_version_id: media },
    { ...receipt, confirmed_version_id: version, revision: 99 },
  ])
    expect(() => readBibleReceipt(bad, command)).toThrow();
  expect(() =>
    readBibleReceipt(
      { ...receipt, redirect_id: media },
      {
        action: "merge",
        kind: "character",
        id: entry,
        body: {
          expected_revision: 2,
          target_id: rendition,
          expected_target_revision: 4,
        },
      },
    ),
  ).toThrow();
  expect(() =>
    readBibleReceipt(receipt, {
      action: "split",
      kind: "character",
      id: entry,
      body: {
        expected_revision: 2,
        character: { name: "拆分新角色", definition: {} },
      },
    }),
  ).toThrow();
});
