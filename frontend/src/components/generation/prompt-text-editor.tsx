"use client";

import { useEffect, useRef, useState } from "react";
import { PromptEditor } from "./prompt-editor";
import type { ComposerSession } from "./composer-session";
import { emptyPromptDraft } from "./prompt-document";

function textDraft(text: string) {
  const draft = emptyPromptDraft();
  draft.document.content = text.split("\n").map((line) => ({
    type: "paragraph",
    ...(line ? { content: [{ type: "text" as const, text: line }] } : {}),
  }));
  return draft;
}

/** Share the rich editor engine with fields whose API contract remains plain text. */
export function PromptTextEditor({
  id,
  text,
  onChange,
  onReady,
  onRequestSubmit,
  label,
  placeholder,
  className,
}: {
  id: string;
  text: string;
  onChange: (text: string) => void;
  onReady?: (session: ComposerSession) => void;
  onRequestSubmit?: () => void;
  label: string;
  placeholder?: string;
  className?: string;
}) {
  const [session, setSession] = useState<ComposerSession | null>(null);
  const syncing = useRef(false);
  const lastText = useRef(text);
  const [draft] = useState(() => textDraft(text));
  useEffect(() => {
    if (!session || session.editor.getText({ blockSeparator: "\n" }) === text)
      return;
    syncing.current = true;
    lastText.current = text;
    session.adoptDraft(textDraft(text));
    syncing.current = false;
  }, [session, text]);
  return (
    <div className="w-full min-w-0">
      <PromptEditor
        contextKey={id}
        replicaId={id}
        draft={draft}
        label={label}
        placeholder={placeholder}
        className={className}
        inputGroup
        onReady={(value) => {
          setSession(value);
          onReady?.(value);
        }}
        onRequestSubmit={onRequestSubmit}
        onChange={(snapshot) => {
          if (syncing.current) return;
          const value = snapshot.draft.document.content
            .map((p) =>
              (p.content ?? [])
                .map((n) =>
                  n.type === "text"
                    ? n.text
                    : n.type === "hardBreak"
                      ? "\n"
                      : "",
                )
                .join(""),
            )
            .join("\n");
          const bounded = value.slice(0, 10000);
          if (bounded === lastText.current) return;
          lastText.current = bounded;
          onChange(bounded);
        }}
      />
    </div>
  );
}
