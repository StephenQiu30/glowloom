import { expect, it } from "vitest";
import {
  parseWorkflow,
  buildContext,
  contextMatches,
} from "../../src/workflows/definition";

const source =
  "# 功能开发\n\n```workspace-workflow\n" +
  JSON.stringify({
    id: "feature",
    stages: [
      {
        id: "read",
        label: "理解",
        needs: [],
        outputs: ["scope"],
        optional: false,
      },
      {
        id: "accept",
        label: "接受",
        needs: ["read"],
        outputs: ["decision"],
        optional: false,
      },
    ],
  }) +
  "\n```";
it("rejects a workflow whose dependencies form a cycle", () => {
  expect(() =>
    parseWorkflow(source.replace('"needs":[]', '"needs":["accept"]')),
  ).toThrow(/cycle|循环/);
  expect(parseWorkflow(source).stages.map((s) => s.id)).toEqual([
    "read",
    "accept",
  ]);
});
it("keeps authorization and acceptance intact while bounding knowledge excerpts", () => {
  const context = buildContext({
    goal: "修复搜索",
    project: "workspace",
    repoCommit: "abc",
    codeDigest: "code1",
    documents: [
      { id: "design-43", revision: "rev1", source: "知识".repeat(5000) },
    ],
    skillRevisions: { locator: "skill1" },
    workflowRevision: "flow1",
    allowedActions: ["read"],
    expiresAt: 2000,
    acceptance: ["中文检索正确"],
    budget: 300,
  });
  expect(context.documents[0].excerpt.length).toBeLessThanOrEqual(300);
  expect(context.acceptance).toEqual(["中文检索正确"]);
  expect(context.allowedActions).toEqual(["read"]);
  expect(
    contextMatches(
      context,
      {
        codeDigest: "code1",
        revisions: { "design-43": "rev2" },
        skillRevisions: { locator: "skill1" },
        workflowRevision: "flow1",
      },
      1000,
    ),
  ).toBe(false);
});
it("invalidates an expired authorization and a changed skill before execution", () => {
  const c = buildContext({
    goal: "定位",
    project: "workspace",
    repoCommit: "a",
    codeDigest: "c",
    documents: [],
    skillRevisions: { a: "1" },
    workflowRevision: "w",
    allowedActions: ["read"],
    expiresAt: 100,
    acceptance: ["找到入口"],
  });
  expect(
    contextMatches(
      c,
      {
        codeDigest: "c",
        revisions: {},
        skillRevisions: { a: "1" },
        workflowRevision: "w",
      },
      101,
    ),
  ).toBe(false);
  expect(
    contextMatches(
      c,
      {
        codeDigest: "c",
        revisions: {},
        skillRevisions: { a: "2" },
        workflowRevision: "w",
      },
      1,
    ),
  ).toBe(false);
});
