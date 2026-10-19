"use client";

import {
  createContext,
  useContext,
  useEffect,
  useMemo,
  useState,
  useSyncExternalStore,
  type ReactNode,
} from "react";
import { useRouter } from "next/navigation";
import Link from "next/link";
import { createProject, getProject, getProjectCreation } from "@/api/projects";
import { getCanvas } from "@/api/canvases";
import { AuthDialog } from "@/components/auth/auth-dialog";
import {
  useOptionalSession,
  useSessionUpdate,
  ensureSessionFresh,
} from "@/components/auth/session";
import { useSpace } from "@/components/spaces/space-context";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from "@/components/ui/dialog";
import {
  getSessionGeneration,
  getScopeGeneration,
  getCreationScope,
  RequestError,
} from "@/lib/request";
import { projectIntentStore } from "./project-intent-store";
import { ProjectCreationController } from "./project-creation-controller";
import { requireProject } from "./project";
import type {
  ProjectCreationState,
  IntentTarget,
} from "./project-creation-state";

type CreationActions = {
  start: () => Promise<void>;
  state: ProjectCreationState;
  disabled: boolean;
};
const CreationContext = createContext<CreationActions>({
  start: async () => {},
  state: { phase: "idle" },
  disabled: false,
});
export const useProjectCreation = () => useContext(CreationContext);
let guestCreateIntent = false;

