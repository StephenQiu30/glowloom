"use client";

import { useState, useEffect, useRef } from "react";
import type { ComposerSession } from "@/components/generation/composer-session";
import { PromptEditor } from "@/components/generation/prompt-editor";
import { emptyPromptDraft } from "@/components/generation/prompt-document";

/** Text resources share the editor engine; their persisted contract remains plain text. */
export function CanvasTextEditor({
  id,
  text,
  onChange,
}: {
  id: string;
  text: string;
  onChange: (text: string) => void;
}) {
  const [session, setSession] = useState<ComposerSession | null>(null);
  const syncing = useRef(false);
  const [draft] = useState(() => {
    const value = emptyPromptDraft();
    value.document.content = text.split("\n").map((line) => ({
      type: "paragraph",
      ...(line ? { content: [{ type: "text" as const, text: line }] } : {}),
    }));
    return value;
  });
  useEffect(() => {
    if (!session || session.editor.getText({ blockSeparator: "\n" }) === text)
      return;
    syncing.current = true;
    const next = emptyPromptDraft();
    next.document.content = text.split("\n").map((line) => ({
      type: "paragraph",
      ...(line ? { content: [{ type: "text" as const, text: line }] } : {}),
    }));
    session.adoptDraft(next);
    syncing.current = false;
  }, [session, text]);
  return (
    <PromptEditor
      onReady={setSession}
      label="文本内容"
      contextKey={`text/${id}`}
      replicaId={`text/${id}`}
      draft={draft}
      placeholder="输入文本内容"
      className="max-h-80 min-h-28 overflow-y-auto outline-none"
      onChange={(snapshot) => {
        if (syncing.current) return;
        const value = snapshot.draft.document.content
          .map((p) =>
            (p.content ?? [])
              .map((n) =>
                n.type === "text" ? n.text : n.type === "hardBreak" ? "\n" : "",
              )
              .join(""),
          )
          .join("\n");
        onChange(value.slice(0, 10000));
      }}
    />
  );
}
