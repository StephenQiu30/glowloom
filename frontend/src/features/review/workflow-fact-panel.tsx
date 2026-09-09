"use client";

import { Card, CardHeader, CardTitle, CardDescription, CardContent } from "@/components/ui/card";
import { Alert, AlertTitle, AlertDescription } from "@/components/ui/alert";
import { AlertCircle, LoaderCircle } from "lucide-react";
import { appApiErrorMessage } from "@/lib/server-state";
import { Fact } from "./review-fact";
import { shortId, shortHash } from "./review-presentation";

export function WorkflowFactPanel({
  coordination,
  error,
  gateNode,
  isFetching,
  run,
  verified,
}: {
  coordination: API.HumanGateCoordinationResponse | null | undefined;
  error: unknown;
  gateNode: API.WorkflowNodeRunResponse | undefined;
  isFetching: boolean;
  run: API.WorkflowRunResponse | undefined;
  verified: boolean;
}) {
  return (
    <Card className="border" id="workflow-run">
      <CardHeader className="border-b">
        <CardTitle>WorkflowRun 复核</CardTitle>
        <CardDescription>
          Resume 完成后仍需重取匹配的 NodeRun 和 Gate Output，不能用本地成功代替。
        </CardDescription>
      </CardHeader>
      <CardContent className="grid gap-4 pt-1">
        {error ? (
          <Alert variant="destructive">
            <AlertCircle aria-hidden="true" />
            <AlertTitle>运行事实暂时无法读取</AlertTitle>
            <AlertDescription>{appApiErrorMessage(error)}</AlertDescription>
          </Alert>
        ) : !run || !gateNode ? (
          <p className="flex items-center gap-2 text-sm text-muted-foreground">
            {isFetching ? <LoaderCircle aria-hidden="true" className="size-4 animate-spin" /> : null}
            正在重取 WorkflowRun 与审核节点事实。
          </p>
        ) : (
          <div className="grid gap-4 text-sm sm:grid-cols-2">
            <Fact label="WorkflowRun" value={shortId(run.id)} mono />
            <Fact label="Run 状态" value={run.status} />
            <Fact label="审核 NodeRun" value={shortId(gateNode.id)} mono />
            <Fact label="Node 状态" value={gateNode.status} />
            <Fact label="Gate Output hash" value={shortHash(gateNode.output_hash)} mono />
            <Fact
              label="复核结论"
              value={coordination?.repair_workflow_run_id
                ? `有界修复已创建 ${shortId(coordination.repair_workflow_run_id)}`
                : verified
                  ? "工作流已继续"
                : coordination?.workflow_resume_status === "completed"
                  ? "恢复已确认，运行事实尚未收敛"
                  : "等待 Workflow Resume 完成"}
            />
          </div>
        )}
      </CardContent>
    </Card>
  );
}
