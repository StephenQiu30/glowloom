import { afterEach, expect, it, vi } from "vitest";
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { webcrypto } from "node:crypto";
import { Blob as NodeBlob } from "node:buffer";
import {
  TimelineExport,
  requireTimelineExport,
  requireTimelinePreview,
  verifyTimelineDownload,
  verifyArtifactDownload,
  timelineJianyingLaunchURL,
} from "@/components/canvas/timeline/timeline-export";
import { RequestError } from "@/lib/request";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogTitle,
} from "@/components/ui/dialog";
import type { CanvasYjsLiveSession } from "@/components/canvas/yjs-live-session";
const api = vi.hoisted(() => ({
  createMediaExport: vi.fn(),
  getMediaExport: vi.fn(),
  listMediaExports: vi.fn(),
  cancelMediaExport: vi.fn(),
  retryMediaExport: vi.fn(),
  previewMediaExport: vi.fn(),
  reviewMediaExport: vi.fn(),
  downloadMediaExport: vi.fn(),
  downloadMediaExportSubtitles: vi.fn(),
}));
vi.mock("@/api/mediaExports", () => api);
const files = vi.hoisted(() => ({ saveAs: vi.fn() }));
vi.mock("file-saver", () => files);
afterEach(() => {
  cleanup();
  vi.resetAllMocks();
  vi.restoreAllMocks();
  localStorage.removeItem("glowloom:timeline:jianying:skip-reminder");
  vi.unstubAllGlobals();
});
const projectID = "10000000-0000-4000-8000-000000000001",
  canvasID = "10000000-0000-4000-8000-000000000002",
  nodeID = "10000000-0000-4000-8000-000000000003";
const job = (): API.ExportJobResponse => ({
  id: "10000000-0000-4000-8000-000000000004",
  project_id: projectID,
  revision: 3,
  source: { canvas_id: canvasID, node_id: nodeID, revision: 9 },
  output_kind: "video",
  status: "review_required",
  asset_id: "10000000-0000-4000-8000-000000000005",
  sha256: "a".repeat(64),
});
const preview = (): API.ExportPreview => ({
  job_id: job().id,
  revision: 3,
  sha256: job().sha256,
  asset: { id: job().asset_id, project_id: projectID, kind: "video" },
  url: "https://media.example/export.mp4",
  expires_at: new Date(Date.now() + 60000).toISOString(),
});
function mount(
  ratio = "16:9",
  onAddToCanvas = vi.fn().mockResolvedValue(undefined),
  hasVisual = true,
) {
  api.listMediaExports.mockResolvedValue({ items: [] });
  const prepareTimeline = vi
    .fn()
    .mockResolvedValue({ canvas_id: canvasID, node_id: nodeID, revision: 9 });
  render(
    <TimelineExport
      client={{ prepareTimeline } as unknown as CanvasYjsLiveSession}
      projectID={projectID}
      canvasID={canvasID}
      nodeID={nodeID}
      disabled={false}
      hasVisual={hasVisual}
      nodeTitle="时间线 甲"
      onAddToCanvas={onAddToCanvas}
      aspectRatio={ratio}
    />,
  );
  fireEvent.click(screen.getByRole("button", { name: "导出" }));
  return { prepareTimeline };
}
it("rejects wrong project, node, unknown statuses, output kinds and non-authoritative source revision", () => {
  expect(requireTimelineExport(job(), projectID, canvasID, nodeID)).toEqual(
    job(),
  );
  for (const patch of [
    { project_id: crypto.randomUUID() },
    { source: { ...job().source, revision: 0 } },
    { source: { ...job().source, node_id: crypto.randomUUID() } },
    { status: "invented" },
    { output_kind: "xml" },
    { sha256: "" },
    { asset_id: "blob:fake" },
  ])
    expect(() =>
      requireTimelineExport(
        { ...job(), ...patch } as API.ExportJobResponse,
        projectID,
        canvasID,
        nodeID,
      ),
    ).toThrow();
});
it("binds preview to exact job revision, digest, asset, project, output and unexpired HTTP URL", () => {
  const source = preview();
  expect(requireTimelinePreview(source, job())).toEqual(source);
  for (const patch of [
    { revision: 2 },
    { sha256: "b".repeat(64) },
    { asset: { ...preview().asset, id: crypto.randomUUID() } },
    { asset: { ...preview().asset, project_id: crypto.randomUUID() } },
    { asset: { ...preview().asset, kind: "audio" } },
    { url: "blob:fake" },
    { expires_at: "2020-01-01" },
  ])
    expect(() =>
      requireTimelinePreview({ ...preview(), ...patch }, job()),
    ).toThrow();
});
it("checks actual downloaded bytes against the reviewed SHA256", async () => {
  vi.stubGlobal("crypto", webcrypto);
  vi.stubGlobal("Blob", NodeBlob);
  const blob = new Blob(["actual-export-file"]);
  const bytes = await crypto.subtle.digest("SHA-256", await blob.arrayBuffer());
  const digest = Array.from(new Uint8Array(bytes), (n) =>
    n.toString(16).padStart(2, "0"),
  ).join("");
  await expect(verifyTimelineDownload(blob, digest)).resolves.toBeUndefined();
  await expect(verifyTimelineDownload(blob, "a".repeat(64))).rejects.toThrow(
    "校验不匹配",
  );
  await expect(verifyTimelineDownload(new Blob(), digest)).rejects.toThrow(
    "为空",
  );
});
it("rejects a status response for a different job or stale revision even within the same timeline", () => {
  expect(() =>
    requireTimelineExport(
      { ...job(), id: crypto.randomUUID() },
      projectID,
      canvasID,
      nodeID,
      job(),
    ),
  ).toThrow();
  expect(() =>
    requireTimelineExport(
      { ...job(), revision: 2 },
      projectID,
      canvasID,
      nodeID,
      job(),
    ),
  ).toThrow();
});

