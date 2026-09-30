import { WorkspaceShell } from "@/features/workbench/workspace-shell";
export default function ProjectsLayout({ children }: LayoutProps<"/projects">) {
  return <WorkspaceShell>{children}</WorkspaceShell>;
}
