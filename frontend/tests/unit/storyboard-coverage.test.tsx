import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import { StoryboardCoverage } from "@/app/studio/[episodeId]/storyboard-coverage";

const workspaceId = "019fb7d0-a000-7000-8000-000000000001";
const episodeId = "019fb7d0-a000-7000-8000-000000000002";
const firstShotId = "019fb7d0-a000-7000-8000-000000000003";
const secondShotId = "019fb7d0-a000-7000-8000-000000000004";
const firstSpecId = "019fb7d0-a000-7000-8000-000000000005";
const secondSpecId = "019fb7d0-a000-7000-8000-000000000006";
const firstUnitId = "019fb7d0-a000-7000-8000-000000000007";
const secondUnitId = "019fb7d0-a000-7000-8000-000000000008";
const now = "2026-08-13T12:00:00Z";

const shots: API.ShotResponse[] = [
  {
    id: firstShotId,
    workspace_id: workspaceId,
    episode_id: episodeId,
    position: 1,
    title: "雨幕中的脚步",
    source_script_version_id: workspaceId,
    source_scene_id: workspaceId,
    source_candidate_id: null,
    source_draft_shot_id: null,
    status: "active",
    current_spec_version_id: firstSpecId,
    revision: 2,
    created_at: now,
    updated_at: now,
  },
  {
    id: secondShotId,
    workspace_id: workspaceId,
    episode_id: episodeId,
    position: 2,
    title: "空站台转场",
    source_script_version_id: workspaceId,
    source_scene_id: workspaceId,
    source_candidate_id: null,
    source_draft_shot_id: null,
    status: "active",
    current_spec_version_id: secondSpecId,
    revision: 1,
    created_at: now,
    updated_at: now,
  },
];

const report: API.CoverageReportResponse = {
  episode_id: episodeId,
  status: "blocked",
  ready: false,
  basis_hash: "a".repeat(64),
  evaluation_hash: "b".repeat(64),
  summary: {
    required_total: 2,
    covered: 1,
    approved_omitted: 0,
    uncovered: 1,
    shots_total: 2,
    linked: 1,
    approved_invented: 0,
    orphan: 1,
    stale: 0,
  },
  units: [
    {
      narrative_unit_id: firstUnitId,
      unit_version_id: firstUnitId,
      position: 1,
      kind: "action",
      exact_text: "雨幕中的脚步逼近站台",
      required_for_coverage: true,
      required_channel: "visual",
      status: "covered",
      shot_ids: [firstShotId],
    },
    {
      narrative_unit_id: secondUnitId,
      unit_version_id: secondUnitId,
      position: 2,
      kind: "dialogue",
      exact_text: "你终于来了。",
      required_for_coverage: true,
      required_channel: "audio",
      status: "uncovered",
      shot_ids: [],
    },
  ],
  shots: [
    {
      shot_id: firstShotId,
      spec_version_id: firstSpecId,
      position: 1,
      title: "雨幕中的脚步",
      status: "linked",
      unit_version_ids: [firstUnitId],
    },
    {
      shot_id: secondShotId,
      spec_version_id: secondSpecId,
      position: 2,
      title: "空站台转场",
      status: "orphan",
      unit_version_ids: [],
    },
  ],
  references: [
    {
      id: "019fb7d0-a000-7000-8000-000000000009",
      shot_id: firstShotId,
      shot_spec_version_id: firstSpecId,
      narrative_unit_id: firstUnitId,
      unit_version_id: firstUnitId,
      channel: "visual",
      role: "primary",
      coverage_mode: "full",
      segment_start: null,
      segment_end: null,
      contribution: "required",
      origin: "human",
      created_by: workspaceId,
      created_at: now,
    },
  ],
  stale_reference_ids: [],
  stale_decision_ids: [],
  next_actions: ["map_or_omit_narrative_units", "map_or_approve_invented_shots"],
};