const artifactJob = (): API.ExportJobResponse => ({
  ...job(),
  output_kind: "fcpxml",
  attempt: 1,
  asset_id: undefined,
  artifact_id: "10000000-0000-4000-8000-000000000006",
});
const artifactPreview = (): API.ExportPreview => ({
  ...preview(),
  asset: undefined,
  url: "https://media.example/export.zip",
  artifact: {
    id: artifactJob().artifact_id,
    job_id: job().id,
    project_id: projectID,
    attempt: 1,
    format: "fcpxml",
    format_version: "1.10",
    sha256: job().sha256,
    mime_type: "application/zip",
    file_name: `timeline-${job().id}.zip`,
    byte_size: 8,
    created_at: new Date().toISOString(),
  },
});
it("requires exclusive artifact identity and its exact format, attempt, project and file receipt", () => {
  expect(
    requireTimelineExport(artifactJob(), projectID, canvasID, nodeID),
  ).toEqual(artifactJob());
  expect(() =>
    requireTimelineExport(
      { ...artifactJob(), asset_id: job().asset_id },
      projectID,
      canvasID,
      nodeID,
    ),
  ).toThrow();
  expect(() =>
    requireTimelineExport(
      { ...artifactJob(), artifact_id: undefined },
      projectID,
      canvasID,
      nodeID,
    ),
  ).toThrow();
  expect(
    requireTimelinePreview(artifactPreview(), artifactJob()).artifact
      ?.format_version,
  ).toBe("1.10");
  for (const patch of [
    { id: crypto.randomUUID() },
    { job_id: crypto.randomUUID() },
    { project_id: crypto.randomUUID() },
    { attempt: 2 },
    { format: "xmeml" },
    { format_version: "5" },
    { mime_type: "video/mp4" },
    { sha256: "b".repeat(64) },
    { byte_size: 0 },
    { byte_size: 501 * 1024 * 1024 },
    { created_at: "bad" },
    { file_name: "../outside.zip" },
  ])
    expect(() =>
      requireTimelinePreview(
        {
          ...artifactPreview(),
          artifact: { ...artifactPreview().artifact, ...patch } as API.Artifact,
        },
        artifactJob(),
      ),
    ).toThrow();
  for (const patch of [
    { url: "file:///tmp/project.zip" },
    { url: "https://user:pass@example.com/a" },
    { expires_at: "2020-01-01" },
  ])
    expect(() =>
      requireTimelinePreview({ ...artifactPreview(), ...patch }, artifactJob()),
    ).toThrow();
});
async function packageReceipt() {
  vi.stubGlobal("crypto", webcrypto);
  vi.stubGlobal("Blob", NodeBlob);
  const blob = new Blob([new Uint8Array([80, 75, 3, 4, 0, 1, 2, 3])], {
    type: "application/zip",
  });
  const sha256 = Array.from(
    new Uint8Array(
      await crypto.subtle.digest("SHA-256", await blob.arrayBuffer()),
    ),
    (n) => n.toString(16).padStart(2, "0"),
  ).join("");
  const receipt = artifactPreview();
  receipt.sha256 = sha256;
  receipt.artifact!.sha256 = sha256;
  return { blob, receipt, current: { ...artifactJob(), sha256 } };
}
it("verifies package bytes, MIME, size and ZIP signature before offering an inspection download", async () => {
  const { blob, receipt } = await packageReceipt();
  await expect(
    verifyArtifactDownload(blob, receipt.artifact!),
  ).resolves.toBeUndefined();
  await expect(
    verifyArtifactDownload(blob, { ...receipt.artifact, byte_size: 1 }),
  ).rejects.toThrow("大小");
  await expect(
    verifyArtifactDownload(
      new Blob([await blob.arrayBuffer()], { type: "video/mp4" }),
      receipt.artifact!,
    ),
  ).rejects.toThrow("类型");
  await expect(
    verifyArtifactDownload(blob, {
      ...receipt.artifact,
      sha256: "b".repeat(64),
    }),
  ).rejects.toThrow("校验");
  const fake = new Blob(["notazip!"], { type: "application/zip" });
  const sha256 = Array.from(
    new Uint8Array(
      await crypto.subtle.digest("SHA-256", await fake.arrayBuffer()),
    ),
    (n) => n.toString(16).padStart(2, "0"),
  ).join("");
  await expect(
    verifyArtifactDownload(fake, { ...receipt.artifact, sha256 }),
  ).rejects.toThrow("ZIP");
});

