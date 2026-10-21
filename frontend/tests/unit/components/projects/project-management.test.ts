import { expect, it, vi, beforeEach } from "vitest";
import { projectPresentationStore } from "@/components/projects/project-presentation-store";
import { RequestError } from "@/lib/request";
import { manageProject } from "@/components/projects/project-management";
const api = vi.hoisted(() => ({
  read: vi.fn(),
  write: vi.fn(),
  list: vi.fn(),
}));
vi.mock("@/api/projects", () => ({
  getProject: api.read,
  listProjects: api.list,
  archiveProject: api.write,
  unarchiveProject: api.write,
  deleteProject: api.write,
  restoreProject: api.write,
}));
const p = {
  id: "00000000-0000-4000-8000-000000000001",
  canvas_id: null,
  name: "验收画布",
  aspect_ratio: "16:9",
  style_key: null,
  revision: 2,
  update_time: "2026-10-20T00:00:00Z",
  status: "active" as const,
  is_delete: false,
};
beforeEach(() => {
  vi.resetAllMocks();
  projectPresentationStore.clear();
  api.read.mockResolvedValue(p);
});
it("归档绑定所选版本和幂等键", async () => {
  api.write.mockResolvedValue({ ...p, revision: 3, status: "archived" });
  await manageProject(p, "archive", () => {});
  expect(api.write).toHaveBeenCalledWith(
    { pid: p.id },
    { expected_revision: 2 },
    expect.objectContaining({
      sessionRecovery: "never",
      headers: { "Idempotency-Key": expect.any(String) },
    }),
  );
});
it("版本变化时拒绝提交", async () => {
  api.read.mockResolvedValue({ ...p, revision: 3 });
  await expect(manageProject(p, "archive", () => {})).rejects.toThrow(
    "发生变化",
  );
  expect(api.write).not.toHaveBeenCalled();
});
it("丢回执只读确认，不重复写入", async () => {
  api.read
    .mockResolvedValueOnce(p)
    .mockResolvedValueOnce({ ...p, revision: 3, status: "archived" });
  api.write.mockRejectedValue(new Error("network"));
  expect((await manageProject(p, "archive", () => {})).status).toBe("archived");
  expect(api.write).toHaveBeenCalledTimes(1);
  expect(api.read).toHaveBeenCalledTimes(2);
});
it("身份或空间变化时中止", async () => {
  await expect(
    manageProject(p, "archive", () => {
      throw new Error("scope");
    }),
  ).rejects.toThrow("scope");
  expect(api.write).not.toHaveBeenCalled();
  expect(api.read).not.toHaveBeenCalled();
});
it("写入失败且未达目标不报告成功", async () => {
  api.write.mockRejectedValue(new Error("denied"));
  await expect(manageProject(p, "archive", () => {})).rejects.toThrow("未确认");
  expect(api.write).toHaveBeenCalledTimes(1);
});
it("回收站恢复使用列表版本，不请求不可见的普通详情", async () => {
  api.write.mockResolvedValue({ ...p, revision: 3, is_delete: false });
  const restored = await manageProject(
    {
      ...p,
      is_delete: true,
      delete_time: "2026-10-21T00:00:00Z",
      purge_after: "2026-11-20T00:00:00Z",
      server_time: "2026-10-21T00:00:00Z",
    },
    "restore",
    () => {},
  );
  expect(restored.is_delete).toBe(false);
  expect(api.read).not.toHaveBeenCalled();
  expect(api.write).toHaveBeenCalledTimes(1);
});

