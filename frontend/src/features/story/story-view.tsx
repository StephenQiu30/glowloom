"use client";

import { useState, type FormEvent } from "react";

import { FeedbackState } from "@/components/feedback-state";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Separator } from "@/components/ui/separator";
import { Textarea } from "@/components/ui/textarea";

type VersionStatus = "draft" | "confirmed" | "superseded";

export interface StoryVersionItem {
  id: string;
  version: number;
  status: VersionStatus;
  resourceVersion: number;
}

interface StoryViewProps {
  sources: readonly StoryVersionItem[];
  scripts: readonly StoryVersionItem[];
  storyboards: readonly StoryVersionItem[];
  conflictStage?: string;
  loading?: boolean;
  error?: string;
  onCreateSource: (input: { content: string; rights: "original" | "licensed" }) => void;
  onConfirmSource: (id: string, resourceVersion: number) => void;
  onConfirmScript: (id: string, resourceVersion: number) => void;
  onConfirmStoryboard: (id: string, resourceVersion: number) => void;
  onGenerateScript: () => void;
  onGenerateStoryboard: () => void;
}

const labels = { draft: "草稿", confirmed: "已确认", superseded: "历史" } as const;

function VersionSection({
  noun,
  versions,
  proposal,
  onConfirm,
}: {
  noun: string;
  versions: readonly StoryVersionItem[];
  proposal?: boolean;
  onConfirm: (id: string, resourceVersion: number) => void;
}) {
  return (
    <div className="space-y-3">
      {versions.length === 0 ? <p className="text-sm text-muted-foreground">暂无{noun}版本</p> : null}
      {versions.map((version) => (
        <div className="flex flex-wrap items-center justify-between gap-3 rounded-lg border p-3" key={version.id}>
          <div className="flex items-center gap-2">
            <span className="font-medium">{noun} v{version.version} · {labels[version.status]}</span>
            {proposal && version.status === "draft" ? <Badge variant="secondary">AI 提案</Badge> : null}
          </div>
          {version.status === "draft" ? (
            <Button onClick={() => onConfirm(version.id, version.resourceVersion)} size="sm" variant="outline">
              确认{noun} v{version.version}
            </Button>
          ) : null}
        </div>
      ))}
    </div>
  );
}

export function StoryView(props: StoryViewProps) {
  const [content, setContent] = useState("");
  const [rights, setRights] = useState<"original" | "licensed">("licensed");
  const hasSource = props.sources.some((item) => item.status === "confirmed");
  const hasScript = props.scripts.some((item) => item.status === "confirmed");
  const hasStoryboard = props.storyboards.some((item) => item.status === "confirmed");
  const missing = !hasSource
    ? "缺少已确认来源"
    : !hasScript
      ? "缺少已确认剧本"
      : !hasStoryboard
        ? "缺少已确认分镜"
        : "故事基线已就绪";

  function submitSource(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    props.onCreateSource({ content, rights });
  }

  return (
    <main className="mx-auto w-full max-w-6xl space-y-6 px-6 py-10">
      <header><p className="text-sm font-medium text-muted-foreground">Story</p><h1 className="text-3xl font-semibold tracking-tight">故事与分镜</h1></header>
      <p className="text-sm text-muted-foreground">当前阶段：故事设计 · {missing}</p>
      {props.loading ? <FeedbackState description="正在恢复来源、剧本和分镜版本。" state="loading" title="正在加载故事" /> : null}
      {props.error ? <FeedbackState description="服务端事实未改变，请重新读取。" details={props.error} state="error" title="故事读取失败" /> : null}
      {props.conflictStage ? (
        <Alert variant="destructive"><AlertTitle>{props.conflictStage}的服务端版本已更新</AlertTitle><AlertDescription>请重新读取后再次确认</AlertDescription></Alert>
      ) : null}
      <Card>
        <CardHeader><CardTitle>获权来源</CardTitle><CardDescription>正文必须为 300–3000 个规范化代码点并包含汉字。</CardDescription></CardHeader>
        <CardContent className="space-y-5">
          <form className="grid gap-4" onSubmit={submitSource}>
            <div className="space-y-2"><Label htmlFor="source-content">获权故事正文</Label><Textarea id="source-content" maxLength={3000} minLength={300} onChange={(event) => setContent(event.target.value)} required value={content} /></div>
            <div className="space-y-2"><Label htmlFor="rights-basis">权利依据</Label><Select onValueChange={(value) => setRights(value as "original" | "licensed")} value={rights}><SelectTrigger aria-label="权利依据" id="rights-basis"><SelectValue /></SelectTrigger><SelectContent><SelectItem value="original">原创</SelectItem><SelectItem value="licensed">已获许可</SelectItem></SelectContent></Select></div>
            <Button className="w-fit" type="submit">保存来源草稿</Button>
          </form>
          <Separator />
          <VersionSection noun="来源" onConfirm={props.onConfirmSource} versions={props.sources} />
        </CardContent>
      </Card>
      <Card><CardHeader><CardTitle>剧本</CardTitle><CardDescription>仅确认后的剧本可生成分镜。</CardDescription></CardHeader><CardContent className="space-y-4"><Button disabled={!hasSource} onClick={props.onGenerateScript}>生成剧本提案</Button><VersionSection noun="剧本" onConfirm={props.onConfirmScript} proposal versions={props.scripts} /></CardContent></Card>
      <Card><CardHeader><CardTitle>分镜</CardTitle><CardDescription>联合确认分镜和全部创作资产版本。</CardDescription></CardHeader><CardContent className="space-y-4"><Button disabled={!hasScript} onClick={props.onGenerateStoryboard}>生成分镜提案</Button><VersionSection noun="分镜" onConfirm={props.onConfirmStoryboard} proposal versions={props.storyboards} /></CardContent></Card>
    </main>
  );
}