it("submits named 720P canvas settings for automatic aspect without an approval step", async () => {
  api.createMediaExport.mockResolvedValue({ ...job(), status: "queued" });
  const { prepareTimeline } = mount("auto");
  expect(screen.getByLabelText("文件名")).toHaveProperty("value", "时间线 甲");
  fireEvent.click(screen.getByRole("button", { name: "确认" }));
  await waitFor(() => expect(api.createMediaExport).toHaveBeenCalledTimes(1));
  expect(api.createMediaExport.mock.calls[0][1]).toEqual({
    canvas_id: canvasID,
    node_id: nodeID,
    revision: 9,
    output_kind: "video",
    settings: {
      file_name: "时间线 甲",
      destination: "canvas",
      resolution: 720,
    },
  });
  expect(prepareTimeline).toHaveBeenCalledTimes(1);
  expect(screen.queryByRole("checkbox")).toBeNull();
});
it("replays the frozen settings and idempotency key after an uncertain create", async () => {
  api.createMediaExport
    .mockRejectedValueOnce(new Error("connection lost"))
    .mockResolvedValueOnce({ ...job(), status: "queued" });
  const { prepareTimeline } = mount();
  fireEvent.click(screen.getByRole("button", { name: "确认" }));
  await screen.findByText("导出操作未完成");
  fireEvent.change(screen.getByLabelText("文件名"), {
    target: { value: "改变名字" },
  });
  fireEvent.click(screen.getByRole("button", { name: "确认" }));
  await waitFor(() => expect(api.createMediaExport).toHaveBeenCalledTimes(2));
  expect(api.createMediaExport.mock.calls[0]).toEqual(
    api.createMediaExport.mock.calls[1],
  );
  expect(prepareTimeline).toHaveBeenCalledTimes(1);
});
it("does not claim canvas delivery until acknowledged and retries delivery alone", async () => {
  const deliver = vi
    .fn()
    .mockRejectedValueOnce(new Error("画布保存失败"))
    .mockResolvedValueOnce(undefined);
  api.createMediaExport.mockResolvedValue({
    ...job(),
    status: "succeeded",
    settings: {
      file_name: "时间线 甲",
      destination: "canvas",
      resolution: 720,
    },
  });
  mount("auto", deliver);
  fireEvent.click(screen.getByRole("button", { name: "确认" }));
  await screen.findByText("画布保存失败");
  expect(screen.queryByText("已添加到画布")).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "重试交付" }));
  await screen.findByText("已添加到画布");
  expect(deliver).toHaveBeenCalledTimes(2);
  expect(api.createMediaExport).toHaveBeenCalledTimes(1);
  expect(api.reviewMediaExport).not.toHaveBeenCalled();
});

it("keeps polling after a transient failure and delivers the completed task", async () => {
  const settings = {
    file_name: "时间线 甲",
    destination: "canvas" as const,
    resolution: 720 as const,
  };
  api.createMediaExport.mockResolvedValue({
    ...job(),
    status: "queued",
    settings,
  });
  api.getMediaExport
    .mockRejectedValueOnce(new RequestError("dependency_unavailable", 503))
    .mockResolvedValueOnce({
      ...job(),
      revision: 4,
      status: "succeeded",
      settings,
    });
  const deliver = vi.fn().mockResolvedValue(undefined);
  mount("auto", deliver);
  fireEvent.click(screen.getByRole("button", { name: "确认" }));
  await screen.findByText(
    "服务暂时不可用，请稍后重试。",
    {},
    { timeout: 2500 },
  );
  await screen.findByText("已添加到画布", {}, { timeout: 15000 });
  expect(api.getMediaExport).toHaveBeenCalledTimes(2);
  expect(deliver).toHaveBeenCalledTimes(1);
}, 10000);

