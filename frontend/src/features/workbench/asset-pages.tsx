"use client";

import Image from "next/image";
import Link from "next/link";
import { useState } from "react";
import { Upload, Play, ImageIcon } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Input } from "@/components/ui/input";
import { Field, FieldLabel } from "@/components/ui/field";
import {
  Dialog,
  DialogTrigger,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
} from "@/components/ui/dialog";
import { Tabs, TabsList, TabsTrigger, TabsContent } from "@/components/ui/tabs";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";
import { entities, media } from "./data";
import {
  PageHeading,
  Panel,
  DataTable,
  EmptyMessage,
  usePreviewQuery,
} from "./poc-components";

const categories = {
  characters: "角色",
  scenes: "场景",
  props: "道具",
  voices: "声音",
};
export function BiblePage({ projectId }: { projectId: string }) {
  const { params, set } = usePreviewQuery();
  const requestedCategory = params.get("category") ?? "";
  const category = Object.keys(entities).includes(requestedCategory)
    ? requestedCategory
    : "characters";
  return (
    <div className="space-y-8">
      <PageHeading
        title="让世界保持一致。"
        description="角色、场景、道具与声音共同定义故事的世界。锁定资产版本后，再用于单集与镜头。"
      />
      <Tabs value={category} onValueChange={(v) => set("category", v)}>
        <TabsList variant="line">
          {Object.entries(categories).map(([key, label]) => (
            <TabsTrigger key={key} value={key}>
              {label}
            </TabsTrigger>
          ))}
        </TabsList>
        {Object.entries(entities).map(([key, list]) => (
          <TabsContent key={key} value={key} className="mt-6">
            <div className="grid gap-6 md:grid-cols-2 xl:grid-cols-3">
              {list.map((e) => (
                <Panel key={e.id} title={e.name} description={e.detail}>
                  <div className="mb-5 flex aspect-[4/3] items-center justify-center rounded-lg bg-muted">
                    <span className="text-4xl font-semibold text-muted-foreground">
                      {e.name.slice(0, 2)}
                    </span>
                  </div>
                  <div className="flex items-center justify-between">
                    <Badge variant="secondary">{e.status}</Badge>
                    <span className="text-xs text-muted-foreground">
                      {e.version}
                    </span>
                  </div>
                  <Dialog>
                    <DialogTrigger asChild>
                      <Button variant="ghost" className="mt-4">
                        查看资产详情
                      </Button>
                    </DialogTrigger>
                    <DialogContent className="sm:max-w-lg">
                      <DialogHeader>
                        <DialogTitle>{e.name}</DialogTitle>
                        <DialogDescription>
                          {e.detail} · 样例资产
                        </DialogDescription>
                      </DialogHeader>
                      <DataTable
                        caption="资产详情"
                        columns={["字段", "内容"]}
                        rows={[
                          ["版本", e.version],
                          ["选定状态", e.status],
                          [
                            "授权",
                            key === "voices"
                              ? "需要声线授权证据，当前未提供"
                              : "样例素材，仅用于界面",
                          ],
                          ["引用范围", "第一集 / 镜头 01"],
                        ]}
                      />
                      <Button variant="secondary" asChild>
                        <Link
                          href={`/projects/${projectId}/episodes/ep-01/assets`}
                        >
                          查看单集资产
                        </Link>
                      </Button>
                      <Button disabled>锁定与版本服务待接入</Button>
                    </DialogContent>
                  </Dialog>
                </Panel>
              ))}
            </div>
          </TabsContent>
        ))}
      </Tabs>
    </div>
  );
}
export function AssetsPage({
  projectId,
  episodeId,
}: {
  projectId: string;
  episodeId: string;
}) {
  return (
    <div className="space-y-8">
      <PageHeading
        title="准备这一集的资产。"
        description="核对引用与锁定版本。素材缺失时先补齐，再进入分镜与生成。"
        action={
          <Button asChild>
            <Link href={`/projects/${projectId}/episodes/${episodeId}/shots`}>
              进入分镜
            </Link>
          </Button>
        }
      />
      <Panel title="资产清单">
        <DataTable
          caption="单集资产引用"
          columns={["资产", "类型", "版本", "状态", "关联"]}
          rows={[
            [
              "林夏",
              "角色 / 主服装",
              "v3",
              <Badge key="lin" variant="secondary">
                已锁定
              </Badge>,
              "镜头 01 / 02 / 03",
            ],
            ["旧港码头", "场景", "v2", "已锁定", "镜头 01"],
            ["泛黄信封", "道具", "v2", "已锁定", "镜头 02"],
            [
              "邮局内景",
              "场景",
              "v1",
              <Badge key="post" variant="destructive">
                待选定
              </Badge>,
              "镜头 02",
            ],
            ["林夏声线", "声音", "v1", "授权待核对", "配音 01 / 03"],
          ]}
        />
      </Panel>
      <Panel
        title="待处理事项"
        description="场景未选定 / 声线授权缺失不会在界面上自动解除。"
      >
        <div className="flex flex-wrap gap-3">
          <Button variant="secondary" asChild>
            <Link href={`/projects/${projectId}/bible?category=scenes`}>
              核对场景
            </Link>
          </Button>
          <Button variant="secondary" asChild>
            <Link href={`/projects/${projectId}/bible?category=voices`}>
              核对声音授权
            </Link>
          </Button>
        </div>
      </Panel>
    </div>
  );
}
export function MediaPage() {
  const { params, set, readOnly } = usePreviewQuery();
  const q = params.get("q") ?? "";
  const type = ["image", "video", "audio"].includes(params.get("type") ?? "")
    ? params.get("type")!
    : "all";
  const list = media.filter(
    (m) => (type === "all" || type === m.type) && m.name.includes(q),
  );
  const [fileName, setFileName] = useState("");
  return (
    <div className="space-y-8">
      <PageHeading
        title="所有素材，各得其所。"
        description="按素材身份追溯来源、审核和引用。当前使用内置样例预览，不发送文件。"
        action={
          <Dialog>
            <DialogTrigger asChild>
              <Button disabled={readOnly}>
                <Upload data-icon="inline-start" />
                上传预览
              </Button>
            </DialogTrigger>
            <DialogContent>
              <DialogHeader>
                <DialogTitle>上传素材预览</DialogTitle>
                <DialogDescription>
                  仅显示所选文件名，不读取文件内容，不发起上传。
                </DialogDescription>
              </DialogHeader>
              <Field>
                <FieldLabel htmlFor="upload-poc">选择本地素材</FieldLabel>
                <Input
                  id="upload-poc"
                  type="file"
                  accept="image/*,video/*,audio/*"
                  onChange={(e) => setFileName(e.target.files?.[0]?.name ?? "")}
                />
              </Field>
              {fileName && (
                <p role="status">已选择：{fileName}。上传服务待接入。</p>
              )}
              <Button disabled>上传服务待接入</Button>
            </DialogContent>
          </Dialog>
        }
      />
      <div className="flex flex-wrap items-center justify-between gap-4">
        <ToggleGroup
          type="single"
          aria-label="素材类型"
          value={type}
          onValueChange={(v) => {
            if (v) set("type", v);
          }}
        >
          <ToggleGroupItem value="all">全部</ToggleGroupItem>
          <ToggleGroupItem value="image">图像</ToggleGroupItem>
          <ToggleGroupItem value="video">视频</ToggleGroupItem>
          <ToggleGroupItem value="audio">音频</ToggleGroupItem>
        </ToggleGroup>
        <Input
          aria-label="搜索素材"
          className="sm:max-w-xs"
          placeholder="搜索素材…"
          value={q}
          onChange={(e) => set("q", e.target.value)}
        />
      </div>
      {list.length === 0 ? (
        <EmptyMessage title="没有匹配的素材" />
      ) : (
        <div className="grid gap-6 md:grid-cols-2 xl:grid-cols-3">
          {list.map((m) => (
            <Panel key={m.id} title={m.name}>
              <div className="relative mb-5 aspect-video overflow-hidden rounded-lg bg-muted">
                <Image
                  src="/poc/scene.svg"
                  alt="内置素材样例：山景插画，非真实生成产物"
                  fill
                  sizes="(max-width: 768px) 90vw, 30vw"
                  className="object-cover"
                />
                {m.type === "video" ? (
                  <Play
                    aria-hidden="true"
                    className="absolute top-3 right-3 size-5 text-background"
                  />
                ) : (
                  <ImageIcon
                    aria-hidden="true"
                    className="absolute top-3 right-3 size-5 text-background"
                  />
                )}
              </div>
              <div className="flex flex-wrap items-center justify-between gap-2">
                <Badge variant="secondary">{m.status}</Badge>
                <span className="text-xs text-muted-foreground">
                  {m.refs} 个引用
                </span>
              </div>
              <Dialog>
                <DialogTrigger asChild>
                  <Button variant="ghost" className="mt-3">
                    预览与来源
                  </Button>
                </DialogTrigger>
                <DialogContent className="sm:max-w-2xl">
                  <DialogHeader>
                    <DialogTitle>{m.name}</DialogTitle>
                    <DialogDescription>
                      {m.origin} · {m.id} · 所有媒体是内置 PoC 样例。
                    </DialogDescription>
                  </DialogHeader>
                  {m.type === "video" ? (
                    <video
                      src="/poc/preview.mp4"
                      poster="/poc/scene.svg"
                      controls
                      preload="none"
                      className="aspect-video w-full rounded-lg"
                      aria-label="内置视频样例"
                    />
                  ) : (
                    <Image
                      width={560}
                      height={340}
                      src="/poc/scene.svg"
                      alt="内置山景插画样例"
                      className="w-full rounded-lg"
                    />
                  )}
                  <DataTable
                    caption="素材来源与引用"
                    columns={["来源", "审核", "引用"]}
                    rows={[
                      [m.origin, m.status, `${m.refs} 个业务引用（样例）`],
                    ]}
                  />
                </DialogContent>
              </Dialog>
            </Panel>
          ))}
        </div>
      )}
    </div>
  );
}
