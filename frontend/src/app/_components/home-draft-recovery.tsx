"use client";

import { useEffect, useRef, useState } from "react";
import { Button } from "@/components/ui/button";
import {
  branchJournal,
  type LocalBranch,
} from "@/components/generation/draft-save";
import {
  emptyPromptDraft,
  type PromptDraft,
} from "@/components/generation/prompt-document";
import { getCreationScope } from "@/lib/request";

/** Page-lifetime recovery keeps text only; references need fresh authorization. */
export function recoverableHomeText(draft: PromptDraft): PromptDraft {
  const result = emptyPromptDraft();
  result.document.content = draft.document.content.map((paragraph) => ({
    type: "paragraph",
    content: (paragraph.content ?? []).filter(
      (node) => node.type !== "reference",
    ),
  }));
  return result;
}
function hasText(branch: LocalBranch) {
  return branch.snapshot.draft.document.content.some((p) =>
    p.content?.some((n) => n.type === "text" && n.text.trim()),
  );
}

export function useHomeDraftRecovery(
  contextKey: string | null,
  onChange: (draft: PromptDraft) => void,
) {
  const [replicaId] = useState(() => crypto.randomUUID());
  const revision = useRef(0);
  const edited = useRef(new Set<string>());
  const owner = useRef<object | null>(null);
  const [recovery, setRecovery] = useState<{
    key: string;
    branch: LocalBranch;
  } | null>(null);
  const [unavailable, setUnavailable] = useState<string | null>(null);
  useEffect(() => {
    const token = {};
    owner.current = token;
    if (contextKey) {
      const keys = [contextKey];
      // A legacy shared entry must never appear under several creation types.
      if (contextKey.endsWith("/home-input:image"))
        keys.push(contextKey.replace(/:image$/, ""));
      void Promise.all(keys.map((key) => branchJournal.list(key)))
        .then((groups) => {
          if (owner.current !== token || edited.current.has(contextKey)) return;
          const candidate = groups
            .flat()
            .filter((b) => b.replicaId !== replicaId && hasText(b))[0];
          setRecovery(
            candidate ? { key: contextKey, branch: candidate } : null,
          );
        })
        .catch(() => {
          if (owner.current === token) setUnavailable(contextKey);
        });
    }
    return () => {
      owner.current = null;
    };
  }, [contextKey, replicaId]);

  async function persist(draft: PromptDraft) {
    if (!contextKey || getCreationScope() !== contextKey.split("/")[1])
      return false;
    const localSeq = ++revision.current;
    const token = owner.current;
    try {
      await branchJournal.write({
        inputRecovery: true,
        inputScope: "space",
        contextKey,
        replicaId,
        localRecordRevision: localSeq,
        serverRevision: 0,
        draftId: contextKey.split("/")[2],
        ackedSeq: -1,
        snapshot: {
          contextKey,
          replicaId,
          localSeq,
          draft: recoverableHomeText(draft),
        },
      });
      return true;
    } catch {
      if (owner.current === token) setUnavailable(contextKey);
      return false;
    }
  }
  function change(draft: PromptDraft) {
    if (contextKey) edited.current.add(contextKey);
    setRecovery(null);
    onChange(draft);
    void persist(draft);
  }
  async function restore() {
    if (!contextKey || recovery?.key !== contextKey) return;
    const token = owner.current;
    const currentRevision = revision.current;
    try {
      const branches = await branchJournal.list(recovery.branch.contextKey);
      if (
        !token ||
        owner.current !== token ||
        revision.current !== currentRevision ||
        edited.current.has(contextKey)
      )
        return;
      const saved = branches.find(
        (b) => b.replicaId === recovery.branch.replicaId,
      );
      if (!saved) {
        setRecovery(null);
        return;
      }
      const draft = recoverableHomeText(saved.snapshot.draft);
      onChange(draft);
      edited.current.add(contextKey);
      setRecovery(null);
      // Preserve the original recovery branch if writing its replacement fails.
      if ((await persist(draft)) && owner.current === token)
        await branchJournal.remove(saved);
    } catch {
      if (owner.current === token) setUnavailable(contextKey);
    }
  }
  return {
    change,
    notice: (
      <>
        {contextKey && recovery?.key === contextKey ? (
          <div className="flex flex-wrap items-center gap-2 text-sm text-muted-foreground">
            <span>此类型的上次文字可恢复，参考素材与生成设置需重新选择。</span>
            <Button
              type="button"
              variant="outline"
              onClick={() => void restore()}
            >
              恢复上次文字
            </Button>
          </div>
        ) : null}
        {contextKey && unavailable === contextKey ? (
          <p className="text-sm text-muted-foreground">
            文字恢复暂不可用，当前输入仍可编辑。
          </p>
        ) : null}
      </>
    ),
  };
}
