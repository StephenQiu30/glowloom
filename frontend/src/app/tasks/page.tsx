import { Suspense } from "react";
import { WorkspaceShell } from "@/components/workbench/workspace-shell";
import { TaskWorkspace } from "@/components/operation/task-workspace";
export default function TasksPage() {
  return (
    <WorkspaceShell>
      <Suspense fallback={<p>载入工作区…</p>}>
        <TaskWorkspace />
      </Suspense>
    </WorkspaceShell>
  );
}
