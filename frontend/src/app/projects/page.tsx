import { Suspense } from "react";
import { ProjectBrowser } from "@/features/workbench/project-pages";
export default function ProjectsPage() {
  return (
    <Suspense
      fallback={
        <p role="status" className="p-6">
          正在加载项目…
        </p>
      }
    >
      <ProjectBrowser />
    </Suspense>
  );
}
