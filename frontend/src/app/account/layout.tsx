import { WorkspaceShell } from "@/features/workbench/workspace-shell";
export default function Layout({ children }: { children: React.ReactNode }) {
  return <WorkspaceShell>{children}</WorkspaceShell>;
}
