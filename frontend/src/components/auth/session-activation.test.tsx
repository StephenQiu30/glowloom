import { SessionProvider } from "./session";
import {
  act,
  cleanup,
  fireEvent,
  render as renderUI,
  screen,
  waitFor,
} from "@testing-library/react";
import axios from "axios";
import { afterEach, expect, it, vi } from "vitest";
import { SessionGate, useOptionalSession, useSessionUpdate } from "./session";
import {
  getSessionGeneration,
  notifySessionInvalidation,
  request,
  setCreationScope,
  setCSRFToken,
} from "@/lib/request";

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
vi.mock("@/components/generation/draft-save", () => ({
  branchJournal: {
    clear: mocks.clear,
    checkpoint: async () => null,
    activateAccount: async () => {},
  },
}));

function session(name: string): API.EmailSessionView {
  const digit = name === "old" ? "1" : name === "next" ? "2" : "3";
  return {
    actor: {
      id: `${digit.repeat(8)}-1111-4111-8111-111111111111`,
      email: `${name}@example.com`,
      email_verified_at: new Date().toISOString(),
      avatar_asset_id: "",
      display_name: name,
      platform_role: "creator",
      status: "active",
      revision: "1",
      capabilities: [],
    },
    session_id: `${digit.repeat(8)}-2222-4222-8222-222222222222`,
    csrf_token: `proof-${name}`,
    credential_epoch: "1",
    persistent: false,
    last_active_at: new Date().toISOString(),
    server_time: new Date().toISOString(),
    idle_expires_at: new Date(Date.now() + 12 * 3600000).toISOString(),
    session_protocol: "v1",
    upgrade_available: false,
    absolute_expires_at: new Date(Date.now() + 86400000).toISOString(),
  };
}

function GuestLogin() {
  const current = useOptionalSession();
  const update = useSessionUpdate();
  return current ? (
    <p>private {current.actor.display_name}</p>
  ) : (
    <>
      <p>guest</p>
      {["next", "latest"].map((name) => (
        <button
          key={name}
          onClick={() => {
            const next = session(name);
            // AuthView receives the proof before reporting its session to the gate.
            setCSRFToken(next.csrf_token);
            update(next);
          }}
        >
          login {name}
        </button>
      ))}
    </>
  );
}

afterEach(() => {
  cleanup();
  setCSRFToken(null);
  vi.resetAllMocks();
});

it("guest login after 401 waits for journal cleanup and activates only the latest identity", async () => {
  const actual =
    await vi.importActual<typeof import("@/lib/request")>("@/lib/request");
  vi.mocked(request).mockImplementation(async (url) =>
    url === "/api/session"
      ? session("old")
      : { theme: "system", prompt_suffix: "", revision: "1" },
  );
  let finishCleanup!: () => void;
  const pendingCleanup = new Promise<void>((resolve) => {
    finishCleanup = resolve;
  });
  mocks.clear.mockReturnValue(pendingCleanup);
  const proofState = () =>
    actual.request<{ proof: string | undefined; scope: boolean }>("/api/me", {
      method: "POST",
      adapter: async (config) => ({
        data: {
          proof: config.headers.get("X-CSRF-Token"),
          scope: config.headers.has("X-Creation-Scope"),
        },
        status: 200,
        statusText: "OK",
        headers: {},
        config,
      }),
    });
  render(
    <SessionGate allowGuest>
      <GuestLogin />
    </SessionGate>,
  );
  await screen.findByText("private old");
  const oldGeneration = getSessionGeneration();
  setCreationScope("33333333-3333-4333-8333-333333333333");
  try {
    await act(async () => {
      await actual
        .request("/api/me", {
          method: "PATCH",
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
        })
        .catch(() => {});
    });
    await screen.findByText("guest");
    await waitFor(() => expect(mocks.clear).toHaveBeenCalledTimes(1));
    fireEvent.click(screen.getByText("login next"));
    expect(screen.queryByText("private next")).toBeNull();
    expect(await proofState()).toEqual({ proof: undefined, scope: false });
    fireEvent.click(screen.getByText("login latest"));
    expect(screen.queryByText("private latest")).toBeNull();
    expect(await proofState()).toEqual({ proof: undefined, scope: false });
  } finally {
    await act(async () => finishCleanup());
  }
  await screen.findByText("private latest");
  expect(screen.queryByText("private next")).toBeNull();
  expect(await proofState()).toEqual({ proof: "proof-latest", scope: false });
  act(() => notifySessionInvalidation(oldGeneration));
  expect(screen.getByText("private latest")).toBeTruthy();
  expect(mocks.clear).toHaveBeenCalledTimes(1);
});

function render(ui: React.ReactNode) {
  return renderUI(ui, { wrapper: SessionProvider });
}
