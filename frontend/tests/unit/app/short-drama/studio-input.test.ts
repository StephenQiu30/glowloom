import { beforeEach, expect, it, vi } from "vitest";

const context = vi.hoisted(() => ({ changed: () => {} }));
vi.mock("@/lib/request", () => ({
  subscribeContextChange: (changed: () => void) => {
    context.changed = changed;
    return () => {};
  },
}));
import {
  clearStudioInput,
  readStudioInput,
  saveStudioInput,
  type StudioInput,
} from "@/app/short-drama/studio-input";

beforeEach(() => context.changed());

it("短剧输入只交给同一账号空间及目标项目，不向其他身份或项目泄漏文件", () => {
  const input: StudioInput = {
    kind: "upload",
    text: "第一集",
    file: new File(["剧本"], "剧本.txt"),
  };
  saveStudioInput("actor-a/space-a", "project-a", input);
  expect(readStudioInput("actor-a/space-a", "project-a")).toBe(input);
  for (const [owner, project] of [
    ["actor-b/space-a", "project-a"],
    ["actor-a/space-b", "project-a"],
    ["actor-a/space-a", "project-b"],
  ]) {
    expect(readStudioInput(owner, project)).toBeUndefined();
  }
});

it("旧项目迟到清理不会移除新的短剧输入", () => {
  const previous: StudioInput = { kind: "create", prompt: "旧想法" };
  const next: StudioInput = { kind: "create", prompt: "新想法" };
  saveStudioInput("owner", "previous", previous);
  saveStudioInput("owner", "next", next);
  clearStudioInput(previous);
  expect(readStudioInput("owner", "next")).toBe(next);
  clearStudioInput(next);
  expect(readStudioInput("owner", "next")).toBeUndefined();
});

it("认证或空间上下文改变立即释放尚未消费的短剧输入", () => {
  saveStudioInput("owner", "project", {
    kind: "redraw",
    file: new File(["video"], "source.mp4"),
    style: "水墨",
    market: "原片语言",
  });
  context.changed();
  expect(readStudioInput("owner", "project")).toBeUndefined();
});
