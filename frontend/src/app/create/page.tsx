import { Suspense } from "react";
import { WorkspaceShell } from "@/components/workbench/workspace-shell";
import { CreateWorkspace } from "@/components/operation/create-workspace";
export default function CreatePage() {
  return (
    <WorkspaceShell>
      <Suspense fallback={<p>载入创作工作区…</p>}>
        <CreateWorkspace />
      </Suspense>
    </WorkspaceShell>
  );
}