it("prepares the current saved revision after a definitive rejected create", async () => {
  api.createMediaExport
    .mockRejectedValueOnce(new RequestError("media_export_conflict", 409))
    .mockResolvedValueOnce({
      ...job(),
      source: { ...job().source, revision: 10 },
      status: "queued",
    });
  const { prepareTimeline } = mount();
  fireEvent.click(screen.getByRole("button", { name: "确认" }));
  await screen.findByText("时间线或导出状态已更新，请重试。");
  prepareTimeline.mockResolvedValue({
    canvas_id: canvasID,
    node_id: nodeID,
    revision: 10,
  });
  fireEvent.click(screen.getByRole("button", { name: "确认" }));
  await waitFor(() => expect(api.createMediaExport).toHaveBeenCalledTimes(2));
  expect(prepareTimeline).toHaveBeenCalledTimes(2);
  expect(api.createMediaExport.mock.calls[1][1].revision).toBe(10);
  expect(
    api.createMediaExport.mock.calls[1][2].headers["Idempotency-Key"],
  ).not.toBe(api.createMediaExport.mock.calls[0][2].headers["Idempotency-Key"]);
});

it("anchors settings at export and shows a compact cancellable notification during work", async () => {
  api.createMediaExport.mockResolvedValue({
    ...job(),
    status: "queued",
    settings: {
      file_name: "时间线 甲",
      destination: "canvas",
      resolution: 720,
    },
  });
  mount();
  expect(screen.getByRole("dialog").getAttribute("data-slot")).toBe(
    "popover-content",
  );
  fireEvent.click(screen.getByRole("button", { name: "确认" }));
  await screen.findByText("导出中...");
  expect(screen.queryByRole("dialog")).toBeNull();
  expect(
    (screen.getByRole("button", { name: "导出" }) as HTMLButtonElement)
      .disabled,
  ).toBe(true);
  expect(
    screen
      .getByRole("region", { name: "时间线导出通知" })
      .querySelector('button[aria-label="取消导出"]'),
  ).not.toBeNull();
});

it("compact canvas export keeps an accessible icon entry without the full editor menu", async () => {
  api.listMediaExports.mockResolvedValue({ items: [] });
  render(
    <TimelineExport
      compact
      hasVisual
      client={{} as CanvasYjsLiveSession}
      projectID={projectID}
      canvasID={canvasID}
      nodeID={nodeID}
      disabled={false}
      aspectRatio="16:9"
    />,
  );
  const entry = screen.getByRole("button", { name: "导出" });
  expect(entry.textContent).toBe("");
  expect(screen.queryByRole("button", { name: "更多导出方式" })).toBeNull();
  fireEvent.click(entry);
  expect(await screen.findByRole("dialog", { name: "导出设置" })).toBeTruthy();
});

it("cancels the current receipt once and waits for the authoritative cancellation status", async () => {
  api.createMediaExport.mockResolvedValue({ ...job(), status: "queued" });
  api.cancelMediaExport.mockResolvedValue(undefined);
  api.getMediaExport.mockResolvedValue({
    ...job(),
    revision: 4,
    status: "cancel_requested",
  });
  mount();
  fireEvent.click(screen.getByRole("button", { name: "确认" }));
  const cancel = await screen.findByRole("button", { name: "取消导出" });
  fireEvent.click(cancel);
  fireEvent.click(cancel);
  await screen.findByText("正在取消...");
  expect(api.cancelMediaExport).toHaveBeenCalledTimes(1);
  expect(api.cancelMediaExport.mock.calls[0][0]).toEqual({ job_id: job().id });
  expect(api.cancelMediaExport.mock.calls[0][1]).toEqual({
    project_id: projectID,
    revision: 3,
  });
  expect(
    (screen.getByRole("button", { name: "取消导出" }) as HTMLButtonElement)
      .disabled,
  ).toBe(true);
  expect(screen.queryByText("已取消")).toBeNull();
});

it("refreshes one worker revision conflict and retries the same cancellation intent once", async () => {
  api.createMediaExport.mockResolvedValue({ ...job(), status: "running" });
  api.cancelMediaExport
    .mockRejectedValueOnce(new RequestError("conflict", 409))
    .mockResolvedValueOnce(undefined);
  api.getMediaExport
    .mockResolvedValueOnce({ ...job(), revision: 4, status: "running" })
    .mockResolvedValue({ ...job(), revision: 5, status: "cancel_requested" });
  mount();
  fireEvent.click(screen.getByRole("button", { name: "确认" }));
  fireEvent.click(await screen.findByRole("button", { name: "取消导出" }));
  await screen.findByText("正在取消...");
  expect(api.cancelMediaExport).toHaveBeenCalledTimes(2);
  expect(
    api.cancelMediaExport.mock.calls.map((call) => call[1].revision),
  ).toEqual([3, 4]);
  expect(
    api.cancelMediaExport.mock.calls[0][2].headers["Idempotency-Key"],
  ).not.toBe(api.cancelMediaExport.mock.calls[1][2].headers["Idempotency-Key"]);
});

