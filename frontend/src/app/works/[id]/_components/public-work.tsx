"use client";

import { useState } from "react";
import Link from "next/link";
import useSWR from "swr";
import { ContentState } from "@/components/layout/content-state";
import { Button } from "@/components/ui/button";
import { getPublishedWork } from "@/api/works";

type Work = {
  id: string;
  title: string;
  description: string;
  kind: "image" | "video";
  prompt?: string;
  created_at: string;
  media_url: string;
  media_expires_at: string;
};
function requireWork(value: unknown, id: string): Work {
  if (!value || typeof value !== "object") throw new Error("作品数据不可用");
  const row = value as Partial<Work>;
  let url: URL;
  try {
    url = new URL(row.media_url ?? "");
  } catch {
    throw new Error("作品媒体不可用");
  }
  if (
    row.id !== id ||
    typeof row.title !== "string" ||
    typeof row.description !== "string" ||
    !["image", "video"].includes(row.kind ?? "") ||
    !["https:", "http:"].includes(url.protocol) ||
    url.username ||
    url.password ||
    !Number.isFinite(Date.parse(row.created_at ?? "")) ||
    !(Date.parse(row.media_expires_at ?? "") > Date.now()) ||
    (row.prompt !== undefined && typeof row.prompt !== "string")
  )
    throw new Error("作品数据不可用");
  return row as Work;
}

export function PublicWork({ id }: { id: string }) {
  const [failedMedia, setFailedMedia] = useState<string | null>(null);
  const { data, error, isLoading, mutate } = useSWR(
    /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i.test(id)
      ? ["published-work", id]
      : null,
    async () =>
      requireWork(
        await getPublishedWork({ work_id: id }, { sessionRecovery: "never" }),
        id,
      ),
    {
      refreshInterval: 240_000,
      revalidateOnFocus: true,
      shouldRetryOnError: false,
    },
  );
  return (
    <main className="mx-auto flex w-full max-w-5xl flex-col gap-6 p-6 sm:p-10">
      <div>
        <Button asChild variant="ghost">
          <Link href="/assets">返回资产</Link>
        </Button>
      </div>
      {isLoading ? (
        <ContentState state="loading" title="正在读取作品" />
      ) : error || !data ? (
        <ContentState
          state="error"
          title="作品暂不可用"
          description="作品可能已撤下，或当前无法读取。"
          actions={
            <Button variant="outline" onClick={() => void mutate()}>
              重试
            </Button>
          }
        />
      ) : (
        <article className="flex min-w-0 flex-col gap-6">
          <header className="space-y-2">
            <h1 className="text-2xl font-semibold break-words">{data.title}</h1>
            <p className="text-sm text-muted-foreground">
              <time dateTime={data.created_at}>
                {new Date(data.created_at).toLocaleDateString("zh-CN")}
              </time>{" "}
              · AI 生成作品
            </p>
          </header>
          {failedMedia === data.media_url ? (
            <ContentState
              state="error"
              title="媒体读取失败"
              actions={<Button onClick={() => void mutate()}>重新读取</Button>}
            />
          ) : data.kind === "image" ? (
            // An expiring authorized URL is rendered directly; the image optimizer must not cache it.
            // eslint-disable-next-line @next/next/no-img-element
            <img
              src={data.media_url}
              alt={data.title}
              referrerPolicy="no-referrer"
              className="max-h-[70vh] w-full rounded-lg object-contain"
              onError={() => setFailedMedia(data.media_url)}
            />
          ) : (
            <video
              src={data.media_url}
              controls
              preload="metadata"
              playsInline
              className="max-h-[70vh] w-full rounded-lg"
              onError={() => setFailedMedia(data.media_url)}
            />
          )}
          {data.description && (
            <p className="break-words whitespace-pre-wrap">
              {data.description}
            </p>
          )}
          {data.prompt !== undefined && (
            <section className="space-y-2">
              <h2 className="text-sm font-medium">公开提示词</h2>
              <p className="text-sm break-words whitespace-pre-wrap text-muted-foreground">
                {data.prompt || "无提示词"}
              </p>
            </section>
          )}
        </article>
      )}
    </main>
  );
}
