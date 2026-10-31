"use client";

import type { Transaction } from "@tiptap/pm/state";

import { useEffect } from "react";
import { EditorContent } from "@tiptap/react";
import { cn } from "cn";
import { useEditorSurface } from "./editor-surface";
import {
  ComposerSession,
  composerEditorOptions,
  type ComposerOptions,
  type ComposerSnapshot,
} from "./composer-session";

/** The owner supplies one stable replica per mounted editing instance, never per mode.
 * All model changes are transactions within that owner.
 */
export function PromptEditor({
  onReady,
  onDispose,
  onRequestSave,
  onRequestSubmit,
  onChange,
  placeholder,
  className,
  inputGroup = false,
  editable = true,
  label = "创作提示词",
  ...options
}: ComposerOptions & {
  onReady: (session: ComposerSession) => void;
  onChange?: (snapshot: ComposerSnapshot, transaction?: Transaction) => void;
  onDispose?: () => void;
  onRequestSave?: (snapshot: ComposerSnapshot) => void;
  onRequestSubmit?: () => void;
  placeholder?: string;
  className?: string;
  inputGroup?: boolean;
  editable?: boolean;
  label?: string;
}) {
  const editorOptions = composerEditorOptions(options);
  const editor = useEditorSurface(
    {
      ...editorOptions,
      onCreate: ({ editor }) => onReady(new ComposerSession(editor)),
      onDestroy: () => onDispose?.(),
    },
    [options.contextKey, options.replicaId],
    {
      "aria-label": label,
      ...(placeholder === undefined ? {} : { "data-placeholder": placeholder }),
      ...(className === undefined
        ? {}
        : { class: cn("prompt-editor", className) }),
      ...(inputGroup ? { "data-slot": "input-group-control" } : {}),
    },
    editable,
  );
  useEffect(() => {
    if (!editor) return;
    const changed = ({ transaction }: { transaction: Transaction }) => {
      const snapshot = new ComposerSession(editor).snapshot();
      if (options.collaboration) onChange?.(snapshot, transaction);
      else onChange?.(snapshot);
    };
    editor.on("transaction", changed);
    if (onRequestSave) editor.on("composerSaveRequested", onRequestSave);
    if (onRequestSubmit) editor.on("composerSubmitRequested", onRequestSubmit);
    return () => {
      editor.off("transaction", changed);
      if (onRequestSave) editor.off("composerSaveRequested", onRequestSave);
      if (onRequestSubmit)
        editor.off("composerSubmitRequested", onRequestSubmit);
    };
  }, [editor, onRequestSave, onRequestSubmit, onChange, options.collaboration]);

  return <EditorContent editor={editor} className="w-full min-w-0 flex-1" />;
}
