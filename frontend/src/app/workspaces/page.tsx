"use client";

import { Archive, ArrowRight, Check, HardDrive, Plus, Save, Settings2, Users } from "lucide-react";
import Image from "next/image";
import Link from "next/link";
import { type FormEvent, useState } from "react";

import { StudioShell } from "@/components/studio/studio-shell";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { mockWorkspaces } from "@/lib/mock-studio-data";

export default function WorkspacesPage() {
  const [message, setMessage] = useState<string | null>(null);
  const [workspaces, setWorkspaces] = useState(mockWorkspaces);

  function saveProfile(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setMessage("个人资料已保存。");
  }

  function createWorkspace(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const form = event.currentTarget;
    const data = new FormData(form);
    const name = String(data.get("workspaceName"));
    if (!name) return;
    setWorkspaces((items) => [...items, { id: `mock-${items.length}`, name, role: "所有者", projects: 0, members: 1, storage: "0 GB", active: false }]);
    setMessage(`工作空间“${name}”已创建。`);
    form.reset();
  }

  return (
    <StudioShell active="settings" topAction={<Button asChild className="h-10 bg-[#079db3] px-4 text-white hover:bg-[#078da0]"><Link href="/projects">返回项目<ArrowRight aria-hidden="true" /></Link></Button>}>
      <div className="mx-auto max-w-[1120px] px-5 py-9 md:px-8">
        <div><Badge className="border-cyan-100 bg-cyan-50 text-[#087f91]" variant="outline">账户设置</Badge><h1 className="mt-3 text-3xl font-semibold tracking-[-0.035em]">账户与工作空间</h1><p className="mt-2 text-sm text-slate-500">管理个人资料、创作空间和团队资源。</p></div>
        {message ? <div className="mt-6 flex items-center gap-2 rounded-xl border border-emerald-200 bg-emerald-50 px-4 py-3 text-sm text-emerald-800" role="status"><Check className="size-4" aria-hidden="true" />{message}</div> : null}

        <div className="mt-7 grid gap-6 lg:grid-cols-[1fr_340px]">
          <section className="rounded-2xl border border-slate-200 bg-white p-6">
            <div className="flex items-center gap-4"><span className="relative size-16 overflow-hidden rounded-2xl border border-slate-200 bg-slate-100"><Image alt="Stephen 头像" fill sizes="64px" src="/assets/lanverse-studio/lu-chenzhou-portrait.png" className="object-cover" /></span><div><h2 className="text-lg font-semibold">个人资料</h2><p className="mt-1 text-sm text-slate-500">creator@lanverse.ai</p></div></div>
            <form className="mt-6 grid gap-5 sm:grid-cols-2" onSubmit={saveProfile}>
              <div className="grid gap-2"><Label htmlFor="displayName">显示名称</Label><Input id="displayName" defaultValue="Stephen" /></div>
              <div className="grid gap-2"><Label htmlFor="creatorRole">创作角色</Label><Input id="creatorRole" defaultValue="导演 / 制片人" /></div>
              <div className="grid gap-2 sm:col-span-2"><Label htmlFor="bio">个人简介</Label><Input id="bio" defaultValue="专注东方幻想与悬疑题材的 AI 漫剧创作者" /></div>
              <div className="sm:col-span-2"><Button className="bg-[#079db3] text-white hover:bg-[#078da0]" type="submit"><Save aria-hidden="true" />保存个人资料</Button></div>
            </form>
          </section>

          <aside className="rounded-2xl border border-slate-200 bg-white p-6">
            <div className="flex items-center gap-3"><span className="grid size-10 place-items-center rounded-xl bg-slate-100 text-[#079db3]"><Plus className="size-5" aria-hidden="true" /></span><div><h2 className="font-semibold">创建工作空间</h2><p className="mt-1 text-xs text-slate-500">组织项目和团队成员</p></div></div>
            <form className="mt-6 grid gap-4" onSubmit={createWorkspace}><div className="grid gap-2"><Label htmlFor="workspaceName">空间名称</Label><Input id="workspaceName" name="workspaceName" placeholder="例如：青墨工作室" required /></div><Button className="h-10 bg-[#079db3] text-white hover:bg-[#078da0]" type="submit"><Plus aria-hidden="true" />创建工作空间</Button></form>
          </aside>
        </div>

        <section className="mt-7">
          <div className="mb-4 flex items-end justify-between"><div><h2 className="text-xl font-semibold">我的工作空间</h2><p className="mt-1 text-sm text-slate-500">Mock 数据展示空间切换、成员与存储状态。</p></div><span className="text-sm text-slate-400">{workspaces.length} 个空间</span></div>
          <div className="grid gap-4">
            {workspaces.map((workspace) => (
              <div className={`flex flex-wrap items-center gap-5 rounded-2xl border bg-white p-5 ${workspace.active ? "border-cyan-200 shadow-sm shadow-cyan-950/5" : "border-slate-200"}`} key={workspace.id}>
                <span className="grid size-12 place-items-center rounded-xl bg-slate-100 text-[#079db3]"><Settings2 className="size-5" aria-hidden="true" /></span>
                <div className="min-w-48 flex-1"><div className="flex items-center gap-2"><h3 className="font-semibold">{workspace.name}</h3>{workspace.active ? <Badge className="border-cyan-100 bg-cyan-50 text-[#087f91]" variant="outline">当前空间</Badge> : null}{workspace.archived ? <Badge variant="outline">已归档</Badge> : null}</div><p className="mt-1 text-xs text-slate-500">{workspace.role}</p></div>
                <div className="flex gap-6 text-sm text-slate-500"><span className="flex items-center gap-1.5"><Archive className="size-4" aria-hidden="true" />{workspace.projects} 项目</span><span className="flex items-center gap-1.5"><Users className="size-4" aria-hidden="true" />{workspace.members} 成员</span><span className="flex items-center gap-1.5"><HardDrive className="size-4" aria-hidden="true" />{workspace.storage}</span></div>
                <Button disabled={workspace.active || workspace.archived} variant="outline">切换到此空间</Button>
              </div>
            ))}
          </div>
        </section>
      </div>
    </StudioShell>
  );
}
