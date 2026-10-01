import { HomeDashboard } from "@/components/workbench/home-dashboard";
import { WorkspaceShell } from "@/components/workbench/workspace-shell";

export default function HomePage() {
  return (
    <WorkspaceShell>
      <HomeDashboard />
    </WorkspaceShell>
  );
}
