"use client";

import { useId, useState } from "react";
import type { CanvasTextDraft } from "@glowloom/canvas-collab/model";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { FieldLabel } from "@/components/ui/field";
import { RichTextContent } from "@/components/canvas/runtime/components/editor/rich-text/rich-text-content";
import {
  documentText,
  parseRichTextDocument,
} from "@/components/canvas/runtime/components/editor/lib/rich-text-document";
import { notifyError } from "@/components/canvas/runtime/components/feedback/error-notification";
import { canvasGraphFingerprint } from "../lib/canvas-json";

export function CanvasTextDraftRecovery({
  nodeId,
  title,
  document,
  current,
  busy,
  apply,
}: {
  nodeId: string;
  title: string;
  document: CanvasTextDraft;
  current: CanvasContract.CanvasDocumentNodeDTO["textDocument"] | null;
  busy: boolean;
  apply: (
    nodeId: string,
    document: CanvasTextDraft,
    current: CanvasContract.CanvasDocumentNodeDTO["textDocument"] | null,
  ) => Promise<void>;
}) {
  const id = useId();
  const [reviewed, setReviewed] = useState<{
    document: CanvasTextDraft;
    current: typeof current;
  } | null>(null);
  const [applying, setApplying] = useState(false);
  const currentDocument = current ? parseRichTextDocument(current) : undefined;
  const stale =
    !!reviewed &&
    (canvasGraphFingerprint(reviewed.document) !==
      canvasGraphFingerprint(document) ||
      canvasGraphFingerprint(reviewed.current) !==
        canvasGraphFingerprint(current));
  return (
    <section aria-label="保留正文恢复" className="grid gap-3 border-t pt-3">
      <p className="text-sm">{title} · 保留正文</p>
      <dl className="grid gap-2 text-xs">
        <dt className="text-muted-foreground">当前正文</dt>
        <dd className="max-h-24 overflow-auto break-words">
          {currentDocument ? (
            <RichTextContent
              value={documentText(currentDocument.content)}
              document={currentDocument}
            />
          ) : (
            "空白正文"
          )}
        </dd>
        <dt className="text-muted-foreground">保留正文（包含换行和格式）</dt>
        <dd className="max-h-32 overflow-auto break-words">
          <RichTextContent
            value={documentText(document.content)}
            document={document}
          />
        </dd>
      </dl>
      <FieldLabel htmlFor={id} className="flex items-start gap-2 text-sm">
        <Checkbox
          id={id}
          checked={!!reviewed}
          disabled={busy || applying}
          onCheckedChange={(checked) =>
            setReviewed(checked ? structuredClone({ document, current }) : null)
          }
        />
        我已核对这份保留正文
      </FieldLabel>
      {stale && (
        <p role="status" className="text-sm">
          正文已变化，请取消勾选后重新核对。
        </p>
      )}
      <Button
        disabled={busy || applying || !reviewed || stale}
        onClick={() => {
          if (!reviewed || stale || busy || applying) return;
          setApplying(true);
          void apply(nodeId, reviewed.document, reviewed.current)
            .catch((error) => notifyError(error, "保留正文尚未同步"))
            .finally(() => setApplying(false));
        }}
      >
        应用保留正文并同步
      </Button>
    </section>
  );
}
