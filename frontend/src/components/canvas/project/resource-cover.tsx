"use client";

import { useEffect, useRef, useState } from "react";
import { useAccount } from "@/components/canvas/runtime/components/account/account-provider";
import { accountScope } from "@/components/canvas/runtime/store/account-slice";
import { FixedThumbnail } from "@/components/canvas/runtime/components/media/fixed-thumbnail";
import { Badge } from "@/components/ui/badge";
import type { WorkspaceResource } from "../lib/workspace-resources";

export type CoverReference = {
  coverFileId?: string;
  coverFileVersion?: string;
};

/** One cover surface for home, project contents, canvas lists and the edit draft. */
export function ResourceCover({
  title,
  kind,
  coverFileId,
  coverFileVersion,
  showType = true,
}: Pick<WorkspaceResource, "title" | "kind"> &
  CoverReference & {
    showType?: boolean;
  }) {
  const { account, ready, signingOut } = useAccount();
  const element = useRef<HTMLDivElement>(null);
  const [visible, setVisible] = useState(
    () => !globalThis.IntersectionObserver,
  );
  useEffect(() => {
    if (!element.current || !globalThis.IntersectionObserver) return;
    const observer = new IntersectionObserver(([entry]) => {
      if (entry.isIntersecting) setVisible(true);
    });
    observer.observe(element.current);
    return () => observer.disconnect();
  }, []);
  const scope = ready && account && !signingOut ? accountScope(account) : null;
  return (
    <div
      ref={element}
      className="resource-cover"
      data-kind={kind}
      aria-hidden="true"
    >
      {coverFileId && coverFileVersion ? (
        visible && scope ? (
          <CoverImage
            key={`${scope}:${coverFileId}:${coverFileVersion}`}
            scope={scope}
            fileId={coverFileId}
            version={coverFileVersion}
          />
        ) : null
      ) : (
        <div className="resource-cover-title">
          <span>
            {title.trim() || (kind === "project" ? "未命名项目" : "未命名画布")}
          </span>
        </div>
      )}
      {showType && (
        <Badge variant={coverFileId ? "secondary" : "outline"}>
          {kind === "project" ? "项目" : "画布"}
        </Badge>
      )}
    </div>
  );
}

function CoverImage({
  scope,
  fileId,
  version,
}: {
  scope: string;
  fileId: string;
  version: string;
}) {
  return (
    <FixedThumbnail
      scope={scope}
      fileId={fileId}
      version={version}
      errorFallback={
        <span className="m-auto text-xs text-muted-foreground">
          封面暂不可用
        </span>
      }
    />
  );
}
