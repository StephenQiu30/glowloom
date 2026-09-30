import { Suspense } from "react";
import { ProjectBrowser } from "@/features/workbench/project-pages";
export default function ProjectsPage() {
  return (
    <Suspense fallback={<p>正在加载项目…</p>}>
      <ProjectBrowser />
    </Suspense>
  );
}
