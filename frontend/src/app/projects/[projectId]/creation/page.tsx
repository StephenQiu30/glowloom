import { CreationWorkspace } from "@/features/creation/creation-workspace";

export default async function CreationPage({ params, searchParams }: {
  params: Promise<{ projectId: string }>;
  searchParams: Promise<{ run?: string }>;
}) {
  const [{ projectId }, query] = await Promise.all([params, searchParams]);
  return <CreationWorkspace key={projectId} projectId={projectId} initialRunId={query.run} />;
}
