"use client";

import { Button } from "@/components/ui/button";

export default function ProjectsError({
  retry,
}: {
  error: Error & { digest?: string };
  retry: () => void;
}) {
  return (
    <section
      role="alert"
      aria-labelledby="projects-error-title"
      className="flex min-h-72 flex-col items-start justify-center rounded-3xl bg-muted/65 p-8 sm:p-10"
    >
      <p
        translate="no"
        className="font-mono text-xs font-medium tracking-[0.2em] text-muted-foreground"
      >
        LANVERSE / WORKSPACE
      </p>
      <h1
        id="projects-error-title"
        className="mt-6 text-2xl font-semibold tracking-tight text-balance"
      >
        项目页面暂时无法显示
      </h1>
      <p className="mt-3 text-sm leading-7 text-muted-foreground">
        请重试。如果问题持续，稍后再回来。
      </p>
      <Button className="mt-7" onClick={retry}>
        重试加载
      </Button>
    </section>
  );
}
