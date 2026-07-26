"use client";

import Link from "next/link";
import { useState, type FormEvent } from "react";

import { FeedbackState } from "@/components/feedback-state";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";

export interface ProjectItem {
  id: string;
  episodeId: string;
  title: string;
  hasConfirmedSource: boolean;
}

interface ProjectsViewProps {
  projects: readonly ProjectItem[];
  onCreate: (title: string) => void | Promise<void>;
  loading?: boolean;
  error?: string;
}

export function ProjectsView({ projects, onCreate, loading, error }: ProjectsViewProps) {
  const [title, setTitle] = useState("");

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const normalized = title.trim();
    if (normalized) void onCreate(normalized);
  }

  return (
    <main className="mx-auto grid min-h-screen w-full max-w-6xl gap-8 px-6 py-10 lg:grid-cols-[1fr_20rem]">
      <section aria-labelledby="projects-heading" className="space-y-5">
        <header>
          <p className="text-sm font-medium text-muted-foreground">Projects</p>
          <h1 className="text-3xl font-semibold tracking-tight" id="projects-heading">
            AI 短剧项目
          </h1>
        </header>
        {loading ? <FeedbackState description="正在读取服务端项目事实。" state="loading" title="正在加载" /> : null}
        {error ? <FeedbackState description="项目事实未改变，请稍后重试。" details={error} state="error" title="项目读取失败" /> : null}
        {!loading && !error && projects.length === 0 ? (
          <FeedbackState description="使用右侧表单创建唯一单集项目。" state="empty" title="还没有短剧项目" />
        ) : null}
        <div className="grid gap-4 sm:grid-cols-2">
          {projects.map((project) => (
            <Card key={project.id}>
              <CardHeader>
                <CardTitle>{project.title}</CardTitle>
                <CardDescription>
                  {project.hasConfirmedSource ? "故事制作中" : "待输入来源"}
                </CardDescription>
              </CardHeader>
              <CardContent>
                <Button asChild variant="outline">
                  <Link href={`/episodes/${project.episodeId}/story`}>
                    打开{project.title}的故事工作区
                  </Link>
                </Button>
              </CardContent>
            </Card>
          ))}
        </div>
      </section>
      <Card className="h-fit">
        <CardHeader>
          <CardTitle>创建项目</CardTitle>
          <CardDescription>系统会同时创建唯一 Episode。</CardDescription>
        </CardHeader>
        <CardContent>
          <form className="space-y-4" onSubmit={submit}>
            <div className="space-y-2">
              <Label htmlFor="project-title">项目标题</Label>
              <Input id="project-title" maxLength={120} onChange={(event) => setTitle(event.target.value)} required value={title} />
            </div>
            <Button className="w-full" type="submit">创建项目</Button>
          </form>
        </CardContent>
      </Card>
    </main>
  );
}
