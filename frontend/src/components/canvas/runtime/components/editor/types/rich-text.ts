import type { JSONContent } from "@tiptap/core";
import type { CSSProperties, Ref } from "react";
import type { NodeTextBinding } from "@glowloom/canvas-collab/model";

export type RichTextDocument = {
  schemaVersion: 1;
  content: JSONContent;
};

export type RichTextReference = {
  id: string;
  label: string;
  title: string;
  kind: "text" | "image" | "video" | "audio" | "group";
  token?: string;
  previewUrl?: string;
  text?: string;
};

export type RichTextInputProps = {
  collaboration?: NodeTextBinding;
  id?: string;
  value: string;
  document?: RichTextDocument;
  onChange: (value: string, document: RichTextDocument) => void;
  acceptDocument?: (document: RichTextDocument) => boolean;
  references?: RichTextReference[];
  documentMode?: boolean;
  floatingToolbar?: boolean;
  disabled?: boolean;
  required?: boolean;
  maxLength?: number;
  placeholder?: string;
  "aria-label"?: string;
  "aria-invalid"?: boolean;
  className?: string;
  style?: CSSProperties;
  rootRef?: Ref<HTMLDivElement>;
  autoFocus?: boolean;
  onSubmit?: () => void;
  submitShortcut?: "enter" | "mod-enter";
  onEscape?: () => void;
};
