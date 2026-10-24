"use client";

import type { JSONContent } from "@tiptap/core";
import { Fragment, type ReactNode } from "react";
import { MediaPreview } from "@/components/canvas/runtime/components/generation/workspace/media-preview";
import { documentForText, safeContentLink } from "../lib/rich-text-document";
import type { RichTextDocument, RichTextReference } from "../types/rich-text";
import { cn } from "@/lib/utils";

/** Inactive canvas nodes render JSON without mounting an editor instance. */
export function RichTextContent({
  value,
  document,
  references = [],
  className,
}: {
  value: string;
  document?: RichTextDocument;
  references?: RichTextReference[];
  className?: string;
}) {
  const root = documentForText(value, document, references);
  const byId = new Map(references.map((r) => [r.id, r]));
  function render(node: JSONContent, key: string): ReactNode {
    if (node.type === "text") {
      let text: ReactNode = node.text ?? "";
      for (const mark of node.marks ?? []) {
        if (mark.type === "bold") text = <strong>{text}</strong>;
        if (mark.type === "italic") text = <em>{text}</em>;
        if (mark.type === "underline") text = <u>{text}</u>;
        if (mark.type === "strike") text = <s>{text}</s>;
        if (mark.type === "code") text = <code>{text}</code>;
        if (mark.type === "link") {
          const href = safeContentLink(mark.attrs?.href);
          if (href)
            text = (
              <a href={href} target="_blank" rel="noopener noreferrer">
                {text}
              </a>
            );
        }
      }
      return <Fragment key={key}>{text}</Fragment>;
    }
    if (node.type === "mention" || node.type === "reference") {
      const reference = byId.get(
        String(node.attrs?.id ?? node.attrs?.referenceId),
      );
      return reference?.kind === "image" && reference.previewUrl ? (
        <span key={key} className="rich-text-reference-image">
          <MediaPreview
            src={reference.previewUrl}
            alt={reference.title}
            className="max-h-40 max-w-full object-contain"
          />
        </span>
      ) : (
        <span key={key} className="rich-text-reference-label">
          {reference?.label ??
            String(node.attrs?.label ?? node.attrs?.token ?? "素材引用")}
        </span>
      );
    }
    const children = node.content?.map((child, index) =>
      render(child, `${key}-${index}`),
    );
    switch (node.type) {
      case "paragraph":
        return <p key={key}>{children?.length ? children : <br />}</p>;
      case "heading":
        return node.attrs?.level === 1 ? (
          <h1 key={key}>{children}</h1>
        ) : node.attrs?.level === 3 ? (
          <h3 key={key}>{children}</h3>
        ) : (
          <h2 key={key}>{children}</h2>
        );
      case "bulletList":
        return <ul key={key}>{children}</ul>;
      case "orderedList":
        return (
          <ol
            key={key}
            start={
              Number.isSafeInteger(node.attrs?.start)
                ? node.attrs?.start
                : undefined
            }
          >
            {children}
          </ol>
        );
      case "listItem":
        return <li key={key}>{children}</li>;
      case "blockquote":
        return <blockquote key={key}>{children}</blockquote>;
      case "codeBlock":
        return (
          <pre key={key}>
            <code>{children}</code>
          </pre>
        );
      case "hardBreak":
        return <br key={key} />;
      case "horizontalRule":
        return <p key={key} className="rich-text-rule" aria-hidden="true" />;
      default:
        return <Fragment key={key}>{children}</Fragment>;
    }
  }
  return (
    <div className={cn("rich-text-readonly", className)}>
      {render(root, "doc")}
    </div>
  );
}
