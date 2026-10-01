import { describe, expect, it } from "vitest";
import {
  parseSrt,
  resegmentSrtEntries,
  serializeSrtEntries,
} from "./subtitles";
describe("字幕迁移", () => {
  it("读取和导出多行中文字幕保留精确毫秒", () => {
    const content = "1\n00:00:01,123 --> 00:00:02,999\n你好\n第二行\n";
    expect(serializeSrtEntries(parseSrt(content))).toBe(content);
  });
  it("拒绝无效时间和错误块，不能静默丢字幕", () => {
    expect(() =>
      parseSrt("1\n00:70:00,000 --> 00:71:00,000\n坏时间"),
    ).toThrow();
    expect(() => parseSrt("1\n00:00:02,000 --> 00:00:01,000\n倒置")).toThrow();
    expect(() => parseSrt("文本不是SRT")).toThrow();
  });
  it("标点优先切分并保留原始结束时间", () => {
    const split = resegmentSrtEntries(
      [
        {
          index: 1,
          startMs: 0,
          endMs: 6000,
          text: "今天我们去海边，看看落日，然后拍一部属于自己的短片。",
        },
      ],
      20,
    );
    expect(split.map((item) => item.text).join("")).toBe(
      "今天我们去海边，看看落日，然后拍一部属于自己的短片。",
    );
    expect(split.at(-1)!.endMs).toBe(6000);
    expect(split.every((item) => item.endMs > item.startMs)).toBe(true);
  });
});
