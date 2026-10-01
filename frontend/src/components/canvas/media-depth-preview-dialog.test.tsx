import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { MediaDepthPreviewDialog } from "./media-depth-preview-dialog";
const query = vi.hoisted(() => vi.fn());
vi.mock("./media-depth-queries", () => ({ previewDepth: query }));
const id = "fba9de81-80c4-49ce-8f59-1dac39717a04",
  project = "b580ad59-17a5-4ed2-985b-cb0cdd04b4c4",
  canvas = "5db0c65d-d391-45a0-ab1a-d7f6eb4c868d",
  node = "0c6e50ee-0912-4da2-96b3-c84e27b61389";
const job = {
  id,
  project_id: project,
  source: { canvas_id: canvas, node_id: node, revision: 4 },
  source_asset_id: node,
  source_asset_revision: 1,
  source_sha256: "a".repeat(64),
  profile_id: "vda-small-relative-v1" as const,
  status: "review_required" as const,
  stage: "review" as const,
  attempt: 1,
  revision: 8,
  asset_id: canvas,
  sha256: "b".repeat(64),
  failure_code: null,
  retryable: false,
  needs_reconciliation: false,
  reconciliation_requested: false,
  execution_unconfirmed: false,
  cancellation_requested: false,
  created_at: "2026-10-01T19:00:00Z",
  updated_at: "2026-10-01T19:00:00Z",
};
const preview = {
  job_id: id,
  revision: 8,
  sha256: job.sha256!,
  url: "http://127.0.0.1:9000/synthetic.mp4",
  expires_at: new Date(Date.now() + 60000).toISOString(),
  asset: {
    id: canvas,
    project_id: project,
    kind: "video" as const,
    mime_type: "video/mp4" as const,
    file_name: "depth.mp4",
    byte_size: 2000,
    width: 1920 as const,
    height: 1080 as const,
    duration_ms: 2000,
    revision: 1,
  },
};
const clients: QueryClient[] = [];
beforeEach(() =>
  vi.stubGlobal(
    "ResizeObserver",
    class {
      observe() {}
      unobserve() {}
      disconnect() {}
    },
  ),
);
afterEach(() => {
  cleanup();
  clients.splice(0).forEach((c) => c.clear());
  vi.clearAllMocks();
  vi.unstubAllGlobals();
});
it("真实视频可解码后才接受明确人工确认，审核提交绑定确切 SHA 与修订", async () => {
  query.mockResolvedValue(preview);
  const review = vi.fn(),
    client = new QueryClient({
      defaultOptions: { queries: { retry: false, gcTime: 0 } },
    });
  clients.push(client);
  render(
    <QueryClientProvider client={client}>
      <MediaDepthPreviewDialog
        job={job}
        locked={false}
        onClose={vi.fn()}
        onReview={review}
      />
    </QueryClientProvider>,
  );
  const video = await screen.findByLabelText("确切深度结果预览");
  expect(
    screen
      .getByRole("button", { name: "确认人工审核通过" })
      .hasAttribute("disabled"),
  ).toBe(true);
  Object.defineProperties(video, {
    videoWidth: { value: 1920 },
    videoHeight: { value: 1080 },
    duration: { value: 2 },
  });
  fireEvent.loadedMetadata(video);
  fireEvent.canPlay(video);
  fireEvent.click(screen.getByRole("checkbox", { name: /我已实际查看/ }));
  fireEvent.click(screen.getByRole("button", { name: "确认人工审核通过" }));
  await waitFor(() => expect(review).toHaveBeenCalledWith(preview));
});
it("不匹配的真实播放尺寸禁止审核；未知请求锁住关闭和审核", async () => {
  query.mockResolvedValue(preview);
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: 0 } },
  });
  clients.push(client);
  const close = vi.fn();
  render(
    <QueryClientProvider client={client}>
      <MediaDepthPreviewDialog
        job={job}
        locked
        onClose={close}
        onReview={vi.fn()}
      />
    </QueryClientProvider>,
  );
  const video = await screen.findByLabelText("确切深度结果预览");
  Object.defineProperties(video, {
    videoWidth: { value: 160 },
    videoHeight: { value: 90 },
    duration: { value: 2 },
  });
  fireEvent.loadedMetadata(video);
  fireEvent.canPlay(video);
  await screen.findByText(/视频尺寸或时长/);
  expect(
    screen
      .getByRole("button", { name: "关闭深度预览" })
      .hasAttribute("disabled"),
  ).toBe(true);
  fireEvent.keyDown(document, { key: "Escape" });
  expect(close).not.toHaveBeenCalled();
});
it("审核未知后在仍锁住的预览内人工核验原请求，键盘焦点无需离开对话框", async () => {
  query.mockResolvedValue(preview);
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: 0 } },
  });
  clients.push(client);
  const verify = vi.fn().mockResolvedValue(undefined);
  render(
    <QueryClientProvider client={client}>
      <MediaDepthPreviewDialog
        job={job}
        locked
        onClose={vi.fn()}
        onReview={vi.fn()}
        onVerifyOriginal={verify}
      />
    </QueryClientProvider>,
  );
  fireEvent.click(
    await screen.findByRole("button", { name: "核验原审核请求" }),
  );
  expect(verify).toHaveBeenCalledOnce();
  expect(
    screen
      .getByRole("button", { name: "关闭深度预览" })
      .hasAttribute("disabled"),
  ).toBe(true);
});
it("审核控件锁住后把焦点交给可用的原请求核验或存储恢复按钮", async () => {
  query.mockResolvedValue(preview);
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: 0 } },
  });
  clients.push(client);
  const props = { job, onClose: vi.fn(), onReview: vi.fn() };
  const view = render(
    <QueryClientProvider client={client}>
      <MediaDepthPreviewDialog {...props} locked={false} />
    </QueryClientProvider>,
  );
  const video = await screen.findByLabelText("确切深度结果预览");
  Object.defineProperties(video, {
    videoWidth: { value: 1920 },
    videoHeight: { value: 1080 },
    duration: { value: 2 },
  });
  fireEvent.loadedMetadata(video);
  fireEvent.click(screen.getByRole("checkbox", { name: /我已实际查看/ }));
  const submit = screen.getByRole("button", { name: "确认人工审核通过" });
  submit.focus();
  expect(document.activeElement).toBe(submit);
  view.rerender(
    <QueryClientProvider client={client}>
      <MediaDepthPreviewDialog
        {...props}
        locked
        onVerifyOriginal={vi.fn().mockResolvedValue(undefined)}
      />
    </QueryClientProvider>,
  );
  const original = await screen.findByRole("button", {
    name: "核验原审核请求",
  });
  await waitFor(() => expect(document.activeElement).toBe(original));
  view.rerender(
    <QueryClientProvider client={client}>
      <MediaDepthPreviewDialog {...props} locked onRecoverStorage={vi.fn()} />
    </QueryClientProvider>,
  );
  const storage = await screen.findByRole("button", {
    name: "重新检查浏览器存储",
  });
  await waitFor(() => expect(document.activeElement).toBe(storage));
});
