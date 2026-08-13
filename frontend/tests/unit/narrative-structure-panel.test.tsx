import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import { NarrativeStructurePanel } from "@/app/studio/[episodeId]/narrative-structure-panel";

const body = "内景·门厅·夜\n👩‍🚀林澈：停下。";
const source = Array.from(body);
const dialogueStart = source.indexOf("👩");
const now = "2026-08-13T06:00:00Z";

const structure: API.NarrativeStructureResponse = {
  id: "019ff900-0000-7000-8000-000000000001",
  workspace_id: "019ff900-0000-7000-8000-000000000002",
  episode_id: "019ff900-0000-7000-8000-000000000003",
  script_version_id: "019ff900-0000-7000-8000-000000000004",
  input_hash: "a".repeat(64),
  parser_version: "deterministic-lines-v1",
  structure_hash: "b".repeat(64),
  dependency_hash: "c".repeat(64),
  revision: 1,
  units: [
    {
      id: "019ff900-0000-7000-8000-000000000005",
      unit_id: "019ff900-0000-7000-8000-000000000006",
      kind: "scene_heading",
      position: 1,
      version_no: 1,
      source_range: { start: 0, end: 7 },
      exact_text: "内景·门厅·夜",
      text_hash: "d".repeat(64),
      prefix_text: "",
      suffix_text: body.slice(7),
      required_for_coverage: true,
      source_scene_id: null,
      source_dialogue_id: null,
      origin: "deterministic",
      created_at: now,
    },
    {
      id: "019ff900-0000-7000-8000-000000000007",
      unit_id: "019ff900-0000-7000-8000-000000000008",
      kind: "dialogue",
      position: 2,
      version_no: 1,
      source_range: { start: dialogueStart, end: source.length },
      exact_text: "👩‍🚀林澈：停下。",
      text_hash: "e".repeat(64),
      prefix_text: body.slice(0, dialogueStart),
      suffix_text: "",
      required_for_coverage: true,
      source_scene_id: null,
      source_dialogue_id: null,
      origin: "deterministic",
      created_at: now,
    },
  ],
  created_at: now,
  updated_at: now,
};

describe("稳定叙事单元面板", () => {
  it("按 code point 定位原文并提交完整稳定 ID 集合", async () => {
    const user = userEvent.setup();
    const onRevise = vi.fn(
      async (_request: API.NarrativeStructureRevisionRequest) => undefined,
    );
    render(
      <NarrativeStructurePanel
        busy={false}
        scriptBody={body}
        structure={structure}
        onRevise={onRevise}
      />,
    );

    const dialogueText = screen.getAllByText("👩‍🚀林澈：停下。")[0];
    await user.click(dialogueText.closest("button")!);
    expect(document.querySelector("mark")).toHaveTextContent("👩‍🚀林澈：停下。");
    expect(screen.getAllByLabelText("类型")[0]).toHaveAttribute("readonly");

    const required = screen.getAllByLabelText("必须被分镜覆盖");
    await user.click(required[1]);
    await user.click(screen.getByRole("button", { name: "保存结构修正" }));

    expect(onRevise).toHaveBeenCalledTimes(1);
    const [request] = onRevise.mock.calls[0]!;
    expect(request.expected_revision).toBe(1);
    expect(request.expected_current_script_version_id).toBe(
      structure.script_version_id,
    );
    expect(request.units).toEqual([
      {
        unit_id: structure.units[0].unit_id,
        kind: "scene_heading",
        source_start: 0,
        source_end: 7,
        required_for_coverage: true,
      },
      {
        unit_id: structure.units[1].unit_id,
        kind: "dialogue",
        source_start: dialogueStart,
        source_end: source.length,
        required_for_coverage: false,
      },
    ]);
  });

  it("阻止越界或重叠范围写入", async () => {
    const user = userEvent.setup();
    const onRevise = vi.fn(
      async (_request: API.NarrativeStructureRevisionRequest) => undefined,
    );
    render(
      <NarrativeStructurePanel
        busy={false}
        scriptBody={body}
        structure={structure}
        onRevise={onRevise}
      />,
    );

    const secondUnit = screen
      .getAllByText("👩‍🚀林澈：停下。")[0]
      .closest("article")!;
    const start = within(secondUnit).getByLabelText("起点");
    await user.clear(start);
    await user.type(start, "3");

    expect(screen.getByText("字符范围无效")).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "保存结构修正" }),
    ).toBeDisabled();
    expect(onRevise).not.toHaveBeenCalled();
  });
});
