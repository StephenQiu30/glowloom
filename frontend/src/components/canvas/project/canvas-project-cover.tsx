import type { CanvasLibraryItem } from "../hooks/use-projects";
import { ResourceCover } from "./workspace-resource-card";

export function CanvasProjectCover({
  project,
}: {
  project: CanvasLibraryItem;
}) {
  return (
    <ResourceCover
      title={project.title}
      kind="canvas"
      coverFileId={project.coverFileId}
      coverFileVersion={project.coverFileVersion}
    />
  );
}