describe("分镜覆盖工作台", () => {
  it("展示缺口并支持从叙事单元定位到镜头", async () => {
    const user = userEvent.setup();
    const onSelectShot = vi.fn();
    render(
      <StoryboardCoverage
        busy={false}
        report={report}
        selectedShotId={firstShotId}
        shots={shots}
        onDecide={vi.fn()}
        onReplace={vi.fn()}
        onSelectShot={onSelectShot}
      />,
    );

    expect(screen.getByText("1 个未覆盖")).toBeInTheDocument();
    expect(screen.getByText("1 个无来源镜头")).toBeInTheDocument();
    await user.click(
      screen.getByRole("button", { name: /雨幕中的脚步逼近站台/ }),
    );
    expect(onSelectShot).toHaveBeenCalledWith(firstShotId);
  });

  it("为当前镜头显式保存完整叙事来源集合", async () => {
    const user = userEvent.setup();
    const onReplace = vi.fn().mockResolvedValue(true);
    render(
      <StoryboardCoverage
        busy={false}
        report={report}
        selectedShotId={firstShotId}
        shots={shots}
        onDecide={vi.fn()}
        onReplace={onReplace}
        onSelectShot={vi.fn()}
      />,
    );

    await user.click(screen.getByRole("button", { name: "编辑来源映射" }));
    await user.click(
      screen.getByLabelText("映射叙事单元 台词：你终于来了。"),
    );
    await user.click(screen.getByRole("button", { name: "保存来源映射" }));

    await waitFor(() => expect(onReplace).toHaveBeenCalledOnce());
    expect(onReplace).toHaveBeenCalledWith(
      shots[0],
      expect.arrayContaining([
        expect.objectContaining({
          unit_version_id: firstUnitId,
          channel: "visual",
        }),
        expect.objectContaining({
          unit_version_id: secondUnitId,
          channel: "audio",
        }),
      ]),
    );
  });

  it("保存映射时保留同一叙事单元的多条既有关系", async () => {
    const user = userEvent.setup();
    const onReplace = vi.fn().mockResolvedValue(true);
    const multiReferenceReport: API.CoverageReportResponse = {
      ...report,
      references: [
        ...report.references,
        {
          ...report.references[0],
          id: "019fb7d0-a000-7000-8000-000000000010",
          channel: "audio",
          role: "reaction",
          contribution: "supporting",
        },
      ],
    };
    render(
      <StoryboardCoverage
        busy={false}
        report={multiReferenceReport}
        selectedShotId={firstShotId}
        shots={shots}
        onDecide={vi.fn()}
        onReplace={onReplace}
        onSelectShot={vi.fn()}
      />,
    );

    await user.click(screen.getByRole("button", { name: "编辑来源映射" }));
    await user.click(screen.getByRole("button", { name: "保存来源映射" }));

    await waitFor(() => expect(onReplace).toHaveBeenCalledOnce());
    expect(onReplace.mock.calls[0]?.[1]).toEqual([
      expect.objectContaining({ channel: "visual", role: "primary" }),
      expect.objectContaining({ channel: "audio", role: "reaction" }),
    ]);
  });

  it("遗漏和原创镜头都要求填写理由后提交版本化决议", async () => {
    const user = userEvent.setup();
    const onDecide = vi.fn().mockResolvedValue(true);
    render(
      <StoryboardCoverage
        busy={false}
        report={report}
        selectedShotId={secondShotId}
        shots={shots}
        onDecide={onDecide}
        onReplace={vi.fn()}
        onSelectShot={vi.fn()}
      />,
    );

    await user.click(
      screen.getByRole("button", { name: "批准省略 你终于来了。" }),
    );
    await user.type(screen.getByLabelText("覆盖决议原因"), "由上一镜动作完整表达");
    await user.click(screen.getByRole("button", { name: "确认批准省略" }));
    await waitFor(() => expect(onDecide).toHaveBeenCalledOnce());
    expect(onDecide).toHaveBeenLastCalledWith(
      expect.objectContaining({
        action: "approve_omission",
        unit_version_id: secondUnitId,
        shot_spec_version_id: null,
        expected_evaluation_hash: report.evaluation_hash,
      }),
    );

    await user.click(
      screen.getByRole("button", { name: "批准原创 空站台转场" }),
    );
    await user.type(screen.getByLabelText("覆盖决议原因"), "建立下一场空间方向");
    await user.click(screen.getByRole("button", { name: "确认批准原创" }));
    await waitFor(() => expect(onDecide).toHaveBeenCalledTimes(2));
    expect(onDecide).toHaveBeenLastCalledWith(
      expect.objectContaining({
        action: "approve_invented",
        unit_version_id: null,
        shot_spec_version_id: secondSpecId,
        expected_evaluation_hash: report.evaluation_hash,
      }),
    );
  });
});
