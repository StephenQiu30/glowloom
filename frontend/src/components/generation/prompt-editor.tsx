"use client";

import { useEffect } from "react";
import { EditorContent } from "@tiptap/react";
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
  label = "创作提示词",
  ...options
}: ComposerOptions & {
  onReady: (session: ComposerSession) => void;
  onChange?: (snapshot: ComposerSnapshot) => void;
  onDispose?: () => void;
  onRequestSave?: (snapshot: ComposerSnapshot) => void;
  onRequestSubmit?: () => void;
  placeholder?: string;
  className?: string;
  inputGroup?: boolean;
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
      ...(className === undefined ? {} : { class: className }),
      ...(inputGroup ? { "data-slot": "input-group-control" } : {}),
    },
  );
  useEffect(() => {
    if (!editor) return;
    const changed = () => onChange?.(new ComposerSession(editor).snapshot());
    editor.on("transaction", changed);
    if (onRequestSave) editor.on("composerSaveRequested", onRequestSave);
    if (onRequestSubmit) editor.on("composerSubmitRequested", onRequestSubmit);
    return () => {
      editor.off("transaction", changed);
      if (onRequestSave) editor.off("composerSaveRequested", onRequestSave);
      if (onRequestSubmit)
        editor.off("composerSubmitRequested", onRequestSubmit);
    };
  }, [editor, onRequestSave, onRequestSubmit, onChange]);

  return <EditorContent editor={editor} />;
}
