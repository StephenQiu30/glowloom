import { expect, it } from "vitest";
import { nativeTextEdit } from "@/components/generation/text-input-intent";
it("uses the real selection when duplicate characters are indistinguishable", () => {
  expect(
    nativeTextEdit(
      { value: "aaa", from: 1, to: 2, inputType: "deleteContentBackward" },
      "aa",
    ),
  ).toEqual({ from: 1, to: 2, text: "" });
  expect(
    nativeTextEdit(
      { value: "aaa", from: 2, to: 2, inputType: "deleteContentBackward" },
      "aa",
    ),
  ).toEqual({ from: 1, to: 2, text: "" });
});
it("keeps emoji boundaries and native replacement ranges", () => {
  expect(
    nativeTextEdit(
      { value: "A😀B", from: 3, to: 3, inputType: "deleteContentBackward" },
      "AB",
    ),
  ).toEqual({ from: 1, to: 3, text: "" });
  expect(
    nativeTextEdit(
      { value: "aa", from: 1, to: 1, inputType: "insertFromPaste" },
      "a中😀a",
    ),
  ).toEqual({ from: 1, to: 1, text: "中😀" });
  expect(() =>
    nativeTextEdit(
      { value: "aaa", from: 1, to: 1, inputType: "insertText" },
      "baa",
    ),
  ).toThrow();
});
