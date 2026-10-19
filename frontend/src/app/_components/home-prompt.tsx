"use client";

import { useEffect, useRef, useState } from "react";
import { PromptEditor } from "@/components/generation/prompt-editor";
import type { ComposerSession } from "@/components/generation/composer-session";
import {
  emptyPromptDraft,
  type PromptDraft,
} from "@/components/generation/prompt-document";

function textDraft(text: string) {
  const draft = emptyPromptDraft();
  draft.document.content = text.split("\n").map((line) => ({
    type: "paragraph",
    ...(line ? { content: [{ type: "text" as const, text: line }] } : {}),
  }));
  return draft;
}
function draftText(draft: PromptDraft) {
  return draft.document.content
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
}

/** Before a project is chosen, only text is retained; private references stay project-owned. */
export function HomePrompt({
  value,
  onChange,
  placeholder,
  onSubmit,
}: {
  value: string;
  onChange: (value: string) => void;
  placeholder: string;
  onSubmit: () => void;
}) {
  const [replicaId] = useState(() => crypto.randomUUID());
  const [initial] = useState(() => textDraft(value));
  const [session, setSession] = useState<ComposerSession | null>(null);
  const latestText = useRef(value);
  useEffect(() => {
    if (
      !session ||
      session.editor.isDestroyed ||
      draftText(session.snapshot().draft) === value
    )
      return;
    latestText.current = value;
    session.adoptDraft(textDraft(value));
  }, [session, value]);
  return (
    <div className="home-prompt">
      <PromptEditor
        contextKey="home-input"
        replicaId={replicaId}
        draft={initial}
        placeholder={placeholder}
        className="max-h-80 min-h-24 overflow-y-auto outline-none"
        inputGroup
        onReady={setSession}
        onChange={(snapshot) => {
          const text = draftText(snapshot.draft);
          if (text !== latestText.current) {
            latestText.current = text;
            onChange(text);
          }
        }}
        onRequestSubmit={onSubmit}
      />
    </div>
  );
}
