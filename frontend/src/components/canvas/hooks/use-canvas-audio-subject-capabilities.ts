"use client";
import {
  useLayoutEffect,
  useMemo,
  useState,
  useSyncExternalStore,
} from "react";
import { getSubjectVersion } from "@/api/subjects";
import {
  getCreationScope,
  getScopeGeneration,
  getSessionGeneration,
  subscribeContextChange,
} from "@/lib/request";
import type { UnifiedSubject } from "@/components/subjects/unified-contracts";
import type { CanvasNodeData } from "../types/canvas";
import type { CanvasAudioCatalogIdentity } from "../lib/canvas-audio-connection-capability";
import { requireCanvasSubjectVersion } from "../lib/canvas-subject-source";
import { subjectConfig } from "../subject/subject-node-state";

const empty = new Map<string, UnifiedSubject>();
const contextGeneration = () =>
  `${getSessionGeneration()}/${getScopeGeneration()}`;
type Pin = { subject_id: string; version_id: string };
const pinKey = (pin: Pin) => `${pin.subject_id}/${pin.version_id}`;

function createSubjectCapabilityRuntime(
  projectID: string,
  scopeID: string,
  generation: string,
  enabled: boolean,
  onChange: (values: ReadonlyMap<string, UnifiedSubject>) => void,
) {
  let active = true;
  let selection = new Map<string, Pin>();
  const cache = new Map<string, UnifiedSubject>();
  const pending = new Map<string, AbortController>();
  let published: ReadonlyMap<string, UnifiedSubject> = empty;
  const publish = () => {
    const next = new Map<string, UnifiedSubject>();
    for (const [nodeID, pin] of selection) {
      const value = cache.get(pinKey(pin));
      if (value) next.set(nodeID, value);
    }
    if (
      next.size === published.size &&
      [...next].every(([id, value]) => published.get(id) === value)
    )
      return;
    published = next;
    onChange(next);
  };
  return {
    start() {
      active = true;
    },
    dispose() {
      active = false;
      for (const request of pending.values()) request.abort();
      pending.clear();
      cache.clear();
      selection.clear();
      published = empty;
    },
    prepare(sources: readonly CanvasNodeData[]) {
      if (
        !active ||
        !enabled ||
        !projectID ||
        !scopeID ||
        getCreationScope() !== scopeID ||
        contextGeneration() !== generation
      )
        return;
      const next = new Map<string, Pin>();
      for (const source of sources) {
        if (source.type !== "subject") continue;
        try {
          const config = subjectConfig(source);
          if (!config.local && config.subject_id && config.version_id)
            next.set(source.id, {
              subject_id: config.subject_id,
              version_id: config.version_id,
            });
        } catch {
          // Malformed persisted subjects stay unavailable without breaking selection.
        }
      }
      selection = next;
      const pins = new Map([...next.values()].map((pin) => [pinKey(pin), pin]));
      for (const [id, request] of pending)
        if (!pins.has(id)) {
          request.abort();
          pending.delete(id);
        }
      for (const id of cache.keys()) if (!pins.has(id)) cache.delete(id);
      publish();
      for (const [id, pin] of pins) {
        if (cache.has(id) || pending.has(id)) continue;
        const controller = new AbortController();
        pending.set(id, controller);
        void getSubjectVersion(
          { id: pin.subject_id, version: pin.version_id },
          { signal: controller.signal },
        )
          .then((value) => {
            if (
              !active ||
              controller.signal.aborted ||
              pending.get(id) !== controller ||
              contextGeneration() !== generation ||
              getCreationScope() !== scopeID
            )
              return;
            cache.set(id, requireCanvasSubjectVersion(value, scopeID, pin));
            publish();
          })
          .catch(() => {
            // Unknown/denied pins remain unavailable; a later selection can retry.
          })
          .finally(() => {
            if (pending.get(id) === controller) pending.delete(id);
          });
      }
    },
  };
}

/** Resolve only selected pinned subjects; physical media never come from a guessed cover kind. */
export function useCanvasAudioSubjectCapabilities(
  identity: CanvasAudioCatalogIdentity,
  enabled = true,
) {
  const { projectID, scopeID } = identity;
  const generation = useSyncExternalStore(
    subscribeContextChange,
    contextGeneration,
    () => "server",
  );
  const key = JSON.stringify([projectID, scopeID, generation, enabled]);
  const [snapshot, setSnapshot] = useState<{
    key: string;
    values: ReadonlyMap<string, UnifiedSubject>;
  }>({ key: "", values: empty });
  const runtime = useMemo(
    () =>
      createSubjectCapabilityRuntime(
        projectID,
        scopeID,
        generation,
        enabled,
        (values) => setSnapshot({ key, values }),
      ),
    [enabled, generation, key, projectID, scopeID],
  );
  useLayoutEffect(() => {
    runtime.start();
    return () => runtime.dispose();
  }, [runtime]);
  return {
    prepare: runtime.prepare,
    pinnedSubjects: snapshot.key === key ? snapshot.values : empty,
  };
}
