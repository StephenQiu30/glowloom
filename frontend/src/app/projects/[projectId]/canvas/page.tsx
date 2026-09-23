import { CanvasWorkspace } from "@/components/canvas/canvas-workspace";

export default async function CanvasPage({ params }: { params: Promise<{ projectId: string }> }) {
  const { projectId } = await params;
  return <CanvasWorkspace projectId={projectId} />;
}
