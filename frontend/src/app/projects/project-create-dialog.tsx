"use client";

import { LoaderCircle, Plus } from "lucide-react";
import { type FormEvent } from "react";

import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Textarea } from "@/components/ui/textarea";

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
    <Dialog onOpenChange={onOpenChange} open={open}>
      <DialogContent className="max-h-[calc(100vh-2rem)] overflow-y-auto sm:max-w-lg">
          <DialogHeader>
            <DialogTitle>创建漫剧项目</DialogTitle>
            <DialogDescription>先固定基础规格，项目创建后再按单集推进剧本与资产。</DialogDescription>
          </DialogHeader>
          <form className="mt-6 grid gap-5" onSubmit={submit}>
            <div className="grid gap-2">
              <Label htmlFor="projectName">项目名称</Label>
              <Input id="projectName" name="name" placeholder="例如：镜中长安" required maxLength={120} />
            </div>
            <div className="grid gap-2">
              <Label htmlFor="projectDescription">项目简介</Label>
              <Textarea className="min-h-24 resize-y" id="projectDescription" name="description" maxLength={1_000} />
            </div>
            <div className="grid gap-4 sm:grid-cols-2">
              <div className="grid gap-2">
                <Label htmlFor="projectRatio">画幅</Label>
                <Select defaultValue="9:16" name="aspectRatio">
                  <SelectTrigger className="w-full" id="projectRatio"><SelectValue /></SelectTrigger>
                  <SelectContent>
                    <SelectItem value="9:16">9:16 竖屏</SelectItem>
                    <SelectItem value="16:9">16:9 横屏</SelectItem>
                    <SelectItem value="1:1">1:1 方形</SelectItem>
                  </SelectContent>
                </Select>
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
            <DialogFooter>
              <DialogClose asChild><Button type="button" variant="outline">取消</Button></DialogClose>
              <Button disabled={isSubmitting} type="submit">
                {isSubmitting ? <LoaderCircle className="animate-spin" aria-hidden="true" /> : <Plus aria-hidden="true" />}
                确认创建
              </Button>
            </DialogFooter>
          </form>
      </DialogContent>
    </Dialog>
  );
}
