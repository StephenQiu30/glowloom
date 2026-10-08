"use client";

import { demoNotice } from "@/components/feedback/demo-notice";
import { ProductShell } from "@/components/layout/product-shell";
import { Choice } from "@/components/forms/choice";
import { SearchInput } from "@/components/forms/search-input";
import { Placeholder } from "@/components/media/placeholder";
import { MoreMenu } from "@/components/controls/more-menu";
import { cn } from "cn";
import Link from "next/link";
import { useRef, useState } from "react";
import { ArrowRight, Grid2X2, List, Upload } from "lucide-react";
import { useRouter } from "next/navigation";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardHeader,
  CardTitle,
  CardDescription,
  CardContent,
} from "@/components/ui/card";
import { Field, FieldGroup, FieldLabel } from "@/components/ui/field";
import { InputGroup, InputGroupInput } from "@/components/ui/input-group";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";
import {
  Empty,
  EmptyHeader,
  EmptyTitle,
  EmptyDescription,
} from "@/components/ui/empty";
import { screenHref } from "@/components/layout/routes";

const initialProjects = [
  { name: "雾港来信", style: "写实电影感", ratio: "9:16" },
  { name: "第七个夏天", style: "日系胶片", ratio: "9:16" },
  { name: "镜中客", style: "国风水墨", ratio: "9:16" },
  { name: "霓虹追缉", style: "赛博霓虹", ratio: "16:9" },
  { name: "小满", style: "温暖插画", ratio: "9:16" },
  { name: "未命名项目", style: "未设风格", ratio: "16:9" },
];

