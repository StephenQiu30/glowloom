import { expect, it } from "vitest";
import {
  requireSubjectSourceItem,
  requireSubjectSourcePage,
} from "@/components/subjects/unified-source-contracts";
const actor = "11111111-1111-4111-8111-111111111111",
  scope = "22222222-2222-4222-8222-222222222222",
  asset = "33333333-3333-4333-8333-333333333333",
  output = "44444444-4444-4444-8444-444444444444",
  project = "55555555-5555-4555-8555-555555555555";
const item = {
  id: asset,
  source: "personal_asset",
  kind: "audio",
  title: "豆包正式音频",
  favorite: false,
  upscaled: false,
  revision: 0,
  created_at: "2026-10-28T00:00:00Z",
  updated_at: "2026-10-28T00:00:00Z",
  media: {
    id: asset,
    revision: 1,
    kind: "audio",
    mime_type: "audio/mpeg",
    file_name: "配音.mp3",
    byte_size: 100,
    origin: "system",
  },
};
it("个人system音频保留真实origin，不虚构个人LibraryPage", () => {
  const page = {
    current_actor_id: actor,
    current_scope_id: scope,
    page: 1,
    page_size: 12,
    total: 1,
    items: [item],
    capabilities: { upscaled_filter: true },
  };
  expect(
    requireSubjectSourcePage(page, actor, scope, "audio").items[0].media.origin,
  ).toBe("system");
  expect(requireSubjectSourceItem(item).source).toBe("personal_asset");
});
it("生成输出必须有实际project归属，个人原图不得伪装超清", () => {
  expect(
    requireSubjectSourceItem({
      ...item,
      id: output,
      source: "generated_output",
      project_id: project,
      upscaled: true,
    }).project_id,
  ).toBe(project);
  expect(() =>
    requireSubjectSourceItem({
      ...item,
      id: output,
      source: "generated_output",
    }),
  ).toThrow("invalid_receipt");
  expect(() => requireSubjectSourceItem({ ...item, upscaled: true })).toThrow(
    "invalid_receipt",
  );
  expect(() =>
    requireSubjectSourceItem({
      ...item,
      media: { ...item.media, revision: 0 },
    }),
  ).toThrow("invalid_receipt");
});
it("账号空间、重复source身份和请求kind必须与当前目录一致", () => {
  const page = {
    current_actor_id: actor,
    current_scope_id: scope,
    page: 1,
    page_size: 12,
    total: 2,
    items: [item, item],
    capabilities: { upscaled_filter: true },
  };
  expect(() => requireSubjectSourcePage(page, actor, scope)).toThrow(
    "invalid_receipt",
  );
  expect(() =>
    requireSubjectSourcePage({ ...page, items: [item] }, scope, scope),
  ).toThrow("invalid_receipt");
  expect(() =>
    requireSubjectSourcePage({ ...page, items: [item] }, actor, scope, "image"),
  ).toThrow("invalid_receipt");
  expect(() => requireSubjectSourceItem({ ...item, title: 3 })).toThrow(
    "invalid_receipt",
  );
});
