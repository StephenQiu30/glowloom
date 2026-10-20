"use client";

import { useEffect, useRef, type DependencyList } from "react";
import { useEditor } from "@tiptap/react";
import type { EditorOptions } from "@tiptap/core";

/** Shared Tiptap lifecycle for prompt and document schemas. */
export function useEditorSurface(
  options: Partial<EditorOptions>,
  dependencies: DependencyList,
  attributes: Record<string, string>,
  editable = true,
) {
  const initial = useRef(options.editorProps?.attributes);
  const editor = useEditor(
    {
      ...options,
      editable,
      immediatelyRender: false,
      shouldRerenderOnTransaction: false,
      editorProps: {
        ...options.editorProps,
        attributes: (state) => ({
          ...(typeof initial.current === "function"
            ? initial.current(state)
            : initial.current),
          ...attributes,
        }),
      },
    },
    dependencies,
  );
  useEffect(() => {
    if (!editor) return;
    editor.setEditable(editable, false);
    editor.setOptions({
      editorProps: {
        ...editor.options.editorProps,
        attributes: (state) => ({
          ...(typeof initial.current === "function"
            ? initial.current(state)
            : initial.current),
          ...attributes,
        }),
      },
    });
  }, [editor, editable, attributes]);
  return editor;
}
