import { HomePage } from "@/components/workbench/home-page";
import { WorkspaceShell } from "@/components/workbench/workspace-shell";

export default function Home() {
  return (
    <WorkspaceShell>
      <HomePage />
    </WorkspaceShell>
  );
}
