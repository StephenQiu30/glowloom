import { cleanup, render } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import type { MediaUploadDialogProps } from "@/components/canvas/media-upload-dialog";
import { ProjectFolderCoverUpload } from "./project-folder-cover-upload";

const ports = vi.hoisted(() => ({
  list: vi.fn(),
  upload: vi.fn(),
  props: null as MediaUploadDialogProps | null,
}));
vi.mock("@/components/canvas/media-upload-dialog", () => ({
  MediaUploadDialog: (props: MediaUploadDialogProps) => {
    ports.props = props;
    return <p>上传</p>;
  },
}));
vi.mock("@/components/canvas/queries", () => ({
  uploadCanvasMedia: ports.upload,
}));
vi.mock("./folder-queries", () => ({
  listFolders: ports.list,
  requireFolderScope: (
    page: { current_actor_id: string },
    scope: { actorId: string },
  ) => {
    if (page.current_actor_id !== scope.actorId)
      throw new Error("scope_changed");
  },
}));
const scope = {
  origin: window.location.origin,
  actorId: "acafc61c-2bf8-48ba-b6c4-f7a84d95c518",
  orgId: "1a39f9d1-a43c-4d98-8947-901b385d5457",
};
const project = "322f64a1-6ce2-40aa-bf94-6f1f85b8bba4";
const asset = {
  id: "9f3e948c-550b-47c8-8fa2-f2c31c88927c",
  project_id: project,
  kind: "image" as const,
  file_name: "cover.png",
  byte_size: 1,
  mime_type: "image/png",
  revision: 1,
};
beforeEach(() => {
  vi.resetAllMocks();
  ports.list.mockResolvedValue({ current_actor_id: scope.actorId });
  ports.upload.mockResolvedValue(asset);
});
afterEach(cleanup);
it("复用上传原key和取消信号，唯一正式图片只选为目录草稿", async () => {
  const selected = vi.fn();
  render(
    <ProjectFolderCoverUpload
      projectId={project}
      scope={scope}
      onClose={vi.fn()}
      onSelected={selected}
    />,
  );
  const file = new File(["x"], "cover.png", { type: "image/png" });
  const key = crypto.randomUUID();
  const options = { signal: new AbortController().signal, onProgress: vi.fn() };
  expect(ports.props?.maximumFiles).toBe(1);
  expect(ports.props?.target).toBe("folder-cover");
  const formal = await ports.props!.upload(file, key, options);
  expect(ports.upload).toHaveBeenCalledExactlyOnceWith(
    project,
    file,
    key,
    options,
  );
  await ports.props!.onImported([formal]);
  expect(selected).toHaveBeenCalledExactlyOnceWith({
    project_id: project,
    asset_id: asset.id,
  });
  await expect(
    ports.props!.onImported([{ ...formal, project_id: scope.actorId }]),
  ).rejects.toMatchObject({ code: "invalid_response" });
});
it("发送前scope变化拒绝上传，非法非image回执仍用原upload意图恢复", async () => {
  render(
    <ProjectFolderCoverUpload
      projectId={project}
      scope={scope}
      onClose={vi.fn()}
      onSelected={vi.fn()}
    />,
  );
  const file = new File(["x"], "cover.png");
  const options = { signal: new AbortController().signal, onProgress: vi.fn() };
  ports.list.mockResolvedValue({ current_actor_id: project });
  await expect(ports.props!.upload(file, asset.id, options)).rejects.toThrow(
    "scope_changed",
  );
  expect(ports.upload).not.toHaveBeenCalled();
  ports.list.mockResolvedValue({ current_actor_id: scope.actorId });
  ports.upload.mockResolvedValue({ ...asset, kind: "video" });
  await expect(
    ports.props!.upload(file, asset.id, options),
  ).rejects.toMatchObject({ code: "invalid_response" });
});
