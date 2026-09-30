import { Suspense } from "react";
import { AssetsWorkspace } from "@/components/workbench/assets-workspace";
import { WorkspaceShell } from "@/components/workbench/workspace-shell";

export default function AssetsPage() {
  return (
    <Suspense
      fallback={
        <p role="status" className="p-6">
          正在加载资源…
        </p>
      }
    >
      <WorkspaceShell>
        <AssetsWorkspace />
      </WorkspaceShell>
    </Suspense>
  );
}
