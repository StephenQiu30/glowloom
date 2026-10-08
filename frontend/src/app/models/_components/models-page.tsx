"use client";

import { ProductShell } from "@/components/layout/product-shell";
import { PageHeading } from "@/components/layout/page-heading";
import { StatusBadge } from "@/components/feedback/status-badge";
import { Choice } from "@/components/forms/choice";
import { demoNotice } from "@/components/feedback/demo-notice";
import { LocalDialog } from "@/components/controls/local-dialog";
import { useState } from "react";
import { Plus } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import {
  Card,
  CardHeader,
  CardTitle,
  CardDescription,
  CardContent,
} from "@/components/ui/card";
import {
  Table,
  TableHeader,
  TableBody,
  TableRow,
  TableCell,
  TableHead,
} from "@/components/ui/table";
import { FieldGroup, Field, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { Tabs, TabsList, TabsTrigger, TabsContent } from "@/components/ui/tabs";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";
import { Switch } from "@/components/ui/switch";
import { Slider } from "@/components/ui/slider";

const modelRows = [
  {
    name: "[图像模型]",
    id: "ark.image.auto",
    provider: "火山方舟",
    kind: "生图",
    capability: "生图 · 改图",
    status: "启用",
    version: "v3",
  },
  {
    name: "[视频模型 A]",
    id: "ark.video.a",
    provider: "火山方舟",
    kind: "生视频",
    capability: "生视频",
    status: "启用",
    version: "v3",
  },
  {
    name: "[视频模型 B]",
    id: "ark.video.b",
    provider: "火山方舟",
    kind: "生视频",
    capability: "生视频 · 全能参考",
    status: "启用",
    version: "v2",
  },
  {
    name: "[配音模型]",
    id: "minimax.tts",
    provider: "MiniMax",
    kind: "配音",
    capability: "配音",
    status: "凭据失效",
    version: "v2",
  },
  {
    name: "[音色复刻]",
    id: "minimax.voice",
    provider: "MiniMax",
    kind: "配音",
    capability: "音色复刻",
    status: "凭据失效",
    version: "v1",
  },
  {
    name: "[文本模型]",
    id: "ark.text",
    provider: "火山方舟",
    kind: "文本 / Agent",
    capability: "文本 · Agent",
    status: "启用",
    version: "v5",
  },
  {
    name: "[境外视频模型]",
    id: "intl.video",
    provider: "[境外供应商]",
    kind: "生视频",
    capability: "生视频",
    status: "停用",
    version: "v1",
  },
];

export function ModelsPage() {
  const [kind, setKind] = useState("全部 12");
  const [selected, setSelected] = useState(1);
  const [duration, setDuration] = useState("5s");
  const [resolution, setResolution] = useState("1080p");
  const [motionAmount, setMotionAmount] = useState([60]);
  const [fixedCamera, setFixedCamera] = useState(false);
  const [published, setPublished] = useState(false);
  const [enabled, setEnabled] = useState(true);
  const [creating, setCreating] = useState(false);
  const model = modelRows[selected];
  return (
    <ProductShell screen="models" contextual>
      <PageHeading
        title="模型注册表"
        description="能力、上限与参数均由注册配置决定；每次编辑产生新版本。"
      >
        <Button onClick={() => setCreating(true)}>
          <Plus data-icon="inline-start" />
          新增模型
        </Button>
      </PageHeading>
      <ToggleGroup
        type="single"
        value={kind}
        onValueChange={(value) => value && setKind(value)}
        className="mb-5 flex-wrap"
        variant="pill"
        size="sm"
        aria-label="模型能力"
      >
        {["全部 12", "生图", "生视频", "配音", "文本 / Agent"].map((value) => (
          <ToggleGroupItem key={value} value={value}>
            {value}
          </ToggleGroupItem>
        ))}
      </ToggleGroup>
      <div className="grid items-start gap-4 xl:grid-cols-[minmax(0,1fr)_380px]">
        <div className="overflow-hidden rounded-lg bg-surface-1">
          <Table>
            <TableHeader>
              <TableRow>
                {["模型", "供应商", "能力", "区域", "状态", "版本"].map(
                  (head) => (
                    <TableHead key={head}>{head}</TableHead>
                  ),
                )}
              </TableRow>
            </TableHeader>
            <TableBody>
              {modelRows.map((row, index) =>
                kind === "全部 12" || kind === row.kind ? (
                  <TableRow
                    key={row.id}
                    data-state={selected === index ? "selected" : undefined}
                  >
                    <TableCell className="py-2.5">
                      <Button
                        variant="link"
                        size="sm"
                        onClick={() => {
                          setSelected(index);
                          setPublished(false);
                          setEnabled(row.status !== "停用");
                        }}
                      >
                        {row.name}
                      </Button>
                      <p className="font-mono text-[10px] text-muted-foreground">
                        {row.id}
                      </p>
                    </TableCell>
                    <TableCell>{row.provider}</TableCell>
                    <TableCell>{row.capability}</TableCell>
                    <TableCell>{index === 6 ? "境外" : "境内"}</TableCell>
                    <TableCell>
                      <StatusBadge value={row.status} />
                    </TableCell>
                    <TableCell>{row.version}</TableCell>
                  </TableRow>
                ) : null,
              )}
            </TableBody>
          </Table>
        </div>
        <Card variant="panel">
          <CardHeader>
            <div className="flex flex-wrap justify-between gap-2">
              <CardTitle>{model.name}</CardTitle>
              <StatusBadge value={published ? "已发布" : "未发布修改"} />
            </div>
            <CardDescription>{model.id} · v4 草稿</CardDescription>
          </CardHeader>
          <CardContent>
            <Tabs defaultValue="form">
              <TabsList>
                <TabsTrigger value="form">表单预览</TabsTrigger>
                <TabsTrigger value="json">JSON</TabsTrigger>
                <TabsTrigger value="history">版本差异</TabsTrigger>
              </TabsList>
              <TabsContent value="form" className="pt-5">
                <FieldGroup className="gap-5">
                  <Field>
                    <FieldLabel>支持模式</FieldLabel>
                    <div className="flex flex-wrap gap-2">
                      <Badge variant="secondary">图生视频</Badge>
                      <Badge variant="secondary">首尾帧</Badge>
                      <Badge variant="muted">全能参考 · 不支持</Badge>
                    </div>
                  </Field>
                  <Field>
                    <FieldLabel>参考上限</FieldLabel>
                    <p className="text-xs text-muted-foreground">
                      图片 9 · 视频 3 · 音频 3
                    </p>
                  </Field>
                  <Field className="rounded-lg bg-background p-3.5">
                    <FieldLabel>生成面板预览</FieldLabel>
                    <div className="grid grid-cols-2 gap-3">
                      <Field>
                        <FieldLabel>时长</FieldLabel>
                        <Choice
                          value={duration}
                          onChange={setDuration}
                          label="时长"
                          options={["5s", "10s", "15s"]}
                          className="w-full"
                        />
                      </Field>
                      <Field>
                        <FieldLabel>分辨率</FieldLabel>
                        <Choice
                          value={resolution}
                          onChange={setResolution}
                          label="分辨率"
                          options={["720p", "1080p"]}
                          className="w-full"
                        />
                      </Field>
                      <Field>
                        <FieldLabel htmlFor="motion-amount">
                          运动幅度
                        </FieldLabel>
                        <Slider
                          id="motion-amount"
                          aria-label="运动幅度"
                          value={motionAmount}
                          onValueChange={setMotionAmount}
                          max={100}
                          step={1}
                        />
                      </Field>
                      <Field>
                        <FieldLabel htmlFor="fixed-camera">固定镜头</FieldLabel>
                        <Switch
                          id="fixed-camera"
                          checked={fixedCamera}
                          onCheckedChange={setFixedCamera}
                        />
                      </Field>
                    </div>
                  </Field>
                </FieldGroup>
              </TabsContent>
              <TabsContent value="json">
                <pre className="mt-4 overflow-auto rounded-lg bg-background p-3 text-xs">
                  {JSON.stringify(
                    {
                      id: model.id,
                      duration,
                      resolution,
                      motionAmount: motionAmount[0],
                      fixedCamera,
                      enabled,
                    },
                    null,
                    2,
                  )}
                </pre>
              </TabsContent>
              <TabsContent value="history">
                <p className="py-5 text-xs text-muted-foreground">
                  v3 → v4：默认分辨率更新为 {resolution}，时长 {duration}。
                </p>
              </TabsContent>
            </Tabs>
            <div className="mt-6 flex gap-2">
              <Button
                className="flex-1"
                onClick={() => {
                  setPublished(true);
                  demoNotice("模型 v4 已发布");
                }}
              >
                发布 v4
              </Button>
              <Button
                variant="outline"
                onClick={() => {
                  setEnabled(!enabled);
                  demoNotice(enabled ? "模型已停用" : "模型已启用");
                }}
              >
                {enabled ? "停用模型" : "启用模型"}
              </Button>
            </div>
          </CardContent>
        </Card>
      </div>
      <LocalDialog
        open={creating}
        onOpenChange={setCreating}
        title="新增模型"
        onConfirm={() => demoNotice("模型草稿已保存")}
      >
        <FieldGroup>
          <Field>
            <FieldLabel htmlFor="model-name">模型名称</FieldLabel>
            <Input id="model-name" placeholder="输入模型名称" />
          </Field>
          <Field>
            <FieldLabel htmlFor="model-id">模型 ID</FieldLabel>
            <Input id="model-id" placeholder="provider.model" />
          </Field>
        </FieldGroup>
      </LocalDialog>
    </ProductShell>
  );
}
