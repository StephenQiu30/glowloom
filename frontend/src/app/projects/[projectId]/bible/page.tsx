import { Suspense } from "react";
import { notFound } from "next/navigation";
import { ProjectBibleWorkspace } from "@/components/bible/project-bible-workspace";

export const metadata = { title: "设定集" };

export default async function ProjectBiblePage({
  params,
}: {
  params: Promise<{ projectId: string }>;
}) {
  const { projectId } = await params;
  if (
    projectId === "00000000-0000-0000-0000-000000000000" ||
    !/^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i.test(
      projectId,
    )
  )
    notFound();
  return (
    <Suspense
      fallback={
        <p role="status" className="p-8">
          正在加载设定集…
        </p>
      }
    >
      <ProjectBibleWorkspace projectId={projectId} />
    </Suspense>
  );
}
