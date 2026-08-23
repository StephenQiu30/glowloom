import { ProjectDashboard } from "./project-dashboard";

type ProjectsPageProps = {
  searchParams: Promise<{ workspace?: string | string[] }>;
};

export default async function ProjectsPage({ searchParams }: ProjectsPageProps) {
  const { workspace } = await searchParams;
  const requestedWorkspaceId = typeof workspace === "string" ? workspace : undefined;

  return <ProjectDashboard requestedWorkspaceId={requestedWorkspaceId} />;
}
