"use client";
import { useState, useRef, useEffect, type SetStateAction } from "react";
import { PageLayout } from "@/components/layout/page-layout";
import { useSpace } from "@/components/spaces/space-context";
import {
  branchJournal,
  type LocalBranch,
} from "@/components/generation/draft-save";
import { emptyPromptDraft } from "@/components/generation/prompt-document";
import { getCreationScope } from "@/lib/request";
import { useOptionalSession } from "@/components/auth/session";
import { Button } from "@/components/ui/button";
import { InspirationGallery } from "@/components/inspiration/gallery";
import { HomeCreation } from "./home-creation";

function HomeContent({ searchQuery }: { searchQuery: string }) {
  const session = useOptionalSession();
  const [prompt, setPromptState] = useState("");
  const { selected: space } = useSpace();
  const contextKey =
    session && space?.writable
      ? `${session.actor.id}/${space.id}/home-input`
      : null;
  const [replicaId] = useState(() => crypto.randomUUID());
  const inputRevision = useRef(0);
  const recoveryOwner = useRef<object | null>(null);
  const [recovery, setRecovery] = useState<LocalBranch | null>(null);
  const [recoveryUnavailable, setRecoveryUnavailable] = useState(false);
  const recoveryText = (branch: LocalBranch) =>
    branch.snapshot.draft.document.content
      .map((paragraph) =>
        (paragraph.content ?? [])
          .map((node) =>
            node.type === "text"
              ? node.text
              : node.type === "hardBreak"
                ? "\n"
                : "",
          )
          .join(""),
      )
      .join("\n");
  useEffect(() => {
    const owner = {};
    recoveryOwner.current = owner;
    if (contextKey)
      void branchJournal
        .list(contextKey)
        .then((branches) => {
          if (recoveryOwner.current === owner)
            setRecovery(
              branches.find(
                (branch) =>
                  branch.replicaId !== replicaId && recoveryText(branch),
              ) ?? null,
            );
        })
        .catch(() => {
          if (recoveryOwner.current === owner) setRecoveryUnavailable(true);
        });
    return () => {
      recoveryOwner.current = null;
    };
  }, [contextKey, replicaId]);
  function setPrompt(value: SetStateAction<string>) {
    const text = typeof value === "function" ? value(prompt) : value;
    setPromptState(text);
    if (!contextKey || !space || getCreationScope() !== space.id) return;
    const draft = emptyPromptDraft();
    draft.document.content[0].content = text ? [{ type: "text", text }] : [];
    const revision = ++inputRevision.current;
    void branchJournal
      .write({
        inputRecovery: true,
        inputScope: "space",
        contextKey,
        replicaId,
        localRecordRevision: revision,
        serverRevision: 0,
        draftId: "home-input",
        ackedSeq: -1,
        snapshot: { contextKey, replicaId, localSeq: revision, draft },
      })
      .catch(() => {
        if (recoveryOwner.current) setRecoveryUnavailable(true);
      });
  }
  async function restorePrompt() {
    if (!contextKey || !recovery) return;
    const owner = recoveryOwner.current;
    try {
      const branches = await branchJournal.list(contextKey);
      if (!owner || recoveryOwner.current !== owner) return;
      const saved = branches.find(
        (branch) => branch.replicaId === recovery.replicaId,
      );
      if (!saved) {
        setRecovery(null);
        return;
      }
      setPrompt(recoveryText(saved));
      setRecovery(null);
      await branchJournal.remove(saved);
    } catch {
      if (recoveryOwner.current === owner) setRecoveryUnavailable(true);
    }
  }
  return (
    <div className="home-page">
      <HomeCreation
        key={contextKey ?? "guest"}
        prompt={prompt}
        onPromptChange={setPrompt}
        recovery={
          <>
            {recovery?.contextKey === contextKey && recovery ? (
              <div className="flex items-center gap-2 text-sm text-muted-foreground">
                <span>上次文字可恢复，参考素材与生成设置需重新选择。</span>
                <Button variant="outline" onClick={() => void restorePrompt()}>
                  恢复上次文字
                </Button>
              </div>
            ) : null}
            {recoveryUnavailable ? (
              <p className="text-sm text-muted-foreground">
                文字恢复暂不可用，当前输入仍可编辑。
              </p>
            ) : null}
          </>
        }
      />
      <InspirationGallery searchQuery={searchQuery} />
    </div>
  );
}
export function HomePage() {
  const [search, setSearch] = useState("");
  const [query, setQuery] = useState("");
  return (
    <PageLayout
      screen="home"
      fullBleed
      search={{
        value: search,
        onValueChange: setSearch,
        onSearch: setQuery,
        label: "搜索灵感",
      }}
    >
      <HomeContent searchQuery={query} />
    </PageLayout>
  );
}
