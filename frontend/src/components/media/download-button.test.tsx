import { render, findFeedback } from "../../../tests/feedback";
import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { DownloadButton } from "./download-button";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});
it("下载生成 API 返回的原始二进制并释放临时地址", async () => {
  const blob = new Blob([new Uint8Array([0, 255, 1])]);
  const create = vi.fn(() => "blob:download");
  const revoke = vi.fn();
  vi.stubGlobal(
    "URL",
    class extends URL {
      static createObjectURL = create;
      static revokeObjectURL = revoke;
    },
  );
  const click = vi
    .spyOn(HTMLAnchorElement.prototype, "click")
    .mockImplementation(function (this: HTMLAnchorElement) {
      expect(this.download).toBe("原件.png");
      expect(this.href).toBe("blob:download");
      expect(this.isConnected).toBe(true);
    });
  const load = vi.fn(async () => blob);
  render(
    <DownloadButton load={load} filename="原件.png">
      下载原件
    </DownloadButton>,
  );
  fireEvent.click(screen.getByRole("button", { name: "下载原件" }));
  await waitFor(() => expect(revoke).toHaveBeenCalledWith("blob:download"));
  expect(create).toHaveBeenCalledWith(blob);
  expect(click).toHaveBeenCalledOnce();
  expect(document.querySelector("a[download]")).toBeNull();
});
it("失败不会保存错误响应，显示重试入口并阻止重复请求", async () => {
  let reject!: (error: Error) => void;
  const load = vi.fn(
    () =>
      new Promise<Blob>((_, fail) => {
        reject = fail;
      }),
  );
  const click = vi.spyOn(HTMLAnchorElement.prototype, "click");
  render(
    <DownloadButton load={load} filename="原件.png">
      下载原件
    </DownloadButton>,
  );
  fireEvent.click(screen.getByRole("button", { name: "下载原件" }));
  const pending = screen.getByRole("button", { name: "正在下载…" });
  fireEvent.click(pending);
  expect(load).toHaveBeenCalledOnce();
  reject(new Error("forbidden"));
  await findFeedback();
  expect(click).not.toHaveBeenCalled();
  expect(
    screen.getByRole("button", { name: "下载原件" }).hasAttribute("disabled"),
  ).toBe(false);
});
