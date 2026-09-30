import { Suspense } from "react";
import { TaskListPage } from "@/features/workbench/task-pages";
import { PreviewBoundary } from "@/features/workbench/poc-components";
export default function TasksPage() {
  return (
    <Suspense fallback={<p>正在加载页面…</p>}>
      <PreviewBoundary>
        <TaskListPage />
      </PreviewBoundary>
    </Suspense>
  );
}
