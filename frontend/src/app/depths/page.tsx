import { Suspense } from "react";
import { WorkspaceShell } from "@/components/workbench/workspace-shell";
import { DepthWorkspace } from "@/components/canvas/depth-workspace";
export default function DepthsPage() {
  return (
    <WorkspaceShell>
      <Suspense fallback={<p>载入视频深度工作区…</p>}>
        <DepthWorkspace />
      </Suspense>
    </WorkspaceShell>
  );
}