it.each(["cancel_requested", "cancelled", "failed", "succeeded"])(
  "does not resend cancellation after a conflict reads back %s",
  async (status) => {
    api.createMediaExport.mockResolvedValue({ ...job(), status: "running" });
    api.cancelMediaExport.mockRejectedValue(new RequestError("conflict", 409));
    api.getMediaExport.mockResolvedValue({ ...job(), revision: 4, status });
    mount();
    fireEvent.click(screen.getByRole("button", { name: "确认" }));
    fireEvent.click(await screen.findByRole("button", { name: "取消导出" }));
    await waitFor(() => expect(api.getMediaExport).toHaveBeenCalledTimes(1));
    expect(api.cancelMediaExport).toHaveBeenCalledTimes(1);
  },
);

it.each([409, 403, 422, 503])(
  "bounds cancellation retries and never retries non-conflict %s",
  async (status) => {
    api.createMediaExport.mockResolvedValue({ ...job(), status: "running" });
    api.cancelMediaExport.mockRejectedValue(
      new RequestError("request_failed", status),
    );
    api.getMediaExport.mockResolvedValue({
      ...job(),
      revision: 4,
      status: "running",
    });
    mount();
    fireEvent.click(screen.getByRole("button", { name: "确认" }));
    fireEvent.click(await screen.findByRole("button", { name: "取消导出" }));
    await screen.findByRole("alert");
    expect(api.cancelMediaExport).toHaveBeenCalledTimes(status === 409 ? 2 : 1);
    expect(api.getMediaExport).toHaveBeenCalledTimes(status === 409 ? 1 : 0);
  },
);

it("keeps the enabled cancellation notice inside the fullscreen modal pointer and focus boundary", async () => {
  api.listMediaExports.mockResolvedValue({ items: [] });
  api.createMediaExport.mockResolvedValue({ ...job(), status: "queued" });
  api.cancelMediaExport.mockResolvedValue(undefined);
  api.getMediaExport.mockResolvedValue({
    ...job(),
    revision: 4,
    status: "cancel_requested",
  });
  render(
    <Dialog open>
      <DialogContent>
        <DialogTitle>时间线全屏</DialogTitle>
        <DialogDescription>编辑时间线</DialogDescription>
        <TimelineExport
          client={
            {
              prepareTimeline: async () => ({
                canvas_id: canvasID,
                node_id: nodeID,
                revision: 9,
              }),
            } as unknown as CanvasYjsLiveSession
          }
          projectID={projectID}
          canvasID={canvasID}
          nodeID={nodeID}
          disabled={false}
          hasVisual
          nodeTitle="时间线"
          aspectRatio="16:9"
        />
      </DialogContent>
    </Dialog>,
  );
  fireEvent.click(screen.getByRole("button", { name: "导出" }));
  fireEvent.click(screen.getByRole("button", { name: "确认" }));
  const cancel = await screen.findByRole("button", { name: "取消导出" });
  await waitFor(() =>
    expect((cancel as HTMLButtonElement).disabled).toBe(false),
  );
  const modal = screen.getByRole("dialog", { name: "时间线全屏" });
  // Radix blocks body hit-testing. A body portal can look enabled while unreachable.
  expect(document.body.style.pointerEvents).toBe("none");
  expect(modal.contains(cancel)).toBe(true);
  cancel.focus();
  expect(document.activeElement).toBe(cancel);
  fireEvent.click(cancel);
  await waitFor(() => expect(api.cancelMediaExport).toHaveBeenCalledTimes(1));
  expect(api.cancelMediaExport.mock.calls[0][0]).toEqual({ job_id: job().id });
  expect(screen.getByRole("dialog", { name: "时间线全屏" })).toBeTruthy();
});

