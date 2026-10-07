import { WorkspaceShell } from "@/components/workbench/workspace-shell";
import { Suspense } from "react";
import { CanvasWorkspace } from "@/components/canvas/workspace";

export const metadata = { title: "无限画布" };
export default function CanvasPage() {
  return (
    <WorkspaceShell>
      <Suspense
        fallback={
          <div className="p-10">
            <p role="status">正在加载画布工作区…</p>
          </div>
        }
      >
        <CanvasWorkspace />
      </Suspense>
    </WorkspaceShell>
  );
}
