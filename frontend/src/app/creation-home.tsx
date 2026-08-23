"use client";

import {
  AudioLines,
  CircleCheck,
  CirclePlay,
  Clapperboard,
  FileText,
  LoaderCircle,
  Upload,
  UsersRound,
} from "lucide-react";
import Image from "next/image";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useEffect } from "react";

import { LayoutContainer } from "@/components/layout/layout-container";
import { BasicLayout } from "@/components/layout/basic-layout";
import { StudioShell } from "@/components/studio/studio-shell";
import { Button } from "@/components/ui/button";
import { useAuthSessionState } from "@/hooks/use-auth-session";

const productionStages = [
  { label: "剧本解析", detail: "场次、对白与实体", icon: FileText },
  { label: "视觉开发", detail: "角色、场景与风格", icon: UsersRound },
  { label: "镜头生产", detail: "分镜、候选与选用", icon: Clapperboard },
  { label: "声音时间线", detail: "声音、字幕与编排", icon: AudioLines },
  { label: "审核交付", detail: "返工、标识与成片", icon: CirclePlay },
];

export function CreationHome() {
  const router = useRouter();
  const sessionState = useAuthSessionState();

  useEffect(() => {
    if (sessionState === "authenticated") {
      router.replace("/projects");
    }
  }, [router, sessionState]);

  if (sessionState !== "anonymous") {
    return (
      <BasicLayout active="create" authState="loading">
        <div className="grid min-h-[70dvh] place-items-center">
          <LoaderCircle
            aria-label={sessionState === "authenticated" ? "正在进入项目工作区" : "正在读取登录状态"}
            className="animate-spin text-primary"
          />
        </div>
      </BasicLayout>
    );
  }

  return (
    <StudioShell active="create">
      <LayoutContainer>
        <div className="py-12 lg:py-14">
          <section
            aria-label="产品欢迎"
            className="grid items-center gap-12 lg:grid-cols-[minmax(0,1fr)_minmax(240px,0.62fr)] xl:gap-20"
          >
            <div>
              <p className="text-sm font-medium">欢迎来到 Lanverse</p>
              <h1 className="mt-5 max-w-xl text-[2.75rem] leading-[1.04] font-semibold tracking-[-0.055em] sm:text-6xl lg:text-5xl xl:text-6xl">
                把剧本，变成<br />可追踪的成片。
              </h1>
              <p className="mt-6 max-w-lg text-base leading-7 text-muted-foreground">
                从固定剧本版本到 9:16 成片，每个资产、镜头、候选、费用与审核决定都可确认、可返工、可追溯。
              </p>
              <div className="mt-8 flex flex-wrap gap-3">
                <Button asChild className="h-11 px-5" size="lg">
                  <Link href="/register"><Upload aria-hidden="true" />导入剧本</Link>
                </Button>
                <Button asChild className="h-11 px-5" size="lg" variant="outline">
                  <Link href="/login"><CirclePlay aria-hidden="true" />继续制作</Link>
                </Button>
              </div>
            </div>

            <figure className="mx-auto w-full max-w-[280px]">
              <div className="relative aspect-[9/16] overflow-hidden rounded-xl bg-muted">
                <Image
                  alt="长安夜航项目封面"
                  className="object-cover grayscale"
                  fill
                  priority
                  sizes="280px"
                  src="/assets/lanverse-studio/changan-night-cover.png"
                />
                <div className="absolute inset-x-0 bottom-0 bg-black/75 p-5 text-white">
                  <p className="font-mono text-xs text-white/60">9:16 · EPISODE</p>
                  <p className="mt-1 text-lg font-medium">从故事到交付</p>
                </div>
              </div>
              <figcaption className="mt-2 text-center font-mono text-xs text-muted-foreground">1080 × 1920</figcaption>
            </figure>
          </section>

          <section aria-labelledby="production-pipeline-title" className="mt-12 bg-muted/35 px-5 py-7 md:px-7 lg:mt-14 lg:py-8">
            <div className="flex flex-col gap-3 sm:flex-row sm:items-end sm:justify-between">
              <div>
                <p className="text-xs font-medium text-muted-foreground">真实生产阶段</p>
                <h2 className="mt-2 text-xl font-semibold tracking-tight" id="production-pipeline-title">可恢复的生产链</h2>
              </div>
              <p className="max-w-xl text-sm leading-6 text-muted-foreground">AI 结果先成为候选，只有人工确认后才进入下游事实。</p>
            </div>
            <ol className="mt-6 grid gap-1.5 sm:grid-cols-2 xl:grid-cols-5">
              {productionStages.map((stage, index) => (
                <li className="relative min-h-32 bg-background px-4 py-5" key={stage.label}>
                  <div className="flex items-start justify-between gap-3">
                    <span className="font-mono text-xs text-muted-foreground">0{index + 1}</span>
                    <CircleCheck className="size-4 text-muted-foreground" aria-hidden="true" />
                  </div>
                  <div className="mt-5 flex items-center gap-2 text-sm font-medium"><stage.icon className="size-4" aria-hidden="true" />{stage.label}</div>
                  <p className="mt-2 text-sm text-muted-foreground">{stage.detail}</p>
                </li>
              ))}
            </ol>
          </section>
        </div>
      </LayoutContainer>
    </StudioShell>
  );
}
