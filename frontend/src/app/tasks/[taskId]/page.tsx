import { notFound } from "next/navigation";
import { tasks } from "@/features/workbench/data";
import { TaskDetailPage } from "@/features/workbench/task-pages";
import { PreviewBoundary } from "@/features/workbench/workbench-components";
export default async function TaskPage({
  params,
}: {
  params: Promise<{ taskId: string }>;
}) {
  const { taskId } = await params;
  if (!tasks.some((t) => t.id === taskId)) notFound();
  return (
    <PreviewBoundary>
      <TaskDetailPage taskId={taskId} />
    </PreviewBoundary>
  );
}
