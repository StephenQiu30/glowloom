import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import {
  CanvasConnectionState,
  canvasConnectionMessage,
} from "@/components/canvas/connection-state";

afterEach(cleanup);
it("keeps a space selection path when a refreshed team canvas opens in personal scope", () => {
  render(
    <CanvasConnectionState
      message="forbidden"
      onRetry={vi.fn()}
      scopeControl={
        <select aria-label="当前创作空间">
          <option>团队空间</option>
        </select>
      }
    />,
  );
  expect(screen.getByRole("combobox", { name: "当前创作空间" })).toBeTruthy();
});
it("explains incompatible versions without exposing the transport code", () => {
  const retry = vi.fn();
  render(
    <CanvasConnectionState
      message="unsupported_protocol_version"
      onRetry={retry}
    />,
  );
  expect(screen.getByRole("alert").textContent).toContain("画布服务版本不兼容");
  expect(screen.queryByText("unsupported_protocol_version")).toBeNull();
  expect(
    screen.getByRole("link", { name: "返回画布列表" }).getAttribute("href"),
  ).toBe("/projects");
  fireEvent.click(screen.getByRole("button", { name: "重新连接" }));
  expect(retry).toHaveBeenCalledOnce();
});
it("shows pending without an actionable second retry", () => {
  render(<CanvasConnectionState message="" onRetry={vi.fn()} />);
  expect(screen.getByRole("status").getAttribute("aria-busy")).toBe("true");
  expect(screen.queryByRole("button", { name: "重新连接" })).toBeNull();
});
it("keeps recovery guidance and does not promise that unknown writes were saved", () => {
  expect(
    canvasConnectionMessage("原版本的提交结果尚未确认，输入继续保留在本页"),
  ).toContain("尚未确认");
  expect(
    canvasConnectionMessage("internal_secret_error /private/server"),
  ).not.toContain("internal_secret");
  expect(
    canvasConnectionMessage("internal_secret_error /private/server"),
  ).toContain("尚未确认");
});

it("offers space recovery only for access failures, separate from retry actions", () => {
  const props = { onRetry: vi.fn(), scopeControl: <button>切换空间</button> };
  const view = render(
    <CanvasConnectionState {...props} message="unsupported_protocol_version" />,
  );
  expect(screen.queryByRole("button", { name: "切换空间" })).toBeNull();
  view.rerender(
    <CanvasConnectionState {...props} message="画布访问权限已失效" />,
  );
  expect(screen.getByRole("button", { name: "切换空间" })).toBeTruthy();
  expect(screen.queryByRole("button", { name: "重新连接" })).toBeNull();
});
