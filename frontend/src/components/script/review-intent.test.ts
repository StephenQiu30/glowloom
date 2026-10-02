import { expect, it } from "vitest";
import {
  loadReviewIntent,
  saveReviewIntent,
  readReviewReceipt,
  reviewCommandSchema,
} from "./review-intent";
const id = (n: number) =>
  `${String(n).repeat(8)}-${String(n).repeat(4)}-4${String(n).repeat(3)}-8${String(n).repeat(3)}-${String(n).repeat(12)}`;
const scope = {
  origin: "http://127.0.0.1:3000",
  actorId: id(1),
  orgId: id(2),
  projectId: id(3),
};
const command = {
  action: "confirm_split" as const,
  charCount: 6,
  body: {
    version_id: id(4),
    expected_revision: 7,
    expected_split_revision: 2,
    candidate_set_id: id(5),
    boundaries: [{ seq_no: 1, title: "一", span_start: 0, span_end: 6 }],
    ack_invalidate: false,
  },
};
it("分集未知意图持久原键原完整正文、隔离身份，不覆盖旧请求", () => {
  const values = new Map<string, string>();
  const storage = {
    getItem: (key: string) => values.get(key) ?? null,
    setItem: (key: string, value: string) => {
      values.set(key, value);
    },
    removeItem: (key: string) => {
      values.delete(key);
    },
  };
  const intent = { ...scope, ...command, version: 1 as const, key: id(6) };
  saveReviewIntent(storage, intent);
  expect(loadReviewIntent(storage, scope)).toEqual(intent);
  expect(loadReviewIntent(storage, { ...scope, actorId: id(7) })).toBeNull();
  expect(() => saveReviewIntent(storage, { ...intent, key: id(8) })).toThrow();
  expect(() =>
    reviewCommandSchema.parse({
      ...command,
      body: {
        ...command.body,
        boundaries: [{ seq_no: 1, title: "一", span_start: 1, span_end: 6 }],
      },
    }),
  ).toThrow();
});
it("永久旧回执核原CAS与全分集身份/范围，重复映射和错误确认不能当成功", () => {
  const intent = { ...scope, ...command, version: 1 as const, key: id(6) };
  const episode = {
    id: id(7),
    org_id: scope.orgId,
    project_id: scope.projectId,
    script_version_id: id(4),
    split_set_id: id(8),
    seq_no: 1,
    title: "一",
    span_start: 0,
    span_end: 6,
    revision: 1,
    inherit_status: "not_inherited",
    is_delete: false,
  };
  const receipt = {
    script_revision: 8,
    project_revision: 12,
    split_revision: 3,
    split_set_id: id(8),
    confirmation_id: id(9),
    episodes: [episode],
    episode_mappings: [{ episode_id: id(7), inherit_status: "not_inherited" }],
    renamed_episode_ids: [],
  };
  expect(readReviewReceipt(receipt, intent).script_revision).toBe(8);
  expect(() =>
    readReviewReceipt(
      { ...receipt, episodes: [{ ...episode, span_end: 5 }] },
      intent,
    ),
  ).toThrow();
  expect(() =>
    readReviewReceipt(
      {
        ...receipt,
        episode_mappings: [
          ...receipt.episode_mappings,
          ...receipt.episode_mappings,
        ],
      },
      intent,
    ),
  ).toThrow();
  expect(() =>
    readReviewReceipt({ ...receipt, script_revision: 9 }, intent),
  ).toThrow();
});
it("采纳重复回执保原版本与确认集，不能把不变事实当新修订或跨版成功", () => {
  const intent = {
    ...scope,
    version: 1 as const,
    key: id(6),
    action: "adopt" as const,
    versionId: id(4),
    splitSetId: id(5),
    episodeIds: [id(7)],
    body: {
      expected_revision: 9,
      expected_split_revision: 3,
      ack_invalidate: false,
    },
  };
  const receipt = {
    script_revision: 9,
    project_revision: 12,
    version_id: id(4),
    previous_version_id: id(4),
    split_set_id: id(5),
    episode_mappings: [{ episode_id: id(7), inherit_status: "retained" }],
    changed: false,
    duplicate: true,
  };
  expect(readReviewReceipt(receipt, intent).script_revision).toBe(9);
  expect(() =>
    readReviewReceipt({ ...receipt, script_revision: 10 }, intent),
  ).toThrow();
  expect(() =>
    readReviewReceipt({ ...receipt, version_id: id(8) }, intent),
  ).toThrow();
});
