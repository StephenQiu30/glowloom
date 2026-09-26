import { Clapperboard } from "lucide-react";

const stages = ["剧本", "设定集", "分镜", "故事板"] as const;

export default function ProjectsPage() {
  return (
    <div className="space-y-16">
      <section aria-labelledby="workspace-title" className="max-w-3xl">
        <p className="font-mono text-xs font-medium tracking-[0.22em] text-muted-foreground">
          LANVERSE / WORKSPACE
        </p>
        <h1
          id="workspace-title"
          className="mt-7 text-5xl leading-[1.15] font-semibold tracking-[-0.055em] text-balance sm:text-6xl lg:text-[4.5rem]"
        >
          让每个故事，
          <br />
          都有自己的画面。
        </h1>
        <p className="mt-7 max-w-xl text-base leading-8 text-pretty text-muted-foreground sm:text-lg">
          从剧本到分镜，再到镜头和声音。你的创作项目会在这里汇集。
        </p>
      </section>

      <div className="grid gap-14 lg:grid-cols-[minmax(0,1.6fr)_minmax(230px,0.6fr)] lg:gap-16">
        <section
          aria-labelledby="projects-title"
          className="flex min-h-72 flex-col gap-9 rounded-3xl bg-muted/65 p-8 sm:p-10"
        >
          <div className="flex size-12 items-center justify-center rounded-2xl bg-background">
            <Clapperboard
              aria-hidden="true"
              className="size-5"
              strokeWidth={1.7}
            />
          </div>
          <div className="max-w-lg">
            <h2
              id="projects-title"
              className="text-2xl font-semibold tracking-tight"
            >
              项目列表即将开放
            </h2>
            <p className="mt-3 text-sm leading-7 text-pretty text-muted-foreground sm:text-base">
              登录与项目管理接入后，你可以在这里查看和创建项目。
            </p>
          </div>
        </section>

        <aside aria-label="创作流程" className="max-w-md lg:pt-6">
          <p className="font-mono text-xs font-medium tracking-[0.2em] text-muted-foreground">
            CREATIVE FLOW
          </p>
          <h2 className="mt-5 text-xl font-semibold tracking-tight">
            一条连贯的创作流程
          </h2>
          <ol className="mt-9 space-y-7">
            {stages.map((stage, index) => (
              <li key={stage} className="flex items-baseline gap-5">
                <span className="font-mono text-xs text-muted-foreground tabular-nums">
                  {String(index + 1).padStart(2, "0")}
                </span>
                <span className="text-base font-medium">{stage}</span>
              </li>
            ))}
          </ol>
        </aside>
      </div>
    </div>
  );
}
