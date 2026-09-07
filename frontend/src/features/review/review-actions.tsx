"use client";

import { type DecisionValue, shortId, formatTimestamp, decisionLabels } from "./review-presentation";
import { Card, CardHeader, CardTitle, CardDescription, CardContent } from "@/components/ui/card";
import { Fact } from "./review-fact";
import { Button } from "@/components/ui/button";
import { RefreshCcw } from "lucide-react";

export function ReviewActions({
  busy,
  canWrite,
  claimToken,
  coordination,
  decision,
  effectiveCandidate,
  onClaim,
  onDecision,
  onRelease,
  onRenew,
  onResume,
  task,
}: {
  busy: boolean;
  canWrite: boolean;
  claimToken?: string;
  coordination: API.HumanGateCoordinationResponse | null | undefined;
  decision: API.ReviewDecisionResponse | null;
  effectiveCandidate: string;
  onClaim: () => void;
  onDecision: (decision: DecisionValue) => void;
  onRelease: () => void;
  onRenew: () => void;
  onResume: () => void;
  task: API.HumanTaskResponse;
}) {
  const canClaim = canWrite
    && !decision
    && (task.status === "OPEN" || (task.status === "CLAIMED" && !claimToken));
  const canUseClaim = canWrite && !decision && task.status === "CLAIMED" && Boolean(claimToken);
  const canResume = canWrite
    && Boolean(decision)
    && coordination?.workflow_resume_status !== "completed"
    && coordination?.workflow_resume_status !== "conflict"
    && coordination?.owner_apply_status !== "conflict";

  return (
    <Card className="border">
      <CardHeader className="border-b">
        <CardTitle>可执行动作</CardTitle>
        <CardDescription>所有写入都重新校验服务端 revision、租约和冻结事实。</CardDescription>
      </CardHeader>
      <CardContent className="grid gap-4 pt-1">
        {task.claim ? (
          <div aria-label="审核租约" className="grid gap-2 border p-3 text-sm sm:grid-cols-2">
            <Fact
              label={claimToken ? "当前租约" : "租约所有者"}
              value={claimToken ? "当前账号持有" : shortId(task.claim.claimed_by)}
            />
            <div>
              <p className="text-xs font-medium text-muted-foreground">服务端到期时间</p>
              <time className="mt-1 block" dateTime={task.claim.expires_at}>
                {formatTimestamp(task.claim.expires_at)}
              </time>
            </div>
          </div>
        ) : null}
        {canClaim ? (
          <div className="flex flex-wrap items-center gap-3">
            <Button disabled={busy} onClick={onClaim}>
              {task.status === "CLAIMED" ? "尝试接管审核" : "领取审核"}
            </Button>
            {task.status === "CLAIMED" ? (
              <p className="text-xs text-muted-foreground">服务端仅在原租约过期后允许接管。</p>
            ) : null}
          </div>
        ) : null}
        {canUseClaim ? (
          <div className="flex flex-wrap gap-3">
            <Button disabled={busy} onClick={onRenew} variant="outline">续期租约</Button>
            <Button disabled={busy} onClick={onRelease} variant="outline">释放审核</Button>
          </div>
        ) : null}
        {canUseClaim ? (
          <div className="flex flex-wrap gap-3 border-t pt-4">
            {task.allowed_decisions.map((value) => (
              <Button
                disabled={busy || (value === "selected" && !effectiveCandidate)}
                key={value}
                onClick={() => onDecision(value)}
                variant={value === "rejected" ? "outline" : "default"}
              >
                {decisionLabels[value]}
              </Button>
            ))}
          </div>
        ) : null}
        {canResume ? (
          <Button className="w-fit" disabled={busy} onClick={onResume}>
            <RefreshCcw aria-hidden="true" />
            按原决议恢复工作流
          </Button>
        ) : null}
        {!canClaim && !canUseClaim && !canResume ? (
          <p className="text-sm text-muted-foreground">
            当前状态没有可执行写命令；仍可查看全部持久化状态。
          </p>
        ) : null}
      </CardContent>
    </Card>
  );
}
