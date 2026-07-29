"use client";

import {
  ArrowLeft,
  Clapperboard,
  Clock3,
  LoaderCircle,
  Plus,
  ScrollText,
} from "lucide-react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { type FormEvent, useEffect, useState } from "react";

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
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { useAuthSessionState } from "@/hooks/use-auth-session";
import {
  appApiErrorMessage,
  useCreateEpisodeMutation,
  useEpisodesQuery,
  useProjectQuery,
  useProjectSnapshotQuery,
} from "@/lib/app-api";

export function ProjectWorkspace({ projectId }: { projectId: string }) {
  const router = useRouter();
  const authState = useAuthSessionState();
  const isAuthenticated = authState === "authenticated";
  const [commandError, setCommandError] = useState<string | null>(null);
  const project = useProjectQuery(projectId, { skip: !isAuthenticated });
  const episodes = useEpisodesQuery(projectId, { skip: !isAuthenticated });
  const snapshot = useProjectSnapshotQuery(projectId, { skip: !isAuthenticated });
  const [createEpisode, createState] = useCreateEpisodeMutation();

  useEffect(() => {
    if (authState === "anonymous") router.replace("/login");
  }, [authState, router]);

  async function handleCreateEpisode(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const formElement = event.currentTarget;
    const form = new FormData(formElement);
    setCommandError(null);
    try {
      await createEpisode({
        projectId,
        body: {
          name: String(form.get("episodeName")),
          target_duration_ms: 90_000,
        },
      }).unwrap();
      formElement.reset();
    } catch (error: unknown) {
      setCommandError(appApiErrorMessage(error));
    }
  }

  if (authState === "checking" || project.isLoading) {
    return (
      <main className="grid min-h-screen place-items-center" aria-live="polite">
        <LoaderCircle className="size-6 animate-spin" aria-hidden="true" />
        <span className="sr-only">正在加载项目详情</span>
      </main>
    );
  }

  if (authState === "anonymous") return null;

  if (project.isError || !project.data) {
    return (
      <main className="mx-auto max-w-xl px-6 py-20">
        <Alert variant="destructive">
          <AlertTitle>无法加载项目</AlertTitle>
          <AlertDescription>{appApiErrorMessage(project.error)}</AlertDescription>
        </Alert>
        <Button asChild className="mt-5" variant="outline">
          <Link href="/projects">返回项目列表</Link>
        </Button>
      </main>
    );
  }

  return (
    <main className="min-h-screen bg-muted/30">
      <header className="border-b bg-background">
        <div className="mx-auto flex max-w-6xl items-center justify-between px-6 py-5">
          <Link className="flex items-center gap-3 font-semibold" href="/projects">
            <span className="grid size-9 place-items-center rounded-xl bg-primary text-primary-foreground">
              <Clapperboard className="size-5" aria-hidden="true" />
            </span>
            Lanverse
          </Link>
          <Button asChild variant="ghost">
            <Link href="/projects">
              <ArrowLeft aria-hidden="true" />
              返回项目
            </Link>
          </Button>
        </div>
      </header>

      <div className="mx-auto max-w-6xl px-6 py-10">
        <div className="flex flex-wrap items-end justify-between gap-5">
          <div>
            <div className="flex gap-2">
              <Badge variant="secondary">{project.data.aspect_ratio}</Badge>
              <Badge variant="outline">{project.data.language}</Badge>
            </div>
            <h1 className="mt-4 text-3xl font-semibold tracking-tight">{project.data.name}</h1>
            <p className="mt-2 max-w-2xl text-muted-foreground">
              {project.data.description || "尚未添加项目简介"}
            </p>
          </div>
          <div className="min-w-52 rounded-xl border bg-background p-4">
            <div className="flex items-center justify-between text-sm">
              <span>制作完成度</span>
              <span>{snapshot.data?.completion ?? 0}%</span>
            </div>
            <div
              aria-label="制作完成度"
              aria-valuemax={100}
              aria-valuemin={0}
              aria-valuenow={snapshot.data?.completion ?? 0}
              className="mt-3 h-2 overflow-hidden rounded-full bg-muted"
              role="progressbar"
            >
              <div
                className="h-full bg-primary transition-[width]"
                style={{ width: `${snapshot.data?.completion ?? 0}%` }}
              />
            </div>
          </div>
        </div>

        {(episodes.isError || snapshot.isError) && (
          <Alert variant="destructive" className="mt-7">
            <AlertTitle>部分生产信息加载失败</AlertTitle>
            <AlertDescription>
              {appApiErrorMessage(episodes.error ?? snapshot.error)}
            </AlertDescription>
          </Alert>
        )}

        <div className="mt-9 grid gap-8 lg:grid-cols-[1fr_20rem]">
          <section>
            <div className="mb-5 flex items-center justify-between">
              <div>
                <h2 className="text-xl font-semibold">单集</h2>
                <p className="mt-1 text-sm text-muted-foreground">按服务端顺序组织短剧内容。</p>
              </div>
              <Badge variant="outline">{episodes.data?.length ?? 0} 集</Badge>
            </div>

            {episodes.isLoading ? (
              <p className="text-sm text-muted-foreground">正在加载单集…</p>
            ) : episodes.data?.length ? (
              <div className="grid gap-4">
                {episodes.data.map((episode) => {
                  const episodeSnapshot = snapshot.data?.episodes.find(
                    (item) => item.episode_id === episode.id,
                  );
                  return (
                    <Card key={episode.id}>
                      <CardHeader>
                        <div className="flex items-start justify-between gap-4">
                          <div>
                            <CardTitle>{episode.name}</CardTitle>
                            <CardDescription className="mt-1">
                              第 {episode.position} 集 · {Math.round(episode.target_duration_ms / 1000)} 秒
                            </CardDescription>
                          </div>
                          <Badge variant="secondary">
                            {episodeSnapshot?.completion ?? 0}%
                          </Badge>
                        </div>
                      </CardHeader>
                      {episodeSnapshot?.blocking_reasons.length ? (
                        <CardContent className="text-sm text-muted-foreground">
                          {episodeSnapshot.blocking_reasons[0].summary}
                        </CardContent>
                      ) : null}
                    </Card>
                  );
                })}
              </div>
            ) : (
              <Card className="border-dashed py-12 text-center">
                <CardContent>
                  <ScrollText className="mx-auto size-8 text-muted-foreground" aria-hidden="true" />
                  <p className="mt-4 font-medium">还没有单集</p>
                  <p className="mt-1 text-sm text-muted-foreground">创建单集后即可进入剧本阶段。</p>
                </CardContent>
              </Card>
            )}
          </section>

          <aside className="grid content-start gap-5">
            <Card>
              <CardHeader>
                <CardTitle>创建单集</CardTitle>
                <CardDescription>默认目标时长为 90 秒。</CardDescription>
              </CardHeader>
              <CardContent>
                <form className="grid gap-4" onSubmit={handleCreateEpisode}>
                  <div className="grid gap-2">
                    <Label htmlFor="episodeName">单集名称</Label>
                    <Input id="episodeName" name="episodeName" maxLength={120} required />
                  </div>
                  {commandError ? (
                    <p className="text-sm text-destructive" role="alert">{commandError}</p>
                  ) : null}
                  <Button disabled={createState.isLoading} type="submit">
                    {createState.isLoading ? (
                      <LoaderCircle className="animate-spin" aria-hidden="true" />
                    ) : (
                      <Plus aria-hidden="true" />
                    )}
                    创建单集
                  </Button>
                </form>
              </CardContent>
            </Card>

            <Card>
              <CardHeader>
                <CardTitle>下一步</CardTitle>
                <CardDescription>由生产快照计算，不由前端推断。</CardDescription>
              </CardHeader>
              <CardContent>
                {snapshot.isLoading ? (
                  <p className="text-sm text-muted-foreground">正在计算…</p>
                ) : snapshot.data?.next_actions.length ? (
                  <div className="grid gap-3">
                    {snapshot.data.next_actions.map((action) => (
                      <div className="rounded-lg bg-muted p-3" key={action.code}>
                        <p className="font-medium">{action.label}</p>
                        <p className="mt-1 text-xs text-foreground">
                          {action.code === "import_script"
                            ? "剧本导入将在下一实施切片开放。"
                            : "完成此动作后，生产快照会重新计算。"}
                        </p>
                      </div>
                    ))}
                  </div>
                ) : (
                  <div className="flex items-center gap-2 text-sm text-muted-foreground">
                    <Clock3 className="size-4" aria-hidden="true" />
                    先创建一个单集
                  </div>
                )}
              </CardContent>
            </Card>
          </aside>
        </div>
      </div>
    </main>
  );
}
