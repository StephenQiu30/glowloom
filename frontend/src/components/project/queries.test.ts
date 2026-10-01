import { beforeEach, expect, it, vi } from "vitest";
import { ApiError } from "@/lib/request";
import {
  createProject,
  getProject,
  listProjects,
  listStylePresets,
} from "./queries";

const api = vi.hoisted(() => ({
  create: vi.fn(),
  list: vi.fn(),
  presets: vi.fn(),
  detail: vi.fn(),
}));
vi.mock("@/gen/api/projects", () => ({
  createProject: api.create,
  listProjects: api.list,
  listStylePresets: api.presets,
  getProject: api.detail,
}));
const id = "584ad191-2932-4d7c-bccb-0b9d481c5a76";
const body = {
  name: "逆光",
  description: "",
  aspect_ratio: "16:9" as const,
  style_type: "realistic" as const,
};
const created = {
  ...body,
  id,
  resolution: "1080p",
  allow_overseas_models: false,
  status: "active",
  revision: 1,
  create_time: "2026-09-30T08:00:00Z",
  update_time: "2026-09-30T08:00:00Z",
};
beforeEach(() => vi.resetAllMocks());
it("详情不得将其它项目或缺少实际设置的回执缓存到当前项目", async () => {
  const detail = {
    ...created,
    style_preset_id: null,
    default_models: {},
    is_delete: false,
    archived_at: null,
    delete_time: null,
    purge_after: null,
  };
  api.detail.mockResolvedValue({
    ...detail,
    id: "d77d3c2a-7092-4e30-bf41-a3c5d4d26418",
  });
  await expect(getProject(id)).rejects.toMatchObject({
    status: 502,
    code: "invalid_response",
  });
  api.detail.mockResolvedValue({ ...detail, default_models: undefined });
  await expect(getProject(id)).rejects.toMatchObject({
    status: 502,
    code: "invalid_response",
  });
});
it("创建只调用生成端口，原样传正文与幂等键并校验服务端回执", async () => {
  api.create.mockResolvedValue(created);
  expect(await createProject(body, id)).toEqual(created);
  expect(api.create).toHaveBeenCalledExactlyOnceWith(body, {
    headers: { "Idempotency-Key": id },
  });
  api.create.mockResolvedValue({ ...created, id: "sample-project" });
  await expect(createProject(body, id)).rejects.toMatchObject({
    status: 502,
    code: "invalid_response",
  });
});
it("真实列表向生成端口传筛选和服务端游标，拒绝无UUID或假默认列表", async () => {
  const signal = new AbortController().signal;
  const params = {
    status: "archived" as const,
    deleted: false,
    q: "逆光",
    cursor: "cursor",
    limit: 20,
  };
  const item = {
    id,
    name: "逆光",
    aspect_ratio: "16:9",
    style_type: "realistic",
    status: "archived",
    is_delete: false,
    revision: 2,
  };
  api.list.mockResolvedValue({ items: [item], next_cursor: "next" });
  expect(await listProjects(params, signal)).toEqual({
    items: [item],
    next_cursor: "next",
  });
  expect(api.list).toHaveBeenCalledExactlyOnceWith(params, { signal });
  api.list.mockResolvedValue({
    items: [{ ...item, id: "proj-demo" }],
    next_cursor: null,
  });
  await expect(listProjects(params)).rejects.toBeInstanceOf(ApiError);
});
it("预设真实分页仅暴露安全字段，空列表保持空且服务失败不补seed", async () => {
  api.presets.mockResolvedValue({
    items: [
      {
        id,
        name: "写实",
        style_type: "realistic",
        prompt_fragment: "never display",
      },
    ],
    next_cursor: "more",
  });
  expect(await listStylePresets("cursor")).toEqual({
    items: [{ id, name: "写实", style_type: "realistic" }],
    next_cursor: "more",
  });
  expect(api.presets).toHaveBeenCalledExactlyOnceWith(
    { limit: 20, cursor: "cursor" },
    { signal: undefined },
  );
  api.presets.mockResolvedValue({ items: [], next_cursor: null });
  expect((await listStylePresets()).items).toEqual([]);
  api.presets.mockRejectedValue(new ApiError(503, "dependency_unavailable"));
  await expect(listStylePresets()).rejects.toMatchObject({ status: 503 });
});
