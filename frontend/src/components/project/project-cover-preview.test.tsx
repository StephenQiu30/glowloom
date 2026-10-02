import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { ProjectCoverPreview } from "./project-cover-preview";
import { createElement } from "react";
const api = vi.hoisted(() => ({ preview: vi.fn(), disconnect: vi.fn() }));
vi.mock("./project-cover-queries", () => ({
  getProjectCoverPreview: api.preview,
  projectCoverPreviewKey: (
    scope: { actorId: string; orgId: string },
    pid: string,
    aid: string,
  ) => ["cover", scope.actorId, scope.orgId, pid, aid],
}));
vi.mock("next/image", () => ({
  default: ({
    src,
    alt,
    onError,
  }: {
    src: string;
    alt: string;
    onError: () => void;
  }) => createElement("img", { src, alt, onError }),
}));
const pid = "9817c918-e49d-4dc8-b8b6-c92833b135e4",
  aid = "c13b18f1-cd43-4f35-bef4-f06801746458";
const scope = { origin: window.location.origin, actorId: pid, orgId: aid };
let intersection: IntersectionObserverCallback;
beforeEach(() => {
  vi.resetAllMocks();
  api.preview.mockResolvedValue({
    url: "https://objects.invalid/signed-first",
    expires_at: new Date(Date.now() + 600_000).toISOString(),
  });
  vi.stubGlobal(
    "IntersectionObserver",
    class {
      constructor(callback: IntersectionObserverCallback) {
        intersection = callback;
      }
      observe() {}
      disconnect() {
        api.disconnect();
      }
    },
  );
});
afterEach(() => {
  cleanup();
  vi.useRealTimers();
  vi.unstubAllGlobals();
});
function setup(props: { unavailable?: boolean; eager?: boolean } = {}) {
  return render(
    <QueryClientProvider
      client={
        new QueryClient({ defaultOptions: { queries: { retry: false } } })
      }
    >
      <ProjectCoverPreview
        projectId={pid}
        assetId={aid}
        scope={scope}
        {...props}
      />
    </QueryClientProvider>,
  );
}
it("未进视口零预览请求，进入后只取媒体，不请求 project detail", async () => {
  const view = setup();
  expect(api.preview).not.toHaveBeenCalled();
  await act(async () =>
    intersection(
      [{ isIntersecting: true } as IntersectionObserverEntry],
      {} as IntersectionObserver,
    ),
  );
  await waitFor(() =>
    expect(view.container.querySelector("img")?.src).toBe(
      "https://objects.invalid/signed-first",
    ),
  );
  expect(api.preview).toHaveBeenCalledExactlyOnceWith(
    pid,
    aid,
    expect.any(AbortSignal),
  );
  expect(api.disconnect).toHaveBeenCalled();
});
it("主图不可用不请求，签名图片加载失败立即移除旧 URL，明确重试新租约", async () => {
  const unavailable = setup({ unavailable: true, eager: true });
  expect(screen.getByText("主图不可用")).toBeTruthy();
  expect(api.preview).not.toHaveBeenCalled();
  unavailable.unmount();
  const view = setup({ eager: true });
  await waitFor(() => expect(view.container.querySelector("img")).toBeTruthy());
  fireEvent.error(view.container.querySelector("img")!);
  expect(view.container.querySelector("img")).toBeNull();
  api.preview.mockResolvedValue({
    url: "https://objects.invalid/signed-second",
    expires_at: new Date(Date.now() + 600_000).toISOString(),
  });
  fireEvent.click(screen.getByRole("button", { name: "重新读取主图" }));
  await waitFor(() =>
    expect(view.container.querySelector("img")?.src).toBe(
      "https://objects.invalid/signed-second",
    ),
  );
});

it("租约到期前移除私有 URL，卸载清理定时器且不自动重新签名", async () => {
  vi.useFakeTimers({ toFake: ["setTimeout", "clearTimeout", "Date"] });
  const schedule = vi.spyOn(window, "setTimeout");
  const cancel = vi.spyOn(window, "clearTimeout");
  api.preview.mockResolvedValue({
    url: "https://objects.invalid/short-lease",
    expires_at: new Date(Date.now() + 6000).toISOString(),
  });
  const first = setup({ eager: true });
  await act(async () => {
    await vi.advanceTimersByTimeAsync(0);
  });
  const timerIndex = schedule.mock.calls.findIndex(
    ([, delay]) => delay === 1000,
  );
  expect(timerIndex).toBeGreaterThanOrEqual(0);
  first.unmount();
  expect(cancel).toHaveBeenCalledWith(schedule.mock.results[timerIndex].value);
  const view = setup({ eager: true });
  await act(async () => {
    await vi.advanceTimersByTimeAsync(0);
  });
  expect(view.container.querySelector("img")).toBeTruthy();
  await act(async () => {
    await vi.advanceTimersByTimeAsync(1000);
  });
  expect(view.container.querySelector("img")).toBeNull();
  expect(api.preview).toHaveBeenCalledTimes(2);
  view.unmount();
  await act(async () => {
    await vi.advanceTimersByTimeAsync(60_000);
  });
  expect(api.preview).toHaveBeenCalledTimes(2);
});
