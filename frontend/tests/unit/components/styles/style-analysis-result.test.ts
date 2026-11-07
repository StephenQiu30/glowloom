import { expect, it } from "vitest";
import { parseStyleAnalysisReply } from "@/components/styles/style-analysis-result";
it("parses only explicitly labeled name and description", () => {
  expect(
    parseStyleAnalysisReply(
      "风格名称： 电影感\n\n风格描述：低饱和色彩。\n柔和侧光。 ",
    ),
  ).toEqual({ name: "电影感", description: "低饱和色彩。\n柔和侧光。" });
});
it("does not turn a description sentence or markdown preamble into a style name", () => {
  expect(() =>
    parseStyleAnalysisReply("低饱和电影感，柔和侧光。\n这是建议名称。"),
  ).toThrow();
  expect(() =>
    parseStyleAnalysisReply(
      "以下是分析：\n风格名称：电影感\n\n风格描述：侧光。",
    ),
  ).toThrow();
});
it("accepts twenty Unicode name code points and rejects twenty-one without truncation", () => {
  expect(
    parseStyleAnalysisReply(`风格名称：${"😀".repeat(20)}\n\n风格描述：侧光。`)
      .name,
  ).toBe("😀".repeat(20));
  expect(() =>
    parseStyleAnalysisReply(`风格名称：${"😀".repeat(21)}\n\n风格描述：侧光。`),
  ).toThrow();
});
it("rejects overlong or empty descriptions rather than silently truncating", () => {
  expect(() =>
    parseStyleAnalysisReply(
      `风格名称：电影感\n\n风格描述：${"光".repeat(2001)}`,
    ),
  ).toThrow();
  expect(() =>
    parseStyleAnalysisReply("风格名称：电影感\n\n风格描述： "),
  ).toThrow();
});
