"use client";

import { shotFixtures } from "@/components/storyboard/mock-shots";
import { ProductShell } from "@/components/layout/product-shell";
import { PageHeading } from "@/components/layout/page-heading";
import { Choice } from "@/components/forms/choice";
import { demoNotice } from "@/components/feedback/demo-notice";
import { Placeholder } from "@/components/media/placeholder";
import { LocalDialog } from "@/components/controls/local-dialog";
import { useState } from "react";
import Link from "next/link";
import { Grid2X2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Card, CardHeader, CardTitle, CardContent } from "@/components/ui/card";
import { Checkbox } from "@/components/ui/checkbox";
import { Field, FieldGroup, FieldLabel } from "@/components/ui/field";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";
import {
  Table,
  TableHeader,
  TableBody,
  TableRow,
  TableCell,
  TableHead,
} from "@/components/ui/table";
import { screenHref } from "@/components/layout/routes";

function ShotBadge({ label, status }: { label: string; status: string }) {
  return (
    <Badge
      variant={
        status === "已选定"
          ? "default"
          : status === "生成中"
            ? "warning"
            : status === "失败"
              ? "destructive"
              : "muted"
      }
    >
      {label} · {status}
    </Badge>
  );
}

export function StoryboardPage() {
  const [selected, setSelected] = useState(["02-04", "02-05", "02-06"]);
  const [view, setView] = useState("故事板");
  const [status, setStatus] = useState("状态：全部");
  const [mode, setMode] = useState("模式：全部");
  const [batch, setBatch] = useState(false);
  const [generated, setGenerated] = useState<string[]>([]);
  const [duration, setDuration] = useState("4.0s");
  const shots = shotFixtures.filter(
    (shot) =>
      (status.endsWith("全部") || status.includes(shot.status)) &&
      (mode.endsWith("全部") || mode.includes(shot.mode)),
  );
  return (
    <ProductShell screen="storyboard" contextual>
      <PageHeading
        title="第 3 集 · 分镜"
        description="场 2 · 旧码头 · 12 个镜头"
      >
        <ToggleGroup
          type="single"
          value={view}
          onValueChange={(value) => value && setView(value)}
          aria-label="分镜视图"
        >
          <ToggleGroupItem value="镜头表">镜头表</ToggleGroupItem>
          <ToggleGroupItem value="故事板">故事板</ToggleGroupItem>
        </ToggleGroup>
        <Button variant="outline" asChild>
          <Link href={screenHref("canvas")}>
            <Grid2X2 data-icon="inline-start" />
            在画布中打开
          </Link>
        </Button>
      </PageHeading>
      <div className="mb-6 flex flex-wrap items-center gap-3 rounded-lg bg-surface-1 p-3">
        <span className="text-xs">已选 {selected.length} 项</span>
        <Button variant="ghost" size="xs" onClick={() => setSelected([])}>
          取消选择
        </Button>
        <Choice
          value={status}
          onChange={setStatus}
          label="镜头状态"
          options={[
            "状态：全部",
            "状态：已选定",
            "状态：未生成",
            "状态：生成中",
            "状态：失败",
          ]}
        />
        <Choice
          value={mode}
          onChange={setMode}
          label="生成模式"
          options={[
            "模式：全部",
            "模式：图生视频",
            "模式：首尾帧",
            "模式：全能参考",
          ]}
        />
        <div className="ml-auto flex gap-2">
          <Button
            variant="outline"
            disabled={!selected.length}
            onClick={() => setBatch(true)}
          >
            批量改参数
          </Button>
          <Button
            disabled={!selected.length}
            onClick={() => {
              setGenerated([...new Set([...generated, ...selected])]);
              demoNotice(`已为 ${selected.length} 个镜头创建演示任务`);
            }}
          >
            批量生成 {selected.length} 项
          </Button>
        </div>
      </div>
      {view === "故事板" ? (
        <div className="grid grid-cols-2 gap-x-5 gap-y-7 sm:grid-cols-3 xl:grid-cols-6">
          {shots.map((shot) => (
            <Card
              variant="project"
              key={shot.id}
              data-selected={selected.includes(shot.id) || undefined}
            >
              <div className="relative">
                <Link
                  href={screenHref("shot")}
                  aria-label={`打开镜头 ${shot.id}`}
                >
                  <Placeholder className="aspect-[9/16]">
                    <span className="absolute right-2 bottom-2 rounded-sm bg-background px-1 font-mono text-[10px]">
                      {shot.duration}
                    </span>
                  </Placeholder>
                </Link>
                <Checkbox
                  aria-label={`选择镜头 ${shot.id}`}
                  checked={selected.includes(shot.id)}
                  onCheckedChange={(checked) =>
                    setSelected((current) =>
                      checked
                        ? [...current, shot.id]
                        : current.filter((id) => id !== shot.id),
                    )
                  }
                  className="absolute top-2 left-2 opacity-0 group-hover/card:opacity-100 focus-visible:opacity-100 data-[state=checked]:opacity-100"
                />
              </div>
              <CardHeader className="mt-2">
                <CardTitle>
                  <div className="flex items-center justify-between gap-1">
                    <Link
                      href={screenHref("shot")}
                      className="font-mono text-xs"
                    >
                      {shot.id}
                    </Link>
                    <span className="text-[10px] font-normal text-muted-foreground">
                      {shot.mode}
                    </span>
                  </div>
                </CardTitle>
              </CardHeader>
              <CardContent className="mt-2 flex flex-wrap gap-1">
                <ShotBadge
                  label="画面"
                  status={generated.includes(shot.id) ? "生成中" : shot.status}
                />
                <ShotBadge label="视频" status={shot.video} />
              </CardContent>
            </Card>
          ))}
        </div>
      ) : (
        <div className="rounded-lg bg-surface-1">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>选择</TableHead>
                <TableHead>编号</TableHead>
                <TableHead>画面描述</TableHead>
                <TableHead>时长</TableHead>
                <TableHead>状态</TableHead>
                <TableHead />
              </TableRow>
            </TableHeader>
            <TableBody>
              {shots.map((shot) => (
                <TableRow key={shot.id}>
                  <TableCell>
                    <Checkbox
                      aria-label={`选择镜头 ${shot.id}`}
                      checked={selected.includes(shot.id)}
                      onCheckedChange={(checked) =>
                        setSelected((current) =>
                          checked
                            ? [...current, shot.id]
                            : current.filter((id) => id !== shot.id),
                        )
                      }
                    />
                  </TableCell>
                  <TableCell className="font-mono">{shot.id}</TableCell>
                  <TableCell>{shot.text}</TableCell>
                  <TableCell>{shot.duration}</TableCell>
                  <TableCell>
                    <ShotBadge label="画面" status={shot.status} />
                  </TableCell>
                  <TableCell>
                    <Button asChild variant="ghost">
                      <Link href={screenHref("shot")}>打开</Link>
                    </Button>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </div>
      )}
      <LocalDialog
        open={batch}
        onOpenChange={setBatch}
        title={`修改 ${selected.length} 个镜头参数`}
        onConfirm={() => demoNotice("镜头参数已更新")}
      >
        <FieldGroup>
          <Field>
            <FieldLabel>时长</FieldLabel>
            <Choice
              value={duration}
              onChange={setDuration}
              options={["3.0s", "4.0s", "5.0s", "10.0s"]}
              label="批量镜头时长"
            />
          </Field>
        </FieldGroup>
      </LocalDialog>
    </ProductShell>
  );
}
