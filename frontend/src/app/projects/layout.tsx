import { WorkspaceShell } from "@/components/workbench/workspace-shell";
export default function ProjectsLayout({ children }: LayoutProps<"/projects">) {
  return <WorkspaceShell>{children}</WorkspaceShell>;
}
