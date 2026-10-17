import { SessionProvider } from "@/components/auth/session";
import {
  cleanup,
  fireEvent,
  render as renderUI,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import axios from "axios";
import { SessionGate } from "@/components/auth/session";
import { Settings } from "./account-page";
import { request, setCSRFToken, setCreationScope } from "@/lib/request";
vi.mock("@/lib/request", async (original) => ({
  ...(await original<typeof import("@/lib/request")>()),
  request: vi.fn(),
}));
const mocks = vi.hoisted(() => ({
  replace: vi.fn(),
  clear: vi.fn().mockResolvedValue(undefined),
  theme: vi.fn(),
}));
vi.mock("next/navigation", () => ({
  usePathname: () => "/",
  useRouter: () => mocks,
}));
vi.mock("next-themes", () => ({ useTheme: () => ({ setTheme: mocks.theme }) }));
vi.mock("@/components/generation/draft-save", async (original) => ({
  ...(await original<typeof import("@/components/generation/draft-save")>()),
  branchJournal: { clear: mocks.clear },
}));
const profile = {
  id: "11111111-1111-4111-8111-111111111111",
  email: "private@example.com",
  display_name: "Private creator",
  platform_role: "creator",
  status: "active",
  revision: "1",
  capabilities: [],
  avatar_asset_id: null,
  email_verified_at: new Date().toISOString(),
};
const session = {
  actor: profile,
  session_id: "22222222-2222-4222-8222-222222222222",
  csrf_token: "current-proof",
  credential_epoch: "1",
  must_change_password: false,
  persistent: false,
  last_active_at: new Date().toISOString(),
  absolute_expires_at: new Date(Date.now() + 86400000).toISOString(),
};
afterEach(() => {
  cleanup();
  setCSRFToken(null);
  vi.resetAllMocks();
});
it("a loaded account mutation 401 clears identity and unmounts private fields", async () => {
  const actual =
    await vi.importActual<typeof import("@/lib/request")>("@/lib/request");
  vi.mocked(request).mockImplementation(async (url, options) => {
    if (options?.method === "PATCH")
      return actual.request(url, {
        ...options,
        adapter: async (config) => {
          throw new axios.AxiosError(
            "unauthenticated",
            undefined,
            config,
            undefined,
            {
              status: 401,
              statusText: "Unauthorized",
              headers: {},
              config,
              data: { code: "unauthenticated" },
            },
          );
        },
      });
    if (url === "/api/session") return session;
    if (url === "/api/me") return profile;
    if (url === "/api/me/preferences")
      return { theme: "system", prompt_suffix: "", revision: "1" };
    return { items: [], next_cursor: null };
  });
  render(
    <SessionGate>
      <Settings />
    </SessionGate>,
  );
  const field = await screen.findByLabelText("显示名");
  fireEvent.change(field, { target: { value: "Updated" } });
  setCreationScope("33333333-3333-4333-8333-333333333333");
  fireEvent.click(screen.getByText("保存资料"));
  await waitFor(() => expect(mocks.replace).toHaveBeenCalledWith("/login"));
  expect(screen.queryByLabelText("显示名")).toBeNull();
  expect(screen.queryByDisplayValue("private@example.com")).toBeNull();
  await waitFor(() => expect(mocks.clear).toHaveBeenCalled());
  const state = await actual.request<{ proof: boolean; scope: boolean }>(
    "/api/me",
    {
      method: "POST",
      adapter: async (config) => ({
        data: {
          proof: config.headers.has("X-CSRF-Token"),
          scope: config.headers.has("X-Creation-Scope"),
        },
        status: 200,
        statusText: "OK",
        headers: {},
        config,
      }),
    },
  );
  expect(state).toEqual({ proof: false, scope: false });
});

function render(ui: React.ReactNode) {
  return renderUI(ui, { wrapper: SessionProvider });
}
