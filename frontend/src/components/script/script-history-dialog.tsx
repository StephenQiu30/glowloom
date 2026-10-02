"use client";
import { useState } from "react";
import dynamic from "next/dynamic";
import { useInfiniteQuery, useQuery } from "@tanstack/react-query";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  getSourceSnapshot,
  listSourceHistory,
  listVersions,
  reviewScopeKey,
} from "./review-queries";
import type { ScriptScope } from "./source-intent";
import type { SourceSnapshot } from "./review-model";
import { downloadDocument, findDocument } from "./document-media";
import { SourceExtractionWarnings } from "./source-extraction-warnings";
const RichSourceEditor = dynamic(
  () =>
    import("./rich-source-editor").then((module) => module.RichSourceEditor),
  { ssr: false, loading: () => <p role="status">正在加载历史富文本…</p> },
);
export function ScriptHistoryDialog({
  scope,
  lineageId,
  onClose,
  onChooseVersion,
}: {
  scope: ScriptScope;
  lineageId?: string;
  onClose: () => void;
  onChooseVersion: (id: string) => void;
}) {
  const [snapshotId, setSnapshotId] = useState<string>();
  const versions = useInfiniteQuery({
    queryKey: [...reviewScopeKey(scope), "versions"],
    initialPageParam: 0,
    queryFn: ({ pageParam, signal }) => listVersions(scope, pageParam, signal),
    getNextPageParam: (page) => page.next_version_no,
    staleTime: 0,
  });
  const sources = useInfiniteQuery({
    queryKey: [...reviewScopeKey(scope), "lineage-history", lineageId],
    initialPageParam: 0,
    queryFn: ({ pageParam, signal }) =>
      listSourceHistory(scope, lineageId!, pageParam, signal),
    getNextPageParam: (page) => page.next_revision,
    enabled: Boolean(lineageId),
    staleTime: 0,
  });
  const snapshot = useQuery({
    queryKey: [...reviewScopeKey(scope), "snapshot", snapshotId],
    queryFn: ({ signal }) => getSourceSnapshot(scope, snapshotId!, signal),
    enabled: Boolean(snapshotId),
    staleTime: 0,
    gcTime: 0,
  });
  const versionItems = versions.data?.pages.flatMap((page) => page.items) ?? [];
  const sourceItems = sources.data?.pages.flatMap((page) => page.items) ?? [];
  const duplicateVersions =
    new Set(versionItems.map((item) => item.id)).size !== versionItems.length;
  const duplicateSources =
    new Set(sourceItems.map((item) => item.id)).size !== sourceItems.length;
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
    >
      <DialogContent className="max-h-[calc(100dvh-2rem)] overflow-y-auto sm:max-w-5xl">
        <DialogHeader>
          <DialogTitle>
            {lineageId ? "来源快照与剧本完整历史" : "剧本完整版本历史"}
          </DialogTitle>
          <DialogDescription>
            列表只读取不可变摘要，选择某个来源快照后才读取原正文。格式、标题、顺序和被移除的来源继续保留历史；历史选择不替换当前草稿。
          </DialogDescription>
        </DialogHeader>
        <div className="grid min-w-0 gap-4 lg:grid-cols-[320px_minmax(0,1fr)]">
          <section className="min-w-0 space-y-3" aria-label="剧本版本列表">
            {versions.isPending && <p role="status">正在读取剧本版本…</p>}
            {(versions.error || duplicateVersions) && (
              <p role="alert">
                {duplicateVersions
                  ? "版本分页出现重复身份，请重新读取。"
                  : versions.error?.message}
              </p>
            )}
            <ul className="max-h-96 space-y-2 overflow-y-auto">
              {!duplicateVersions &&
                versionItems.map((version) => (
                  <li
                    key={version.id}
                    className="space-y-1 rounded-lg border p-3"
                  >
                    <p>
                      剧本 {version.version_no} · {version.source_count} 个来源
                      · {version.char_count.toLocaleString("zh-CN")} 字符
                    </p>
                    <p className="text-xs">
                      {new Date(version.created_at).toLocaleString("zh-CN")}
                    </p>
                    <details>
                      <summary className="cursor-pointer text-sm">
                        三种独立指纹
                      </summary>
                      <p className="text-xs break-all">
                        正文：{version.content_hash}
                      </p>
                      <p className="text-xs break-all">
                        格式：{version.document_sha256}
                      </p>
                      <p className="text-xs break-all">
                        来源清单：{version.source_manifest_sha256}
                      </p>
                    </details>
                    <Button
                      type="button"
                      variant="outline"
                      onClick={() => onChooseVersion(version.id)}
                    >
                      查看剧本 {version.version_no} 的完整来源
                    </Button>
                  </li>
                ))}
            </ul>
            {!versions.isPending &&
              !versionItems.length &&
              !versions.isError && <p>尚无已发布剧本版本。</p>}
            {(versions.hasNextPage || versions.isError) && (
              <Button
                type="button"
                variant="outline"
                disabled={versions.isFetching}
                onClick={() =>
                  void (versions.isError
                    ? versions.refetch()
                    : versions.fetchNextPage())
                }
              >
                {versions.isError ? "重试读取剧本历史" : "读取更早剧本版本"}
              </Button>
            )}
          </section>
          <section className="min-w-0 space-y-3" aria-label="来源历史与原正文">
            {!lineageId && (
              <p>
                整版来源以选定版本的不可变顺序读取。进入某个来源后可查看它的所有被替代或被移除快照。
              </p>
            )}
            {lineageId && (
              <>
                <p className="text-xs break-all">稳定来源身份：{lineageId}</p>
                {sources.isPending && (
                  <p role="status">正在读取来源全部快照…</p>
                )}
                {(sources.error || duplicateSources) && (
                  <p role="alert">
                    {duplicateSources
                      ? "来源历史分页出现重复身份，请重新读取。"
                      : sources.error?.message}
                  </p>
                )}
                <ul className="max-h-52 space-y-2 overflow-y-auto">
                  {!duplicateSources &&
                    sourceItems.map((source) => (
                      <li key={source.id}>
                        <Button
                          type="button"
                          variant={
                            source.id === snapshotId ? "secondary" : "outline"
                          }
                          className="h-auto w-full justify-start whitespace-normal"
                          aria-current={
                            source.id === snapshotId ? "true" : undefined
                          }
                          onClick={() => setSnapshotId(source.id)}
                        >
                          {source.title} · 来源版本 {source.source_revision} ·{" "}
                          {source.char_count.toLocaleString("zh-CN")} 字符
                        </Button>
                      </li>
                    ))}
                </ul>
                {(sources.hasNextPage || sources.isError) && (
                  <Button
                    type="button"
                    variant="outline"
                    disabled={sources.isFetching}
                    onClick={() =>
                      void (sources.isError
                        ? sources.refetch()
                        : sources.fetchNextPage())
                    }
                  >
                    {sources.isError ? "重试读取来源历史" : "读取更早来源快照"}
                  </Button>
                )}
              </>
            )}
            {snapshotId && snapshot.isPending && (
              <p role="status">正在读取选中的私有历史正文…</p>
            )}
            {snapshot.error && (
              <>
                <p role="alert">{snapshot.error.message}</p>
                <Button
                  variant="outline"
                  onClick={() => void snapshot.refetch()}
                >
                  重新读取选中快照
                </Button>
              </>
            )}
            {snapshot.data && (
              <SnapshotView
                key={snapshot.data.id}
                scope={scope}
                source={snapshot.data}
              />
            )}
          </section>
        </div>
        <DialogFooter>
          <Button type="button" variant="outline" onClick={onClose}>
            关闭剧本历史
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
function SnapshotView({
  source,
  scope,
}: {
  source: SourceSnapshot;
  scope: ScriptScope;
}) {
  const [invalid, setInvalid] = useState<string | null>(null);
  const [showHTML, setShowHTML] = useState(false);
  const [downloading, setDownloading] = useState(false);
  const [downloadError, setDownloadError] = useState<string>();
  async function originalFile() {
    if (!source.media_asset_id || downloading) return;
    setDownloading(true);
    setDownloadError(undefined);
    try {
      const asset = await findDocument(scope, source.media_asset_id);
      const file = await downloadDocument(scope, asset);
      const url = URL.createObjectURL(file);
      const link = document.createElement("a");
      link.href = url;
      link.download = asset.file_name;
      document.body.append(link);
      link.click();
      link.remove();
      window.setTimeout(() => URL.revokeObjectURL(url), 1000);
    } catch (cause) {
      setDownloadError(
        cause instanceof Error ? cause.message : "历史原件读取失败。",
      );
    } finally {
      setDownloading(false);
    }
  }
  function downloadHTML() {
    if (source.original_html === undefined) return;
    const url = URL.createObjectURL(
      new Blob([source.original_html], { type: "text/html;charset=utf-8" }),
    );
    const link = document.createElement("a");
    link.href = url;
    link.download = `source-${source.id}.html`;
    document.body.append(link);
    link.click();
    link.remove();
    window.setTimeout(() => URL.revokeObjectURL(url), 1000);
  }
  return (
    <section className="min-w-0 space-y-3" aria-label="选中来源不可变快照">
      <h3 className="font-semibold">
        {source.title} · 来源版本 {source.source_revision}
      </h3>
      <p className="text-xs break-all">
        来源快照：{source.id}；稳定身份：{source.source_lineage_id}
        {source.previous_source_id
          ? `；上一快照：${source.previous_source_id}`
          : ""}
      </p>
      <p className="text-xs break-all">
        正文 SHA：{source.content_hash}；格式 SHA：{source.rich_sha256}
      </p>
      {source.media_asset_id && (
        <p className="text-sm break-all">
          历史文档素材：{source.media_asset_id}
          。此快照保留确切原件，手工新稿的正文与历史原文件分别读取。
        </p>
      )}
      {source.media_asset_id && (
        <Button
          type="button"
          variant="outline"
          disabled={downloading}
          onClick={() => void originalFile()}
        >
          {downloading ? "正在读取历史原件…" : "下载此快照的文档原件"}
        </Button>
      )}
      {downloadError && <p role="alert">{downloadError}</p>}
      <SourceExtractionWarnings warnings={source.provenance.warnings} />
      <details>
        <summary className="cursor-pointer">原来源标签与字符映射</summary>
        <pre className="max-h-52 overflow-auto text-xs break-all whitespace-pre-wrap">
          {JSON.stringify(source.provenance, null, 2)}
        </pre>
      </details>
      <RichSourceEditor
        initialDocument={source.document}
        disabled
        onChange={() => {}}
        onInvalid={setInvalid}
      />
      {invalid && <p role="alert">{invalid}</p>}
      <details>
        <summary className="cursor-pointer">规范纯文本</summary>
        <pre className="max-h-64 overflow-auto break-words whitespace-pre-wrap">
          {source.plain_text}
        </pre>
      </details>
      {source.original_html !== undefined && (
        <>
          <details onToggle={(event) => setShowHTML(event.currentTarget.open)}>
            <summary className="cursor-pointer">
              逐字查看原始 HTML（只读）
            </summary>
            {showHTML && (
              <pre className="max-h-64 overflow-auto text-xs break-all whitespace-pre-wrap">
                {source.original_html}
              </pre>
            )}
          </details>
          <Button type="button" variant="outline" onClick={downloadHTML}>
            下载此快照原始 HTML
          </Button>
        </>
      )}
    </section>
  );
}
