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
      aspect_ratio: "9:16",
      language: "zh-CN",
      visual_style: null,
      target_duration_ms: 90_000,
      idempotency_key: `project-create:${crypto.randomUUID()}`,
    });
    if (completed) formElement.reset();
  }

  return (
    <Dialog onOpenChange={onOpenChange} open={open}>
      <DialogContent className="max-h-[calc(100vh-2rem)] overflow-y-auto sm:max-w-lg">
          <DialogHeader>
            <DialogTitle>创建项目</DialogTitle>
            <DialogDescription>创建后在项目页继续制作。</DialogDescription>
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
