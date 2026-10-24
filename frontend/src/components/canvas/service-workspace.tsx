"use client";
import Link from "next/link";
import { useSearchParams } from "next/navigation";
import { useOptionalSession, SessionGate } from "@/components/auth/session";
import { useSpace, SpaceGate } from "@/components/spaces/space-context";
import { Button } from "@/components/ui/button";
import { isProjectID } from "@/components/projects/project";
import { ContentState } from "@/components/layout/content-state";
import { CanvasLocationGate } from "./canvas-location-gate";
import { YjsCanvasWorkspace } from "./yjs-workspace";

function Entry() {
  const search = useSearchParams();
  const session = useOptionalSession();
  const space = useSpace();
  const canvasID = search.get("canvas_id"),
    projectID = search.get("project_id");
  if (!isProjectID(canvasID) || !isProjectID(projectID))
    return (
      <ContentState
        className="h-dvh"
        title="请选择一张画布"
        description="从画布列表打开已有创作，或新建画布。"
        actions={
          <Button asChild>
            <Link href="/projects">我的画布</Link>
          </Button>
        }
      />
    );
  if (!session || !space.selected) return null;
  return (
    <CanvasLocationGate
      key={`${session.actor.id}/${canvasID}`}
      canvasID={canvasID}
    >
      {(location) => (
        <YjsCanvasWorkspace
          key={`${session.actor.id}/${location.scope_id}/${canvasID}`}
          canvasID={canvasID}
          projectID={location.project_id}
          actorID={session.actor.id}
          scopeID={location.scope_id}
        />
      )}
    </CanvasLocationGate>
  );
}
export function ServiceCanvasWorkspace() {
  return (
    <SessionGate>
      <SpaceGate>
        <Entry />
      </SpaceGate>
    </SessionGate>
  );
}