it.each(["running", "succeeded"] as const)(
  "restores the latest %s receipt after compact → fullscreen → compact without duplicate notices",
  async (status) => {
    let latest: API.ExportJobResponse = { ...job(), status: "queued" };
    api.listMediaExports.mockImplementation(async () => ({ items: [latest] }));
    api.getMediaExport.mockImplementation(async () => latest);
    api.cancelMediaExport.mockResolvedValue(undefined);
    const props = {
      client: {} as CanvasYjsLiveSession,
      projectID,
      canvasID,
      nodeID,
      disabled: false,
      hasVisual: true,
      aspectRatio: "16:9",
    };
    function View({ expanded }: { expanded: boolean }) {
      return (
        <>
          <TimelineExport {...props} compact suspended={expanded} />
          <Dialog open={expanded}>
            <DialogContent>
              <DialogTitle>时间线全屏</DialogTitle>
              <DialogDescription>编辑时间线</DialogDescription>
              <TimelineExport {...props} />
            </DialogContent>
          </Dialog>
        </>
      );
    }
    const view = render(<View expanded={false} />);
    await screen.findByRole("button", { name: "取消导出" });
    latest = {
      ...job(),
      id: "10000000-0000-4000-8000-000000000006",
      revision: 7,
      status,
    };
    view.rerender(<View expanded />);
    await screen.findByRole("dialog", { name: "时间线全屏" });
    await waitFor(() => {
      const notices = document.querySelectorAll(
        '[aria-label="时间线导出通知"]',
      );
      expect(notices).toHaveLength(1);
      expect(notices[0].closest('[data-slot="dialog-content"]')).not.toBeNull();
    });
    view.rerender(<View expanded={false} />);
    await waitFor(() => expect(api.listMediaExports).toHaveBeenCalledTimes(3));
    await waitFor(() =>
      expect(
        document.querySelectorAll('[aria-label="时间线导出通知"]'),
      ).toHaveLength(1),
    );
    expect(
      document.querySelector('[aria-label="时间线导出通知"]')?.parentElement,
    ).toBe(document.body);
    if (status === "running") {
      fireEvent.click(await screen.findByRole("button", { name: "取消导出" }));
      await waitFor(() =>
        expect(api.cancelMediaExport).toHaveBeenCalledTimes(1),
      );
      expect(api.cancelMediaExport.mock.calls[0][0]).toEqual({
        job_id: latest.id,
      });
      expect(api.cancelMediaExport.mock.calls[0][1]).toEqual({
        project_id: projectID,
        revision: 7,
      });
    } else {
      await screen.findByText("导出完成");
      expect(screen.queryByRole("button", { name: "取消导出" })).toBeNull();
      expect(screen.getByRole("button", { name: "下载到本地" })).toBeTruthy();
    }
  },
);

it("retries a failed frozen export without creating another job or preparing a newer timeline", async () => {
  api.createMediaExport.mockResolvedValue({ ...job(), status: "failed" });
  api.retryMediaExport.mockResolvedValue(undefined);
  api.getMediaExport.mockResolvedValue({
    ...job(),
    revision: 4,
    status: "queued",
  });
  const { prepareTimeline } = mount();
  fireEvent.click(screen.getByRole("button", { name: "确认" }));
  fireEvent.click(await screen.findByRole("button", { name: "重试导出" }));
  await screen.findByText("导出中...");
  expect(api.retryMediaExport).toHaveBeenCalledTimes(1);
  expect(api.retryMediaExport.mock.calls[0][0]).toEqual({ job_id: job().id });
  expect(api.retryMediaExport.mock.calls[0][1]).toEqual({
    project_id: projectID,
    revision: 3,
  });
  expect(api.createMediaExport).toHaveBeenCalledTimes(1);
  expect(prepareTimeline).toHaveBeenCalledTimes(1);
});

it("does not submit an export after the editor has unmounted during source preparation", async () => {
  let finish!: (value: unknown) => void;
  const { prepareTimeline } = mount();
  prepareTimeline.mockReturnValue(
    new Promise((resolve) => {
      finish = resolve;
    }),
  );
  fireEvent.click(screen.getByRole("button", { name: "确认" }));
  cleanup();
  finish({ canvas_id: canvasID, node_id: nodeID, revision: 9 });
  await Promise.resolve();
  await Promise.resolve();
  expect(api.createMediaExport).not.toHaveBeenCalled();
});

it("audio-only timelines disable the video entry but retain editable and media exports", async () => {
  vi.stubGlobal(
    "ResizeObserver",
    class {
      observe() {}
      unobserve() {}
      disconnect() {}
    },
  );
  mount("auto", undefined, false);
  const video = screen.getByRole("button", { name: "导出" });
  expect(video.getAttribute("aria-disabled")).toBe("true");
  expect(screen.queryByRole("dialog")).toBeNull();
  fireEvent.focus(video);
  expect(await screen.findByRole("tooltip")).toHaveProperty(
    "textContent",
    "当前时间线暂不支持此操作",
  );
  const more = screen.getByRole("button", { name: "更多导出方式" });
  expect((more as HTMLButtonElement).disabled).toBe(false);
  fireEvent.keyDown(more, { key: "Enter" });
  expect(
    await screen.findByRole("menuitem", { name: "导出为 XML" }),
  ).toHaveProperty("ariaDisabled", null);
  expect(
    screen.getByRole("menuitem", { name: "批量导出时间线素材" }),
  ).toHaveProperty("ariaDisabled", null);
  expect(api.createMediaExport).not.toHaveBeenCalled();
  api.createMediaExport.mockResolvedValue({
    ...job(),
    status: "queued",
    output_kind: "xmeml",
  });
  fireEvent.click(screen.getByRole("menuitem", { name: "导出为 XML" }));
  await waitFor(() => expect(api.createMediaExport).toHaveBeenCalledTimes(1));
  expect(api.createMediaExport.mock.calls[0][1].output_kind).toBe("xmeml");
});

