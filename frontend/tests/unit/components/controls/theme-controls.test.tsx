import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
} from "@testing-library/react";
import { ThemeProvider } from "next-themes";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import {
  ThemePreference,
  ThemeToggle,
} from "@/components/controls/theme-controls";

const preferenceMocks = vi.hoisted(() => ({
  session: null as { actor: { id: string } } | null,
  get: vi.fn(),
  update: vi.fn(),
  toast: vi.fn(),
}));
vi.mock("@/components/auth/session", () => ({
  useOptionalSession: () => preferenceMocks.session,
}));
vi.mock("@/api/identity", () => ({
  getMyPreferences: preferenceMocks.get,
  updateMyPreferences: preferenceMocks.update,
}));
vi.mock("sonner", () => ({ toast: preferenceMocks.toast }));

let systemDark = false;
const mediaListeners = new Set<(event: { matches: boolean }) => void>();

beforeEach(() => {
  preferenceMocks.session = null;
  vi.clearAllMocks();
  localStorage.clear();
  systemDark = false;
  mediaListeners.clear();
  vi.stubGlobal("matchMedia", (query: string) => ({
    matches: systemDark,
    media: query,
    addListener: (listener: (event: { matches: boolean }) => void) =>
      mediaListeners.add(listener),
    removeListener: (listener: (event: { matches: boolean }) => void) =>
      mediaListeners.delete(listener),
  }));
});

it("已登录主题保存保留提示词后缀、使用真实版本并读回；菜单关闭不取消保存", async () => {
  preferenceMocks.session = { actor: { id: "creator" } };
  preferenceMocks.get
    .mockResolvedValueOnce({
      theme: "dark",
      prompt_suffix: "保持原后缀",
      revision: "7",
    })
    .mockResolvedValueOnce({
      theme: "light",
      prompt_suffix: "保持原后缀",
      revision: "8",
    });
  preferenceMocks.update.mockResolvedValue({});
  render(
    <ThemeProvider attribute="class" defaultTheme="dark" enableSystem>
      <ThemeToggle />
    </ThemeProvider>,
  );
  fireEvent.keyDown(screen.getByRole("button", { name: "切换主题" }), {
    key: "ArrowDown",
  });
  fireEvent.click(await screen.findByRole("menuitemradio", { name: "浅色" }));
  const { waitFor } = await import("@testing-library/react");
  await waitFor(() =>
    expect(document.documentElement.classList.contains("light")).toBe(true),
  );
  expect(preferenceMocks.update).toHaveBeenCalledWith(
    { expected_revision: "7", theme: "light", prompt_suffix: "保持原后缀" },
    { signal: expect.any(AbortSignal) },
  );
  expect(preferenceMocks.get).toHaveBeenCalledTimes(2);
  expect(screen.queryByRole("menu")).toBeNull();
});

it("主题保存失败不会假装切换成功，允许用户重试", async () => {
  preferenceMocks.session = { actor: { id: "creator" } };
  preferenceMocks.get.mockResolvedValue({
    theme: "dark",
    prompt_suffix: "",
    revision: "7",
  });
  preferenceMocks.update.mockRejectedValue(new Error("conflict"));
  render(
    <ThemeProvider attribute="class" defaultTheme="dark" enableSystem>
      <ThemeToggle />
    </ThemeProvider>,
  );
  fireEvent.keyDown(screen.getByRole("button", { name: "切换主题" }), {
    key: "ArrowDown",
  });
  fireEvent.click(await screen.findByRole("menuitemradio", { name: "浅色" }));
  const { waitFor } = await import("@testing-library/react");
  await waitFor(() =>
    expect(preferenceMocks.toast).toHaveBeenCalledWith("主题未能保存，请重试"),
  );
  expect(document.documentElement.classList.contains("dark")).toBe(true);
  expect(
    screen.getByRole("button", { name: "切换主题" }).hasAttribute("disabled"),
  ).toBe(false);
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  document.documentElement.removeAttribute("class");
  document.documentElement.removeAttribute("style");
});

function renderPreference() {
  return render(
    <ThemeProvider attribute="class" defaultTheme="dark" enableSystem>
      <ThemePreference />
    </ThemeProvider>,
  );
}

it("默认深色，切换浅色即时生效并在重新挂载后恢复", () => {
  const view = renderPreference();
  expect(document.documentElement.classList.contains("dark")).toBe(true);
  fireEvent.click(screen.getByRole("radio", { name: "浅色" }));
  expect(document.documentElement.classList.contains("light")).toBe(true);
  expect(localStorage.getItem("theme")).toBe("light");
  view.unmount();
  renderPreference();
  expect(
    screen.getByRole("radio", { name: "浅色" }).getAttribute("aria-checked"),
  ).toBe("true");
  expect(document.documentElement.classList.contains("light")).toBe(true);
});

it("跟随系统响应外观变化，主动选择深色后不再跟随", () => {
  renderPreference();
  fireEvent.click(screen.getByRole("radio", { name: "跟随系统" }));
  expect(localStorage.getItem("theme")).toBe("system");
  expect(document.documentElement.classList.contains("light")).toBe(true);
  act(() => {
    systemDark = true;
    mediaListeners.forEach((listener) => listener({ matches: true }));
  });
  expect(document.documentElement.classList.contains("dark")).toBe(true);
  expect(
    screen
      .getByRole("radio", { name: "跟随系统" })
      .getAttribute("aria-checked"),
  ).toBe("true");
  fireEvent.click(screen.getByRole("radio", { name: "深色" }));
  act(() => {
    systemDark = false;
    mediaListeners.forEach((listener) => listener({ matches: false }));
  });
  expect(localStorage.getItem("theme")).toBe("dark");
  expect(document.documentElement.classList.contains("dark")).toBe(true);
});

it("再次点击已选主题不会清空选择", () => {
  renderPreference();
  fireEvent.click(screen.getByRole("radio", { name: "浅色" }));
  fireEvent.click(screen.getByRole("radio", { name: "浅色" }));
  expect(localStorage.getItem("theme")).toBe("light");
  expect(
    screen.getByRole("radio", { name: "浅色" }).getAttribute("aria-checked"),
  ).toBe("true");
});

it("独立主题菜单可键盘打开，切换后与偏好控件保持同步并恢复焦点", async () => {
  render(
    <ThemeProvider attribute="class" defaultTheme="dark" enableSystem>
      <ThemeToggle />
      <ThemePreference />
    </ThemeProvider>,
  );
  const trigger = screen.getByRole("button", { name: "切换主题" });
  fireEvent.keyDown(trigger, { key: "ArrowDown" });
  const light = await screen.findByRole("menuitemradio", { name: "浅色" });
  expect(
    screen
      .getByRole("menuitemradio", { name: "深色" })
      .getAttribute("aria-checked"),
  ).toBe("true");
  expect(screen.getByRole("menuitemradio", { name: "跟随系统" })).toBeTruthy();
  fireEvent.click(light);
  expect(localStorage.getItem("theme")).toBe("light");
  expect(document.documentElement.classList.contains("light")).toBe(true);
  const { waitFor } = await import("@testing-library/react");
  await waitFor(() => expect(document.activeElement).toBe(trigger));
  expect(
    screen.getByRole("radio", { name: "浅色" }).getAttribute("aria-checked"),
  ).toBe("true");
});
