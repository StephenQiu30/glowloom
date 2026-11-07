"use client";
import { useEffect, useEffectEvent, useRef, useState } from "react";
import {
  createPersonalReferenceCopy,
  executePersonalReferenceCopy,
  type PersonalReferenceCopy,
} from "@/components/media/copy-personal-references";
import {
  studioReferenceItems,
  validateStudioInputReferences,
  type StudioInput,
} from "./studio-input";
import { mediaError } from "@/components/media/library";

export function useStudioReferenceCopy(
  input: StudioInput | undefined,
  projectID: string,
  owner: string,
  enabled: boolean,
  onCopied: (assets: API.AssetSummary[]) => void,
) {
  const copied = useEffectEvent(onCopied);
  const [snapshot] = useState(() => {
    const error = validateStudioInputReferences(input);
    return { error, items: error ? [] : studioReferenceItems(input) };
  });
  const { items } = snapshot;
  const [assets, setAssets] = useState<API.AssetSummary[] | null>(
    items.length || snapshot.error ? null : [],
  );
  const [error, setError] = useState("");
  const [attempt, setAttempt] = useState(0);
  const command = useRef<PersonalReferenceCopy | null>(null);
  useEffect(() => {
    if (!items.length || !enabled || assets) return;
    const c = new AbortController();
    void (async () => {
      try {
        command.current ??= await createPersonalReferenceCopy(
          projectID,
          items,
          owner.split("/")[0]!,
          c.signal,
        );
        const result = await executePersonalReferenceCopy(
          command.current,
          c.signal,
        );
        if (!c.signal.aborted) {
          copied(result);
          setAssets(result);
          setError("");
        }
      } catch (e) {
        if (!c.signal.aborted)
          setError(
            e instanceof Error && !("status" in e) ? e.message : mediaError(e),
          );
      }
    })();
    return () => c.abort();
  }, [enabled, projectID, owner, items, attempt, assets]);
  return {
    assets,
    error: snapshot.error || error,
    pending: !assets && !snapshot.error && !error,
    retry: () => {
      setError("");
      setAttempt((n) => n + 1);
    },
  };
}
