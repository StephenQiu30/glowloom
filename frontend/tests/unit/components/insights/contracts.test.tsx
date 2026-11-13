import { act, renderHook, waitFor } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import {
  numberValue,
  rateValue,
  filterQuery,
} from "@/components/insights/contracts";
import { useInsightRead } from "@/components/insights/use-insight-read";
describe("insights source and read boundaries", () => {
  it("distinguishes unknown denominators from true zero", () => {
    expect(rateValue(null)).toBe("不适用");
    expect(rateValue(0)).toBe("0%");
    expect(numberValue(0)).toBe("0");
    expect(numberValue(null)).toBe("未接入");
  });
  it("interprets explicit date inputs in Shanghai and bounds the window", () => {
    expect(
      filterQuery(
        { from: "2026-11-01T00:00", to: "2026-11-02T00:00", origin: "" },
        7,
      ),
    ).toEqual({
      from: "2026-10-31T16:00:00.000Z",
      to: "2026-11-01T16:00:00.000Z",
    });
    expect(() =>
      filterQuery({ from: "2026-11-01T00:00", to: "2026-11-10T00:00" }, 7),
    ).toThrow();
  });
  it("retains real content during refresh and clears on scope switch, ignoring late reads", async () => {
    let resolveOld!: (v: string) => void;
    const loader = vi
      .fn()
      .mockResolvedValueOnce("mine")
      .mockImplementationOnce(
        () =>
          new Promise<string>((r) => {
            resolveOld = r;
          }),
      )
      .mockResolvedValue("platform");
    const { result, rerender } = renderHook(
      ({ scope }) => useInsightRead(scope, loader),
      { initialProps: { scope: "personal" } },
    );
    await waitFor(() => expect(result.current.data).toBe("mine"));
    act(() => result.current.refresh());
    await waitFor(() => expect(result.current.pending).toBe(true));
    expect(result.current.data).toBe("mine");
    rerender({ scope: "platform" });
    expect(result.current.data).toBeNull();
    await waitFor(() => expect(result.current.data).toBe("platform"));
    await act(async () => resolveOld("late private"));
    expect(result.current.data).toBe("platform");
  });
  it("removes private content on forbidden refresh", async () => {
    const loader = vi
      .fn()
      .mockResolvedValueOnce("private")
      .mockRejectedValueOnce({ status: 403 });
    const { result } = renderHook(() => useInsightRead("personal", loader));
    await waitFor(() => expect(result.current.data).toBe("private"));
    act(() => result.current.refresh());
    await waitFor(() => expect(result.current.error).toBe("forbidden"));
    expect(result.current.data).toBeNull();
  });
});