export function ProjectCreationProvider({ children }: { children: ReactNode }) {
  const session = useOptionalSession(),
    setSession = useSessionUpdate(),
    space = useSpace(),
    router = useRouter();
  const [auth, setAuth] = useState(false),
    [dismissed, setDismissed] = useState(false);
  const actorId = session?.actor.id,
    scopeId = space.selected?.id,
    writable = space.selected?.writable,
    pending = space.pending;
  const scopeKey = space.spaces.map((s) => s.id).join(",");
  const controller = useMemo(
    () =>
      new ProjectCreationController({
        intents: projectIntentStore,
        context: async () => {
          const fenceEpoch = await projectIntentStore.checkpoint();
          const scopeGeneration = getScopeGeneration();
          const current = await ensureSessionFresh();
          if (
            !actorId ||
            current.actor.id !== actorId ||
            !scopeId ||
            pending ||
            getCreationScope() !== scopeId ||
            getScopeGeneration() !== scopeGeneration
          )
            throw new Error("creation_context_changed");
          const context = {
            actorId,
            scopeId,
            sessionGeneration: getSessionGeneration(),
            scopeGeneration,
            fenceEpoch,
          };
          await projectIntentStore.activate(context);
          return context;
        },
        isCurrent: (context) =>
          context.actorId === actorId &&
          context.scopeId === scopeId &&
          getCreationScope() === scopeId &&
          context.sessionGeneration === getSessionGeneration() &&
          context.scopeGeneration === getScopeGeneration(),
        allowedScopes: () => (scopeKey ? scopeKey.split(",") : []),
        resolve: (key) => getProjectCreation({ key }),
        create: (body, key) => {
          if (!writable) throw new RequestError("forbidden", 403);
          return createProject(body, {
            headers: { "Idempotency-Key": key },
            sessionRecovery: "never",
          });
        },
        readTarget: async (target: IntentTarget) => {
          const [p, c] = await Promise.all([
            getProject({ pid: target.projectId }),
            getCanvas({ id: target.canvasId }),
          ]);
          const project = requireProject(p, true);
          if (
            project.id !== target.projectId ||
            project.canvas_id !== target.canvasId ||
            project.is_delete !== false ||
            !["active", "archived"].includes(project.status ?? "") ||
            c.id !== target.canvasId ||
            c.project_id !== target.projectId ||
            !Array.isArray(c.nodes) ||
            !Array.isArray(c.edges)
          )
            throw new RequestError("invalid_receipt");
        },
        navigate: (target) =>
          router.push(
            `/canvas?project_id=${target.projectId}&canvas_id=${target.canvasId}`,
          ),
      }),
    [actorId, scopeId, writable, pending, scopeKey, router],
  );
  const state = useSyncExternalStore(
    controller.subscribe,
    controller.snapshot,
    controller.snapshot,
  );
  useEffect(() => {
    if (!actorId || !scopeId || pending) return;
    let active = true;
    controller.resume();
    queueMicrotask(() => {
      if (active) {
        if (guestCreateIntent) {
          guestCreateIntent = false;
          void controller.start();
        } else void controller.restore();
      }
    });
    return () => {
      active = false;
      controller.dispose();
    };
  }, [actorId, scopeId, pending, controller]);
  async function start() {
    setDismissed(false);
    if (!session) {
      guestCreateIntent = true;
      setAuth(true);
      return;
    }
    await controller.start();
  }
  const disabled =
    (!!session && (!scopeId || pending || !writable)) ||
    [
      "checking_context",
      "preparing",
      "preflight",
      "submitting",
      "opening",
    ].includes(state.phase);
  const visible = !dismissed && !["idle", "ready"].includes(state.phase);
  const heading =
    state.phase === "created_storage_failed"
      ? "画布已创建，恢复信息保存失败"
      : state.phase === "ack_failed"
        ? "画布已创建，恢复信息待清理"
        : state.phase === "submitting"
          ? "正在创建画布"
          : state.phase === "preflight" || state.phase === "preflight_failed"
            ? "正在确认创建服务"
            : state.phase === "opening"
              ? "画布已创建，正在打开"
              : "target" in state
                ? "画布已创建，暂时无法完成打开"
                : state.phase === "unrecorded"
                  ? "尚未找到创建回执"
                  : state.phase === "rejected"
                    ? "本次创建已被拒绝"
                    : state.phase === "scope_required"
                      ? "选择待恢复的创作归属"
                      : "确认原创建结果";
  return (
    <CreationContext.Provider value={{ start, state, disabled }}>
      {children}
      <AuthDialog
        open={auth}
        onOpenChange={(open) => {
          setAuth(open);
          if (!open) guestCreateIntent = false;
        }}
        onAuthenticated={(next) => {
          setSession(next);
          setAuth(false);
        }}
      />
      <Dialog
        open={visible && !auth}
        onOpenChange={(open) => {
          if (!open) setDismissed(true);
        }}
      >
        <DialogContent>
          <DialogTitle>{heading}</DialogTitle>
          <DialogDescription>
            {state.phase === "created_storage_failed"
              ? "画布已创建，但浏览器未能保存恢复信息。请选择重新打开重试，不会再次创建。"
              : state.phase === "ack_failed"
                ? "画布读取已确认，但浏览器未能清理恢复信息。请选择重新打开重试，不会再次创建。"
                : "message" in state && state.message
                  ? state.message
                  : state.phase === "unrecorded"
                    ? "原请求仍可能在途。只有继续原创建才会再次提交同一请求，不会更换幂等键。"
                    : state.phase === "preflight_failed"
                      ? "尚未发送创建请求，请只读重试确认服务。"
                      : "保留原创建状态；已创建的画布仅重新打开，不另建副本。"}
          </DialogDescription>
          {space.selected && (
            <p className="text-sm text-muted-foreground">
              创作归属：{space.selected.name}
            </p>
          )}
          {state.phase === "scope_required" &&
            state.scopeIds.map((id) => {
              const candidate = space.spaces.find((s) => s.id === id);
              return candidate ? (
                <Button
                  key={id}
                  variant="outline"
                  onClick={() => void space.select(id)}
                >
                  恢复 {candidate.name}
                </Button>
              ) : null;
            })}
          <DialogFooter>
            <Button variant="ghost" asChild>
              <Link href="/projects" onClick={() => setDismissed(true)}>
                返回我的画布
              </Link>
            </Button>
            {["preflight_failed", "unknown", "blocked"].includes(
              state.phase,
            ) && (
              <Button onClick={() => void controller.resolve()}>
                确认创建结果
              </Button>
            )}
            {state.phase === "unrecorded" && (
              <Button
                disabled={!writable}
                onClick={() => void controller.continueOriginal()}
              >
                继续原创建
              </Button>
            )}
            {"target" in state &&
              state.phase !== "opening" &&
              state.phase !== "ready" && (
                <Button onClick={() => void controller.reopen()}>
                  重新打开
                </Button>
              )}
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </CreationContext.Provider>
  );
}
