import { Suspense } from "react";
import { WorkspaceShell } from "@/components/workbench/workspace-shell";
import { SettingsWorkspace } from "@/components/catalog/settings-workspace";

export default function SettingsPage() {
  return (
    <WorkspaceShell>
      <Suspense fallback={<p>载入模型设置…</p>}>
        <SettingsWorkspace />
      </Suspense>
    </WorkspaceShell>
  );
}