it("translates an observed dependency 503 and replays the same uncertain export intent", async () => {
  api.createMediaExport
    .mockRejectedValueOnce(new RequestError("dependency_unavailable", 503))
    .mockResolvedValueOnce({ ...job(), status: "queued" });
  const { prepareTimeline } = mount();
  fireEvent.click(screen.getByRole("button", { name: "确认" }));
  await screen.findByText("服务暂时不可用，请稍后重试。");
  expect(screen.queryByText("dependency_unavailable")).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "确认" }));
  await waitFor(() => expect(api.createMediaExport).toHaveBeenCalledTimes(2));
  expect(prepareTimeline).toHaveBeenCalledTimes(1);
  expect(api.createMediaExport.mock.calls[1][1]).toEqual(
    api.createMediaExport.mock.calls[0][1],
  );
  expect(
    api.createMediaExport.mock.calls[1][2].headers["Idempotency-Key"],
  ).toBe(api.createMediaExport.mock.calls[0][2].headers["Idempotency-Key"]);
});

it("keeps an explicit delivery instruction but hides unknown internal error strings", async () => {
  const { prepareTimeline } = mount();
  prepareTimeline.mockRejectedValueOnce(new Error("请等待画布保存后重试"));
  fireEvent.click(screen.getByRole("button", { name: "确认" }));
  await screen.findByText("请等待画布保存后重试");
  prepareTimeline.mockRejectedValueOnce(
    new Error("internal_storage_object_key"),
  );
  fireEvent.click(screen.getByRole("button", { name: "确认" }));
  await screen.findByText("导出操作未完成");
  expect(screen.queryByText("internal_storage_object_key")).toBeNull();
});

it("does not expose dependency codes when restoring export history fails", async () => {
  api.listMediaExports.mockRejectedValueOnce(
    new RequestError("dependency_unavailable", 503),
  );
  mount();
  await screen.findByText("服务暂时不可用，请稍后重试。");
  expect(screen.queryByText("dependency_unavailable")).toBeNull();
});

