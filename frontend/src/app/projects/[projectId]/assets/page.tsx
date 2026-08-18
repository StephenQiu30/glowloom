import { ComicProductionStudio } from "@/app/studio/comic-production-studio";

export default async function ProjectAssetsPage({
  params,
}: {
  params: Promise<{ projectId: string }>;
}) {
  const { projectId } = await params;

  return <ComicProductionStudio initialProjectId={projectId} />;
}
