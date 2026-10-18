import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { ProvidersPage } from "./providers-page";
const api = vi.hoisted(() => ({ providers: vi.fn(), presets: vi.fn() }));
vi.mock("@/components/auth/session", () => ({
  useSession: () => ({ actor: { id: "creator", platform_role: "creator" } }),
}));
vi.mock("@/api/settings", () => ({
  listAdminProviders: api.providers,
  listProviderPresets: api.presets,
  getConfigReceipt: vi.fn(),
  updateProviderPresetState: vi.fn(),
  exportProviderPreset: vi.fn(),
}));
vi.mock("./preset-editor", () => ({ PresetEditor: () => null }));
vi.mock("./preset-connect", () => ({ PresetConnect: () => null }));
vi.mock("./connection-detail", () => ({ ConnectionDetail: () => null }));
afterEach(cleanup);
it("普通账号不发管理查询", () => {
  render(<ProvidersPage />);
  expect(screen.getByText("暂无访问权限")).toBeTruthy();
  expect(api.providers).not.toHaveBeenCalled();
  expect(api.presets).not.toHaveBeenCalled();
});
