import { cleanup, render } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { ProjectCoverUpload } from "./project-cover-upload";
import type { MediaUploadDialogProps } from "@/components/canvas/media-upload-dialog";
import { ApiError } from "@/lib/request";
const api = vi.hoisted(() => ({ upload: vi.fn(), folders: vi.fn() }));
let props: MediaUploadDialogProps;
vi.mock("@/components/canvas/media-upload-dialog", () => ({
  MediaUploadDialog: (next: MediaUploadDialogProps) => {
    props = next;
    return null;
  },
}));
vi.mock("@/components/canvas/queries", () => ({
  uploadCanvasMedia: api.upload,
}));
vi.mock("./folder-queries", async (original) => ({
  ...(await original<typeof import("./folder-queries")>()),
  listFolders: api.folders,
}));
const pid = "9817c918-e49d-4dc8-b8b6-c92833b135e4",
  aid = "c13b18f1-cd43-4f35-bef4-f06801746458";
const scope = { origin: window.location.origin, actorId: pid, orgId: aid };
const asset = {
  id: aid,
  project_id: pid,
  kind: "image" as const,
  mime_type: "image/png",
  file_name: "主图.png",
  byte_size: 12,
  revision: 1,
};
const file = new File(["png"], "主图.png", { type: "image/png" });
const options = { signal: new AbortController().signal, onProgress: vi.fn() };
beforeEach(() => {
  vi.resetAllMocks();
  api.folders.mockResolvedValue({
    current_actor_id: pid,
    current_org_id: aid,
    items: [],
    next_cursor: null,
  });
  api.upload.mockResolvedValue(asset);
});
afterEach(cleanup);
it("上传传正式 project/key/人工确认端口，已上传结果只回填草稿 ID", async () => {
  const selected = vi.fn();
  render(
    <ProjectCoverUpload
      projectId={pid}
      scope={scope}
      onClose={vi.fn()}
      onSelected={selected}
    />,
  );
  expect(props.target).toBe("project-cover");
  expect(props.maximumFiles).toBe(1);
  expect(await props.upload(file, aid, options)).toEqual(asset);
  expect(api.upload).toHaveBeenCalledExactlyOnceWith(pid, file, aid, options);
  await props.onImported([asset]);
  expect(selected).toHaveBeenCalledExactlyOnceWith(aid);
});
it("实际身份变化在上传前拒绝，跨项目/错误kind回执不得进主图草稿", async () => {
  const selected = vi.fn();
  render(
    <ProjectCoverUpload
      projectId={pid}
      scope={scope}
      onClose={vi.fn()}
      onSelected={selected}
    />,
  );
  api.folders.mockResolvedValue({
    current_actor_id: aid,
    current_org_id: aid,
    items: [],
    next_cursor: null,
  });
  await expect(props.upload(file, aid, options)).rejects.toMatchObject({
    code: "scope_changed",
  });
  expect(api.upload).not.toHaveBeenCalled();
  await expect(
    props.onImported([{ ...asset, project_id: aid }]),
  ).rejects.toBeInstanceOf(ApiError);
  await expect(
    props.onImported([{ ...asset, kind: "video" }]),
  ).rejects.toBeInstanceOf(ApiError);
  expect(selected).not.toHaveBeenCalled();
});
