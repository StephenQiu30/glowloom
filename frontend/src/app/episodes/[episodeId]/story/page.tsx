import { StoryWorkspace } from "@/features/story/story-workspace";

export default async function StoryPage({
  params,
}: {
  params: Promise<{ episodeId: string }>;
}) {
  const { episodeId } = await params;
  return <StoryWorkspace episodeId={episodeId} />;
}
