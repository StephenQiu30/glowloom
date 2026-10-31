"use client";

import { useEffect, useRef, useState } from "react";
import { useRouter } from "next/navigation";
import useSWR from "swr";
import * as projects from "@/components/canvas/runtime/api/personal-workspace-bff";
import * as canvases from "@/components/canvas/runtime/api/canvas-document-bff";
import { ApiError, unwrap } from "@/components/canvas/runtime/lib/request";
import {
  notifyError,
  useErrorNotification,
} from "@/components/canvas/runtime/components/feedback/error-notification";
import { Button } from "@/components/ui/button";
import { Spinner } from "@/components/ui/spinner";
import { Input } from "@/components/ui/input";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from "@/components/ui/dialog";
import {
  Field,
  FieldDescription,
  FieldGroup,
  FieldLabel,
} from "@/components/ui/field";
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";

type CanvasOperation =
  | { kind: "create"; body: CanvasContract.ProjectCanvasCreateRequest }
  | { kind: "attach"; body: CanvasContract.ProjectCanvasAttachRequest };

export function ProjectCanvasDialog({
  identity,
  projectId,
  projectPath,
  mode,
  open,
  onOpenChange,
  onCompleted,
  onCloseAutoFocus,
}: {
  identity: string;
  projectId: string;
  projectPath: string;
  mode: "create" | "attach";
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onCompleted: () => void;
  onCloseAutoFocus?: (event: Event) => void;
}) {
  const router = useRouter();
  const [title, setTitle] = useState("");
  const [sourceId, setSourceId] = useState("");
  const [page, setPage] = useState(1);
  const [busy, setBusy] = useState(false);
  const active = useRef(true);
  const submitting = useRef(false);
  const openRef = useRef(open);
  const originals = useRef<
    Partial<Record<"create" | "attach", CanvasOperation>>
  >({});
  const returnFocus = useRef<HTMLElement | null>(null);
  useEffect(() => {
    openRef.current = open;
  }, [open]);
  useEffect(() => {
    active.current = true;
    return () => {
      active.current = false;
    };
  }, []);
  const kind = mode;
  const available = useSWR(
    open && mode === "attach"
      ? ["project-canvas-sources", identity, page]
      : null,
    async () =>
      unwrap(await canvases.listCanvases({ current: page, pageSize: 10 })),
    { shouldRetryOnError: false },
  );
  useErrorNotification(available.error, "独立画布读取失败");
  const accept = (
    value: CanvasContract.ProjectCanvasOperationStatusVO,
    original: CanvasOperation,
  ) => {
    if (!active.current) return true;
    if (
      value.commandId !== original.body.commandId ||
      String(value.projectId) !== projectId
    )
      throw new Error("画布操作结果不匹配");
    if (value.status === "COMPLETED" && value.result?.canvasId) {
      delete originals.current[original.kind];
      if (original.kind === "create") setTitle("");
      else setSourceId("");
      onCompleted();
      if (openRef.current) {
        onOpenChange(false);
        router.push(
          `/canvases/${value.result.canvasId}?projectId=${encodeURIComponent(projectId)}`,
        );
      }
      return true;
    }
    if (value.status === "ABORTED") {
      delete originals.current[original.kind];
      if (original.kind === "attach") void available.mutate();
      onCompleted();
      notifyError(
        new Error("操作未能完成，已有项目和画布均已保留。"),
        "画布操作未完成",
      );
      return true;
    }
    return false;
  };
  const submit = async () => {
    if (
      submitting.current ||
      (mode === "attach" && (available.isValidating || available.error))
    )
      return;
    let original = originals.current[mode];
    if (mode === "create") {
      const name = title.trim();
      if (!name) {
        notifyError(new Error("请输入画布名称"));
        return;
      }
      if (original?.kind !== "create" || original.body.title !== name)
        original = {
          kind: "create",
          body: { commandId: crypto.randomUUID(), projectId, title: name },
        };
    } else {
      if (
        original?.kind !== "attach" ||
        String(original.body.sourceCanvasId) !== sourceId
      ) {
        const source = available.data?.records.find(
          (item) => String(item.canvasId) === sourceId,
        );
        if (!source) {
          notifyError(new Error("请选择要归入的独立画布"));
          return;
        }
        original = {
          kind: "attach",
          body: {
            commandId: crypto.randomUUID(),
            projectId,
            sourceCanvasId: sourceId,
            expectedRevision: source.revision,
            expectedProjectBindingRevision: source.projectBindingRevision,
          },
        };
      }
    }
    originals.current[mode] = original;
    submitting.current = true;
    setBusy(true);
    let failure: unknown = new Error("操作暂未完成，请稍后重试");
    try {
      try {
        const response =
          original.kind === "create"
            ? await projects.createProjectCanvas(original.body)
            : await projects.attachProjectCanvas(original.body);
        if (accept(unwrap(response), original)) return;
      } catch (error) {
        if (!active.current) return;
        if (
          error instanceof ApiError &&
          [40000, 40300, 40301, 40400, 40900, 40901].includes(error.code)
        ) {
          delete originals.current[original.kind];
          if (original.kind === "attach") void available.mutate();
          onCompleted();
          notifyError(error, "画布操作未完成");
          return;
        }
        failure = error;
      }
      if (!active.current) return;
      // A single read resolves a lost response; no background polling or automatic writes.
      try {
        const result = unwrap(
          await projects.getProjectCanvasOperation({
            commandId: original.body.commandId,
          }),
        );
        if (accept(result, original)) return;
      } catch {
        /* Keep the original key for an explicit retry on this page. */
      }
      if (active.current) notifyError(failure, "画布操作未完成");
    } finally {
      submitting.current = false;
      if (active.current) setBusy(false);
    }
  };
  return (
    <>
      <Dialog
        open={open}
        onOpenChange={(value) => {
          if (!busy) onOpenChange(value);
        }}
      >
        <DialogContent
          onOpenAutoFocus={() => {
            returnFocus.current =
              document.activeElement instanceof HTMLElement
                ? document.activeElement
                : null;
          }}
          onCloseAutoFocus={(event) => {
            onCloseAutoFocus?.(event);
            if (!event.defaultPrevented && returnFocus.current?.isConnected) {
              event.preventDefault();
              returnFocus.current.focus();
            }
          }}
        >
          <DialogHeader>
            <DialogTitle>
              {kind === "create" ? "新建画布" : "添加已有画布"}
            </DialogTitle>
            <DialogDescription>位置：{projectPath}</DialogDescription>
          </DialogHeader>
          <form
            onSubmit={(event) => {
              event.preventDefault();
              void submit();
            }}
          >
            <FieldGroup>
              {kind === "create" ? (
                <Field>
                  <FieldLabel htmlFor="project-canvas-title">
                    画布名称
                  </FieldLabel>
                  <Input
                    id="project-canvas-title"
                    maxLength={100}
                    value={title}
                    disabled={busy}
                    onChange={(event) => setTitle(event.target.value)}
                  />
                </Field>
              ) : (
                <Field>
                  <FieldLabel htmlFor="project-canvas-source">
                    独立画布
                  </FieldLabel>
                  <Select
                    value={sourceId}
                    onValueChange={setSourceId}
                    disabled={busy || available.isLoading}
                  >
                    <SelectTrigger id="project-canvas-source">
                      <SelectValue
                        placeholder={
                          available.isLoading ? "正在读取" : "选择画布"
                        }
                      />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectGroup>
                        {available.data?.records.map((item) => (
                          <SelectItem
                            key={String(item.canvasId)}
                            value={String(item.canvasId)}
                          >
                            {item.title}
                          </SelectItem>
                        ))}
                      </SelectGroup>
                    </SelectContent>
                  </Select>
                  {available.data?.records.length === 0 && (
                    <FieldDescription>暂无可归入的独立画布。</FieldDescription>
                  )}
                  {available.error && (
                    <Button
                      type="button"
                      variant="outline"
                      onClick={() => void available.mutate()}
                    >
                      重新读取
                    </Button>
                  )}
                  {available.data && (
                    <div className="flex gap-2">
                      <Button
                        type="button"
                        variant="outline"
                        disabled={page === 1}
                        onClick={() => {
                          setPage(page - 1);
                          setSourceId("");
                        }}
                      >
                        上一页
                      </Button>
                      <Button
                        type="button"
                        variant="outline"
                        disabled={page * 10 >= Number(available.data.total)}
                        onClick={() => {
                          setPage(page + 1);
                          setSourceId("");
                        }}
                      >
                        下一页
                      </Button>
                    </div>
                  )}
                </Field>
              )}
              <DialogFooter>
                <Button
                  type="button"
                  variant="outline"
                  disabled={busy}
                  onClick={() => onOpenChange(false)}
                >
                  取消
                </Button>
                <Button
                  type="submit"
                  disabled={
                    busy ||
                    (mode === "attach" &&
                      (available.isValidating || !!available.error))
                  }
                >
                  {busy ? <Spinner data-icon="inline-start" /> : null}
                  {busy
                    ? "正在处理"
                    : mode === "create"
                      ? "创建画布"
                      : "确认归入"}
                </Button>
              </DialogFooter>
            </FieldGroup>
          </form>
        </DialogContent>
      </Dialog>
    </>
  );
}
