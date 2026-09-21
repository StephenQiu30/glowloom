"use client";

import { useRef } from "react";
import { Plus } from "lucide-react";
import { Spinner } from "@/components/ui/spinner";
import { FieldGroup, Field, FieldLabel, FieldError } from "@/components/ui/field";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";

import { Alert, AlertTitle, AlertDescription } from "@/components/ui/alert";
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
import { Textarea } from "@/components/ui/textarea";

const projectFormSchema = z.object({
  name: z.string().trim().min(1, "请输入项目名称").max(120, "项目名称不能超过 120 个字符"),
  description: z.string().trim().max(1000, "项目简介不能超过 1000 个字符"),
});

export function ProjectCreateDialog({
  isSubmitting,
  errorMessage,
  onOpenChange,
  onSubmit,
  open,
  workspaceId,
}: {
  isSubmitting: boolean;
  errorMessage?: string;
  onOpenChange: (open: boolean) => void;
  onSubmit: (request: API.ProjectCreateRequest) => Promise<boolean>;
  open: boolean;
  workspaceId: string;
}) {
  const openerRef = useRef<HTMLElement | null>(null);
  const form = useForm<z.infer<typeof projectFormSchema>>({
    resolver: zodResolver(projectFormSchema),
    defaultValues: { name: "", description: "" },
  });
  const busy = isSubmitting || form.formState.isSubmitting;
  const errors = form.formState.errors;
  async function submit(values: z.infer<typeof projectFormSchema>) {
    const completed = await onSubmit({
      workspace_id: workspaceId,
      name: values.name,
      description: values.description || null,
      aspect_ratio: "9:16",
      language: "zh-CN",
      visual_style: null,
      target_duration_ms: 90_000,
      idempotency_key: `project-create:${crypto.randomUUID()}`,
    });
    if (completed) form.reset();
  }

  return (
    <Dialog onOpenChange={onOpenChange} open={open}>
      <DialogContent
        className="max-h-[calc(100vh-2rem)] overflow-y-auto sm:max-w-lg"
        onOpenAutoFocus={() => {
          openerRef.current =
            document.activeElement instanceof HTMLElement ? document.activeElement : null;
        }}
        onCloseAutoFocus={(event) => {
          if (openerRef.current?.isConnected) {
            event.preventDefault();
            openerRef.current.focus();
          }
        }}
      >
        <DialogHeader>
          <DialogTitle>创建项目</DialogTitle>
          <DialogDescription>创建后在项目页继续制作。</DialogDescription>
        </DialogHeader>
        <form onSubmit={form.handleSubmit(submit)} noValidate>
          <FieldGroup className="mt-6 grid gap-5">
            <Field data-invalid={Boolean(errors.name)} data-disabled={busy}>
              <FieldLabel htmlFor="projectName">项目名称</FieldLabel>
              <Input
                id="projectName"
                {...form.register("name")}
                disabled={busy}
                aria-invalid={Boolean(errors.name)}
                aria-describedby={errors.name ? "projectName-error" : undefined}
                placeholder="例如：镜中长安"
                required
                maxLength={120}
              />
              <FieldError id="projectName-error" errors={[errors.name]} />
            </Field>
            <Field data-invalid={Boolean(errors.description)} data-disabled={busy}>
              <FieldLabel htmlFor="projectDescription">项目简介</FieldLabel>
              <Textarea
                className="min-h-24 resize-y"
                id="projectDescription"
                {...form.register("description")}
                disabled={busy}
                aria-invalid={Boolean(errors.description)}
                aria-describedby={errors.description ? "projectDescription-error" : undefined}
                maxLength={1_000}
              />
              <FieldError id="projectDescription-error" errors={[errors.description]} />
            </Field>
            {errorMessage ? (
              <Alert variant="destructive">
                <AlertTitle>创建失败</AlertTitle>
                <AlertDescription>{errorMessage}</AlertDescription>
              </Alert>
            ) : null}
            <DialogFooter>
              <DialogClose asChild>
                <Button disabled={busy} type="button" variant="outline">
                  取消
                </Button>
              </DialogClose>
              <Button disabled={busy} type="submit">
                {busy ? (
                  <Spinner data-icon="inline-start" aria-hidden="true" />
                ) : (
                  <Plus data-icon="inline-start" aria-hidden="true" />
                )}
                确认创建
              </Button>
            </DialogFooter>
          </FieldGroup>
        </form>
      </DialogContent>
    </Dialog>
  );
}
