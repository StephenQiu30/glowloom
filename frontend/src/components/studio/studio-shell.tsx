"use client";

import {
  ArrowRight,
  Box,
  Check,
  ChevronDown,
  Folder,
  Home,
  Plus,
  Settings,
  ShieldCheck,
  Sparkles,
  WandSparkles,
  X,
} from "lucide-react";
import Image from "next/image";
import Link from "next/link";
import { type FormEvent, type ReactNode, useState } from "react";
import { Dialog } from "radix-ui";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { cn } from "@/lib/class-names";
import { mockProductionStages } from "@/lib/mock-studio-data";

export type StudioNavigation =
  | "create"
  | "projects"
  | "assets"
  | "governance"
  | "settings";

export function StudioBrand() {
  return (
    <Link className="flex items-center gap-2 px-1 font-semibold tracking-tight" href="/">
      <span className="grid size-8 place-items-center rounded-xl bg-[#09a6bc] text-white shadow-sm shadow-cyan-500/15">
        <Sparkles className="size-[18px]" strokeWidth={2.3} aria-hidden="true" />
      </span>
      <span className="hidden text-[17px] lg:inline">Lanverse</span>
    </Link>
  );
}

function NewComicDialog({ onCreated }: { onCreated: (name: string) => void }) {
  const [open, setOpen] = useState(false);

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const form = new FormData(event.currentTarget);
    onCreated(String(form.get("projectName") || "未命名漫剧"));
    setOpen(false);
  }

  return (
    <Dialog.Root open={open} onOpenChange={setOpen}>
      <Dialog.Trigger asChild>
        <Button className="h-10 w-full bg-[#079db3] text-white hover:bg-[#078da0]">
          <Plus aria-hidden="true" />
          <span className="hidden lg:inline">新建漫剧</span>
        </Button>
      </Dialog.Trigger>
      <Dialog.Portal>
        <Dialog.Overlay className="fixed inset-0 z-50 bg-slate-950/25 backdrop-blur-[2px]" />
        <Dialog.Content className="fixed top-1/2 left-1/2 z-50 w-[calc(100%-2rem)] max-w-md -translate-x-1/2 -translate-y-1/2 rounded-2xl border border-slate-200 bg-white p-6 shadow-2xl shadow-slate-950/10">
          <div className="flex items-start justify-between gap-4">
            <div>
              <Dialog.Title className="text-xl font-semibold tracking-tight">创建一部新漫剧</Dialog.Title>
              <Dialog.Description className="mt-1 text-sm leading-6 text-slate-500">先定义画幅与视觉风格，稍后再完善剧本。</Dialog.Description>
            </div>
            <Dialog.Close asChild><Button aria-label="关闭" size="icon" variant="ghost"><X aria-hidden="true" /></Button></Dialog.Close>
          </div>
          <form className="mt-6 grid gap-5" onSubmit={submit}>
            <div className="grid gap-2">
              <Label htmlFor="newComicName">项目名称</Label>
              <Input id="newComicName" name="projectName" placeholder="例如：镜中长安" required />
            </div>
            <div className="grid grid-cols-2 gap-4">
              <div className="grid gap-2">
                <Label htmlFor="newComicRatio">画幅</Label>
                <select className="h-9 rounded-lg border border-slate-200 bg-white px-3 text-sm" id="newComicRatio" defaultValue="9:16"><option>9:16</option><option>16:9</option><option>1:1</option></select>
              </div>
              <div className="grid gap-2">
                <Label htmlFor="newComicStyle">视觉风格</Label>
                <select className="h-9 rounded-lg border border-slate-200 bg-white px-3 text-sm" id="newComicStyle" defaultValue="水墨幻想"><option>水墨幻想</option><option>国漫电影感</option><option>赛博都市</option></select>
              </div>
            </div>
            <Button className="h-10 bg-[#079db3] text-white hover:bg-[#078da0]" type="submit"><WandSparkles aria-hidden="true" />创建项目草稿</Button>
          </form>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  );
}

function ProductionProgress({ currentStep }: { currentStep: number }) {
  return (
    <ol className="hidden min-w-0 flex-1 items-center justify-center gap-2 xl:flex" aria-label="制作进度">
      {mockProductionStages.map((stage, index) => (
        <li className="flex items-center gap-2" key={stage}>
          <span className={cn(
            "grid size-6 place-items-center rounded-full border text-xs font-medium",
            index <= currentStep ? "border-[#079db3] bg-[#079db3] text-white" : "border-slate-300 text-slate-500",
          )}>{index < currentStep ? <Check className="size-3.5" aria-hidden="true" /> : index + 1}</span>
          <span className={cn("text-sm", index === currentStep ? "font-medium text-slate-900" : "text-slate-500")}>{stage}</span>
          {index < mockProductionStages.length - 1 ? <span className="mx-1 h-px w-8 bg-slate-200 2xl:w-12" /> : null}
        </li>
      ))}
    </ol>
  );
}

