import { fireEvent, render, screen } from "@testing-library/react";
import { expect, it, vi } from "vitest";
import { SkillWorkflowControl } from "@/components/canvas/editor/skill-workflow-control";
import type { SkillWorkflow } from "@/components/canvas/editor/skill-workflow-graph";

vi.mock("next/dynamic", () => ({
  default: () =>
    function WorkflowConversation({ workflow }: { workflow: SkillWorkflow }) {
      return <output data-testid="workflow">{JSON.stringify(workflow)}</output>;
    },
}));

it("imports a starter into the draft and opens the saved graph for execution", async () => {
  let draft: CanvasContract.CanvasDocumentGraphDTO = {
    schemaVersion: 2,
    nodes: [],
    connections: [],
  };
  const canvasId = "9f5cd35b-5041-4413-8755-b3f343d2c0b6";
  const projectId = "f083a17b-c878-4cbd-b0fc-d16a10f42dc3";
  const graphHash = "a".repeat(64);
  const prepare = vi.fn(async () => ({
    canvasId,
    projectId,
    title: "模板验收",
    revision: "5",
    projectBindingRevision: "1",
    commitSeq: "2",
    schemaVersion: 2,
    epoch: "d581af59-afaf-4391-aa3b-4ca0dae7e999",
    graph: { ...draft, connections: [] },
    graphHash,
    access: { read: true, edit: true, manage: true },
    createdAt: "",
    updatedAt: "",
  }));
  const view = () => (
    <SkillWorkflowControl
      graph={draft}
      canvasId={canvasId}
      projectId={projectId}
      identity="owner"
      enabled
      onEdit={(graph) => {
        draft = graph;
      }}
      prepare={prepare}
    />
  );
  const { rerender } = render(view());
  fireEvent.click(screen.getByRole("button", { name: "技能流程" }));
  fireEvent.click(screen.getByRole("button", { name: "小说改编与分镜" }));
  expect(draft.nodes).toHaveLength(8);
  expect(draft.connections.length).toBeGreaterThan(0);
  rerender(view());
  fireEvent.click(screen.getByRole("button", { name: "技能流程" }));
  fireEvent.click(screen.getByRole("button", { name: "运行" }));
  const run = JSON.parse(
    (await screen.findByTestId("workflow")).textContent ?? "",
  ) as SkillWorkflow;
  expect(prepare).toHaveBeenCalledOnce();
  expect(run.nodes).toHaveLength(7);
  expect(run.nodes.every((node) => node.after.length === 0)).toBe(true);
  expect(run.source).toEqual({ canvasId, revision: "5", graphHash });
});
