import { Suspense } from "react";
import { notFound } from "next/navigation";
import { ProjectScriptWorkspace } from "@/components/script/project-script-workspace";

export const metadata = { title: "剧本 | Lanverse" };

export default async function ProjectScriptPage({
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
  ) {
    notFound();
  }
  return (
    <Suspense
      fallback={
        <p role="status" className="p-8">
          正在加载剧本…
        </p>
      }
    >
      <ProjectScriptWorkspace projectId={projectId} />
    </Suspense>
  );
}
