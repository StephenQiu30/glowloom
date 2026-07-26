import { Clapperboard, Film, Images, ScrollText } from "lucide-react";

import { LandingAction } from "@/components/landing-action";

const stages = [
  { label: "剧本", icon: ScrollText },
  { label: "分镜", icon: Clapperboard },
  { label: "媒体", icon: Images },
  { label: "成片", icon: Film },
] as const;

export default function Home() {
  return (
    <main className="relative isolate flex min-h-screen items-center overflow-hidden px-6 py-16 sm:px-10">
      <div className="absolute inset-0 -z-10 bg-[radial-gradient(circle_at_top_left,oklch(0.92_0.05_264),transparent_38%),radial-gradient(circle_at_bottom_right,oklch(0.94_0.04_32),transparent_35%)]" />
      <section className="mx-auto w-full max-w-5xl rounded-4xl border bg-background/90 p-8 shadow-2xl shadow-foreground/5 backdrop-blur sm:p-12">
        <p className="mb-5 text-sm font-semibold tracking-[0.2em] text-muted-foreground uppercase">
          Lanverse production workspace
        </p>
        <h1 className="max-w-3xl text-4xl font-semibold tracking-tight text-balance sm:text-6xl">
          把故事变成可交付的 AI 短剧
        </h1>
        <p className="mt-6 max-w-2xl text-lg leading-8 text-muted-foreground">
          以可追溯的版本、任务和媒体事实，串起 30–60 秒竖屏短剧的完整制作闭环。
        </p>
        <LandingAction />
        <p className="mt-10 text-sm font-medium text-foreground">
          剧本 → 分镜 → 媒体 → 成片
        </p>
        <div className="mt-4 grid gap-3 sm:grid-cols-4">
          {stages.map(({ label, icon: Icon }, index) => (
            <div
              className="flex items-center gap-3 rounded-2xl border bg-card p-4 text-card-foreground"
              key={label}
            >
              <span className="flex size-9 items-center justify-center rounded-xl bg-muted text-muted-foreground">
                <Icon aria-hidden="true" className="size-4" />
              </span>
              <span>
                <span className="block text-xs text-muted-foreground">0{index + 1}</span>
                <span className="font-medium">{label}</span>
              </span>
            </div>
          ))}
        </div>
      </section>
    </main>
  );
}
