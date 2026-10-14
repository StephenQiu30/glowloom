"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { Button } from "@/components/ui/button";
import { getProject } from "@/api/projects";
import { getCanvas } from "@/api/canvases";
import {
  isProjectID,
  projectErrorMessage,
  requireProject,
  type Project,
} from "@/components/projects/project";
import { RequestError } from "@/lib/request";

import dynamic from "next/dynamic";
const CanvasEditor = dynamic(
  () => import("./canvas-editor").then((m) => m.CanvasEditor),
  { loading: () => <p role="status">正在加载画布编辑器…</p> },
);
import { requireCanvas, type Canvas } from "./editor-state";

function LoadedCanvas({
  projectID,
  canvasID,
}: {
  projectID: string;
  canvasID: string;
}) {
  const [result, setResult] = useState<{
    project: Project;
    canvas: Canvas;
  } | null>(null);
  const [error, setError] = useState("");
  const [retry, setRetry] = useState(0);
  useEffect(() => {
    const controller = new AbortController();
    // These authenticated reads are independent; neither waits on the other.
    Promise.all([
      getProject({ pid: projectID }, { signal: controller.signal }),
      getCanvas({ id: canvasID }, { signal: controller.signal }),
    ])
      .then(([projectValue, canvasValue]) => {
        const project = requireProject(projectValue, true);
        const canvas = requireCanvas(canvasValue, projectID, canvasID);
        if (project.id !== projectID || project.canvas_id !== canvasID)
          throw new RequestError("invalid_receipt");
        if (!controller.signal.aborted) setResult({ project, canvas });
      })
      .catch((e: unknown) => {
        if (!controller.signal.aborted) setError(projectErrorMessage(e));
      });
    return () => controller.abort();
  }, [projectID, canvasID, retry]);

  if (result && !error)
    return <CanvasEditor project={result.project} initial={result.canvas} />;
  return (
    <main className="grid min-h-dvh place-items-center p-6">
      {error ? (
        <div
          role="alert"
          className="max-w-md space-y-4 rounded-xl border bg-card p-6"
        >
          <p>{error}</p>
          <Button
            variant="outline"
            onClick={() => {
              setError("");
              setResult(null);
              setRetry((n) => n + 1);
            }}
          >
            重新读取
          </Button>
          <Button asChild variant="ghost">
            <Link href="/projects">返回我的画布</Link>
          </Button>
        </div>
      ) : (
        <p role="status">正在读取项目与画布…</p>
      )}
    </main>
  );
}

export function CanvasPage({
  projectID,
  canvasID,
}: {
  projectID?: string;
  canvasID?: string;
}) {
  if (!isProjectID(projectID) || !isProjectID(canvasID))
    return (
      <main className="grid min-h-dvh place-items-center p-6">
        <div className="space-y-4 text-center">
          <h1 className="text-xl font-semibold">请选择有效的项目与画布</h1>
          <p className="text-muted-foreground">从我的画布打开创作以继续。</p>
          <Button asChild>
            <Link href="/projects">返回我的画布</Link>
          </Button>
        </div>
      </main>
    );
  return (
    <LoadedCanvas
      key={`${projectID}:${canvasID}`}
      projectID={projectID}
      canvasID={canvasID}
    />
  );
}
