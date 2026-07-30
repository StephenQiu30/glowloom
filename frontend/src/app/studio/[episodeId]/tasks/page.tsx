import type { Metadata } from "next";

import { EpisodeProductionStudio } from "../episode-production-studio";

export const metadata: Metadata = {
  title: "任务 · Lanverse",
  description: "AI 漫剧生产任务状态、恢复与失败原因",
};

export default async function EpisodeTasksPage({
  params,
}: {
  params: Promise<{ episodeId: string }>;
}) {
  const { episodeId } = await params;
  return <EpisodeProductionStudio episodeId={episodeId} initialPanel="tasks" />;
}
