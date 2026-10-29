import { fireEvent, render, screen } from "@testing-library/react";
import { expect, it } from "vitest";
import {
  AudioSubtitles,
  subtitleSRT,
} from "@/components/generation/audio-subtitles";

const subtitle = {
  text: "早上好",
  sentences: [
    {
      start_time: 1200,
      end_time: 2450,
      text: "早上好",
      words: [{ start_time: 1200, end_time: 1500, text: "早" }],
    },
  ],
};
it("导出供应商毫秒时间，不按整数秒或台词长度推算", () => {
  expect(subtitleSRT(subtitle)).toBe(
    "1\n00:00:01,200 --> 00:00:02,450\n早上好",
  );
});
it("展开已归档字幕可查看字级时间", () => {
  render(<AudioSubtitles subtitle={subtitle} />);
  fireEvent.click(screen.getByRole("button", { name: "查看字幕" }));
  expect(screen.getByText("早上好")).toBeTruthy();
  expect(screen.getByLabelText("字级时间戳").textContent).toContain(
    "00:00:01,200–00:00:01,500",
  );
});
