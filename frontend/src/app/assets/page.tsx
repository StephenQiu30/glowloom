import { Suspense } from "react";
import { WorkspaceShell } from "@/components/workbench/workspace-shell";
import { AssetsWorkspace } from "@/components/media/assets-workspace";
export default function AssetsPage() {
  return (
    <WorkspaceShell>
      <Suspense fallback={<p>载入工作区…</p>}>
        <AssetsWorkspace />
      </Suspense>
    </WorkspaceShell>
  );
}