async function nativeReceipt() {
  const value = await packageReceipt();
  value.current.output_kind = "jianying";
  value.current.status = "succeeded";
  value.receipt.artifact!.format = "jianying";
  value.receipt.artifact!.format_version = "134.0.0";
  value.receipt.launch_url = `${window.location.origin}/api/mediaexports/launch?token=${"a".repeat(64)}`;
  return value;
}
async function chooseJianying() {
  vi.stubGlobal(
    "ResizeObserver",
    class {
      observe() {}
      unobserve() {}
      disconnect() {}
    },
  );
  const cancel = screen.queryByRole("button", { name: "取消" });
  if (cancel) fireEvent.click(cancel);
  fireEvent.keyDown(screen.getByRole("button", { name: "更多导出方式" }), {
    key: "Enter",
  });
  fireEvent.click(await screen.findByRole("menuitem", { name: "导出到剪映" }));
}
it("binds the native launch to the verified Jianying artifact and exact native protocol", async () => {
  const { receipt, current } = await nativeReceipt();
  const url = new URL(timelineJianyingLaunchURL(receipt, current));
  expect(url.protocol).toBe("videocut:");
  expect(url.host).toBe("com.ies.videocut");
  expect(url.pathname).toBe("/uganchor/anchor_point/load_pippit_draft");
  expect(JSON.parse(url.searchParams.get("featureEntry")!)).toEqual({
    sence: "main",
    sence_context: {},
    feature: "load_pippit_draft",
    enter_from: "glowloom_web",
    feature_context: { json_url: receipt.launch_url },
  });
  for (const launch_url of [
    "https://other.example/api/mediaexports/launch?token=" + "a".repeat(64),
    `${window.location.origin}/api/mediaexports/launch?token=bad`,
    `${receipt.launch_url}&job_id=x`,
    `${receipt.launch_url}#fragment`,
    undefined,
  ]) {
    expect(() =>
      timelineJianyingLaunchURL({ ...receipt, launch_url }, current),
    ).toThrow();
  }
  expect(() =>
    timelineJianyingLaunchURL(receipt, { ...current, status: "queued" }),
  ).toThrow();
  expect(() =>
    timelineJianyingLaunchURL(receipt, { ...current, output_kind: "xmeml" }),
  ).toThrow();
  expect(() =>
    timelineJianyingLaunchURL(
      { ...receipt, expires_at: new Date(0).toISOString() },
      current,
    ),
  ).toThrow();
  expect(() =>
    timelineJianyingLaunchURL(
      { ...receipt, artifact: { ...receipt.artifact, format_version: "1" } },
      current,
    ),
  ).toThrow();
});
it("confirms native export before freezing and creating a task, and cancellation creates nothing", async () => {
  api.createMediaExport.mockResolvedValue({
    ...artifactJob(),
    output_kind: "jianying",
    status: "queued",
    artifact_id: undefined,
    sha256: undefined,
  });
  const { prepareTimeline } = mount();
  await chooseJianying();
  expect(screen.getByRole("dialog", { name: "打开剪映专业版" })).toBeTruthy();
  expect(
    screen
      .getByRole("checkbox", { name: "不再提醒" })
      .getAttribute("data-state"),
  ).toBe("unchecked");
  expect(prepareTimeline).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole("button", { name: "取消" }));
  expect(api.createMediaExport).not.toHaveBeenCalled();
  await chooseJianying();
  fireEvent.click(screen.getByRole("button", { name: "打开剪映专业版" }));
  await waitFor(() => expect(api.createMediaExport).toHaveBeenCalledTimes(1));
  expect(api.createMediaExport.mock.calls[0][1]).toMatchObject({
    output_kind: "jianying",
    revision: 9,
    settings: { destination: "local" },
  });
  expect(await screen.findByText("正在导出剪映草稿...")).toBeTruthy();
  expect(screen.getByRole("button", { name: "取消导出" })).toBeTruthy();
});
it("verifies native package before requesting launch, supports repeat launch, and only exposes ZIP fallback on demand", async () => {
  const { blob, receipt, current } = await nativeReceipt();
  api.createMediaExport.mockResolvedValue(current);
  api.downloadMediaExport.mockResolvedValue(blob);
  api.previewMediaExport.mockResolvedValue(receipt);
  const launches: string[] = [];
  vi.spyOn(HTMLAnchorElement.prototype, "click").mockImplementation(function (
    this: HTMLAnchorElement,
  ) {
    launches.push(this.href);
  });
  mount();
  await chooseJianying();
  fireEvent.click(screen.getByRole("button", { name: "打开剪映专业版" }));
  await screen.findByText("已请求打开剪映专业版");
  expect(launches).toEqual([timelineJianyingLaunchURL(receipt, current)]);
  expect(files.saveAs).not.toHaveBeenCalled();
  expect(screen.queryByRole("button", { name: "下载工程包" })).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "再次打开剪映" }));
  expect(launches).toHaveLength(2);
  expect(api.downloadMediaExport).toHaveBeenCalledTimes(1);
  fireEvent.click(screen.getByRole("button", { name: "无法打开？" }));
  fireEvent.click(screen.getByRole("button", { name: "下载工程包" }));
  await waitFor(() =>
    expect(files.saveAs).toHaveBeenCalledWith(blob, "时间线 甲.zip"),
  );
  expect(launches).toHaveLength(2);
});
it("does not launch an unverified native ZIP and permits delivery retry without creating another job", async () => {
  const { blob, receipt, current } = await nativeReceipt();
  api.createMediaExport.mockResolvedValue(current);
  api.downloadMediaExport.mockResolvedValue(
    new Blob(["invalid"], { type: "application/zip" }),
  );
  api.previewMediaExport.mockResolvedValue(receipt);
  const launch = vi
    .spyOn(HTMLAnchorElement.prototype, "click")
    .mockImplementation(() => {});
  mount();
  await chooseJianying();
  fireEvent.click(screen.getByRole("button", { name: "打开剪映专业版" }));
  await screen.findByText("工程文件类型或大小不匹配");
  expect(launch).not.toHaveBeenCalled();
  expect(files.saveAs).not.toHaveBeenCalled();
  api.downloadMediaExport.mockResolvedValue(blob);
  fireEvent.click(screen.getByRole("button", { name: "打开剪映专业版" }));
  await screen.findByText("已请求打开剪映专业版");
  expect(api.createMediaExport).toHaveBeenCalledTimes(1);
  expect(launch).toHaveBeenCalledTimes(1);
});
it("remembers only a confirmed skip-reminder choice and reuses it on the next native export", async () => {
  api.createMediaExport.mockResolvedValue({
    ...artifactJob(),
    output_kind: "jianying",
    status: "queued",
    artifact_id: undefined,
    sha256: undefined,
  });
  mount();
  await chooseJianying();
  fireEvent.click(screen.getByRole("checkbox", { name: "不再提醒" }));
  expect(
    localStorage.getItem("glowloom:timeline:jianying:skip-reminder"),
  ).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "打开剪映专业版" }));
  await waitFor(() => expect(api.createMediaExport).toHaveBeenCalledTimes(1));
  expect(localStorage.getItem("glowloom:timeline:jianying:skip-reminder")).toBe(
    "true",
  );
  cleanup();
  mount();
  await chooseJianying();
  await waitFor(() => expect(api.createMediaExport).toHaveBeenCalledTimes(2));
  expect(screen.queryByRole("dialog", { name: "打开剪映专业版" })).toBeNull();
});
