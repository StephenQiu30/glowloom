import { notFound } from "next/navigation";
import { ProjectScreen } from "@/features/workbench/project-screen";
import { resolveProjectRoute } from "@/features/workbench/routes";
export default async function ProjectPage({
  params,
}: {
  params: Promise<{ projectId: string; section?: string[] }>;
}) {
  const { projectId, section } = await params;
  const route = resolveProjectRoute(projectId, section);
  if (!route) notFound();
  return (
    <ProjectScreen
      key={`${projectId}/${section?.join("/") ?? ""}`}
      route={route}
    />
  );
}
