"use client";
import { FeedbackToast } from "@/components/feedback/feedback-toast";
import {
  ApplicationLoading,
  ApplicationState,
} from "@/components/layout/application-state";

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
  { loading: () => <ApplicationLoading message="正在加载画布编辑器…" /> },
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
  if (!error) return <ApplicationLoading message="正在读取项目与画布…" />;
  return (
    <ApplicationState
      title="暂时无法打开画布"
      description="请重试读取，或返回我的画布选择其他项目。"
    >
      <FeedbackToast message={error} />
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
    </ApplicationState>
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
      <ApplicationState
        title="请选择有效的项目与画布"
        description="从我的画布打开创作以继续。"
      >
        <Button asChild>
          <Link href="/projects">返回我的画布</Link>
        </Button>
      </ApplicationState>
    );
  return (
    <LoadedCanvas
      key={`${projectID}:${canvasID}`}
      projectID={projectID}
      canvasID={canvasID}
    />
  );
}