export function StudioShell({
  active,
  children,
  projectName,
  currentStep,
  topAction,
}: {
  active: StudioNavigation;
  children: ReactNode;
  projectName?: string;
  currentStep?: number;
  topAction?: ReactNode;
}) {
  const [notice, setNotice] = useState<string | null>(null);
  const navItems = [
    { id: "create" as const, label: "创作", icon: Home, href: "/" },
    { id: "projects" as const, label: "项目", icon: Folder, href: "/projects" },
    { id: "assets" as const, label: "资产", icon: Box, href: "/studio" },
    {
      id: "governance" as const,
      label: "治理",
      icon: ShieldCheck,
      href: "/governance",
    },
  ];

  return (
    <main className="min-h-screen bg-[#fbfcfd] text-slate-950">
      <aside className="fixed inset-y-0 left-0 z-30 flex w-[76px] flex-col border-r border-slate-200/80 bg-white px-3 py-5 lg:w-[156px]">
        <StudioBrand />
        <div className="mt-7"><NewComicDialog onCreated={(name) => setNotice(`《${name}》项目草稿已创建`)} /></div>
        <nav className="mt-7 grid gap-2" aria-label="主导航">
          {navItems.map((item) => (
            <Link className={cn(
              "flex h-12 items-center justify-center gap-3 rounded-xl text-sm transition-colors lg:justify-start lg:px-4",
              active === item.id ? "bg-slate-100 text-[#078fa5]" : "text-slate-600 hover:bg-slate-50 hover:text-slate-950",
            )} href={item.href} key={item.id}>
              <item.icon className="size-5" strokeWidth={1.8} aria-hidden="true" />
              <span className="hidden lg:inline">{item.label}</span>
            </Link>
          ))}
        </nav>
        <Link className={cn(
          "mt-auto flex items-center justify-center gap-2 rounded-xl p-2 transition-colors hover:bg-slate-50 lg:justify-start",
          active === "settings" && "bg-slate-100",
        )} href="/workspaces">
          <span className="relative size-9 overflow-hidden rounded-full border border-slate-200 bg-slate-100">
            <Image alt="Stephen 头像" fill sizes="36px" src="/assets/lanverse-studio/lu-chenzhou-portrait.png" className="object-cover" />
          </span>
          <span className="hidden min-w-0 lg:block">
            <span className="block truncate text-sm font-medium">Stephen</span>
            <span className="block text-xs text-slate-400">个人空间</span>
          </span>
          <Settings className="ml-auto hidden size-4 text-slate-400 lg:block" aria-hidden="true" />
        </Link>
      </aside>

      <div className="min-h-screen pl-[76px] lg:pl-[156px]">
        <header className="sticky top-0 z-20 flex h-[76px] items-center gap-5 border-b border-slate-200/70 bg-white/95 px-5 backdrop-blur md:px-8">
          {projectName ? (
            <button className="flex shrink-0 items-center gap-2 text-sm font-medium" type="button">{projectName}<ChevronDown className="size-4 text-slate-400" aria-hidden="true" /></button>
          ) : (
            <div className="flex shrink-0 items-center gap-2 whitespace-nowrap text-sm font-medium"><WandSparkles className="size-4 text-[#079db3]" aria-hidden="true" />AI 漫剧创作台</div>
          )}
          {typeof currentStep === "number" ? <ProductionProgress currentStep={currentStep} /> : <div className="flex-1" />}
          {topAction ?? (
            <Button asChild className="h-10 bg-[#079db3] px-4 text-white hover:bg-[#078da0]"><Link href="/projects">查看项目<ArrowRight aria-hidden="true" /></Link></Button>
          )}
        </header>
        {notice ? <div className="fixed top-24 right-6 z-50 flex items-center gap-2 rounded-xl border border-emerald-200 bg-white px-4 py-3 text-sm shadow-lg" role="status"><Check className="size-4 text-emerald-600" aria-hidden="true" />{notice}</div> : null}
        {children}
      </div>
    </main>
  );
}
