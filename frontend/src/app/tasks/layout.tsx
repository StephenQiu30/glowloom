import { WorkspaceShell } from "@/components/workbench/workspace-shell";
export default function Layout({ children }: { children: React.ReactNode }) {
  return <WorkspaceShell>{children}</WorkspaceShell>;
}
