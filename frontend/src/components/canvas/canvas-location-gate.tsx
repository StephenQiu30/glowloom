"use client";

import {
  useEffect,
  useEffectEvent,
  useRef,
  useState,
  type ReactNode,
} from "react";
import Link from "next/link";
import { useRouter, useSearchParams } from "next/navigation";
import { useSpace } from "@/components/spaces/space-context";
import { ContentState } from "@/components/layout/content-state";
import { Button } from "@/components/ui/button";
import { isProjectID } from "@/components/projects/project";
import { locateCanvas } from "@/api/canvases";
import { RequestError } from "@/lib/request";

type CanvasLocation = {
  canvas_id: string;
  project_id: string;
  scope_id: string;
};
type Destination =
  | { kind: "loading" }
  | { kind: "ready"; location: CanvasLocation }
  | { kind: "error"; unavailable: boolean };

export function CanvasLocationGate({
  canvasID,
  children,
}: {
  canvasID: string;
  children: (location: CanvasLocation) => ReactNode;
}) {
  const space = useSpace();
  const router = useRouter();
  const search = useSearchParams();
  const [destination, setDestination] = useState<Destination>({
    kind: "loading",
  });
  const [attempt, setAttempt] = useState(0);
  const switching = useRef<string | null>(null);

  useEffect(() => {
    const controller = new AbortController();
    void locateCanvas({ id: canvasID }, { signal: controller.signal }).then(
      (location) => {
        if (controller.signal.aborted) return;
        if (
          !location ||
          location.canvas_id !== canvasID ||
          !isProjectID(location.project_id) ||
          !isProjectID(location.scope_id)
        ) {
          setDestination({ kind: "error", unavailable: false });
          return;
        }
        setDestination({
          kind: "ready",
          location: {
            canvas_id: canvasID,
            project_id: location.project_id,
            scope_id: location.scope_id,
          },
        });
      },
      (error: unknown) => {
        if (!controller.signal.aborted)
          setDestination({
            kind: "error",
            unavailable:
              error instanceof RequestError &&
              [403, 404].includes(error.status),
          });
      },
    );
    return () => controller.abort();
  }, [canvasID, attempt]);

  const target = destination.kind === "ready" ? destination.location : null;
  const unavailableSpace =
    target &&
    !space.pending &&
    !space.spaces.some((item) => item.id === target.scope_id);
  const selectSpace = useEffectEvent((id: string) => space.select(id));
  useEffect(() => {
    if (
      !target ||
      unavailableSpace ||
      space.pending ||
      space.error ||
      space.selected?.id === target.scope_id ||
      switching.current === target.scope_id
    )
      return;
    switching.current = target.scope_id;
    void selectSpace(target.scope_id).catch(() =>
      setDestination({ kind: "error", unavailable: false }),
    );
  }, [
    target,
    space.selected?.id,
    space.pending,
    space.error,
    unavailableSpace,
  ]);

  useEffect(() => {
    if (!target || space.selected?.id !== target.scope_id || space.pending)
      return;
    if (
      search.get("scope_id") === target.scope_id &&
      search.get("project_id") === target.project_id
    )
      return;
    const params = new URLSearchParams(search.toString());
    params.set("scope_id", target.scope_id);
    params.set("project_id", target.project_id);
    router.replace(`/canvas?${params}`, { scroll: false });
  }, [target, space.selected?.id, space.pending, router, search]);

  const inaccessible =
    (destination.kind === "error" && destination.unavailable) ||
    unavailableSpace;
  const selectionFailed =
    target &&
    !space.pending &&
    space.selected?.id !== target.scope_id &&
    space.error;
  if (destination.kind === "error" || selectionFailed || unavailableSpace)
    return (
      <ContentState
        className="h-dvh"
        state="error"
        title="无法打开画布"
        description={
          inaccessible ? "链接已失效或暂无访问权限。" : "暂时无法连接，请重试。"
        }
        actions={
          inaccessible ? (
            <Button asChild>
              <Link href="/projects">返回画布列表</Link>
            </Button>
          ) : (
            <Button
              onClick={() => {
                switching.current = null;
                setDestination({ kind: "loading" });
                setAttempt((value) => value + 1);
                if (space.error) void space.refresh();
              }}
            >
              重试
            </Button>
          )
        }
      />
    );
  if (!target || space.pending || space.selected?.id !== target.scope_id)
    return (
      <ContentState className="h-dvh" state="loading" title="正在打开画布" />
    );
  return children(target);
}
