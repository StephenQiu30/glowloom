import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import { MediaWorkspace } from "@/app/studio/[episodeId]/media-workspace";

const workspaceId = "019fb3c0-a000-7000-8000-000000000001";
const mediaObjectId = "019fb3c0-a000-7000-8000-000000000002";
const firstVersionId = "019fb3c0-a000-7000-8000-000000000003";
const secondVersionId = "019fb3c0-a000-7000-8000-000000000004";

function mediaVersion(
  id: string,
  versionNo: number,
  overrides: Partial<API.MediaVersionResponse> = {},
): API.MediaVersionResponse {
  return {
    id,
    workspace_id: workspaceId,
    media_object_id: mediaObjectId,
    media_object_kind: "image",
    media_object_source_type: "upload",
    media_object_status: "active",
    media_object_current_version_id: secondVersionId,
    media_object_revision: 2,
    version_no: versionNo,
    filename: `角色参考-v${versionNo}.png`,
    sha256: String(versionNo).repeat(64),
    size_bytes: 2048,
    mime_type: "image/png",
    probe_status: "ready",
    probe_attempt: 1,
    probe_error_code: null,
    probe_error_summary: null,
    probe_next_action: null,
    width: 1024,
    height: 1024,
    duration_ms: null,
    codec: null,
    container: "png",
    created_at: `2026-08-0${versionNo}T10:00:00Z`,
    ...overrides,
  };
}

describe("MediaWorkspace", () => {
  it("manages immutable versions, current pointer and archive state", async () => {
    const user = userEvent.setup();
    const first = mediaVersion(firstVersionId, 1);
    const second = mediaVersion(secondVersionId, 2);
    const onAppendVersion = vi.fn().mockResolvedValue(true);
    const onSetCurrent = vi.fn().mockResolvedValue(undefined);
    const onToggleArchived = vi.fn().mockResolvedValue(undefined);

    render(
      <MediaWorkspace
        busy={false}
        media={[second, first]}
        onAppendVersion={onAppendVersion}
        onRetry={vi.fn()}
        onSetCurrent={onSetCurrent}
        onToggleArchived={onToggleArchived}
        onUpload={vi.fn()}
      />,
    );

    expect(screen.getByText(/2 个不可变版本/)).toBeInTheDocument();
    expect(screen.getByText("当前版本 v2")).toBeInTheDocument();

    await user.click(
      screen.getByRole("button", { name: "设为当前媒体版本 v1" }),
    );
    expect(onSetCurrent).toHaveBeenCalledWith(first);

    await user.click(screen.getByRole("button", { name: "追加媒体版本" }));
    const file = new File(["new-image"], "角色参考-v3.png", {
      type: "image/png",
    });
    const appendInput = screen.getByLabelText("选择新的媒体文件");
    await user.upload(appendInput, file);
    expect((appendInput as HTMLInputElement).files?.[0]).toBe(file);
    const appendButton = screen.getByRole("button", { name: "上传为新版本" });
    expect(appendButton).toBeEnabled();
    fireEvent.submit(appendButton.closest("form")!);
    await waitFor(() => expect(onAppendVersion).toHaveBeenCalledWith(second, file));

    await user.click(screen.getByRole("button", { name: "归档媒体" }));
    expect(onToggleArchived).toHaveBeenCalledWith(second);
  });

  it("shows archived history and exposes an explicit restore action", async () => {
    const user = userEvent.setup();
    const archived = mediaVersion(secondVersionId, 2, {
      media_object_status: "archived",
      media_object_revision: 5,
    });
    const onToggleArchived = vi.fn().mockResolvedValue(undefined);

    render(
      <MediaWorkspace
        busy={false}
        media={[archived]}
        onAppendVersion={vi.fn()}
        onRetry={vi.fn()}
        onSetCurrent={vi.fn()}
        onToggleArchived={onToggleArchived}
        onUpload={vi.fn()}
      />,
    );

    expect(screen.getByText("已归档")).toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "追加媒体版本" }),
    ).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "恢复媒体" }));
    expect(onToggleArchived).toHaveBeenCalledWith(archived);
  });
});
