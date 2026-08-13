import { Download, FileArchive, RefreshCw, ShieldCheck } from "lucide-react";

import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";

type StoryboardExportsProps = {
  busy: boolean;
  history?: API.ExportHistoryResponse;
  preflight?: API.ExportPreflightResponse;
  onDownload: (mediaVersionId: string) => Promise<void>;
  onExport: (inputHash: string) => Promise<void>;
  onPreflight: () => Promise<void>;
};

const statusLabels: Record<API.ExportResponse["status"], string> = {
  queued: "排队中",
  running: "生成中",
  succeeded: "可下载",
  failed: "失败",
};

function formatBytes(value: number): string {
  if (value < 1024) return `${value} B`;
  if (value < 1024 * 1024) return `${Math.round(value / 1024)} KB`;
  return `${(value / (1024 * 1024)).toFixed(1)} MB`;
}

export function StoryboardExports({
  busy,
  history,
  preflight,
  onDownload,
  onExport,
  onPreflight,
}: StoryboardExportsProps) {
  const ready = preflight?.status === "ready" && Boolean(preflight.input_hash);

  return (
    <Card>
      <CardHeader className="gap-4 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <div className="flex items-center gap-2 text-xs font-medium text-primary">
            <ShieldCheck aria-hidden="true" className="size-4" />
            固定版本 · 可校验交付
          </div>
          <CardTitle className="mt-2">
            <h2>可信分镜包</h2>
          </CardTitle>
          <CardDescription className="mt-1 max-w-2xl leading-6">
            预检固定剧本、叙事单元、资产与分镜规格版本；后台生成 JSON、CSV、HTML
            和带哈希的 Manifest。后续改稿不会改写历史包。
          </CardDescription>
        </div>
        <div className="flex flex-wrap gap-2">
          <Button
            disabled={busy}
            onClick={() => void onPreflight()}
            type="button"
            variant="outline"
          >
            <RefreshCw aria-hidden="true" />
            检查导出条件
          </Button>
          <Button
            disabled={busy || !ready}
            onClick={() => {
              if (preflight?.input_hash) void onExport(preflight.input_hash);
            }}
            type="button"
          >
            <FileArchive aria-hidden="true" />
            生成分镜包
          </Button>
        </div>
      </CardHeader>
      <CardContent className="grid gap-5">
        {preflight ? (
          <div className="rounded-xl border p-4" aria-label="分镜包预检结果">
            <div className="flex flex-wrap items-center justify-between gap-2">
              <div className="font-medium">
                预检结果：
                {preflight.status === "ready"
                  ? "可导出"
                  : preflight.status === "blocked"
                    ? "存在内容阻断"
                    : "依赖暂不可用"}
              </div>
              <Badge variant={ready ? "default" : "outline"}>
                {preflight.shot_spec_version_ids.length} 个镜头 ·{" "}
                {preflight.asset_version_ids.length} 个资产版本
              </Badge>
            </div>
            {preflight.blockers.length ? (
              <div className="mt-3 grid gap-2">
                {preflight.blockers.map((blocker) => (
                  <Alert key={`${blocker.code}:${blocker.shot_id ?? blocker.dependency_id ?? "episode"}`}>
                    <AlertTitle>{blocker.code}</AlertTitle>
                    <AlertDescription>{blocker.summary}</AlertDescription>
                  </Alert>
                ))}
              </div>
            ) : (
              <p className="mt-2 text-sm text-muted-foreground">
                覆盖与准备度均已通过；提交时会再次校验此输入哈希。
              </p>
            )}
          </div>
        ) : (
          <p className="text-sm text-muted-foreground">
            先检查导出条件，系统不会在依赖不完整时生成部分成功包。
          </p>
        )}

        <section aria-labelledby="storyboard-export-history" className="grid gap-3">
          <div className="flex items-center justify-between gap-3">
            <h3 className="font-medium" id="storyboard-export-history">
              导出历史
            </h3>
            <span className="text-xs text-muted-foreground">
              {history?.total ?? 0} 次固定快照
            </span>
          </div>
          {history?.items.length ? (
            history.items.map((item) => (
              <article
                className="flex flex-wrap items-center justify-between gap-3 rounded-xl border p-4"
                key={item.id}
              >
                <div className="min-w-0">
                  <div className="flex flex-wrap items-center gap-2">
                    <span className="font-medium">{statusLabels[item.status]}</span>
                    <Badge variant="outline">{item.input_hash.slice(0, 10)}</Badge>
                    {item.manifest ? (
                      <span className="text-xs text-muted-foreground">
                        {formatBytes(item.manifest.package_size_bytes)}
                      </span>
                    ) : null}
                  </div>
                  <p className="mt-1 text-xs text-muted-foreground">
                    {new Date(item.created_at).toLocaleString("zh-CN")}
                    {item.error_code ? ` · ${item.error_code}` : ""}
                  </p>
                </div>
                {item.status === "succeeded" && item.manifest ? (
                  <Button
                    disabled={busy}
                    onClick={() => void onDownload(item.manifest!.media_version_id)}
                    type="button"
                    variant="outline"
                  >
                    <Download aria-hidden="true" />
                    下载分镜包
                  </Button>
                ) : null}
              </article>
            ))
          ) : (
            <div className="rounded-xl border border-dashed p-5 text-sm text-muted-foreground">
              尚无导出记录。任务成功且对象字节验证通过后才会出现在这里。
            </div>
          )}
        </section>
      </CardContent>
    </Card>
  );
}
