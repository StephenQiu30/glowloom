import { render } from "../../../../../feedback";
import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { PlatformSettingsPage } from "@/app/platform/settings/_components/platform-settings-page";
import { request } from "@/lib/request";
vi.mock("@/lib/request", async (original) => ({
  ...(await original<typeof import("@/lib/request")>()),
  request: vi.fn(),
}));
afterEach(() => {
  cleanup();
  vi.resetAllMocks();
});
it("设置只在明确密码确认后提交当前版本和原因，不提前改变注册开关", async () => {
  vi.mocked(request).mockImplementation(async (url) =>
    url === "/api/platform/settings"
      ? {
          revision: "12",
          registration_enabled: false,
          team_creation_enabled: true,
        }
      : undefined,
  );
  render(<PlatformSettingsPage />);
  const registration = await screen.findByRole("checkbox", {
    name: "允许公开注册",
  });
  expect(registration).toHaveProperty("ariaChecked", "false");
  fireEvent.click(registration);
  fireEvent.click(screen.getByText("保存平台设置"));
  expect(request).toHaveBeenCalledTimes(1);
  fireEvent.change(screen.getByLabelText("当前密码"), {
    target: { value: "password" },
  });
  fireEvent.change(screen.getByLabelText("操作原因"), {
    target: { value: "调整注册策略" },
  });
  fireEvent.click(screen.getByText("确认操作"));
  await waitFor(() =>
    expect(request).toHaveBeenCalledWith(
      "/api/platform/settings",
      expect.objectContaining({
        method: "PATCH",
        data: {
          expected_revision: "12",
          reason: "调整注册策略",
          registration_enabled: true,
          team_creation_enabled: true,
        },
      }),
    ),
  );
});
