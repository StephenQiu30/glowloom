"use client";

import { taskStatusLabels, shortId } from "./review-presentation";

export function TaskStatusPanel({
  coordination,
  decision,
  task,
  workflowFactVerified,
}: {
  coordination: API.HumanGateCoordinationResponse | null | undefined;
  decision: API.ReviewDecisionResponse | null;
  task: API.HumanTaskResponse;
  workflowFactVerified: boolean;
}) {
  const ownerText = !coordination
    ? "尚未开始"
    : coordination.owner_apply_status === "completed"
      ? coordination.workflow_resume_status === "completed"
        ? "业务应用已完成"
        : "业务应用完成，正在恢复工作流"
      : coordination.owner_apply_status === "not_required"
        ? "此决议无需业务应用"
        : coordination.owner_apply_status === "conflict"
          ? "业务应用冲突"
          : "等待业务应用";
  const workflowText = !coordination
    ? "尚未开始"
    : coordination.workflow_resume_status === "unknown"
      ? "结果未知，可安全恢复"
      : coordination.workflow_resume_status === "completed"
        ? decision?.decision === "changes_requested"
          ? coordination.repair_workflow_run_id
            ? "有界修复已启动"
            : "原流程已结束，等待启动修复"
          : workflowFactVerified
            ? "工作流已继续"
            : "恢复已确认，正在核对运行事实"
        : coordination.workflow_resume_status === "conflict"
          ? "工作流恢复冲突"
          : "等待恢复";
  const workflowReference = coordination?.repair_workflow_run_id
    ?? coordination?.workflow_signal_receipt_id;

  return (
    <section aria-label="审核状态" className="grid gap-px overflow-hidden border bg-border sm:grid-cols-2 xl:grid-cols-4" role="region">
      <StatusCell label="任务状态" value={taskStatusLabels[task.status]} />
      <StatusCell
        label="决议状态"
        meta={decision ? shortId(decision.id) : undefined}
        value={decision ? "决议已记录" : "尚未记录"}
      />
      <StatusCell
        label="业务应用"
        meta={coordination?.owner_receipt_id ? shortId(coordination.owner_receipt_id) : undefined}
        value={ownerText}
      />
      <StatusCell
        label="工作流恢复"
        meta={workflowReference ? shortId(workflowReference) : undefined}
        value={workflowText}
      />
    </section>
  );
}

export function StatusCell({ label, meta, value }: { label: string; meta?: string; value: string }) {
  return (
    <div className="min-h-28 bg-card p-4">
      <p className="text-xs font-medium text-muted-foreground">{label}</p>
      <p className="mt-3 text-sm font-semibold">{value}</p>
      {meta ? <p className="mt-1 font-mono text-[11px] text-muted-foreground">{meta}</p> : null}
    </div>
  );
}
