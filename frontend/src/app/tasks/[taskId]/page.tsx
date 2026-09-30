import { notFound } from "next/navigation";
import { tasks } from "@/components/workbench/data";
import { TaskDetailPage } from "@/components/workbench/task-pages";
import { PreviewBoundary } from "@/components/workbench/workbench-components";
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
