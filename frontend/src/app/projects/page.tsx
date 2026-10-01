import { Suspense } from "react";
import { ProjectsWorkspace } from "@/components/project/projects-workspace";
import { WorkspaceShell } from "@/components/workbench/workspace-shell";
import { Skeleton } from "@/components/ui/skeleton";

export default function ProjectsPage() {
  return (
    <WorkspaceShell>
      <Suspense fallback={<Skeleton className="h-80" />}>
        <ProjectsWorkspace />
      </Suspense>
    </WorkspaceShell>
  );
}
