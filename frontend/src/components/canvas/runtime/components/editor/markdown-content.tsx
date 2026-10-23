"use client";

import { MarkdownManager } from "@tiptap/markdown";
import { Extension } from "@tiptap/core";
import StarterKit from "@tiptap/starter-kit";
import { useMemo } from "react";
import { RichTextContent } from "./rich-text/rich-text-content";
import { documentText } from "./lib/rich-text-document";

const literalHtml = Extension.create({
  name: "glowloom-literal-html",
  markdownTokenName: "html",
  parseMarkdown: (token, helpers) => {
    const text = String(token.raw ?? token.text ?? "");
    if (!text) return [];
    const content = helpers.createTextNode(text.trimEnd());
    return token.block
      ? helpers.createNode("paragraph", undefined, [content])
      : content;
  },
});

export function MarkdownContent({
  value,
  className,
}: {
  value: string;
  className?: string;
}) {
  const content = useMemo(() => {
    const markdown = new MarkdownManager({
      extensions: [StarterKit, literalHtml],
    });
    return markdown.parse(value);
  }, [value]);
  return (
    <RichTextContent
      value={documentText(content)}
      document={{ schemaVersion: 1, content }}
      className={className}
    />
  );
}
