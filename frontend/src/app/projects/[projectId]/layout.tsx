import type { ReactNode } from "react";
import { WorkspaceShell } from "@/components/workbench/workspace-shell";
export default function ProjectLayout({ children }: { children: ReactNode }) {
  return <WorkspaceShell>{children}</WorkspaceShell>;
}