it("删除已提交但回执丢失时分页核验回收站，不用普通详情404判成功", async () => {
  const deleted = {
    ...p,
    revision: 3,
    is_delete: true,
    delete_time: "2026-10-21T00:00:00Z",
    purge_after: "2026-11-20T00:00:00Z",
  };
  api.write.mockRejectedValue(new Error("response lost"));
  api.read
    .mockResolvedValueOnce(p)
    .mockRejectedValue(new RequestError("not_found", 404));
  api.list
    .mockReset()
    .mockResolvedValueOnce({
      items: [{ ...deleted, id: "00000000-0000-4000-8000-000000000009" }],
      next_cursor: "next",
    })
    .mockResolvedValueOnce({ items: [deleted], next_cursor: null });
  expect((await manageProject(p, "recycle", () => {})).is_delete).toBe(true);
  expect(api.write).toHaveBeenCalledTimes(1);
  expect(api.read).toHaveBeenCalledTimes(1);
  expect(api.list).toHaveBeenLastCalledWith(
    expect.objectContaining({
      deleted: true,
      include_expired: true,
      cursor: "next",
    }),
  );
});
it("明确业务拒绝保留错误，不被读回覆盖", async () => {
  const conflict = new RequestError("inflight_work", 409);
  api.write.mockRejectedValue(conflict);
  await expect(manageProject(p, "recycle", () => {})).rejects.toBe(conflict);
  expect(api.read).toHaveBeenCalledTimes(1);
  expect(api.list).not.toHaveBeenCalled();
});
it("普通详情404且回收站无目标保持未知", async () => {
  api.write.mockRejectedValue(new Error("lost"));
  api.list.mockResolvedValue({ items: [], next_cursor: null });
  await expect(manageProject(p, "recycle", () => {})).rejects.toThrow("未确认");
  expect(api.write).toHaveBeenCalledTimes(1);
});
it("重复回收站游标保持未知且只写一次", async () => {
  api.write.mockRejectedValue(new Error("lost"));
  api.list.mockResolvedValue({
    items: [{ ...p, id: "00000000-0000-4000-8000-000000000009" }],
    next_cursor: "loop",
  });
  await expect(manageProject(p, "recycle", () => {})).rejects.toThrow("未确认");
  expect(api.write).toHaveBeenCalledTimes(1);
  expect(api.list).toHaveBeenCalledTimes(2);
});

it("关闭后再次核验未知删除保留原命令，不派发第二次写入", async () => {
  const deleted = {
    ...p,
    revision: 3,
    is_delete: true,
    delete_time: "2026-10-21T00:00:00Z",
    purge_after: "2026-11-20T00:00:00Z",
  };
  api.write.mockRejectedValue(new Error("lost"));
  api.list.mockResolvedValueOnce({ items: [], next_cursor: null });
  await expect(
    manageProject(p, "recycle", () => {}, "actor/scope"),
  ).rejects.toThrow("未确认");
  api.list.mockResolvedValueOnce({ items: [deleted], next_cursor: null });
  expect(
    (await manageProject(p, "recycle", () => {}, "actor/scope")).is_delete,
  ).toBe(true);
  expect(api.write).toHaveBeenCalledTimes(1);
  expect(api.read).toHaveBeenCalledTimes(1);
});

it("仅用户明确重试才回放未提交的原命令，保持原key与版本", async () => {
  api.write.mockRejectedValueOnce(new Error("not delivered"));
  api.list.mockResolvedValue({ items: [], next_cursor: null });
  await expect(
    manageProject(p, "recycle", () => {}, "actor/scope"),
  ).rejects.toThrow("未确认");
  await expect(
    manageProject(p, "recycle", () => {}, "actor/scope"),
  ).rejects.toThrow("未确认");
  expect(api.write).toHaveBeenCalledTimes(1);
  api.write.mockResolvedValueOnce({
    ...p,
    revision: 3,
    is_delete: true,
    delete_time: "2026-10-21T00:00:00Z",
    purge_after: "2026-11-20T00:00:00Z",
  });
  await manageProject(p, "recycle", () => {}, "actor/scope", true);
  expect(api.write).toHaveBeenCalledTimes(2);
  expect(api.write.mock.calls[1]).toEqual(api.write.mock.calls[0]);
  expect(api.read).toHaveBeenCalledTimes(1);
});
