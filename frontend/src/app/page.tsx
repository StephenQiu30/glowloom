import { Suspense } from "react";
import { Skeleton } from "@/components/ui/skeleton";
import { HomeDashboard } from "@/components/workbench/home-dashboard";
import { WorkspaceShell } from "@/components/workbench/workspace-shell";

export default function HomePage() {
  return (
    <WorkspaceShell>
      <Suspense fallback={<Skeleton className="h-96" />}>
        <HomeDashboard />
      </Suspense>
    </WorkspaceShell>
  );
}