export function ProjectsPage() {
  const router = useRouter();
  const inputRef = useRef<HTMLInputElement>(null);
  const [name, setName] = useState("");
  const [ratio, setRatio] = useState("9:16");
  const [style, setStyle] = useState("写实电影感");
  const [script, setScript] = useState("");
  const [search, setSearch] = useState("");
  const [sort, setSort] = useState("最近打开");
  const [view, setView] = useState("grid");
  const [projects, setProjects] = useState(initialProjects);
  const visible = projects.filter((project) => project.name.includes(search));
  if (sort === "项目名称")
    visible.sort((a, b) => a.name.localeCompare(b.name, "zh-CN"));
  function create() {
    demoNotice(`已创建${name.trim() || "未命名项目"}`);
    router.push(screenHref("analytics"));
  }
  return (
    <ProductShell screen="home" fullBleed>
      <div className="mx-auto max-w-[1440px] px-5 pb-16 md:px-12 lg:px-[68px]">
        <section className="mx-auto max-w-[760px] pt-11 pb-16">
          <h1 className="text-center text-[32px] leading-[48px] font-semibold tracking-tight md:text-[40px]">
            从一份剧本开始
          </h1>
          <p className="mt-2 mb-7 text-center text-sm text-muted-foreground">
            导入本地剧本，自动分集、抽取设定，再进入画布逐镜生产。
          </p>
          <Card variant="composer">
            <CardContent className="p-4">
              <FieldGroup className="gap-3">
                <Field>
                  <FieldLabel htmlFor="project-name" className="sr-only">
                    项目名称
                  </FieldLabel>
                  <InputGroup variant="quiet" className="h-10">
                    <InputGroupInput
                      id="project-name"
                      placeholder="给新项目起个名字"
                      value={name}
                      onChange={(e) => setName(e.target.value)}
                      maxLength={50}
                    />
                  </InputGroup>
                </Field>
                <Field>
                  <FieldLabel htmlFor="project-script" className="sr-only">
                    上传剧本
                  </FieldLabel>
                  <input
                    ref={inputRef}
                    id="project-script"
                    type="file"
                    accept=".txt,.md,.docx"
                    className="sr-only"
                    onChange={(event) => {
                      const file = event.target.files?.[0];
                      if (file) {
                        setScript(file.name);
                        demoNotice("剧本已加入演示项目");
                      }
                    }}
                  />
                  <Button
                    variant="upload"
                    onDragOver={(event) => event.preventDefault()}
                    onDrop={(event) => {
                      event.preventDefault();
                      const file = event.dataTransfer.files[0];
                      if (file) {
                        setScript(file.name);
                        demoNotice("剧本已加入演示项目");
                      }
                    }}
                    className="h-[76px] w-full justify-start gap-4 px-4"
                    onClick={() => inputRef.current?.click()}
                  >
                    <span className="flex size-10 shrink-0 items-center justify-center rounded-xl bg-surface-3">
                      <Upload />
                    </span>
                    <span className="text-left">
                      <span className="block">
                        {script || "拖入剧本文件，或从本地选择"}
                      </span>
                      <span className="mt-1 block text-xs text-muted-foreground">
                        .txt · .md · .docx，文件只保存在本机
                      </span>
                    </span>
                  </Button>
                </Field>
              </FieldGroup>
              <div className="mt-3 flex flex-wrap items-center gap-3">
                <ToggleGroup
                  type="single"
                  value={ratio}
                  onValueChange={(value) => value && setRatio(value)}
                  aria-label="项目画幅"
                  size="sm"
                  spacing={0}
                  className="bg-background p-1"
                >
                  {["9:16", "16:9", "1:1"].map((value) => (
                    <ToggleGroupItem key={value} value={value}>
                      {value}
                    </ToggleGroupItem>
                  ))}
                </ToggleGroup>
                <Choice
                  value={style}
                  onChange={setStyle}
                  label="项目风格"
                  prefix="风格："
                  options={[
                    "写实电影感",
                    "日系胶片",
                    "国风水墨",
                    "赛博霓虹",
                    "温暖插画",
                  ]}
                />
                <Button variant="ghost" onClick={create}>
                  空白项目
                </Button>
                <div className="ml-auto flex gap-2">
                  <Button size="pill" onClick={create}>
                    创建项目
                    <ArrowRight data-icon="inline-end" />
                  </Button>
                </div>
              </div>
            </CardContent>
          </Card>
        </section>
        <section>
          <div className="mb-4 flex flex-wrap items-center justify-between gap-4">
            <h2 className="text-xl font-semibold">项目</h2>
            <div className="flex flex-wrap items-center gap-2">
              <SearchInput
                value={search}
                onChange={setSearch}
                placeholder="搜索项目"
                className="w-[264px]"
              />
              <Choice
                value={sort}
                onChange={setSort}
                label="项目排序"
                options={["最近打开", "项目名称"]}
              />
              <ToggleGroup
                type="single"
                value={view}
                onValueChange={(value) => value && setView(value)}
                aria-label="项目视图"
                spacing={0}
                className="bg-surface-1 p-1"
              >
                <ToggleGroupItem value="grid" aria-label="网格视图">
                  <Grid2X2 />
                </ToggleGroupItem>
                <ToggleGroupItem value="list" aria-label="列表视图">
                  <List />
                </ToggleGroupItem>
              </ToggleGroup>
            </div>
          </div>
          {visible.length ? (
            <div
              className={cn(
                view === "grid"
                  ? "grid grid-cols-2 gap-x-5 gap-y-8 sm:grid-cols-3 xl:grid-cols-5"
                  : "flex flex-col gap-5",
              )}
            >
              {visible.map((project) => (
                <Card
                  variant="project"
                  key={project.name}
                  className={cn(
                    "group/project",
                    view === "list" ? "flex-row items-center gap-4" : "",
                  )}
                >
                  <Link
                    href={screenHref("analytics")}
                    aria-label={`打开${project.name}`}
                    className={cn(view === "list" ? "w-20 shrink-0" : "")}
                  >
                    <Placeholder
                      className="aspect-[3/4] rounded-xl"
                      label={
                        project.name === "未命名项目"
                          ? undefined
                          : project.ratio
                      }
                      labelStyle="badge"
                      surface="card"
                      showIcon={project.name === "未命名项目"}
                    />
                  </Link>
                  <CardHeader className="mt-2 flex-1">
                    <div className="relative flex items-center justify-between gap-1">
                      <CardTitle>
                        <Link href={screenHref("analytics")}>
                          {project.name}
                        </Link>
                      </CardTitle>
                      <div className="opacity-0 group-focus-within/project:opacity-100 group-hover/project:opacity-100">
                        <MoreMenu
                          label={`${project.name}更多操作`}
                          items={[
                            {
                              label: "打开画布",
                              action: () => router.push(screenHref("canvas")),
                            },
                            {
                              label: "复制项目",
                              action: () => {
                                setProjects((current) => [
                                  ...current,
                                  {
                                    ...project,
                                    name: project.name + " · 副本",
                                  },
                                ]);
                                demoNotice("项目已复制");
                              },
                            },
                          ]}
                        />
                      </div>
                    </div>
                    <CardDescription>
                      {project.ratio} · {project.style}
                    </CardDescription>
                  </CardHeader>
                </Card>
              ))}
            </div>
          ) : (
            <Empty>
              <EmptyHeader>
                <EmptyTitle>没有找到项目</EmptyTitle>
                <EmptyDescription>试试其他关键词。</EmptyDescription>
              </EmptyHeader>
            </Empty>
          )}
        </section>
      </div>
    </ProductShell>
  );
}
