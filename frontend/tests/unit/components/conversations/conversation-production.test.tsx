import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { SWRConfig } from "swr";
import { Blob as NodeBlob } from "node:buffer";
import { webcrypto } from "node:crypto";
import { ConversationProduction } from "@/components/conversations/conversation-production";
import { RequestError } from "@/lib/request";
const api = vi.hoisted(() => ({
  getMediaExport: vi.fn(),
  previewMediaExport: vi.fn(),
  reviewMediaExport: vi.fn(),
  downloadMediaExport: vi.fn(),
}));
const files = vi.hoisted(() => ({ saveAs: vi.fn() }));
vi.mock("@/api/mediaExports", () => api);
vi.mock("file-saver", () => files);
const pid = "11111111-1111-4111-8111-111111111111",
  canvas = "11111111-1111-4111-8111-111111111112",
  node = "11111111-1111-4111-8111-111111111113",
  video = "11111111-1111-4111-8111-111111111114",
  project = "11111111-1111-4111-8111-111111111115",
  asset = "11111111-1111-4111-8111-111111111116";
const job = (): API.ExportJobResponse => ({
  id: video,
  project_id: pid,
  source: { canvas_id: canvas, node_id: node, revision: 7 },
  revision: 3,
  output_kind: "video",
  status: "review_required",
  asset_id: asset,
  sha256: "a".repeat(64),
});
const preview = (): API.ExportPreview => ({
  job_id: video,
  revision: 3,
  sha256: job().sha256!,
  asset: { id: asset, project_id: pid, kind: "video" },
  url: "https://media.example/shot.mp4",
  expires_at: new Date(Date.now() + 60000).toISOString(),
});
function mount(
  facts: unknown = {
    cost: {
      currency: "CNY",
      unit: "micros",
      authorized_limit: 1000000,
      pending_commitment: 200000,
      settled_charge: 300000,
      remaining: 500000,
    },
    deliveries: [
      {
        delivery_id: project,
        canvas_id: canvas,
        timeline_node_id: node,
        video_export_id: video,
        project_export_id: project,
      },
    ],
  },
) {
  const refresh = vi.fn();
  render(
    <SWRConfig value={{ provider: () => new Map(), dedupingInterval: 0 }}>
      <ConversationProduction
        writable
        facts={facts}
        rootRunID={canvas}
        projectID={pid}
        onRefresh={refresh}
      />
    </SWRConfig>,
  );
  return refresh;
}
beforeEach(() => {
  vi.resetAllMocks();
  api.getMediaExport.mockImplementation(async ({ job_id }) =>
    job_id === video
      ? job()
      : {
          ...job(),
          id: project,
          output_kind: "fcpxml",
          status: "queued",
          asset_id: null,
          sha256: null,
        },
  );
  api.previewMediaExport.mockResolvedValue(preview());
});
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

it("requires the actual matching preview and explicit human confirmation, replaying a lost review response with the same key", async () => {
  api.reviewMediaExport
    .mockRejectedValueOnce(new RequestError("network_error"))
    .mockResolvedValueOnce({ ...job(), status: "succeeded", revision: 4 });
  const refresh = mount();
  const confirm = await screen.findByRole("button", { name: "确认成片" });
  expect((confirm as HTMLButtonElement).disabled).toBe(true);
  expect(api.reviewMediaExport).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole("button", { name: "预览成片" }));
  const player = await screen.findByLabelText("成片预览");
  expect(
    (screen.getByRole("checkbox", { name: "已检查成片" }) as HTMLButtonElement)
      .disabled,
  ).toBe(true);
  fireEvent.loadedData(player);
  fireEvent.click(screen.getByRole("checkbox", { name: "已检查成片" }));
  fireEvent.click(confirm);
  await screen.findByRole("alert");
  fireEvent.click(confirm);
  await screen.findByRole("button", { name: "下载成片" });
  expect(api.reviewMediaExport).toHaveBeenCalledTimes(2);
  expect(api.reviewMediaExport.mock.calls[0]![1]).toEqual({
    project_id: pid,
    revision: 3,
    sha256: job().sha256,
    local_review_confirmed: true,
  });
  expect(api.reviewMediaExport.mock.calls[0]![2].headers).toEqual(
    api.reviewMediaExport.mock.calls[1]![2].headers,
  );
  expect(refresh).toHaveBeenCalledTimes(1);
  expect(api.downloadMediaExport).not.toHaveBeenCalled();
});

