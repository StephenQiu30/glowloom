import { beforeEach, expect, it, vi } from "vitest";
import { ApiError } from "@/lib/request";
import {
  getBibleDetail,
  listBible,
  applyBibleIntent,
  bibleScopeKey,
} from "./bible-queries";
import { type BibleIntent } from "./bible-intent";
const api = vi.hoisted(() => ({
  listBibleEntries: vi.fn(),
  getBibleEntry: vi.fn(),
  createBibleEntry: vi.fn(),
  confirmBibleEntry: vi.fn(),
}));
vi.mock("@/api/bible", () => ({
  ...api,
  listBibleVoices: vi.fn(),
  updateBibleEntry: vi.fn(),
  deleteBibleEntry: vi.fn(),
  restoreBibleEntry: vi.fn(),
  mergeBibleCharacter: vi.fn(),
  splitBibleCharacter: vi.fn(),
  listBibleVersions: vi.fn(),
  getBibleVersion: vi.fn(),
  createBibleLook: vi.fn(),
  updateBibleLook: vi.fn(),
  deleteBibleLook: vi.fn(),
  setBibleDefaultLook: vi.fn(),
  replaceBibleReferences: vi.fn(),
  bindBibleVoice: vi.fn(),
  unbindBibleVoice: vi.fn(),
  adoptBibleResult: vi.fn(),
  createBibleEntryFromResult: vi.fn(),
}));
const actor = "11111111-1111-4111-8111-111111111111",
  org = "22222222-2222-4222-8222-222222222222",
  project = "33333333-3333-4333-8333-333333333333",
  id = "44444444-4444-4444-8444-444444444444",
  vid = "55555555-5555-4555-8555-555555555555";
const identity = {
  origin: window.location.origin,
  actorId: actor,
  orgId: org,
  projectId: project,
};
const intent: BibleIntent = {
  ...identity,
  version: 1,
  key: id,
  command: {
    action: "create",
    kind: "character",
    body: {
      expected_revision: 0,
      character: { name: "独占原输入", definition: {} },
    },
  },
};
const page = { current_actor_id: actor, current_org_id: org, entries: [] };
const receipt = {
  entry_id: id,
  kind: "character",
  revision: 1,
  version_id: vid,
  version_number: 1,
  project_revision: 2,
  content_sha256: "a".repeat(64),
};
beforeEach(() => {
  vi.resetAllMocks();
  api.listBibleEntries.mockResolvedValue(page);
  api.createBibleEntry.mockResolvedValue(receipt);
});
it("仅消费generated API，cursor原样服务端查询；cache完整绑定origin actor org project", async () => {
  await listBible(
    project,
    "prop",
    { cursor: "original-opaque", limit: 25 },
    undefined,
    identity,
  );
  expect(api.listBibleEntries).toHaveBeenCalledWith(
    { pid: project, kind: "prop", cursor: "original-opaque", limit: 25 },
    { signal: undefined },
  );
  expect(bibleScopeKey(identity)).toEqual([
    "bible",
    identity.origin,
    actor,
    org,
    project,
  ]);
});
it("warm详情缓存不能越过fresh principal，未完成与403均不读取私有正文", async () => {
  let deny!: (cause: Error) => void;
  api.listBibleEntries.mockImplementation(
    () =>
      new Promise((_, reject) => {
        deny = reject;
      }),
  );
  const pending = getBibleDetail(identity, "character", id);
  const checked = expect(pending).rejects.toBeInstanceOf(ApiError);
  expect(api.getBibleEntry).not.toHaveBeenCalled();
  deny(new ApiError(403, "forbidden"));
  await checked;
  expect(api.getBibleEntry).not.toHaveBeenCalled();
});
it("scope改变拒绝原意图DML；成功必须同key原body，非法200不当成功", async () => {
  api.listBibleEntries.mockResolvedValue({ ...page, current_actor_id: id });
  await expect(applyBibleIntent(intent)).rejects.toMatchObject({ status: 403 });
  expect(api.createBibleEntry).not.toHaveBeenCalled();
  api.listBibleEntries.mockResolvedValue(page);
  expect(await applyBibleIntent(intent)).toEqual(receipt);
  expect(api.createBibleEntry).toHaveBeenCalledWith(
    { pid: project, kind: "character" },
    intent.command.body,
    { headers: { "Idempotency-Key": id, Origin: identity.origin } },
  );
  api.createBibleEntry.mockResolvedValue({ ...receipt, revision: 2 });
  await expect(applyBibleIntent(intent)).rejects.toMatchObject({
    status: 502,
    code: "invalid_bible_response",
  });
});
it("旧确认永久回执不依当前head revision变化；只返回原receipt供invalidate", async () => {
  api.confirmBibleEntry.mockResolvedValue({
    ...receipt,
    revision: 5,
    confirmed_version_id: vid,
  });
  const original: BibleIntent = {
    ...intent,
    command: {
      action: "confirm",
      kind: "character",
      id,
      body: { expected_revision: 4 },
    },
  };
  expect((await applyBibleIntent(original)).revision).toBe(5);
  expect(api.getBibleEntry).not.toHaveBeenCalled();
});
