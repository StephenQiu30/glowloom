"use client";

import { ArrowRight, Clapperboard, Server, Sparkles } from "lucide-react";
import { useEffect, useState } from "react";

import { Button } from "@/components/ui/button";

type HealthState = "checking" | "healthy" | "unavailable";

const apiBaseUrl = process.env.NEXT_PUBLIC_API_BASE_URL ?? "http://127.0.0.1:8000";

export default function Home() {
  const [health, setHealth] = useState<HealthState>("checking");

  useEffect(() => {
    const controller = new AbortController();
    fetch(`${apiBaseUrl}/healthz`, { signal: controller.signal })
      .then((response) => setHealth(response.ok ? "healthy" : "unavailable"))
      .catch(() => setHealth("unavailable"));
    return () => controller.abort();
  }, []);

  return (
    <main className="min-h-screen bg-background text-foreground">
      <nav className="mx-auto flex max-w-6xl items-center justify-between px-6 py-7">
        <div className="flex items-center gap-3 text-lg font-semibold">
          <span className="grid size-9 place-items-center rounded-xl bg-primary text-primary-foreground">
            <Clapperboard className="size-5" aria-hidden="true" />
          </span>
          Lanverse
        </div>
        <a
          className="text-sm text-muted-foreground transition-colors hover:text-foreground"
          href={`${apiBaseUrl}/readyz`}
        >
          查看接口状态
        </a>
      </nav>

      <section className="mx-auto grid max-w-6xl gap-12 px-6 pt-20 pb-24 lg:grid-cols-[1.25fr_0.75fr] lg:items-center">
        <div>
          <div className="mb-6 inline-flex items-center gap-2 rounded-full border bg-card px-3 py-1.5 text-sm text-muted-foreground">
            <Sparkles className="size-4" aria-hidden="true" />
            AI 短剧生产工作台
          </div>
          <h1 className="max-w-3xl text-5xl leading-[1.08] font-semibold tracking-tight md:text-7xl">
            从剧本到成片，保持每一步可控
          </h1>
          <p className="mt-7 max-w-2xl text-lg leading-8 text-muted-foreground">
            以版本、任务和人工决议连接剧本、资产、分镜、生成、审核与交付。刷新或任务恢复后，服务端事实始终是唯一依据。
          </p>
          <div className="mt-10 flex flex-wrap gap-3">
            <Button size="lg" disabled>
              项目工作台将在 S1 开放
              <ArrowRight aria-hidden="true" />
            </Button>
          </div>
        </div>

        <aside className="rounded-3xl border bg-card p-6 shadow-sm">
          <div className="flex items-center gap-3">
            <span className="grid size-10 place-items-center rounded-full bg-muted">
              <Server className="size-5" aria-hidden="true" />
            </span>
            <div>
              <p className="font-medium">服务状态</p>
              <p aria-live="polite" className="text-sm text-muted-foreground">
                {health === "checking" && "正在检查后端服务…"}
                {health === "healthy" && "后端服务正常"}
                {health === "unavailable" && "后端服务暂不可用"}
              </p>
            </div>
          </div>
          <div className="mt-6 grid gap-3 text-sm">
            {["FastAPI 契约", "Next.js App Router", "PostgreSQL 事实源"].map((item) => (
              <div
                className="flex items-center justify-between rounded-xl bg-muted/60 px-4 py-3"
                key={item}
              >
                <span>{item}</span>
                <span className="text-muted-foreground">已启用</span>
              </div>
            ))}
          </div>
        </aside>
      </section>
    </main>
  );
}
