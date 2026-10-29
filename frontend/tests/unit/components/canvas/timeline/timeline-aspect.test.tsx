import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import {
  isTimelineAspectRatio,
  timelineAspectRatios,
  timelinePreviewAspectRatio,
  timelinePreviewDimensions,
} from "@/components/canvas/timeline/timeline-aspect-ratio";
import { TimelineAspectSelect } from "@/components/canvas/timeline/timeline-aspect-select";

afterEach(cleanup);

it("uses every exact source ratio for preview and does not guess auto", () => {
  expect(timelineAspectRatios).toEqual([
    "auto",
    "2.35:1",
    "21:9",
    "1.85:1",
    "16:9",
    "3:2",
    "4:3",
    "1:1",
    "3:4",
    "2:3",
    "9:16",
  ]);
  for (const ratio of timelineAspectRatios.filter(
    (value) => value !== "auto",
  )) {
    const result = timelinePreviewDimensions(ratio);
    expect(result).not.toBeNull();
    const [x, y] = ratio.split(":").map(Number);
    expect(result!.width / result!.height).toBeCloseTo(x / y, 10);
    expect(result!.width % 2).toBe(0);
    expect(result!.height % 2).toBe(0);
    expect(timelinePreviewAspectRatio(ratio)).toBe(
      `${result!.width} / ${result!.height}`,
    );
  }
  expect(timelinePreviewDimensions("auto")).toBeNull();
  expect(timelinePreviewAspectRatio("auto")).toBeUndefined();
  expect(timelinePreviewAspectRatio("auto", { width: 640, height: 480 })).toBe(
    "640 / 480",
  );
  expect(
    timelinePreviewDimensions("auto", { width: Infinity, height: 480 }),
  ).toBeNull();
  for (const value of ["", "adaptive", "2:1", "AUTO", "16/9", null, {}, 1]) {
    expect(isTimelineAspectRatio(value)).toBe(false);
    expect(timelinePreviewDimensions(value)).toBeNull();
  }
});

it("shows all choices and only emits a valid user selection", () => {
  const onChange = vi.fn();
  render(<TimelineAspectSelect value="auto" onChange={onChange} />);
  const trigger = screen.getByRole("button", { name: "画面比例" });
  expect(trigger.textContent).toContain("自动");
  fireEvent.keyDown(trigger, { key: "Enter" });
  expect(screen.getAllByRole("menuitemradio")).toHaveLength(11);
  fireEvent.click(screen.getByRole("menuitemradio", { name: "2.35:1" }));
  expect(onChange).toHaveBeenCalledWith("2.35:1");
});

it("retains unknown values as unsupported and cannot write in read-only mode", () => {
  const onChange = vi.fn();
  const view = render(
    <TimelineAspectSelect value="future" onChange={onChange} readOnly />,
  );
  const trigger = screen.getByRole("button", { name: "画面比例" });
  expect(trigger.textContent).toContain("不支持的比例");
  expect(trigger.getAttribute("aria-invalid")).toBe("true");
  expect(trigger.hasAttribute("disabled")).toBe(true);
  expect(onChange).not.toHaveBeenCalled();
  view.rerender(<TimelineAspectSelect value="3:4" onChange={onChange} />);
  expect(trigger.textContent).toContain("3:4");
  expect(onChange).not.toHaveBeenCalled();
});
