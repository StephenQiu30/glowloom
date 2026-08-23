import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import { TaskWorkspace } from "@/app/studio/[episodeId]/task-workspace";

const workspaceId = "019fb3c0-a000-7000-8000-000000000001";
const scheduleId = "019fb3c0-a000-7000-8000-000000000002";
const uploadId = "019fb3c0-a000-7000-8000-000000000003";

const productionProps: {
  capabilities: API.ModelCapabilityResponse[];
  costs: API.CostQueryResponse | null;
  productionFactsLoading: boolean;
  productionFactsUnavailable: boolean;
} = {
  capabilities: [],
  costs: null,
  productionFactsLoading: false,
  productionFactsUnavailable: false,
};

const task: API.TaskResponse = {
  id: "019fb3c0-a000-7000-8000-000000000004",
  workspace_id: workspaceId,
  task_type: "upload_expiration",
  request_type: "upload_session",
  request_id: uploadId,
  scope: {
    episode_id: null,
    render_snapshot_id: null,
    usage_type: "upload_session",
    usage_id: uploadId,
    input_version_id: null,
    input_hash: null,
  },
  status: "queued",
  progress_stage: "queued",
  error: null,
  next_action: "wait_for_cleanup",
  cancel_status: "none",
  revision: 1,
};

function schedule(
  status: API.ScheduleResponse["status"],
): API.ScheduleResponse {
  return {
    id: scheduleId,
    workspace_id: workspaceId,
    schedule_key: `media.upload.expire:${uploadId}`,
    handler_name: "expire_upload_session",
    scope: { usage_type: "upload_session", usage_id: uploadId },
    kind: "one_off",
    rule: {
      kind: "one_off",
      at: "2026-08-04T12:00:00Z",
      misfire_grace_seconds: 30,
    },
    timezone: "UTC",
    status,
    next_fire_at: status === "completed" ? null : "2026-08-04T12:00:00Z",
    next_attempt_at: null,
    misfire_policy: "run_once",
    max_catch_up: 0,
    failure_count: 0,
    last_error: null,
    revision: 2,
  };
}

