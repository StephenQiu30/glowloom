"use client";
import type { CSSProperties } from "react";
import { RichTextInput } from "@/components/canvas/runtime/components/editor/rich-text/rich-text-input";
import type { CanvasResourceReference } from "../lib/canvas-resource-references";
export function CanvasPromptChipInput({
  value,
  references,
  onChange,
  onSubmit,
  className,
  style,
  placeholder,
}: {
  value: string;
  references: CanvasResourceReference[];
  onChange: (value: string) => void;
  onSubmit?: () => void;
  className?: string;
  style?: CSSProperties;
  placeholder?: string;
}) {
  return (
    <RichTextInput
      value={value}
      references={references.filter((r) => r.active)}
      onChange={onChange}
      onSubmit={onSubmit}
      submitShortcut="mod-enter"
      aria-label="节点提示词"
      className={className}
      style={style}
      placeholder={placeholder}
    />
  );
}
