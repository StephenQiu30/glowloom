"use client";
import dynamic from "next/dynamic";
import { OverviewPage, CostsPage, SettingsPage } from "./project-pages";
import { ScriptPage, ParsePage } from "./script-pages";
import { BiblePage, AssetsPage, MediaPage } from "./asset-pages";
import { ShotsPage, ShotDetailPage, AudioPage } from "./shot-pages";
import { ImpactPage } from "./project-pages";
const CreationCanvas = dynamic(
  () => import("./creation-canvas").then((m) => m.CreationCanvas),
  { ssr: false, loading: () => <p>正在加载创作画布…</p> },
);
import { PreviewBoundary } from "./workbench-components";
import type { ProjectRoute } from "./routes";

export function ProjectScreen({ route }: { route: ProjectRoute }) {
  const projectId = route.project.id;
  const episodeId = route.episodeId ?? "ep-01";
  let page;
  switch (route.kind) {
    case "overview":
      page = <OverviewPage projectId={projectId} />;
      break;
    case "script":
      page = <ScriptPage projectId={projectId} />;
      break;
    case "parse":
      page = <ParsePage projectId={projectId} episodeId={episodeId} />;
      break;
    case "bible":
      page = <BiblePage projectId={projectId} />;
      break;
    case "assets":
      page = <AssetsPage projectId={projectId} episodeId={episodeId} />;
      break;
    case "shots":
      page = <ShotsPage projectId={projectId} episodeId={episodeId} />;
      break;
    case "storyboard":
      page = (
        <ShotsPage projectId={projectId} episodeId={episodeId} storyboard />
      );
      break;
    case "shot":
      page = (
        <ShotDetailPage
          projectId={projectId}
          episodeId={episodeId}
          shotId={route.shotId!}
        />
      );
      break;
    case "audio":
      page = <AudioPage />;
      break;
    case "canvas":
      page = <CreationCanvas projectId={projectId} />;
      break;
    case "media":
      page = <MediaPage />;
      break;
    case "impact":
      page = <ImpactPage />;
      break;
    case "costs":
      page = <CostsPage />;
      break;
    case "settings":
      page = <SettingsPage projectId={projectId} />;
      break;
  }
  return <PreviewBoundary>{page}</PreviewBoundary>;
}
