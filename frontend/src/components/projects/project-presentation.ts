"use client";

import {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
  useSyncExternalStore,
} from "react";
import { useOptionalSession } from "@/components/auth/session";
import { useSpace } from "@/components/spaces/space-context";
import {
  getSessionGeneration,
  getScopeGeneration,
  RequestError,
} from "@/lib/request";
import type { Project } from "./project";
import {
  isProjectProofCurrent,
  projectNamespace,
  projectPresentationStore,
  refreshProjectPresentation,
  subscribeProjectInvalidation,
} from "./project-presentation-store";

export function useProjectPresentation(
  initial: Project,
  refreshOnFocus = true,
) {
  const session = useOptionalSession(),
    space = useSpace();
  const sessionGeneration = getSessionGeneration(),
    scopeGeneration = getScopeGeneration();
  const proof = useMemo(
    () =>
      session && space.selected
        ? {
            actorId: session.actor.id,
            scopeId: space.selected.id,
            sessionGeneration,
            scopeGeneration,
          }
        : null,
    [session, space.selected, sessionGeneration, scopeGeneration],
  );
  const namespace = proof ? projectNamespace(proof) : null;
  const [error, setError] = useState("");
  const alive = useRef(false);
  const cached = useSyncExternalStore(
    projectPresentationStore.subscribe,
    useCallback(
      () =>
        namespace ? projectPresentationStore.read(namespace, initial.id) : null,
      [namespace, initial.id],
    ),
    () => null,
  );
  useEffect(() => {
    alive.current = true;
    return () => {
      alive.current = false;
    };
  }, []);
  useEffect(() => {
    if (namespace && proof && isProjectProofCurrent(proof))
      projectPresentationStore.accept(namespace, initial);
  }, [namespace, proof, initial]);
  const refresh = useCallback(async () => {
    if (!proof) throw new RequestError("session_invalid", 401);
    try {
      const project = await refreshProjectPresentation(proof, initial.id);
      if (alive.current) setError("");
      return project;
    } catch (reason) {
      if (alive.current && isProjectProofCurrent(proof))
        setError("暂时无法确认最新项目资料，请重新读取；当前仅查看。");
      throw reason;
    }
  }, [proof, initial.id]);
  useEffect(() => {
    if (!refreshOnFocus || !proof) return;
    const reread = () => {
      if (document.visibilityState !== "hidden") void refresh().catch(() => {});
    };
    window.addEventListener("focus", reread);
    document.addEventListener("visibilitychange", reread);
    const unsubscribe = subscribeProjectInvalidation((id) => {
      if (id === initial.id) reread();
    });
    return () => {
      window.removeEventListener("focus", reread);
      document.removeEventListener("visibilitychange", reread);
      unsubscribe();
    };
  }, [refreshOnFocus, proof, refresh, initial.id]);
  return { project: cached ?? initial, refresh, error, proof, namespace };
}
