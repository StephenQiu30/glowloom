"use client";

import { useRef, useState } from "react";
import Image from "next/image";
import Link from "next/link";
import { useQuery } from "@tanstack/react-query";
import {
  ArrowUpRight,
  AudioLines,
  ChevronRight,
  Clapperboard,
  FileText,
  Film,
  FolderOpen,
  ImagePlus,
  Images,
  ListChecks,
  PanelsTopLeft,
  Plus,
  ScanFace,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import {
  Dialog,
  DialogContent,
  DialogClose,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  Empty,
  EmptyContent,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from "@/components/ui/empty";
import { Input } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { listProjects, PROJECTS_KEY } from "@/components/project/queries";

const services = [
  { name: "创作画布", icon: PanelsTopLeft, href: "/canvas" },
  {
    name: "视频生成",
    icon: Clapperboard,
    detail:
      "围绕分镜准备视频提示词、参考素材与镜头运动。视频生成服务正在准备中，暂不能提交生成任务。",
  },
  {
    name: "图片生成",
    icon: ImagePlus,
    detail:
      "在画布中组织图片节点、提示词和参考素材。图片生成服务正在准备中，暂不能提交生成任务。",
  },
  {
    name: "音频生成",
    icon: AudioLines,
    detail:
      "规划角色台词、配音与声音素材。音频生成服务正在准备中，暂不能提交生成任务。",
  },
  {
    name: "剧本创作",
    icon: FileText,
    detail:
      "用文字节点整理故事、场景和角色，连接到分镜参考。自动剧本生成正在准备中。",
  },
  {
    name: "角色造型",
    icon: ScanFace,
    detail:
      "将角色文字设定与已有参考图片组织在同一画布中。角色自动生成正在准备中。",
  },
  { name: "素材管理", icon: Images, href: "/assets" },
  { name: "任务中心", icon: ListChecks, href: "/tasks" },
];
const guides = [
  {
    id: "story",
    title: "剧本原创与改编",
    subtitle: "把一个想法，写成一个故事",
    category: "故事",
    image: "script",
    steps: [
      "新建项目，确定画幅和视觉风格。",
      "进入画布，添加文字节点，写下故事梗概和场景。",
      "连接角色与参考图片，为分镜留下清楚的创作线索。",
    ],
  },
  {
    id: "director",
    title: "镜头与导演思维",
    subtitle: "让每个镜头都有自己的表达",
    category: "镜头",
    image: "director",
    steps: [
      "在画布中按镜头组织文字和视觉参考。",
      "描述景别、机位、运动及镜头之间的关系。",
      "使用已有图片或视频检查构图与叙事节奏。",
    ],
  },
  {
    id: "character",
    title: "角色造型室",
    subtitle: "先认识角色，再看见角色",
    category: "角色",
    image: "character",
    steps: [
      "记录角色身份、性格、服装与关键特征。",
      "从资产库查看该项目已有的角色参考。",
      "回到画布连接文字设定与素材，整理角色视觉方向。",
    ],
  },
  {
    id: "motion",
    title: "氛围与视觉叙事",
    subtitle: "从画面里，找到故事的情绪",
    category: "镜头",
    image: "motion",
    steps: [
      "选择项目画幅与视觉风格。",
      "整理色彩、环境与光线的参考素材。",
      "在画布中组合文字、图片和视频节点，明确每个场景的氛围。",
    ],
  },
];
type Guide = (typeof guides)[number];

function RecentProjects() {
  const params = {
    limit: 4,
    status: "active",
    deleted: false,
  } satisfies API.listProjectsParams;
  const query = useQuery({
    queryKey: [...PROJECTS_KEY, "home", params],
    queryFn: ({ signal }) => listProjects(params, signal),
  });
  if (query.isPending)
    return (
      <div
        role="status"
        aria-label="正在读取最近项目"
        className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4"
      >
        {[0, 1, 2, 3].map((item) => (
          <Skeleton key={item} className="h-24 rounded-2xl" />
        ))}
      </div>
    );
  if (query.isError)
    return (
      <Empty className="rounded-2xl bg-card py-6">
        <EmptyHeader>
          <EmptyTitle>最近项目未能加载</EmptyTitle>
          <EmptyDescription>请确认项目服务可用后重试。</EmptyDescription>
        </EmptyHeader>
        <EmptyContent>
          <Button variant="secondary" onClick={() => query.refetch()}>
            重试读取最近项目
          </Button>
        </EmptyContent>
      </Empty>
    );
  if (query.data.items.length === 0)
    return (
      <Empty className="rounded-2xl bg-card py-6">
        <EmptyHeader>
          <EmptyMedia variant="icon">
            <FolderOpen />
          </EmptyMedia>
          <EmptyTitle>你的故事，从这里开始</EmptyTitle>
          <EmptyDescription>
            创建第一个项目，进入画布组织故事和素材。
          </EmptyDescription>
        </EmptyHeader>
        <EmptyContent>
          <Button asChild>
            <Link href="/projects?create=true">
              <Plus data-icon="inline-start" />
              新建项目
            </Link>
          </Button>
        </EmptyContent>
      </Empty>
    );
  return (
    <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
      {query.data.items.map((project) => (
        <Link
          key={project.id}
          href={`/projects/${project.id}/canvas`}
          className="group flex min-w-0 items-center gap-4 rounded-2xl bg-card p-2 transition-colors outline-none hover:bg-accent focus-visible:ring-2 focus-visible:ring-ring"
        >
          <div className="flex size-18 shrink-0 items-center justify-center rounded-xl bg-secondary text-muted-foreground">
            <PanelsTopLeft aria-hidden="true" className="size-7" />
          </div>
          <div className="min-w-0 flex-1">
            <p className="truncate text-sm font-medium">{project.name}</p>
            <p className="mt-2 text-xs text-muted-foreground">
              {project.aspect_ratio} ·{" "}
              {project.style_type === "realistic" ? "写实" : "风格化"}
            </p>
          </div>
          <ChevronRight
            aria-hidden="true"
            className="mr-2 size-4 shrink-0 text-muted-foreground opacity-0 transition-opacity group-hover:opacity-100"
          />
        </Link>
      ))}
    </div>
  );
}

export function HomePage() {
  const serviceTrigger = useRef<HTMLButtonElement | null>(null);
  const guideTrigger = useRef<HTMLButtonElement | null>(null);
  const [selectedService, setSelectedService] = useState<
    (typeof services)[number] | null
  >(null);
  const [selectedGuide, setSelectedGuide] = useState<Guide | null>(null);
  const [category, setCategory] = useState("全部");
  const [search, setSearch] = useState("");
  const filteredGuides = guides.filter(
    (guide) =>
      (category === "全部" || category === guide.category) &&
      `${guide.title}${guide.subtitle}`.includes(search.trim()),
  );
  return (
    <div className="flex flex-col gap-8">
      <h1 className="sr-only">Lanverse 创作工作台</h1>
      <Link
        href="/projects?create=true"
        className="group relative flex h-40 flex-col items-center justify-center gap-4 overflow-hidden rounded-2xl bg-secondary outline-none focus-visible:ring-2 focus-visible:ring-ring"
      >
        <Image
          src="/studio/canvas-grid.png"
          alt=""
          fill
          preload
          sizes="(max-width: 1023px) 100vw, calc(100vw - 320px)"
          className="object-cover"
        />
        <span className="relative flex h-14 w-24 items-center justify-center rounded-2xl bg-white text-black shadow-lg transition-transform group-hover:scale-105 motion-reduce:transform-none motion-reduce:transition-none">
          <Plus aria-hidden="true" className="size-8" />
        </span>
        <span className="relative text-base font-medium text-white">
          新建画布创作
        </span>
      </Link>
      <nav
        aria-label="创作服务"
        className="grid grid-cols-4 gap-x-3 gap-y-6 md:grid-cols-8 lg:gap-x-6"
      >
        {services.map((service) => {
          const content = (
            <>
              <span className="flex h-14 w-full max-w-28 items-center justify-center rounded-2xl bg-secondary transition-colors group-hover:bg-accent">
                <service.icon aria-hidden="true" className="size-6" />
              </span>
              <span className="text-xs sm:text-sm">{service.name}</span>
            </>
          );
          const className =
            "group flex min-w-0 flex-col items-center gap-3 rounded-xl outline-none focus-visible:ring-2 focus-visible:ring-ring";
          return service.href ? (
            <Link key={service.name} href={service.href} className={className}>
              {content}
            </Link>
          ) : (
            <Button
              variant="ghost"
              key={service.name}
              type="button"
              onClick={(event) => {
                serviceTrigger.current = event.currentTarget;
                setSelectedService(service);
              }}
              className={`${className} h-auto p-0 whitespace-normal`}
            >
              {content}
            </Button>
          );
        })}
      </nav>
      <section
        aria-labelledby="recent-projects-title"
        className="flex flex-col gap-3"
      >
        <div className="flex items-center justify-between gap-4">
          <h2
            id="recent-projects-title"
            className="text-base font-medium sm:text-lg"
          >
            最近项目
          </h2>
          <Button variant="ghost" asChild>
            <Link href="/projects">
              查看全部
              <ChevronRight data-icon="inline-end" />
            </Link>
          </Button>
        </div>
        <RecentProjects />
      </section>
      <section
        id="guides"
        aria-labelledby="guides-title"
        className="scroll-mt-6"
      >
        <Tabs value={category} onValueChange={setCategory} className="gap-4">
          <div className="flex flex-wrap items-center justify-between gap-4">
            <h2 id="guides-title" className="text-base font-medium sm:text-lg">
              创作指南
            </h2>
            <TabsList
              variant="line"
              aria-label="创作指南分类"
              className="gap-5"
            >
              {["全部", "故事", "角色", "镜头"].map((value) => (
                <TabsTrigger key={value} value={value}>
                  {value}
                </TabsTrigger>
              ))}
            </TabsList>
            <Input
              aria-label="搜索创作指南"
              value={search}
              onChange={(event) => setSearch(event.target.value)}
              placeholder="搜索创作指南"
              className="h-9 w-full rounded-full bg-card px-4 sm:w-60"
            />
          </div>
          {["全部", "故事", "角色", "镜头"].map((value) => (
            <TabsContent key={value} value={value}>
              <div className="grid gap-5 sm:grid-cols-2 xl:grid-cols-4">
                {filteredGuides.map((guide) => (
                  <Button
                    variant="ghost"
                    key={guide.id}
                    type="button"
                    onClick={(event) => {
                      guideTrigger.current = event.currentTarget;
                      setSelectedGuide(guide);
                    }}
                    className="group block h-auto min-w-0 rounded-2xl p-0 text-left whitespace-normal outline-none focus-visible:ring-2 focus-visible:ring-ring"
                    aria-label={guide.title}
                  >
                    <Card className="gap-0 overflow-hidden border-0 bg-transparent py-0 shadow-none ring-0">
                      <CardContent className="relative aspect-video overflow-hidden rounded-2xl p-0">
                        <Image
                          src={`/studio/${guide.image}-studio.png`}
                          alt={guide.subtitle}
                          fill
                          sizes="(max-width: 639px) calc(100vw - 32px), (max-width: 1279px) 45vw, 23vw"
                          className="object-cover transition-transform duration-300 group-hover:scale-105 motion-reduce:transform-none motion-reduce:transition-none"
                        />
                        <span className="absolute right-3 bottom-3 flex size-7 items-center justify-center rounded-full bg-black/40 text-white backdrop-blur-sm">
                          <ArrowUpRight aria-hidden="true" className="size-4" />
                        </span>
                      </CardContent>
                      <CardHeader className="gap-2 px-1 pt-4">
                        <div className="flex items-center gap-2">
                          <CardTitle className="text-sm font-medium">
                            {guide.title}
                          </CardTitle>
                          <Badge
                            variant="secondary"
                            className="shrink-0 text-primary"
                          >
                            指南
                          </Badge>
                        </div>
                        <CardDescription className="text-xs">
                          {guide.subtitle}
                        </CardDescription>
                      </CardHeader>
                    </Card>
                  </Button>
                ))}
              </div>
              {filteredGuides.length === 0 ? (
                <Empty className="py-10">
                  <EmptyHeader>
                    <EmptyTitle>没有匹配的创作指南</EmptyTitle>
                    <EmptyDescription>试试其他关键词或分类。</EmptyDescription>
                  </EmptyHeader>
                  <EmptyContent>
                    <Button
                      variant="secondary"
                      onClick={() => {
                        setSearch("");
                        setCategory("全部");
                      }}
                    >
                      清除筛选
                    </Button>
                  </EmptyContent>
                </Empty>
              ) : null}
            </TabsContent>
          ))}
        </Tabs>
      </section>
      <section
        aria-labelledby="start-title"
        className="flex flex-wrap items-center justify-between gap-5 rounded-2xl bg-card p-6"
      >
        <div className="flex items-center gap-4">
          <Film
            aria-hidden="true"
            className="hidden size-8 text-muted-foreground sm:block"
          />
          <div>
            <h2 id="start-title" className="text-base font-medium">
              下一个故事，由你开始
            </h2>
            <p className="mt-2 text-sm text-muted-foreground">
              确定画幅、选择风格，将创意放进画布。
            </p>
          </div>
        </div>
        <Button asChild size="lg">
          <Link href="/projects?create=true">
            开始创作
            <ArrowUpRight data-icon="inline-end" />
          </Link>
        </Button>
      </section>
      <Dialog
        open={selectedService !== null}
        onOpenChange={(open) => {
          if (!open) setSelectedService(null);
        }}
      >
        <DialogContent
          className="max-h-[calc(100dvh-2rem)] overflow-y-auto"
          onCloseAutoFocus={(event) => {
            event.preventDefault();
            serviceTrigger.current?.focus();
          }}
        >
          <DialogHeader>
            <DialogTitle>{selectedService?.name}</DialogTitle>
            <DialogDescription>{selectedService?.detail}</DialogDescription>
          </DialogHeader>
          <Badge variant="secondary" className="w-fit">
            服务准备中
          </Badge>
          <p className="text-sm leading-6 text-muted-foreground">
            你现在可以创建项目、整理提示词和已有素材，为后续生成做好准备。
          </p>
          <DialogClose asChild>
            <Button asChild>
              <Link href="/canvas">
                进入创作画布
                <ArrowUpRight data-icon="inline-end" />
              </Link>
            </Button>
          </DialogClose>
          <DialogClose asChild>
            <Button variant="secondary" asChild>
              <Link href="/projects?create=true">新建项目</Link>
            </Button>
          </DialogClose>
        </DialogContent>
      </Dialog>
      <Dialog
        open={selectedGuide !== null}
        onOpenChange={(open) => {
          if (!open) setSelectedGuide(null);
        }}
      >
        <DialogContent
          className="max-h-[calc(100dvh-2rem)] overflow-y-auto sm:max-w-lg"
          onCloseAutoFocus={(event) => {
            event.preventDefault();
            guideTrigger.current?.focus();
          }}
        >
          <DialogHeader>
            <DialogTitle>{selectedGuide?.title}</DialogTitle>
            <DialogDescription>{selectedGuide?.subtitle}</DialogDescription>
          </DialogHeader>
          {selectedGuide ? (
            <div className="relative aspect-video overflow-hidden rounded-xl">
              <Image
                src={`/studio/${selectedGuide.image}-studio.png`}
                alt={selectedGuide.subtitle}
                fill
                sizes="480px"
                className="object-cover"
              />
            </div>
          ) : null}
          <ol className="flex list-decimal flex-col gap-3 pl-5 text-sm leading-6 text-muted-foreground">
            {selectedGuide?.steps.map((step) => (
              <li key={step}>{step}</li>
            ))}
          </ol>
          <p className="text-xs text-muted-foreground">
            图片为创作方向示意，生成服务仍在准备中。
          </p>
          <DialogClose asChild>
            <Button asChild>
              <Link href="/projects?create=true">
                从新项目开始
                <ArrowUpRight data-icon="inline-end" />
              </Link>
            </Button>
          </DialogClose>
        </DialogContent>
      </Dialog>
    </div>
  );
}
