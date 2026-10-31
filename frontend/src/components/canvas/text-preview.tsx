import { ImageIcon, Video, AudioLines, UserRound } from "lucide-react";
import { referenceLabel } from "@/components/generation/prompt-document";
import type { CanvasNodeData } from "./types";

/** Read-only projection of the same document; reference labels are never stored as text. */
export function CanvasTextPreview({ node }: { node: CanvasNodeData }) {
  const draft = node.metadata?.composer;
  if (!draft)
    return node.metadata?.content ? (
      <>{node.metadata.content}</>
    ) : (
      <span className="text-muted-foreground">双击编辑文本</span>
    );
  return (
    <>
      {draft.document.content.map((paragraph, index) => (
        <p key={index} className="min-h-5">
          {paragraph.content?.map((item, i) => {
            if (item.type === "text") return item.text;
            if (item.type === "hardBreak") return <br key={i} />;
            const ref = draft.references.find(
              (r) => r.id === item.attrs.referenceId,
            );
            const Icon =
              ref?.source.kind === "subject" ||
              ref?.source.kind === "canvas_subject"
                ? UserRound
                : ref?.mediaKind === "video"
                  ? Video
                  : ref?.mediaKind === "audio"
                    ? AudioLines
                    : ImageIcon;
            return (
              <span
                key={i}
                className="mx-0.5 inline-flex h-6 items-center gap-1 rounded-md border bg-muted px-1.5 align-middle text-xs"
                data-prompt-reference=""
              >
                <Icon className="size-3" />
                {referenceLabel(draft.labelState, item.attrs.referenceId)}
              </span>
            );
          })}
        </p>
      ))}
    </>
  );
}
