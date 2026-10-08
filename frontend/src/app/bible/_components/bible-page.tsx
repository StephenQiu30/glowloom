"use client";

import { ProductShell } from "@/components/layout/product-shell";
import { PageHeading } from "@/components/layout/page-heading";
import { SearchInput } from "@/components/forms/search-input";
import { EmptySearch } from "@/components/feedback/empty-search";
import { demoNotice } from "@/components/feedback/demo-notice";
import { IconButton } from "@/components/controls/icon-button";
import { Placeholder } from "@/components/media/placeholder";
import { LocalDialog } from "@/components/controls/local-dialog";
import { useState } from "react";
import { cn } from "cn";
import {
  Check,
  CircleDashed,
  Plus,
  Pause,
  Play,
  ShieldAlert,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import {
  Card,
  CardHeader,
  CardTitle,
  CardDescription,
  CardContent,
} from "@/components/ui/card";
import { Field, FieldGroup, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { Switch } from "@/components/ui/switch";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";

const bibleEntries = {
  角色: ["林舟", "沈岚", "老船主", "周警官", "阿梨", "船长"],
  场景: ["旧码头 · 夜", "灯塔", "林舟的家", "渔船甲板"],
  道具: [
    "旧信封",
    "雨伞",
    "录音机",
    "怀表",
    "船票",
    "手电",
    "钥匙",
    "照片",
    "指南针",
  ],
};

export function BiblePage() {
  const [category, setCategory] = useState<keyof typeof bibleEntries>("角色");
  const [search, setSearch] = useState("");
  const [unconfirmed, setUnconfirmed] = useState(false);
  const [selected, setSelected] = useState("林舟");
  const [look, setLook] = useState("雨夜造型 · v2");
  const [dialog, setDialog] = useState<string | null>(null);
  const [newName, setNewName] = useState("");
  const [extra, setExtra] = useState<string[]>([]);
  const [generated, setGenerated] = useState(false);
  const [playing, setPlaying] = useState(false);
  const visible = [...bibleEntries[category], ...extra].filter(
    (name, index) =>
      name.includes(search) && (!unconfirmed || ![0, 1, 4].includes(index)),
  );
  return (
    <ProductShell screen="bible" contextual>
      <PageHeading
        title="设定集"
        description="从剧本提取的角色、场景与道具；变更版本独立管理。"
      >
        <Button variant="secondary" onClick={() => setDialog("合并别名")}>
          合并别名
        </Button>
        <Button onClick={() => setDialog("新建条目")}>
          <Plus data-icon="inline-start" />
          新建条目
        </Button>
      </PageHeading>
      <div className="mb-5 flex flex-wrap items-center justify-between gap-4">
        <ToggleGroup
          type="single"
          value={category}
          onValueChange={(value) => {
            if (value) {
              const cat = value as keyof typeof bibleEntries;
              setCategory(cat);
              setSelected(bibleEntries[cat][0]);
              setExtra([]);
            }
          }}
          size="sm"
          aria-label="设定类型"
          className="rounded-lg bg-surface-2 p-1"
        >
          {Object.entries(bibleEntries).map(([name, entries]) => (
            <ToggleGroupItem key={name} value={name}>
              {name} {entries.length}
            </ToggleGroupItem>
          ))}
        </ToggleGroup>
        <div className="flex flex-wrap items-center gap-4">
          <Field orientation="horizontal" className="w-auto">
            <Switch
              id="unconfirmed"
              checked={unconfirmed}
              onCheckedChange={setUnconfirmed}
            />
            <FieldLabel htmlFor="unconfirmed">只看未确认</FieldLabel>
          </Field>
          <SearchInput
            value={search}
            onChange={setSearch}
            placeholder="搜索名称或别名"
            className="w-60"
          />
        </div>
      </div>
      <div className="grid items-start gap-5 lg:grid-cols-[260px_minmax(0,1fr)]">
        <nav className="flex flex-col gap-1" aria-label="设定条目">
          {visible.map((name, index) => (
            <button
              type="button"
              key={name}
              onClick={() => {
                setSelected(name);
                setGenerated(false);
              }}
              aria-current={name === selected ? "true" : undefined}
              className={cn(
                "flex items-center gap-3 rounded-lg p-3 text-left outline-none hover:bg-surface-2 focus-visible:ring-2 focus-visible:ring-ring",
                name === selected && "bg-surface-3",
              )}
            >
              <span
                className="size-11 shrink-0 rounded-md bg-surface-2"
                aria-hidden="true"
              />
              <span className="flex-1 text-sm font-medium">
                {name}
                <span className="mt-1 block text-[10px] font-normal text-muted-foreground">
                  出场 {[6, 5, 3, 2, 2, 1][index] ?? 1} 集 ·{" "}
                  {[3, 2, 1, 1, 2, 1][index] ?? 1} 个造型
                </span>
              </span>
              <span className="text-[10px] text-muted-foreground">
                {index < 2 || index === 4
                  ? "已锁定"
                  : index === 2
                    ? "待定"
                    : "未生成"}
              </span>
            </button>
          ))}
          {!visible.length ? <EmptySearch /> : null}
        </nav>
        <Card variant="panel">
          <CardHeader>
            <div className="flex flex-wrap items-center justify-between gap-3">
              <div>
                <CardTitle className="text-lg">{selected}</CardTitle>
                <CardDescription>
                  别名：{selected === "林舟" ? "小舟、林先生" : selected} ·
                  出场第 1–6 集
                </CardDescription>
              </div>
              <div className="flex gap-2">
                <Button variant="outline" onClick={() => setDialog("上传参考")}>
                  上传参考
                </Button>
                <Button
                  onClick={() => {
                    setGenerated(true);
                    demoNotice("参考定帧位已生成");
                  }}
                >
                  生成空槽位
                </Button>
              </div>
            </div>
          </CardHeader>
          <CardContent className="flex flex-col gap-4">
            <div className="flex items-center gap-2">
              <ToggleGroup
                type="single"
                value={look}
                onValueChange={(value) => value && setLook(value)}
                aria-label="角色造型"
                size="sm"
              >
                {["日常", "雨夜造型 · v2", "回忆"].map((value) => (
                  <ToggleGroupItem key={value} value={value}>
                    {value === "雨夜造型 · v2" ? <Check /> : null}
                    {value}
                  </ToggleGroupItem>
                ))}
              </ToggleGroup>
              <IconButton
                label="新建造型"
                icon={Plus}
                onClick={() => setDialog("新建造型")}
              />
            </div>
            <p className="text-xs text-muted-foreground">参考定帧位</p>
            <div className="grid grid-cols-2 gap-4 xl:grid-cols-4">
              {["正面半身", "侧面", "全身", "表情组"].map((label, index) => (
                <div key={label}>
                  <div className="relative">
                    {index === 2 ? (
                      <div
                        className="grid aspect-[3/4] grid-cols-2 gap-1.5 rounded-lg bg-surface-2 p-1.5"
                        aria-label="全身参考候选 4 张"
                      >
                        {[0, 1, 2, 3].map((candidate) => (
                          <Placeholder key={candidate} showIcon={false} />
                        ))}
                      </div>
                    ) : (
                      <Placeholder
                        className={cn(
                          "aspect-[3/4]",
                          index === 3 && !generated && "bg-surface-1",
                        )}
                        kind={category === "角色" ? "portrait" : "image"}
                        showIcon={index < 2 || generated}
                      >
                        {index < 2 || generated ? (
                          <Badge className="absolute top-2 left-2">
                            <Check />
                            已锁定
                          </Badge>
                        ) : (
                          <div className="flex flex-col items-center gap-2 text-xs text-muted-foreground">
                            <CircleDashed className="size-4" />
                            未生成
                          </div>
                        )}
                      </Placeholder>
                    )}
                  </div>
                  <div className="mt-2 flex items-center justify-between text-xs">
                    <span>{label}</span>
                    {index === 2 ? <Badge variant="muted">候选 4</Badge> : null}
                  </div>
                </div>
              ))}
            </div>
            <div className="grid gap-3 sm:grid-cols-2">
              <Card className="gap-2">
                <CardHeader>
                  <CardTitle>音色</CardTitle>
                </CardHeader>
                <CardContent>
                  <Button
                    variant="ghost"
                    size="sm"
                    onClick={() => {
                      setPlaying(!playing);
                      demoNotice(playing ? "试听已暂停" : "音色试听演示");
                    }}
                  >
                    <span className="flex size-7 items-center justify-center rounded-full bg-surface-4">
                      {playing ? (
                        <Pause className="size-3 fill-current" />
                      ) : (
                        <Play className="size-3 fill-current" />
                      )}
                    </span>
                    {playing ? "暂停试听" : "[音色名称]"}
                  </Button>
                </CardContent>
              </Card>
              <Card>
                <CardHeader>
                  <div className="flex items-center justify-between">
                    <CardTitle>真人授权</CardTitle>
                    <Badge variant="warning">
                      <ShieldAlert />
                      待声明
                    </Badge>
                  </div>
                </CardHeader>
                <CardContent>
                  <p className="text-[11px] leading-5 text-muted-foreground">
                    上传的声音来自真人时，需完成授权声明才能进入生成队列。
                  </p>
                  <Button
                    variant="link"
                    size="xs"
                    onClick={() => setDialog("真人授权声明")}
                  >
                    填写声明
                  </Button>
                </CardContent>
              </Card>
            </div>
          </CardContent>
        </Card>
      </div>
      <LocalDialog
        open={dialog !== null}
        onOpenChange={(open) => !open && setDialog(null)}
        title={dialog ?? "编辑设定"}
        onConfirm={() => {
          if (dialog === "新建条目" && newName.trim()) {
            setExtra([...extra, newName]);
            setSelected(newName);
            setNewName("");
          }
          demoNotice("设定已更新");
        }}
      >
        <FieldGroup>
          <Field>
            <FieldLabel htmlFor="bible-edit">
              {dialog === "合并别名" ? "别名" : "名称或说明"}
            </FieldLabel>
            <Input
              id="bible-edit"
              value={newName}
              onChange={(e) => setNewName(e.target.value)}
              placeholder={selected}
            />
          </Field>
          {dialog === "真人授权声明" ? (
            <Field>
              <FieldLabel htmlFor="permission-description">授权说明</FieldLabel>
              <Textarea
                id="permission-description"
                placeholder="本地演示，无真实授权提交"
              />
            </Field>
          ) : null}
        </FieldGroup>
      </LocalDialog>
    </ProductShell>
  );
}
