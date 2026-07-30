"use client";

import { LoaderCircle, Plus, X } from "lucide-react";
import { type FormEvent } from "react";
import { Dialog } from "radix-ui";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";

export function ProjectCreateDialog({
  isSubmitting,
  onOpenChange,
  onSubmit,
  open,
  workspaceId,
}: {
  isSubmitting: boolean;
  onOpenChange: (open: boolean) => void;
  onSubmit: (request: API.ProjectCreateRequest) => Promise<boolean>;
  open: boolean;
  workspaceId: string;
}) {
  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const formElement = event.currentTarget;
    const form = new FormData(formElement);
    const completed = await onSubmit({
      workspace_id: workspaceId,
      name: String(form.get("name") ?? "").trim(),
      description: String(form.get("description") ?? "").trim() || null,
      aspect_ratio: String(form.get("aspectRatio")) as API.ProjectCreateRequest["aspect_ratio"],
      language: "zh-CN",
      visual_style: String(form.get("visualStyle") ?? "").trim() || null,
      target_duration_ms: Number(form.get("targetDurationSeconds")) * 1_000,
    });
    if (completed) formElement.reset();
  }

  return (
    <Dialog.Root onOpenChange={onOpenChange} open={open}>
      <Dialog.Portal>
        <Dialog.Overlay className="fixed inset-0 z-50 bg-slate-950/25 backdrop-blur-[2px]" />
        <Dialog.Content className="fixed top-1/2 left-1/2 z-50 max-h-[calc(100vh-2rem)] w-[calc(100%-2rem)] max-w-lg -translate-x-1/2 -translate-y-1/2 overflow-y-auto rounded-2xl border border-slate-200 bg-white p-6 shadow-2xl shadow-slate-950/10">
          <div className="flex items-start justify-between gap-4">
            <div>
              <Dialog.Title className="text-xl font-semibold tracking-tight">创建漫剧项目</Dialog.Title>
              <Dialog.Description className="mt-1 text-sm leading-6 text-slate-500">
                先固定基础规格，项目创建后再按单集推进剧本与资产。
              </Dialog.Description>
            </div>
            <Dialog.Close asChild><Button aria-label="关闭" size="icon" variant="ghost"><X aria-hidden="true" /></Button></Dialog.Close>
          </div>
          <form className="mt-6 grid gap-5" onSubmit={submit}>
            <div className="grid gap-2">
              <Label htmlFor="projectName">项目名称</Label>
              <Input id="projectName" name="name" placeholder="例如：镜中长安" required maxLength={120} />
            </div>
            <div className="grid gap-2">
              <Label htmlFor="projectDescription">项目简介</Label>
              <textarea className="min-h-24 resize-y rounded-lg border border-slate-200 bg-white px-3 py-2 text-sm outline-none focus:border-cyan-400 focus:ring-3 focus:ring-cyan-100" id="projectDescription" name="description" maxLength={1_000} />
            </div>
            <div className="grid gap-4 sm:grid-cols-2">
              <div className="grid gap-2">
                <Label htmlFor="projectRatio">画幅</Label>
                <select className="h-9 rounded-lg border border-slate-200 bg-white px-3 text-sm" defaultValue="9:16" id="projectRatio" name="aspectRatio">
                  <option value="9:16">9:16 竖屏</option>
                  <option value="16:9">16:9 横屏</option>
                  <option value="1:1">1:1 方形</option>
                </select>
              </div>
              <div className="grid gap-2">
                <Label htmlFor="projectDuration">单集目标时长（秒）</Label>
                <Input defaultValue={90} id="projectDuration" min={1} name="targetDurationSeconds" required type="number" />
              </div>
            </div>
            <div className="grid gap-2">
              <Label htmlFor="projectStyle">视觉风格</Label>
              <Input id="projectStyle" name="visualStyle" placeholder="例如：水墨幻想" maxLength={120} />
            </div>
            <div className="flex justify-end gap-2">
              <Dialog.Close asChild><Button type="button" variant="outline">取消</Button></Dialog.Close>
              <Button disabled={isSubmitting} type="submit">
                {isSubmitting ? <LoaderCircle className="animate-spin" aria-hidden="true" /> : <Plus aria-hidden="true" />}
                确认创建
              </Button>
            </div>
          </form>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  );
}
