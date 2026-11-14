import { readFile } from "node:fs/promises";
import { resolve } from "node:path";
import { cache } from "react";
import type { Snapshot } from "./catalog";

export const getSnapshot = cache(async (): Promise<Snapshot> => {
  return JSON.parse(
    await readFile(resolve(process.cwd(), "content-snapshot.json"), "utf8"),
  );
});
export const categoryNames: Record<string, string> = {
  requirement: "原始需求",
  design: "设计方案",
  prd: "产品需求",
  plan: "执行计划",
  research: "调研资料",
  templates: "文档模板",
  skills: "Skill",
  workflows: "工作流",
  "00-文档索引": "总览",
  superpowers: "专项计划",
  工程规范: "工程规范",
};
