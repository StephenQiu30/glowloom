import {
  cleanup,
  act,
  render,
  screen,
  fireEvent,
  waitFor,
} from "@testing-library/react";
import { StrictMode } from "react";
import { afterEach, expect, it, vi } from "vitest";
import { SessionGate } from "./session";
import { request, RequestError } from "@/lib/request";
vi.mock("@/lib/request", async (original) => ({
  ...(await original<typeof import("@/lib/request")>()),
  request: vi.fn(),
}));
vi.mock("next/navigation", () => ({ useRouter: () => router }));
const router = { replace: vi.fn() };
const notifications = vi.hoisted(() => ({ show: vi.fn(), dismiss: vi.fn() }));
vi.mock("sonner", () => ({
  toast: Object.assign(notifications.show, { dismiss: notifications.dismiss }),
}));
afterEach(() => {
  cleanup();
  vi.resetAllMocks();
});
it("startup announces one pending state without rendering private children", () => {
  vi.mocked(request).mockReturnValue(new Promise(() => {}));
  const { container } = render(
    <SessionGate>
      <p>private content</p>
    </SessionGate>,
  );
  expect(screen.getByRole("status").textContent).toBe("正在验证登录状态…");
  expect(screen.getAllByRole("status")).toHaveLength(1);
  expect(screen.queryByText("private content")).toBeNull();
  expect(
    container
      .querySelector('[data-slot="skeleton"]')
      ?.closest('[aria-hidden="true"]'),
  ).toBeTruthy();
  expect(screen.queryByRole("progressbar")).toBeNull();
});

it("protected failure stops loading and retry returns to the pending boundary", async () => {
  vi.mocked(request)
    .mockRejectedValueOnce(new RequestError("temporarily_unavailable", 503))
    .mockReturnValueOnce(new Promise(() => {}));
  render(
    <SessionGate>
      <p>private content</p>
    </SessionGate>,
  );
  await screen.findByRole("heading", { name: "暂时无法确认登录状态" });
  expect(screen.queryByRole("status")).toBeNull();
  expect(screen.queryByText("private content")).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "重试" }));
  expect(await screen.findByRole("status")).toBeTruthy();
  expect(screen.queryByRole("alert")).toBeNull();
});
it("public session failure uses one short toast and keeps the page free of a banner", async () => {
  vi.mocked(request)
    .mockRejectedValueOnce(new RequestError("invalid_receipt"))
    .mockRejectedValueOnce(new RequestError("unauthenticated", 401));
  render(
    <SessionGate allowGuest>
      <p>public content</p>
    </SessionGate>,
  );
  await screen.findByText("public content");
  expect(screen.queryByRole("alert")).toBeNull();
  expect(screen.queryByLabelText("会话状态")).toBeNull();
  expect(notifications.show).toHaveBeenCalledTimes(1);
  expect(notifications.show).toHaveBeenCalledWith(
    "暂时无法确认登录状态",
    expect.objectContaining({
      action: expect.objectContaining({ label: "重试" }),
    }),
  );
  const [title, options] = notifications.show.mock.lastCall!;
  expect(title).not.toMatch(/回执|服务端|结果未确认/);
  act(() => options.action.onClick());
  await waitFor(() => expect(request).toHaveBeenCalledTimes(2));
  expect(notifications.dismiss).toHaveBeenCalledWith(options.id);
  expect(router.replace).not.toHaveBeenCalled();
});

it("normal anonymous browsing does not show any session notification", async () => {
  vi.mocked(request).mockRejectedValue(
    new RequestError("unauthenticated", 401),
  );
  render(
    <SessionGate allowGuest>
      <p>public content</p>
    </SessionGate>,
  );
  await screen.findByText("public content");
  expect(notifications.show).not.toHaveBeenCalled();
  expect(screen.queryByRole("alert")).toBeNull();
});

it("unmounting a public page ignores a late failure", async () => {
  let reject!: (error: unknown) => void;
  vi.mocked(request).mockReturnValue(
    new Promise((_, fail) => {
      reject = fail;
    }),
  );
  const view = render(
    <SessionGate allowGuest>
      <p>public content</p>
    </SessionGate>,
  );
  view.unmount();
  await act(async () => reject(new RequestError("invalid_receipt")));
  expect(notifications.show).not.toHaveBeenCalled();
});

it("StrictMode emits only one toast for a current failed session check", async () => {
  vi.mocked(request).mockRejectedValue(
    new RequestError("temporarily_unavailable", 503),
  );
  const view = render(
    <StrictMode>
      <SessionGate allowGuest>
        <p>public content</p>
      </SessionGate>
    </StrictMode>,
  );
  await screen.findByText("public content");
  expect(notifications.show).toHaveBeenCalledTimes(1);
  const options = notifications.show.mock.lastCall![1];
  view.unmount();
  expect(notifications.dismiss).toHaveBeenCalledWith(options.id);
});

const validSession = {
  actor: {
    id: "11111111-1111-4111-8111-111111111111",
    email: "creator@example.com",
    display_name: "Creator",
    platform_role: "creator",
    status: "active",
    revision: "1",
    capabilities: [],
  },
  session_id: "22222222-2222-4222-8222-222222222222",
  csrf_token: "proof",
  credential_epoch: "1",
  must_change_password: false,
  persistent: false,
  last_active_at: new Date().toISOString(),
  absolute_expires_at: new Date(Date.now() + 86400000).toISOString(),
};
it.each(["authenticated", "guest"])(
  "toast retry keeps the page mounted while checking, then reaches %s",
  async (terminal) => {
    let finish!: (value: unknown) => void;
    let fail!: (error: unknown) => void;
    const pending = new Promise((resolve, reject) => {
      finish = resolve;
      fail = reject;
    });
    vi.mocked(request)
      .mockRejectedValueOnce(new RequestError("temporarily_unavailable", 503))
      .mockReturnValueOnce(pending)
      .mockResolvedValue(validSession);
    render(
      <SessionGate allowGuest>
        <input aria-label="public draft" defaultValue="existing idea" />
      </SessionGate>,
    );
    const draft = await screen.findByLabelText("public draft");
    fireEvent.change(draft, { target: { value: "keep this idea" } });
    const options = notifications.show.mock.lastCall![1];
    act(() => options.action.onClick());
    expect(screen.getByLabelText("public draft")).toBe(draft);
    expect(draft).toHaveProperty("value", "keep this idea");
    await act(async () => {
      if (terminal === "authenticated") finish(validSession);
      else fail(new RequestError("unauthenticated", 401));
    });
    await waitFor(() =>
      expect(notifications.dismiss).toHaveBeenCalledWith(options.id),
    );
    expect(screen.queryByLabelText("会话状态")).toBeNull();
    expect(screen.getByLabelText("public draft")).toBeTruthy();
    if (terminal === "guest")
      expect(screen.getByLabelText("public draft")).toBe(draft);
  },
);

it("旧服务端改密标记为true也正常渲染已登录内容，不强制跳转", async () => {
  vi.mocked(request).mockResolvedValue({
    ...validSession,
    must_change_password: true,
  });
  render(
    <SessionGate>
      <p>正常账号页面</p>
    </SessionGate>,
  );
  expect(await screen.findByText("正常账号页面")).toBeTruthy();
  expect(router.replace).not.toHaveBeenCalledWith("/reset-password");
});
