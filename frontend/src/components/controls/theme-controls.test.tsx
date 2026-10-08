import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
} from "@testing-library/react";
import { ThemeProvider } from "next-themes";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { ThemePreference } from "./theme-controls";

let systemDark = false;
const mediaListeners = new Set<(event: { matches: boolean }) => void>();

beforeEach(() => {
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
