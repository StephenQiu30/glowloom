import { beforeEach, expect, it, vi } from "vitest";
const api = vi.hoisted(() => ({
  list: vi.fn(),
  project: vi.fn(),
  create: vi.fn(),
  read: vi.fn(),
  item: vi.fn(),
}));
vi.mock("@/api/media", () => ({
  listMediaLibrary: api.list,
  createMediaTransfer: api.create,
  getMediaTransfer: api.read,
  getMediaLibraryItem: api.item,
}));
vi.mock("@/api/projects", () => ({ getProject: api.project }));
vi.mock("@/lib/request", () => ({
  getSessionGeneration: () => 1,
  getScopeGeneration: () => 1,
}));
import {
  createPersonalReferenceCopy,
  executePersonalReferenceCopy,
} from "@/components/media/copy-personal-references";
const actor = "11111111-1111-4111-8111-111111111111",
  project = "22222222-2222-4222-8222-222222222222",
  item = "33333333-3333-4333-8333-333333333333",
  asset = "44444444-4444-4444-8444-444444444444",
  targetItem = "55555555-5555-4555-8555-555555555555",
  jobID = "66666666-6666-4666-8666-666666666666";
beforeEach(() => {
  vi.resetAllMocks();
  api.list.mockImplementation(({ scope }) =>
    Promise.resolve({
      current_actor_id: actor,
      current_org_id: actor,
      scope: {
        kind: scope,
        ...(scope === "project" ? { project_id: project } : {}),
      },
      revision: 2,
    }),
  );
  api.item.mockResolvedValue({
    id: item,
    revision: 3,
    asset_id: item,
    catalog_state: "active",
    media: { id: item, kind: "video", revision: 1 },
  });
  api.project.mockResolvedValue({ id: project, revision: 4 });
});
it("复制回执丢失后复用冻结输入和幂等键，不把个人素材ID传成项目素材", async () => {
  const controller = new AbortController();
  const command = await createPersonalReferenceCopy(
    project,
    [{ id: item, revision: 3, asset_id: item, asset_revision: 1 }],
    actor,
    controller.signal,
  );
  api.create
    .mockRejectedValueOnce(new Error("lost receipt"))
    .mockResolvedValueOnce({
      id: jobID,
      revision: 1,
      current_actor_id: actor,
      current_org_id: actor,
      source: { kind: "personal" },
      target: { kind: "project", project_id: project },
      status: "succeeded",
      items: [
        {
          index: 0,
          source_item_id: item,
          target_item_id: targetItem,
          target_asset_id: asset,
          status: "succeeded",
        },
      ],
    });
  api.item.mockImplementation(({ scope }) =>
    Promise.resolve(
      scope === "personal"
        ? {
            id: item,
            revision: 3,
            asset_id: item,
            catalog_state: "active",
            media: { id: item, kind: "video", revision: 1 },
          }
        : {
            id: targetItem,
            catalog_state: "active",
            asset_id: asset,
            media: {
              id: asset,
              kind: "video",
              revision: 1,
              file_name: "clip.mp4",
            },
          },
    ),
  );
  await expect(
    executePersonalReferenceCopy(command, controller.signal),
  ).rejects.toThrow("lost receipt");
  const result = await executePersonalReferenceCopy(command, controller.signal);
  expect(api.create.mock.calls[0][0]).toEqual(api.create.mock.calls[1][0]);
  expect(api.create.mock.calls[0][1].headers).toEqual(
    api.create.mock.calls[1][1].headers,
  );
  expect(result).toEqual([
    expect.objectContaining({
      id: asset,
      project_id: project,
      kind: "video",
      revision: 1,
    }),
  ]);
  expect(api.create.mock.calls[0][0].items).toEqual([
    { id: item, revision: 3 },
  ]);
});
it("异账号回执不成为可发送的项目参考", async () => {
  const signal = new AbortController().signal;
  const command = await createPersonalReferenceCopy(
    project,
    [{ id: item, revision: 3, asset_id: item, asset_revision: 1 }],
    actor,
    signal,
  );
  api.create.mockResolvedValue({
    id: jobID,
    revision: 1,
    current_actor_id: project,
    source: { kind: "personal" },
    target: { kind: "project", project_id: project },
    status: "succeeded",
    items: [],
  });
  await expect(executePersonalReferenceCopy(command, signal)).rejects.toThrow();
  expect(api.item).toHaveBeenCalledTimes(1);
});

it("素材版本在复制期间变化时保留任务而不采用新内容", async () => {
  const signal = new AbortController().signal;
  const command = await createPersonalReferenceCopy(
    project,
    [{ id: item, revision: 3, asset_id: item, asset_revision: 1 }],
    actor,
    signal,
  );
  api.create.mockResolvedValue({
    id: jobID,
    revision: 1,
    current_actor_id: actor,
    current_org_id: actor,
    source: { kind: "personal" },
    target: { kind: "project", project_id: project },
    status: "succeeded",
    items: [
      {
        index: 0,
        source_item_id: item,
        target_item_id: targetItem,
        target_asset_id: asset,
        status: "succeeded",
      },
    ],
  });
  api.item.mockResolvedValue({
    id: item,
    revision: 3,
    asset_id: item,
    catalog_state: "active",
    media: { id: item, revision: 2 },
  });
  await expect(executePersonalReferenceCopy(command, signal)).rejects.toThrow(
    "版本已变化",
  );
  expect(command.job?.id).toBe(jobID);
  expect(api.item.mock.calls.at(-1)?.[0].scope).toBe("personal");
});
