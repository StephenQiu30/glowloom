"use client";

import { useEffect, useRef, useState } from "react";
import { PromptEditor } from "@/components/generation/prompt-editor";
import { canonical } from "@/components/generation/draft-save";
import type { ComposerSession } from "@/components/generation/composer-session";
import { type PromptDraft } from "@/components/generation/prompt-document";

/** The page can retain space-owned subjects before choosing a generation project. */
export function HomePrompt({
  draft,
  contextKey,
  onChange,
  placeholder,
  onSubmit,
  label = "创作提示词",
  onReady,
}: {
  label?: string;
  draft: PromptDraft;
  contextKey: string;
  onChange: (value: PromptDraft) => void;
  placeholder: string;
  onSubmit: () => void;
  onReady?: (session: ComposerSession) => void;
}) {
  const [replicaId] = useState(() => crypto.randomUUID());
  const [initial] = useState(() => structuredClone(draft));
  const lastDraft = useRef(canonical(initial));
  const [session, setSession] = useState<ComposerSession | null>(null);
  useEffect(() => {
    if (
      !session ||
      session.editor.isDestroyed ||
      canonical(session.snapshot().draft) === canonical(draft)
    )
      return;
    lastDraft.current = canonical(draft);
    session.adoptDraft(draft);
  }, [session, draft]);
  return (
    <div className="home-prompt">
      <PromptEditor
        label={label}
        contextKey={contextKey}
        replicaId={replicaId}
        draft={initial}
        placeholder={placeholder}
        className="home-prompt-editor overflow-y-auto outline-none"
        inputGroup
        onReady={(value) => {
          setSession(value);
          onReady?.(value);
        }}
        onChange={(snapshot) => {
          const serialized = canonical(snapshot.draft);
          if (lastDraft.current === serialized) return;
          lastDraft.current = serialized;
          onChange(snapshot.draft);
        }}
        onRequestSubmit={onSubmit}
      />
    </div>
  );
}
