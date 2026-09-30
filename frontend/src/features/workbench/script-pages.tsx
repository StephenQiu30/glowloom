"use client";

import Link from "next/link";
import { useState } from "react";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Textarea } from "@/components/ui/textarea";
import { Field, FieldLabel, FieldGroup } from "@/components/ui/field";
import { sourceScript, episodes, shots } from "./data";
import {
  PageHeading,
  Panel,
  DataTable,
  DraftNotice,
  QuotePreview,
  usePreviewQuery,
} from "./poc-components";

export function ScriptPage({ projectId }: { projectId: string }) {
  const [draft, setDraft] = useState(sourceScript);
  const { readOnly } = usePreviewQuery();
  return (
    <div className="space-y-8">
      <PageHeading
        title="故事从这里开始。"
        description="导入或编辑剧本，先检查单集与场景，再进入资产和分镜。"
        action={
          <QuotePreview
            label="预览解析报价"
            kind="text"
            target="剧本 · 单集解析"
            disabled={readOnly}
          />
        }
      />
      <div className="grid gap-7 xl:grid-cols-[1.4fr_1fr]">
        <Panel title="剧本原文" description="样例版本 v2 · 当前编辑为本页草稿">
          <FieldGroup>
            <Field>
              <FieldLabel htmlFor="script-draft">剧本文本</FieldLabel>
              <Textarea
                id="script-draft"
                value={draft}
                onChange={(e) => setDraft(e.target.value)}
                disabled={readOnly}
                className="min-h-96 leading-7"
              />
            </Field>
            <DraftNotice changed={draft !== sourceScript} />
            <div className="flex gap-2">
              <Button
                variant="secondary"
                disabled={readOnly || draft === sourceScript}
                onClick={() => setDraft(sourceScript)}
              >
                放弃草稿
              </Button>
              <Button disabled>保存服务待接入</Button>
            </div>
          </FieldGroup>
        </Panel>
        <Panel
          title="单集与解析"
          description="解析结果需人工核对，不自动锁定资产。"
        >
          <ul className="space-y-5">
            {episodes.map((e) => (
              <li key={e.id}>
                <Link
                  className="block rounded-lg p-3 hover:bg-muted"
                  href={`/projects/${projectId}/episodes/${e.id}/parse`}
                >
                  <div className="flex items-center justify-between gap-2">
                    <h3 className="text-sm font-medium">{e.name}</h3>
                    <Badge variant="secondary">{e.stages[0]}</Badge>
                  </div>
                  <p className="mt-2 text-xs text-muted-foreground">
                    预估时长 {e.duration} · 查看结构化结果 →
                  </p>
                </Link>
              </li>
            ))}
          </ul>
          <p className="mt-7 text-xs leading-6 text-muted-foreground">
            版本历史：v2 当前版本 / v1
            初稿。文件导入、解析和版本持久化将在服务阶段接入。
          </p>
        </Panel>
      </div>
    </div>
  );
}
export function ParsePage({
  projectId,
  episodeId,
}: {
  projectId: string;
  episodeId: string;
}) {
  const episode = episodes.find((e) => e.id === episodeId)!;
  const { params, set } = usePreviewQuery();
  const selected = shots.find((s) => s.id === params.get("scene")) ?? shots[0];
  return (
    <div className="space-y-8">
      <PageHeading
        title="单集解析"
        description={`${episode.name} · 原文与结构化结果对照。此处展示同一套样例解析结构。`}
        action={
          <Button asChild>
            <Link href={`/projects/${projectId}/episodes/${episodeId}/assets`}>
              准备单集资产
            </Link>
          </Button>
        }
      />
      <div className="grid gap-7 xl:grid-cols-2">
        <Panel
          title="原文片段"
          description="选择右侧场景，查看关联的动作与台词。"
        >
          <p className="text-xs text-muted-foreground">
            场景 {selected.number} · {selected.scene}
          </p>
          <p className="mt-5 rounded-lg bg-muted p-5 text-sm leading-8">
            {selected.prompt}
            <br />
            <mark className="rounded bg-accent px-1 text-accent-foreground">
              {selected.character}：{selected.dialogue}
            </mark>
          </p>
          <p className="mt-5 text-xs text-muted-foreground">
            引用原文：v2 / 场景 {selected.number}；真实服务将提供准确段落位置。
          </p>
        </Panel>
        <Panel title="结构化场景">
          <ul className="space-y-3">
            {shots.map((s) => (
              <li key={s.id}>
                <Button
                  className="h-auto w-full justify-start px-4 py-4 text-left"
                  variant={s.id === selected.id ? "secondary" : "ghost"}
                  aria-pressed={s.id === selected.id}
                  onClick={() => set("scene", s.id)}
                >
                  <span className="flex flex-col gap-2">
                    <span>
                      {s.number} · {s.scene}
                    </span>
                    <span className="text-xs font-normal text-muted-foreground">
                      {s.character} · 动作 1 条 · 台词 1 条
                    </span>
                  </span>
                </Button>
              </li>
            ))}
          </ul>
        </Panel>
      </div>
      <Panel title="人工核对">
        <DataTable
          caption="解析核对项"
          columns={["项目", "样例结果", "需要确认"]}
          rows={[
            ["场景", "3 个", "场景时段与空间连续性"],
            ["人物", "林夏 / 老陈", "同名人物与发声人"],
            ["主线道具", "泛黄信封", "跨场景引用与版本"],
          ]}
        />
      </Panel>
    </div>
  );
}