it("rejects another timeline's job and a changed preview without approving or downloading", async () => {
  api.previewMediaExport.mockResolvedValue({ ...preview(), revision: 99 });
  mount();
  fireEvent.click(await screen.findByRole("button", { name: "预览成片" }));
  await screen.findByRole("alert");
  expect(screen.queryByLabelText("成片预览")).toBeNull();
  expect(api.reviewMediaExport).not.toHaveBeenCalled();
  expect(api.downloadMediaExport).not.toHaveBeenCalled();
});

it("verifies reviewed download bytes before saving the file", async () => {
  vi.stubGlobal("Blob", NodeBlob);
  vi.stubGlobal("crypto", webcrypto);
  api.getMediaExport.mockResolvedValue({ ...job(), status: "succeeded" });
  api.downloadMediaExport.mockResolvedValue(
    new NodeBlob(["corrupt bytes"], { type: "video/mp4" }),
  );
  mount({
    deliveries: [
      {
        delivery_id: project,
        canvas_id: canvas,
        timeline_node_id: node,
        video_export_id: video,
        project_export_id: project,
      },
    ],
  });
  fireEvent.click(
    (await screen.findAllByRole("button", { name: "下载成片" }))[0]!,
  );
  await waitFor(() => expect(api.downloadMediaExport).toHaveBeenCalledTimes(1));
  await screen.findByRole("alert");
  expect(files.saveAs).not.toHaveBeenCalled();
  expect(api.reviewMediaExport).not.toHaveBeenCalled();
});

it("requires a fresh inspection and review intent after the original export advances to a new version", async () => {
  let current = job();
  api.getMediaExport.mockImplementation(async ({ job_id }) =>
    job_id === video
      ? current
      : {
          ...job(),
          id: project,
          output_kind: "fcpxml",
          status: "queued",
          asset_id: null,
          sha256: null,
        },
  );
  api.reviewMediaExport
    .mockRejectedValueOnce(new RequestError("revision_conflict", 409))
    .mockResolvedValueOnce({
      ...job(),
      revision: 6,
      status: "succeeded",
      sha256: "b".repeat(64),
    });
  mount();
  fireEvent.click(await screen.findByRole("button", { name: "预览成片" }));
  fireEvent.loadedData(await screen.findByLabelText("成片预览"));
  fireEvent.click(screen.getByRole("checkbox", { name: "已检查成片" }));
  fireEvent.click(screen.getByRole("button", { name: "确认成片" }));
  await screen.findByRole("alert");
  current = { ...job(), revision: 5, sha256: "b".repeat(64) };
  api.previewMediaExport.mockResolvedValue({
    ...preview(),
    revision: 5,
    sha256: current.sha256,
  });
  fireEvent.click(screen.getByRole("button", { name: "核对原交付" }));
  await waitFor(() =>
    expect(
      (screen.getByRole("button", { name: "确认成片" }) as HTMLButtonElement)
        .disabled,
    ).toBe(true),
  );
  fireEvent.click(screen.getByRole("button", { name: "预览成片" }));
  fireEvent.loadedData(await screen.findByLabelText("成片预览"));
  fireEvent.click(screen.getByRole("checkbox", { name: "已检查成片" }));
  fireEvent.click(screen.getByRole("button", { name: "确认成片" }));
  await screen.findByRole("button", { name: "下载成片" });
  expect(api.reviewMediaExport.mock.calls[1]![1]).toMatchObject({
    revision: 5,
    sha256: current.sha256,
  });
  expect(api.reviewMediaExport.mock.calls[0]![2].headers).not.toEqual(
    api.reviewMediaExport.mock.calls[1]![2].headers,
  );
});
