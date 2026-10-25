import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { createElement, type ReactNode } from "react";
import { SWRConfig } from "swr";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { getMyPreferences, updateMyPreferences } from "@/api/identity";
import {
  useCanvasBackground,
  backgroundIdentity,
} from "@/components/canvas/hooks/use-canvas-background";
vi.mock("@/api/identity", () => ({
  getMyPreferences: vi.fn(),
  updateMyPreferences: vi.fn(),
}));
const initial: API.PreferencesView = {
  canvas_background: "dots",
  theme: "dark",
  prompt_suffix: "",
  revision: "1",
};
function wrapper({ children }: { children: ReactNode }) {
  return createElement(
    SWRConfig,
    { value: { provider: () => new Map(), dedupingInterval: 0 } },
    children,
  );
}
beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(getMyPreferences).mockResolvedValue(initial);
});
afterEach(cleanup);
it("reads server preferences and saves only the background with CAS", async () => {
  const { result } = renderHook(() => useCanvasBackground("a"), { wrapper });
  await waitFor(() => expect(result.current[2].data).toEqual(initial));
  const saved = {
    ...initial,
    canvas_background: "lines" as const,
    revision: "2",
  };
  vi.mocked(updateMyPreferences).mockResolvedValue(saved);
  vi.mocked(getMyPreferences).mockResolvedValue(saved);
  await act(() => result.current[1]("lines"));
  expect(updateMyPreferences).toHaveBeenCalledWith({
    expected_revision: "1",
    canvas_background: "lines",
  });
  expect(result.current[0]).toBe("lines");
});
it("retains the last confirmed value after a failed write", async () => {
  const { result } = renderHook(() => useCanvasBackground("a"), { wrapper });
  await waitFor(() => expect(result.current[2].data).toBeDefined());
  vi.mocked(updateMyPreferences).mockRejectedValue(new Error("conflict"));
  await act(async () => {
    await expect(result.current[1]("blank")).rejects.toThrow("conflict");
  });
  expect(result.current[0]).toBe("dots");
});
it("revalidates a changed server value and isolates identities", async () => {
  const { result, rerender } = renderHook(({ id }) => useCanvasBackground(id), {
    wrapper,
    initialProps: { id: "a" },
  });
  await waitFor(() => expect(result.current[2].data).toBeDefined());
  vi.mocked(getMyPreferences).mockResolvedValue({
    ...initial,
    canvas_background: "blank",
  });
  await act(async () => {
    await result.current[2].mutate();
  });
  expect(result.current[0]).toBe("blank");
  vi.mocked(getMyPreferences).mockResolvedValue(initial);
  rerender({ id: "b" });
  await waitFor(() => expect(result.current[0]).toBe("dots"));
});
it("normalizes canvas account identities", () => {
  expect(
    backgroundIdentity("glowloom:canvas-view:v1:a:team:project:canvas"),
  ).toBe("a");
  expect(
    backgroundIdentity("glowloom:canvas-view:v1:team:USER%3Aa:canvas"),
  ).toBe("a");
  expect(
    backgroundIdentity("glowloom:canvas-view:v1:LOCAL:LOCAL:canvas"),
  ).toBeUndefined();
});
