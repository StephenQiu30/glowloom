import { Suspense } from "react";
import { notFound } from "next/navigation";
import { CanvasWorkspace } from "@/components/canvas/workspace";

export const metadata = { title: "画布 | Lanverse" };

export default async function ProjectCanvasPage({
  params,
}: {
  params: Promise<{ projectId: string }>;
}) {
  const { projectId } = await params;
  if (
    !/^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i.test(
      projectId,
    )
  ) {
    notFound();
  }
  return (
    <Suspense
      fallback={
        <p role="status" className="p-8">
          正在加载画布…
        </p>
      }
    >
      <CanvasWorkspace initialProjectId={projectId} />
    </Suspense>
  );
}