describe("TaskWorkspace", () => {
  it("confirms cancellation only for a queued generation task", async () => {
    const user = userEvent.setup();
    const onCancelGenerationTask = vi.fn().mockResolvedValue(true);
    const generationTask: API.TaskResponse = {
      ...task,
      task_type: "image_generation",
      request_type: "generation_request",
      request_id: "019fb3c0-a000-7000-8000-000000000020",
      scope: {
        ...task.scope,
        episode_id: "019fb3c0-a000-7000-8000-000000000021",
        usage_type: "shot",
        usage_id: "019fb3c0-a000-7000-8000-000000000022",
      },
      next_action: "poll_task",
    };
    const view = render(
      <TaskWorkspace
        {...productionProps}
        busy={false}
        schedules={[]}
        tasks={[generationTask]}
        onCancelGenerationTask={onCancelGenerationTask}
        onConfigureSchedule={vi.fn()}
        onPauseSchedule={vi.fn()}
        onResumeSchedule={vi.fn()}
        onTriggerSchedule={vi.fn()}
      />,
    );

    await user.click(screen.getByRole("button", { name: "取消任务" }));
    expect(
      screen.getByRole("heading", { name: "取消排队中的生成任务" }),
    ).toBeInTheDocument();
    expect(screen.getByText(/释放尚未使用的全部预占费用/)).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "确认取消" }));
    expect(onCancelGenerationTask).toHaveBeenCalledWith(generationTask);
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();

    view.rerender(
      <TaskWorkspace
        {...productionProps}
        busy={false}
        schedules={[]}
        tasks={[
          {
            ...generationTask,
            status: "running",
            progress_stage: "dispatching",
            revision: 2,
          },
        ]}
        onCancelGenerationTask={onCancelGenerationTask}
        onConfigureSchedule={vi.fn()}
        onPauseSchedule={vi.fn()}
        onResumeSchedule={vi.fn()}
        onTriggerSchedule={vi.fn()}
      />,
    );
    expect(
      screen.queryByRole("button", { name: "取消任务" }),
    ).not.toBeInTheDocument();
  });

  it("renders unavailable Ark capabilities and server-owned reserve totals", () => {
    render(
      <TaskWorkspace
        {...productionProps}
        busy={false}
        capabilities={[
          {
            id: "019fb3c0-a000-7000-8000-000000000010",
            provider: "volcengine_ark",
            model: "doubao-seedream-5-0-lite-260128",
            kind: "image",
            config_version: 1,
            input_types: ["shot_spec"],
            parameter_schema: {},
            limits: {},
            pricing: null,
            status: "unavailable",
            unavailable_reason: "provider_contract_unverified",
          },
        ]}
        costs={{
          currency: "CNY",
          summary: {
            reserved: "12.500000",
            settled: "0.000000",
            released: "0.000000",
            adjustments: "0.000000",
            remaining_reserved: "12.500000",
          },
          items: [],
          total: 1,
          limit: 100,
          offset: 0,
        }}
        schedules={[]}
        tasks={[]}
        onCancelGenerationTask={vi.fn()}
        onConfigureSchedule={vi.fn()}
        onPauseSchedule={vi.fn()}
        onResumeSchedule={vi.fn()}
        onTriggerSchedule={vi.fn()}
      />,
    );

    expect(screen.getByText("AI 生成能力与费用事实")).toBeInTheDocument();
    expect(screen.getByText(/doubao-seedream-5-0-lite/)).toBeInTheDocument();
    expect(
      screen.getByText("真实账号参数、计费和权限契约尚未验收"),
    ).toBeInTheDocument();
    expect(screen.getAllByText("CNY 12.50")).toHaveLength(2);
    expect(screen.getByText(/queued 请求只显示预占/)).toBeInTheDocument();
  });

  it("renders server-owned cleanup facts and exposes explicit schedule commands", async () => {
    const user = userEvent.setup();
    const onPauseSchedule = vi.fn().mockResolvedValue(undefined);
    const onConfigureSchedule = vi.fn().mockResolvedValue(true);
    const onResumeSchedule = vi.fn().mockResolvedValue(true);
    const onTriggerSchedule = vi.fn().mockResolvedValue(undefined);
    const view = render(
      <TaskWorkspace
        {...productionProps}
        busy={false}
        schedules={[schedule("active")]}
        tasks={[task]}
        onCancelGenerationTask={vi.fn()}
        onConfigureSchedule={onConfigureSchedule}
        onPauseSchedule={onPauseSchedule}
        onResumeSchedule={onResumeSchedule}
        onTriggerSchedule={onTriggerSchedule}
      />,
    );

    expect(screen.getByText("上传临时文件清理")).toBeInTheDocument();
    expect(screen.getByText("上传会话")).toBeInTheDocument();
    expect(screen.getByText("媒体清理计划")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "暂停" }));
    await user.click(screen.getByRole("button", { name: "立即触发" }));
    expect(onPauseSchedule).toHaveBeenCalledWith(expect.objectContaining({ id: scheduleId }));
    expect(onTriggerSchedule).toHaveBeenCalledWith(expect.objectContaining({ id: scheduleId }));

    view.rerender(
      <TaskWorkspace
        {...productionProps}
        busy={false}
        schedules={[schedule("paused")]}
        tasks={[task]}
        onCancelGenerationTask={vi.fn()}
        onConfigureSchedule={onConfigureSchedule}
        onPauseSchedule={onPauseSchedule}
        onResumeSchedule={onResumeSchedule}
        onTriggerSchedule={onTriggerSchedule}
      />,
    );
    await user.click(screen.getByRole("button", { name: "恢复并执行" }));
    await user.click(screen.getByRole("button", { name: "确认恢复" }));
    expect(onResumeSchedule).toHaveBeenCalledWith(
      expect.objectContaining({ id: scheduleId, status: "paused" }),
      "run_once",
      0,
    );
  });

  it("does not offer a new trigger for a completed one-off cleanup", () => {
    render(
      <TaskWorkspace
        {...productionProps}
        busy={false}
        schedules={[schedule("completed")]}
        tasks={[]}
        onCancelGenerationTask={vi.fn()}
        onConfigureSchedule={vi.fn()}
        onPauseSchedule={vi.fn()}
        onResumeSchedule={vi.fn()}
        onTriggerSchedule={vi.fn()}
      />,
    );

    expect(screen.getByText("已完成")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "立即触发" })).not.toBeInTheDocument();
  });

  it("renders the workspace interval cleanup with semantic task and schedule labels", () => {
    const cleanupTask: API.TaskResponse = {
      ...task,
      task_type: "upload_cleanup",
      request_type: "workspace",
      request_id: workspaceId,
      scope: {
        ...task.scope,
        usage_type: "workspace",
        usage_id: workspaceId,
      },
      status: "succeeded",
      progress_stage: "completed",
      next_action: null,
    };
    const cleanupSchedule: API.ScheduleResponse = {
      ...schedule("active"),
      schedule_key: `media.upload.cleanup:${workspaceId}`,
      handler_name: "cleanup_expired_uploads",
      scope: { usage_type: "workspace", usage_id: workspaceId },
      kind: "interval",
      rule: {
        kind: "interval",
        seconds: 3600,
        misfire_grace_seconds: 30,
      },
    };

    render(
      <TaskWorkspace
        {...productionProps}
        busy={false}
        schedules={[cleanupSchedule]}
        tasks={[cleanupTask]}
        onCancelGenerationTask={vi.fn()}
        onConfigureSchedule={vi.fn()}
        onPauseSchedule={vi.fn()}
        onResumeSchedule={vi.fn()}
        onTriggerSchedule={vi.fn()}
      />,
    );

    expect(screen.getByText("过期上传补偿清理")).toBeInTheDocument();
    expect(screen.getByText("工作空间")).toBeInTheDocument();
    expect(screen.getByText("周期过期上传补偿")).toBeInTheDocument();
  });

  it("configures only the registered cleanup schedule with explicit recovery semantics", async () => {
    const user = userEvent.setup();
    const onConfigureSchedule = vi.fn().mockResolvedValue(true);
    const cleanupSchedule: API.ScheduleResponse = {
      ...schedule("active"),
      schedule_key: `media.upload.cleanup:${workspaceId}`,
      handler_name: "cleanup_expired_uploads",
      scope: { usage_type: "workspace", usage_id: workspaceId },
      kind: "interval",
      rule: {
        kind: "interval",
        seconds: 3600,
        misfire_grace_seconds: 30,
      },
    };

    render(
      <TaskWorkspace
        {...productionProps}
        busy={false}
        schedules={[cleanupSchedule]}
        tasks={[]}
        onCancelGenerationTask={vi.fn()}
        onConfigureSchedule={onConfigureSchedule}
        onPauseSchedule={vi.fn()}
        onResumeSchedule={vi.fn()}
        onTriggerSchedule={vi.fn()}
      />,
    );

    await user.click(screen.getByRole("button", { name: "配置周期" }));
    await user.clear(screen.getByLabelText("间隔秒数"));
    await user.type(screen.getByLabelText("间隔秒数"), "7200");
    await user.clear(screen.getByLabelText("超时宽限（秒）"));
    await user.type(screen.getByLabelText("超时宽限（秒）"), "45");
    await user.click(screen.getByRole("button", { name: "保存计划配置" }));

    expect(onConfigureSchedule).toHaveBeenCalledWith(
      expect.objectContaining({ id: scheduleId }),
      {
        kind: "interval",
        interval_seconds: 7200,
        cron_expression: null,
        timezone: "UTC",
        misfire_policy: "run_once",
        max_catch_up: 0,
        misfire_grace_seconds: 45,
      },
    );
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  });
});
