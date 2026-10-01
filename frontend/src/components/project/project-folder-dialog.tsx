"use client";

import { useEffect, useId, useRef, useState } from "react";
import dynamic from "next/dynamic";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Field,
  FieldDescription,
  FieldError,
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
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { ApiError } from "@/lib/request";
import { getProject, listProjects, type ProjectListedSummary } from "./queries";
import {
  folderName,
  type FolderCover,
  type FolderSummary,
  type FolderCommand,
} from "./folder-model";
import {
  clearFolderIntent,
  loadFolderIntent,
  saveFolderIntent,
  unknownFolderWrite,
  type FolderIntent,
  type FolderScope,
} from "./folder-intent";
import {
  FOLDERS_KEY,
  findFolder,
  findMovableProject,
  listFolders,
  listCoverImages,
  requireFolderScope,
  runFolderIntent,
  folderCoverPreviewKey,
} from "./folder-queries";
import { FolderCoverImage } from "./project-folder-card";

export type FolderSelection =
  | { action: "create" }
  | { action: "rename" | "cover" | "recycle"; folder: FolderSummary }
  | { action: "move"; project: ProjectListedSummary };
type Props = {
  scope: FolderScope;
  selection: FolderSelection | null;
  onClose: () => void;
  onChanged: (intent: FolderIntent) => void;
  returnFocus?: () => void;
};
const names = {
  create: "新建目录",
  rename: "修改目录名称",
  cover: "更换目录封面",
  move: "移动项目",
  recycle: "回收目录及所有项目",
};
const nameForm = z.object({ name: folderName });
const ProjectFolderCoverUpload = dynamic(
  () =>
    import("./project-folder-cover-upload").then(
      (module) => module.ProjectFolderCoverUpload,
    ),
  { ssr: false },
);
function message(error: unknown) {
  if (error instanceof ApiError && error.code === "scope_changed")
    return "身份或组织已变化，当前请求不能核验。请回到原身份和组织后恢复原请求。";
  if (error instanceof ApiError && error.code === "cover_unavailable")
    return "所选封面已不可用。请读取最新事实并选择当前授权的正式图片。";
  return error instanceof Error ? error.message : "请求未完成，请重试。";
}
export function ProjectFolderDialog({
  scope,
  selection,
  onClose,
  onChanged,
  returnFocus,
}: Props) {
  const [recovery] = useState(() => {
    try {
      return { intent: loadFolderIntent(sessionStorage, scope), error: null };
    } catch (error) {
      return { intent: null, error };
    }
  });
  const [intent, setIntent] = useState<FolderIntent | null>(recovery.intent);
  const [storageError, setStorageError] = useState<unknown>(recovery.error);
  const [error, setError] = useState<unknown>(null);
  const [pending, setPending] = useState(false);
  const [conflict, setConflict] = useState(false);
  const [confirmedRecycle, setConfirmedRecycle] = useState(false);
  const [target, setTarget] = useState("root");
  const [uploadOpen, setUploadOpen] = useState(false);
  const [cover, setCover] = useState<FolderCover | null | undefined>(
    selection && "folder" in selection
      ? (selection.folder.cover ?? undefined)
      : undefined,
  );
  const inFlight = useRef(false);
  const recoverButton = useRef<HTMLButtonElement>(null);
  const id = useId();
  const locked =
    pending || intent !== null || storageError !== null || uploadOpen;
  const folderId =
    selection && "folder" in selection ? selection.folder.id : undefined;
  const projectId =
    selection?.action === "move" ? selection.project.id : undefined;
  const facts = useQuery({
    queryKey: [
      ...FOLDERS_KEY,
      scope.origin,
      scope.actorId,
      scope.orgId,
      "dialog",
      folderId ?? projectId,
      id,
    ],
    queryFn: async ({ signal }) =>
      folderId
        ? {
            folder: await findFolder(folderId, scope, signal),
            project: undefined,
          }
        : {
            folder: undefined,
            project: await findMovableProject(projectId!, signal),
          },
    enabled: Boolean((folderId || projectId) && !intent),
    retry: false,
    staleTime: 0,
    refetchOnWindowFocus: false,
  });
  const destination = useQuery({
    queryKey: [
      ...FOLDERS_KEY,
      scope.origin,
      scope.actorId,
      scope.orgId,
      "destination",
      target,
      id,
    ],
    queryFn: ({ signal }) => findFolder(target, scope, signal),
    enabled: selection?.action === "move" && target !== "root" && !intent,
    retry: false,
    staleTime: 0,
    refetchOnWindowFocus: false,
  });
  const form = useForm({
    resolver: zodResolver(nameForm),
    defaultValues: {
      name:
        selection?.action === "rename" ? selection.folder.name : "未命名文件夹",
    },
  });
  const unresolved = intent !== null || storageError !== null;
  useEffect(() => {
    if (!locked) return;
    const warn = (event: BeforeUnloadEvent) => {
      event.preventDefault();
      event.returnValue = "";
    };
    window.addEventListener("beforeunload", warn);
    return () => window.removeEventListener("beforeunload", warn);
  }, [locked]);
  useEffect(() => {
    if (unresolved && !pending) recoverButton.current?.focus();
  }, [unresolved, pending]);

  async function execute(frozen: FolderIntent) {
    if (inFlight.current) return;
    inFlight.current = true;
    setPending(true);
    setError(null);
    let sent = false;
    try {
      setIntent(frozen);
      try {
        saveFolderIntent(sessionStorage, frozen);
        setStorageError(null);
      } catch (failure) {
        setStorageError(failure);
        throw failure;
      }
      if (window.location.origin !== frozen.origin)
        throw new ApiError(409, "scope_changed");
      // Only the current safe GET establishes replay authority; old CAS and body stay frozen.
      requireFolderScope(await listFolders(), frozen);
      sent = true;
      await runFolderIntent(frozen);
      try {
        clearFolderIntent(sessionStorage, frozen);
      } catch (failure) {
        setStorageError(failure);
        throw failure;
      }
      setIntent(null);
      setStorageError(null);
      setConflict(false);
      onChanged(frozen);
      onClose();
    } catch (failure) {
      setError(failure);
      if (sent && !unknownFolderWrite(failure)) {
        try {
          clearFolderIntent(sessionStorage, frozen);
          setIntent(null);
          setStorageError(null);
          setConflict(failure instanceof ApiError && failure.status === 409);
        } catch (storageFailure) {
          setStorageError(storageFailure);
        }
      }
    } finally {
      inFlight.current = false;
      setPending(false);
    }
  }
  function submit(command: FolderCommand) {
    if (locked || conflict) return;
    void execute({
      ...scope,
      version: 1,
      key: crypto.randomUUID(),
      ...command,
    });
  }
  async function refreshFacts() {
    setPending(true);
    setError(null);
    try {
      requireFolderScope(await listFolders(), scope);
      await Promise.all([
        folderId || projectId
          ? facts.refetch({ throwOnError: true })
          : Promise.resolve(),
        selection?.action === "move" && target !== "root"
          ? destination.refetch({ throwOnError: true })
          : Promise.resolve(),
      ]);
      setConflict(false);
      setConfirmedRecycle(false);
    } catch (failure) {
      setError(failure);
    } finally {
      setPending(false);
    }
  }
  const freshFolder = facts.data?.folder;
  const freshProject = facts.data?.project;
  const ready =
    selection?.action === "create" ||
    Boolean(facts.data && !facts.isFetching && !facts.error);
  return (
    <Dialog
      open={selection !== null || unresolved}
      onOpenChange={(open) => {
        if (!open && !locked) onClose();
      }}
    >
      <DialogContent
        showCloseButton={false}
        className="max-h-[90dvh] min-w-0 overflow-y-auto sm:max-w-xl"
        onCloseAutoFocus={(event) => {
          if (returnFocus) {
            event.preventDefault();
            returnFocus();
          }
        }}
        onEscapeKeyDown={(event) => {
          if (locked) event.preventDefault();
        }}
      >
        <DialogHeader>
          <DialogTitle>
            {intent
              ? "恢复目录请求"
              : selection
                ? names[selection.action]
                : "恢复目录存储"}
          </DialogTitle>
          <DialogDescription>
            目录用于当前账号的项目分类。操作会校验当前身份、版本和实际成员。
          </DialogDescription>
        </DialogHeader>
        {(error || storageError || intent) && (
          <Alert variant="destructive">
            <AlertTitle>
              {storageError
                ? "目录请求存储尚未确认"
                : intent
                  ? "目录修改结果尚未确认"
                  : "目录操作未完成"}
            </AlertTitle>
            <AlertDescription>
              {storageError
                ? message(storageError)
                : error
                  ? message(error)
                  : "刷新恢复了原请求。请人工核验，原键、正文和预期修订保持不变。"}
              {error instanceof ApiError && error.requestId && (
                <p>请求编号：{error.requestId}</p>
              )}
            </AlertDescription>
          </Alert>
        )}
        {intent ? (
          <div className="flex min-w-0 flex-col gap-2 text-sm">
            <p className="break-all">原请求编号：{intent.key}</p>
            <p>操作：{intent.action}</p>
            {"folderId" in intent && (
              <p className="break-all">目录：{intent.folderId}</p>
            )}
            {"projectId" in intent && (
              <p className="break-all">项目：{intent.projectId}</p>
            )}
            {"expected_revision" in intent.body && (
              <p>原预期修订：{intent.body.expected_revision}</p>
            )}
            {"name" in intent.body && (
              <p className="break-words">原名称：{intent.body.name}</p>
            )}
            <p>核验使用原请求。不会自动换键、改修订或重复创建新意图。</p>
          </div>
        ) : (
          selection && (
            <>
              {facts.isFetching && (folderId || projectId) && (
                <p role="status">正在读取最新事实…</p>
              )}
              {facts.error && (
                <Alert variant="destructive">
                  <AlertTitle>目录或项目事实未能读取</AlertTitle>
                  <AlertDescription>{message(facts.error)}</AlertDescription>
                  <Button
                    variant="outline"
                    onClick={() => void refreshFacts()}
                    disabled={pending}
                  >
                    重试读取事实
                  </Button>
                </Alert>
              )}
              {freshFolder && (
                <p className="text-sm text-muted-foreground">
                  当前目录修订：{freshFolder.revision} · 编号 {freshFolder.id}
                </p>
              )}
              {freshProject && (
                <p className="text-sm break-all text-muted-foreground">
                  {freshProject.name} · 项目修订 {freshProject.revision} ·
                  放置修订 {freshProject.placement_revision}
                </p>
              )}
              {(selection.action === "create" ||
                selection.action === "rename") && (
                <form
                  onSubmit={(event) =>
                    void form.handleSubmit((value) => {
                      if (selection.action === "create")
                        submit({
                          action: "create",
                          body: { name: value.name },
                        });
                      else if (freshFolder)
                        submit({
                          action: "patch",
                          folderId: freshFolder.id,
                          body: {
                            expected_revision: freshFolder.revision,
                            name: value.name,
                          },
                        });
                    })(event)
                  }
                >
                  <FieldGroup>
                    <Field data-invalid={Boolean(form.formState.errors.name)}>
                      <FieldLabel htmlFor={`${id}-name`}>目录名称</FieldLabel>
                      <Input
                        id={`${id}-name`}
                        {...form.register("name")}
                        disabled={locked}
                        aria-invalid={Boolean(form.formState.errors.name)}
                      />
                      <FieldError>
                        {form.formState.errors.name?.message}
                      </FieldError>
                      <FieldDescription>
                        1–160 个字符，可重名；以目录编号区分。
                      </FieldDescription>
                    </Field>
                    <Button
                      type="submit"
                      disabled={locked || conflict || !ready}
                    >
                      {selection.action === "create" ? "创建目录" : "保存名称"}
                    </Button>
                  </FieldGroup>
                </form>
              )}
              {selection.action === "cover" && freshFolder && (
                <FieldGroup>
                  <CoverPicker
                    scope={scope}
                    initial={freshFolder.cover}
                    unavailable={freshFolder.cover_unavailable}
                    disabled={locked}
                    selected={cover}
                    onChange={setCover}
                    uploadOpen={uploadOpen}
                    onUploadOpenChange={setUploadOpen}
                  />
                  <Button
                    disabled={
                      locked || conflict || !ready || cover === undefined
                    }
                    onClick={() =>
                      submit({
                        action: "patch",
                        folderId: freshFolder.id,
                        body: {
                          expected_revision: freshFolder.revision,
                          cover: cover!,
                        },
                      })
                    }
                  >
                    保存目录封面
                  </Button>
                </FieldGroup>
              )}
              {selection.action === "move" && (
                <FieldGroup>
                  <FolderDestination
                    scope={scope}
                    value={target}
                    onChange={setTarget}
                    disabled={locked}
                  />
                  {destination.error && (
                    <p role="alert" className="text-sm text-destructive">
                      {message(destination.error)}
                    </p>
                  )}
                  <Button
                    disabled={
                      locked ||
                      conflict ||
                      !ready ||
                      (target !== "root" &&
                        (!destination.data ||
                          destination.isFetching ||
                          Boolean(destination.error))) ||
                      freshProject?.folder_id ===
                        (target === "root" ? null : target)
                    }
                    onClick={() => {
                      if (freshProject)
                        submit({
                          action: "move",
                          projectId: freshProject.id,
                          body: {
                            expected_project_revision: freshProject.revision,
                            expected_placement_revision:
                              freshProject.placement_revision,
                            folder_id: target === "root" ? null : target,
                            expected_folder_revision:
                              target === "root"
                                ? 0
                                : destination.data!.revision,
                          },
                        });
                    }}
                  >
                    确认移动项目
                  </Button>
                </FieldGroup>
              )}
              {selection.action === "recycle" && freshFolder && (
                <FieldGroup>
                  <p className="text-sm">
                    包含当前全部 {freshFolder.project_count}{" "}
                    个项目（包括已归档项目）。所有成员均通过服务端检查后才一起回收；任何活动或待核验任务都会整体拒绝。
                  </p>
                  <p className="text-sm text-muted-foreground">
                    项目保留 30
                    天，恢复后位于根目录并保留原归档状态。目录不会恢复。封面原素材不会删除。
                  </p>
                  <Field orientation="horizontal">
                    <Checkbox
                      id={`${id}-recycle`}
                      checked={confirmedRecycle}
                      onCheckedChange={(value) =>
                        setConfirmedRecycle(value === true)
                      }
                      disabled={locked}
                    />
                    <FieldLabel htmlFor={`${id}-recycle`}>
                      确认回收目录和其中全部项目
                    </FieldLabel>
                  </Field>
                  <Button
                    variant="destructive"
                    disabled={locked || conflict || !ready || !confirmedRecycle}
                    onClick={() =>
                      submit({
                        action: "recycle",
                        folderId: freshFolder.id,
                        body: { expected_revision: freshFolder.revision },
                      })
                    }
                  >
                    确认回收目录及所有项目
                  </Button>
                </FieldGroup>
              )}
            </>
          )
        )}
        {conflict && !intent && (
          <Button
            variant="outline"
            disabled={pending}
            onClick={() => void refreshFacts()}
          >
            读取最新事实，保留草稿
          </Button>
        )}
        <DialogFooter>
          <Button variant="ghost" disabled={locked} onClick={onClose}>
            取消
          </Button>
          {unresolved && (
            <Button
              ref={recoverButton}
              disabled={pending}
              onClick={() => {
                if (intent) void execute(intent);
                else {
                  try {
                    const restored = loadFolderIntent(sessionStorage, scope);
                    setIntent(restored);
                    setStorageError(null);
                    setError(null);
                  } catch (failure) {
                    setStorageError(failure);
                  }
                }
              }}
            >
              {pending
                ? "正在核验…"
                : storageError
                  ? "恢复存储并核验原请求"
                  : "用原键核验目录请求"}
            </Button>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function FolderDestination({
  scope,
  value,
  onChange,
  disabled,
}: {
  scope: FolderScope;
  value: string;
  onChange: (value: string) => void;
  disabled: boolean;
}) {
  const [cursor, setCursor] = useState<string | undefined>();
  const page = useQuery({
    queryKey: [
      ...FOLDERS_KEY,
      scope.origin,
      scope.actorId,
      scope.orgId,
      "destinations",
      cursor,
    ],
    queryFn: async ({ signal }) => {
      const result = await listFolders(cursor, signal);
      requireFolderScope(result, scope);
      return result;
    },
    retry: false,
    refetchOnWindowFocus: false,
  });
  const id = useId();
  return (
    <Field>
      <FieldLabel htmlFor={id}>目标目录</FieldLabel>
      <Select value={value} onValueChange={onChange} disabled={disabled}>
        <SelectTrigger id={id}>
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          <SelectGroup>
            <SelectItem value="root">根目录</SelectItem>
            {value !== "root" &&
              !page.data?.items.some((folder) => folder.id === value) && (
                <SelectItem value={value}>
                  已选择目录 · {value.slice(0, 8)}
                </SelectItem>
              )}
            {page.data?.items.map((folder) => (
              <SelectItem key={folder.id} value={folder.id}>
                {folder.name} · {folder.id.slice(0, 8)}
              </SelectItem>
            ))}
          </SelectGroup>
        </SelectContent>
      </Select>
      {page.isFetching && <FieldDescription>正在读取目录…</FieldDescription>}
      {page.error && (
        <FieldDescription role="alert">{message(page.error)}</FieldDescription>
      )}
      <div className="flex flex-wrap gap-2">
        {(cursor || page.error) && (
          <Button
            type="button"
            variant="outline"
            disabled={disabled || page.isFetching}
            onClick={() => {
              setCursor(undefined);
              void page.refetch();
            }}
          >
            重读目录首页
          </Button>
        )}
        {page.data?.next_cursor && (
          <Button
            type="button"
            variant="outline"
            disabled={disabled || page.isFetching}
            onClick={() => setCursor(page.data!.next_cursor!)}
          >
            下一页目录选项
          </Button>
        )}
      </div>
    </Field>
  );
}
function CoverPicker({
  scope,
  initial,
  unavailable,
  disabled,
  selected,
  onChange,
  uploadOpen,
  onUploadOpenChange,
}: {
  scope: FolderScope;
  initial: FolderCover | null;
  unavailable: boolean;
  disabled: boolean;
  selected: FolderCover | null | undefined;
  onChange: (cover: FolderCover | null) => void;
  uploadOpen: boolean;
  onUploadOpenChange: (open: boolean) => void;
}) {
  const cache = useQueryClient();
  const [projectId, setProjectId] = useState(initial?.project_id ?? "");
  const [projectCursor, setProjectCursor] = useState<string | undefined>();
  const [mediaCursor, setMediaCursor] = useState<string | undefined>();
  const id = useId();
  const projects = useQuery({
    queryKey: [
      ...FOLDERS_KEY,
      scope.origin,
      scope.actorId,
      scope.orgId,
      "cover-projects",
      projectCursor,
    ],
    queryFn: ({ signal }) =>
      listProjects(
        { deleted: false, limit: 20, cursor: projectCursor },
        signal,
      ),
    retry: false,
    refetchOnWindowFocus: false,
  });
  const images = useQuery({
    queryKey: [
      ...FOLDERS_KEY,
      scope.origin,
      scope.actorId,
      scope.orgId,
      "cover-images",
      projectId,
      mediaCursor,
    ],
    queryFn: ({ signal }) => listCoverImages(projectId, mediaCursor, signal),
    enabled: Boolean(projectId),
    retry: false,
    refetchOnWindowFocus: false,
  });
  const sourceProject = useQuery({
    queryKey: [
      ...FOLDERS_KEY,
      scope.origin,
      scope.actorId,
      scope.orgId,
      "cover-source",
      projectId,
    ],
    queryFn: ({ signal }) => getProject(projectId, signal),
    enabled: Boolean(projectId),
    retry: false,
    refetchOnWindowFocus: false,
  });
  return (
    <>
      <Field>
        <FieldLabel htmlFor={`${id}-project`}>封面来源项目</FieldLabel>
        <Select
          value={projectId}
          disabled={disabled}
          onValueChange={(value) => {
            setProjectId(value);
            setMediaCursor(undefined);
          }}
        >
          <SelectTrigger id={`${id}-project`}>
            <SelectValue placeholder="选择可访问项目" />
          </SelectTrigger>
          <SelectContent>
            <SelectGroup>
              {projectId &&
                !projects.data?.items.some(
                  (project) => project.id === projectId,
                ) && (
                  <SelectItem value={projectId}>
                    当前来源 · {projectId.slice(0, 8)}
                  </SelectItem>
                )}
              {projects.data?.items.map((project) => (
                <SelectItem key={project.id} value={project.id}>
                  {project.name} · {project.id.slice(0, 8)}
                </SelectItem>
              ))}
            </SelectGroup>
          </SelectContent>
        </Select>
        <FieldDescription>
          只选择当前授权、已准备且已审核的正式图片。本地封面先上传到当前可编辑的来源项目，再保存目录封面。
        </FieldDescription>
        {projects.error && (
          <FieldDescription role="alert">
            {message(projects.error)}
          </FieldDescription>
        )}
        <div className="flex flex-wrap gap-2">
          {(projectCursor || projects.error) && (
            <Button
              type="button"
              variant="outline"
              disabled={disabled || projects.isFetching}
              onClick={() => {
                setProjectCursor(undefined);
                void projects.refetch();
              }}
            >
              重读来源项目首页
            </Button>
          )}
          {projects.data?.next_cursor && (
            <Button
              type="button"
              variant="outline"
              disabled={disabled || projects.isFetching}
              onClick={() => setProjectCursor(projects.data!.next_cursor!)}
            >
              下一页来源项目
            </Button>
          )}
        </div>
      </Field>
      <Button
        type="button"
        variant="outline"
        disabled={
          disabled ||
          !sourceProject.data ||
          sourceProject.isFetching ||
          Boolean(sourceProject.error) ||
          sourceProject.data.status !== "active" ||
          sourceProject.data.is_delete
        }
        onClick={() => onUploadOpenChange(true)}
      >
        上传本地图片作为封面
      </Button>
      {projectId &&
        (sourceProject.error ||
          sourceProject.data?.status === "archived" ||
          sourceProject.data?.is_delete) && (
          <p className="text-sm text-muted-foreground">
            {sourceProject.error
              ? message(sourceProject.error)
              : "所选来源项目不可编辑，请选择当前可编辑项目后上传本地封面。"}
          </p>
        )}
      {uploadOpen && (
        <ProjectFolderCoverUpload
          key={projectId}
          projectId={projectId}
          scope={scope}
          onClose={() => onUploadOpenChange(false)}
          onSelected={(value) => {
            onChange(value);
            setMediaCursor(undefined);
            void images.refetch();
          }}
        />
      )}
      {projectId && (
        <Field>
          <FieldLabel htmlFor={`${id}-asset`}>正式图片</FieldLabel>
          <Select
            value={selected?.project_id === projectId ? selected.asset_id : ""}
            disabled={disabled || images.isFetching}
            onValueChange={(assetId) =>
              onChange({ project_id: projectId, asset_id: assetId })
            }
          >
            <SelectTrigger id={`${id}-asset`}>
              <SelectValue placeholder="选择正式图片" />
            </SelectTrigger>
            <SelectContent>
              <SelectGroup>
                {selected?.project_id === projectId &&
                  !images.data?.items.some(
                    (asset) => asset.id === selected.asset_id,
                  ) && (
                    <SelectItem value={selected.asset_id}>
                      当前封面 · {selected.asset_id.slice(0, 8)}
                    </SelectItem>
                  )}
                {images.data?.items.map((asset) => (
                  <SelectItem key={asset.id} value={asset.id}>
                    {asset.file_name} · {asset.id.slice(0, 8)}
                  </SelectItem>
                ))}
              </SelectGroup>
            </SelectContent>
          </Select>
          {images.isFetching && (
            <FieldDescription>正在读取正式图片…</FieldDescription>
          )}
          {images.data?.items.length === 0 && (
            <FieldDescription>这一页没有正式图片。</FieldDescription>
          )}
          {images.error && (
            <FieldDescription role="alert">
              {message(images.error)}
            </FieldDescription>
          )}
          <div className="flex flex-wrap gap-2">
            {(mediaCursor || images.error) && (
              <Button
                type="button"
                variant="outline"
                disabled={disabled || images.isFetching}
                onClick={() => {
                  setMediaCursor(undefined);
                  void images.refetch();
                }}
              >
                重读图片首页
              </Button>
            )}
            {images.data?.next_cursor && (
              <Button
                type="button"
                variant="outline"
                disabled={disabled || images.isFetching}
                onClick={() => setMediaCursor(images.data!.next_cursor!)}
              >
                下一页正式素材
              </Button>
            )}
          </div>
        </Field>
      )}
      {selected && (
        <div className="relative aspect-video overflow-hidden rounded-lg bg-muted">
          <FolderCoverImage
            key={`${selected.project_id}:${selected.asset_id}`}
            cover={selected}
            scope={scope}
            alt="选中的目录封面"
          />
        </div>
      )}
      {selected && (
        <Button
          type="button"
          variant="outline"
          disabled={disabled}
          onClick={() =>
            void cache.invalidateQueries({
              queryKey: folderCoverPreviewKey(scope, selected),
            })
          }
        >
          重读封面预览
        </Button>
      )}
      {(initial || unavailable || selected) && (
        <Button
          type="button"
          variant="outline"
          disabled={disabled}
          onClick={() => onChange(null)}
        >
          移除目录封面
        </Button>
      )}
      {selected === null && (
        <p className="text-sm text-muted-foreground">
          保存后移除目录封面，保留原素材。
        </p>
      )}
    </>
  );
}
