import { Suspense } from "react";
import { TaskListPage } from "@/components/workbench/task-pages";
import { PreviewBoundary } from "@/components/workbench/workbench-components";
export default function TasksPage() {
  return (
    <Suspense fallback={<p>正在加载页面…</p>}>
      <PreviewBoundary>
        <TaskListPage />
      </PreviewBoundary>
    </Suspense>
  );
}
