"use client";
import dynamic from "next/dynamic";
import Link from "next/link";
import { useRouter, useSearchParams } from "next/navigation";
import {
  useCallback,
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
} from "react";
import {
  useInfiniteQuery,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import {
  CURRENT_SESSION_KEY,
  getCurrentSession,
  logout,
} from "@/features/auth/queries";
import { ApiError } from "@/lib/request";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Field, FieldLabel } from "@/components/ui/field";
import {
  Select,
  SelectContent,
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
import { ThemeToggle } from "@/components/theme-toggle";
import {
  createCanvas,
  deleteCanvas,
  getCanvas,
  listCanvases,
  listProjects,
  renameCanvas,
  saveCanvasCommands,
} from "./queries";
const Editor = dynamic(
  () => import("./editor").then((module) => module.CanvasEditor),
  { ssr: false, loading: () => <p role="status">正在加载无限画布…</p> },
);
const uuid =
  /^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i;
function Failure({
  error,
  retry,
  retryLabel = "重新读取",
}: {
  error: unknown;
  retry?: () => void;
  retryLabel?: string;
}) {
  return (
    <div
      role="alert"
      className="space-y-3 rounded-xl bg-destructive/10 p-4 text-sm"
    >
      <p>{error instanceof Error ? error.message : "请求未完成。"}</p>
      {error instanceof ApiError && error.requestId && (
        <p className="text-xs">请求编号：{error.requestId}</p>
      )}
      {retry && (
        <Button variant="secondary" onClick={retry}>
          {retryLabel}
        </Button>
      )}
    </div>
  );
}
export function CanvasWorkspace({
  initialProjectId,
}: { initialProjectId?: string } = {}) {
  const router = useRouter(),
    params = useSearchParams(),
    cache = useQueryClient();
  const projectId = initialProjectId ?? params.get("project") ?? "",
    canvasId = params.get("canvas") ?? "";
  const [dirty, setDirty] = useState(false),
    [busy, setBusy] = useState(false),
    [leave, setLeave] = useState<{ run: () => void }>(),
    [name, setName] = useState(""),
    [renameName, setRenameName] = useState(""),
    [deleting, setDeleting] = useState(false),
    [epoch, setEpoch] = useState(0);
  const leaving = useRef(false),
    href = useRef(""),
    dirtyRef = useRef(false),
    keys = useRef<Record<string, { body: string; key: string }>>({});
  const session = useQuery({
    queryKey: CURRENT_SESSION_KEY,
    queryFn: ({ signal }) => getCurrentSession(signal),
    retry: false,
    refetchOnWindowFocus: false,
  });
  const allowed = Boolean(
    session.isSuccess &&
    !session.isFetching &&
    !session.error &&
    session.data &&
    !session.data.user.must_change_password,
  );
  const projects = useInfiniteQuery({
    queryKey: ["canvas", "projects", session.data?.user.id],
    enabled: allowed,
    initialPageParam: undefined as string | undefined,
    queryFn: ({ pageParam, signal }) => listProjects(pageParam, signal),
    getNextPageParam: (page) => page.next_cursor ?? undefined,
    retry: false,
    refetchOnWindowFocus: false,
  });
  const items = projects.data?.pages.flatMap((page) => page.items) ?? [],
    project = items.find((item) => item.id === projectId);
  const canvases = useQuery({
    queryKey: ["canvas", "documents", projectId],
    enabled: Boolean(project && allowed),
    queryFn: ({ signal }) => listCanvases(projectId, signal),
    retry: false,
    refetchOnWindowFocus: false,
  });
  const document = useQuery({
    queryKey: ["canvas", "document", canvasId],
    enabled: Boolean(project && allowed && uuid.test(canvasId)),
    queryFn: ({ signal }) => getCanvas(canvasId, signal),
    retry: false,
    refetchOnWindowFocus: false,
  });
  const nameDirty = Boolean(document.data && renameName !== document.data.name);
  const requestLeave = useCallback(
    (run: () => void) => {
      if (dirtyRef.current) setLeave({ run });
      else run();
    },
    [setLeave],
  );
  useEffect(() => {
    href.current = window.location.href;
  }, [projectId, canvasId]);
  const renamedDocumentId = useRef<string | undefined>(undefined);
  useEffect(() => {
    if (renamedDocumentId.current !== document.data?.id) {
      renamedDocumentId.current = document.data?.id;
      setRenameName(document.data?.name ?? "");
    }
  }, [document.data?.id, document.data?.name]);
  useEffect(() => {
    if (
      (session.error instanceof ApiError && session.error.status === 401) ||
      session.data?.user.must_change_password
    ) {
      cache.removeQueries({ queryKey: ["canvas"] });
      router.replace(
        `/login?returnTo=${encodeURIComponent(window.location.pathname + window.location.search)}`,
      );
    }
  }, [session.error, session.data, cache, router]);
  useEffect(() => {
    if (
      allowed &&
      projectId &&
      !project &&
      projects.hasNextPage &&
      !projects.isFetchingNextPage &&
      !projects.error
    )
      void projects.fetchNextPage();
  }, [allowed, projectId, project, projects]);
  useEffect(() => {
    const warn = (event: BeforeUnloadEvent) => {
      if (dirtyRef.current && !leaving.current) {
        event.preventDefault();
        event.returnValue = "";
      }
    };
    const click = (event: MouseEvent) => {
      if (
        !dirtyRef.current ||
        leaving.current ||
        event.button ||
        event.ctrlKey ||
        event.metaKey ||
        event.shiftKey ||
        event.altKey
      )
        return;
      const anchor =
        event.target instanceof Element
          ? event.target.closest("a[href]")
          : null;
      if (
        !(anchor instanceof HTMLAnchorElement) ||
        anchor.target === "_blank" ||
        anchor.hasAttribute("download") ||
        anchor.getAttribute("href")?.startsWith("#")
      )
        return;
      const target = anchor.href;
      if (target === window.location.href) return;
      event.preventDefault();
      event.stopPropagation();
      requestLeave(() => {
        leaving.current = true;
        window.location.assign(target);
      });
    };
    const back = (event: PopStateEvent) => {
      if (!dirtyRef.current || leaving.current) return;
      const target = window.location.href;
      event.stopImmediatePropagation();
      window.history.pushState(window.history.state, "", href.current);
      requestLeave(() => {
        leaving.current = true;
        window.location.assign(target);
      });
    };
    window.addEventListener("beforeunload", warn);
    window.document.addEventListener("click", click, true);
    window.addEventListener("popstate", back, true);
    return () => {
      window.removeEventListener("beforeunload", warn);
      window.document.removeEventListener("click", click, true);
      window.removeEventListener("popstate", back, true);
    };
  }, [requestLeave]);
  function authFailure(error: ApiError) {
    if (error.status === 401) {
      cache.removeQueries({ queryKey: ["canvas"] });
      void cache.invalidateQueries({ queryKey: CURRENT_SESSION_KEY });
    }
  }
  function navigate(pid: string, cid = "") {
    const query = new URLSearchParams();
    if (pid) query.set("project", pid);
    if (cid) query.set("canvas", cid);
    setDirty(false);
    setRenameName("");
    router.replace(`/canvas${query.size ? `?${query}` : ""}`, {
      scroll: false,
    });
  }
  function choose(pid: string, cid = "") {
    requestLeave(() => navigate(pid, cid));
  }
  function key(operation: string, body: unknown) {
    const serialized = JSON.stringify(body);
    if (keys.current[operation]?.body !== serialized)
      keys.current[operation] = { body: serialized, key: crypto.randomUUID() };
    return keys.current[operation].key;
  }
  function refreshList() {
    void cache.invalidateQueries({
      queryKey: ["canvas", "documents", projectId],
    });
  }
  const create = useMutation({
    mutationFn: () =>
      createCanvas(
        projectId,
        name.trim(),
        key("create", { projectId, name: name.trim() }),
      ),
    onSuccess: (created) => {
      setName("");
      delete keys.current.create;
      refreshList();
      cache.setQueryData(["canvas", "document", created.id], created);
      navigate(projectId, created.id);
    },
    onError: (error) => {
      if (error instanceof ApiError) authFailure(error);
    },
  });
  const rename = useMutation({
    mutationFn: () =>
      renameCanvas(
        canvasId,
        document.data!.revision,
        renameName.trim(),
        key("rename", {
          canvasId,
          revision: document.data!.revision,
          name: renameName.trim(),
        }),
      ),
    onSuccess: (saved) => {
      cache.setQueryData(["canvas", "document", canvasId], saved);
      refreshList();
      delete keys.current.rename;
      setRenameName(saved.name);
      setEpoch((value) => value + 1);
    },
    onError: (error) => {
      if (error instanceof ApiError) authFailure(error);
    },
  });
  const remove = useMutation({
    mutationFn: () =>
      deleteCanvas(
        canvasId,
        document.data!.revision,
        key("delete", { canvasId, revision: document.data!.revision }),
      ),
    onSuccess: () => {
      setDeleting(false);
      setDirty(false);
      setRenameName("");
      cache.removeQueries({ queryKey: ["canvas", "document", canvasId] });
      refreshList();
      delete keys.current.delete;
      router.replace(`/canvas?${new URLSearchParams({ project: projectId })}`, {
        scroll: false,
      });
    },
    onError: (error) => {
      if (error instanceof ApiError) authFailure(error);
    },
  });
  const signout = useMutation({
    mutationFn: () => logout(key("logout", {})),
    onSuccess: () => {
      cache.removeQueries({ queryKey: ["canvas"] });
      cache.resetQueries({ queryKey: CURRENT_SESSION_KEY });
      router.replace("/login?returnTo=/canvas");
    },
  });
  const pending =
      create.isPending ||
      rename.isPending ||
      remove.isPending ||
      signout.isPending,
    readonly = project?.status === "archived",
    failure = projects.error ?? canvases.error ?? document.error;
  const hasUnsavedChanges =
    dirty ||
    busy ||
    nameDirty ||
    pending ||
    Boolean(create.error || rename.error || remove.error || signout.error);
  useLayoutEffect(() => {
    dirtyRef.current = hasUnsavedChanges;
  }, [hasUnsavedChanges]);
  async function recoverDocument() {
    if (pending || busy) return;
    const keepRename = nameDirty ? renameName : undefined;
    try {
      const latest = await cache.fetchQuery({
        queryKey: ["canvas", "document", canvasId],
        queryFn: () => getCanvas(canvasId),
        staleTime: 0,
      });
      setRenameName(keepRename ?? latest.name);
      setEpoch((value) => value + 1);
      setDeleting(false);
      rename.reset();
      remove.reset();
      delete keys.current.rename;
      delete keys.current.delete;
      refreshList();
    } catch (error) {
      if (error instanceof ApiError) authFailure(error);
    }
  }
  return (
    <main className="min-h-screen bg-background px-4 py-6 text-foreground lg:px-8">
      <header className="mx-auto mb-8 flex max-w-[1800px] flex-wrap items-center justify-between gap-4">
        <div className="flex items-center gap-5">
          <Link
            href="/projects"
            className="rounded text-lg font-semibold focus-visible:ring-2"
          >
            Lanverse
          </Link>
          <span className="text-xs text-muted-foreground">创作画布</span>
        </div>
        <div className="flex items-center gap-3">
          <ThemeToggle />
          {allowed && (
            <Button
              variant="ghost"
              disabled={pending || busy}
              onClick={() => requestLeave(() => signout.mutate())}
            >
              退出登录
            </Button>
          )}
        </div>
      </header>
      <div className="mx-auto max-w-[1800px] space-y-6">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">无限画布</h1>
          <p className="mt-2 text-sm text-muted-foreground">
            把文字、媒体和关系放在同一张画布中。保存由项目服务确认。
          </p>
        </div>
        {(session.isPending || session.isFetching) && (
          <p role="status">正在检查会话…</p>
        )}
        {session.error &&
          !(
            session.error instanceof ApiError && session.error.status === 401
          ) && (
            <Failure
              error={session.error}
              retry={() => void session.refetch()}
            />
          )}
        {allowed && (
          <>
            <div className="grid items-end gap-4 md:grid-cols-3">
              <Field>
                <FieldLabel htmlFor="canvas-project">项目</FieldLabel>
                <Select
                  value={projectId || undefined}
                  disabled={projects.isPending || pending || busy}
                  onValueChange={(value) => choose(value)}
                >
                  <SelectTrigger id="canvas-project" className="w-full">
                    <SelectValue placeholder="选择有权项目" />
                  </SelectTrigger>
                  <SelectContent>
                    {items.map((item) => (
                      <SelectItem key={item.id} value={item.id}>
                        {item.name}
                        {item.status === "archived" ? "（已归档）" : ""}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
                {projects.hasNextPage && (
                  <Button
                    variant="ghost"
                    disabled={projects.isFetchingNextPage}
                    onClick={() => void projects.fetchNextPage()}
                  >
                    加载更多项目
                  </Button>
                )}
              </Field>
              <Field>
                <FieldLabel htmlFor="canvas-document">画布</FieldLabel>
                <Select
                  value={canvasId || undefined}
                  disabled={!project || canvases.isPending || pending || busy}
                  onValueChange={(value) => choose(projectId, value)}
                >
                  <SelectTrigger id="canvas-document" className="w-full">
                    <SelectValue placeholder="选择已有画布" />
                  </SelectTrigger>
                  <SelectContent>
                    {canvases.data?.items.map((item) => (
                      <SelectItem key={item.id} value={item.id}>
                        {item.name}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </Field>
              <form
                className="flex items-end gap-2"
                onSubmit={(event) => {
                  event.preventDefault();
                  requestLeave(() => {
                    setDirty(false);
                    create.mutate();
                  });
                }}
              >
                <Field className="flex-1">
                  <FieldLabel htmlFor="new-canvas-name">新画布名称</FieldLabel>
                  <Input
                    id="new-canvas-name"
                    value={name}
                    onChange={(event) => setName(event.target.value)}
                    required
                    maxLength={128}
                    disabled={!project || readonly || pending || busy}
                  />
                </Field>
                <Button
                  type="submit"
                  disabled={
                    !project || readonly || !name.trim() || pending || busy
                  }
                >
                  创建画布
                </Button>
              </form>
            </div>
            {failure && (
              <Failure
                error={failure}
                retry={() => {
                  if (failure instanceof ApiError && failure.status === 401)
                    authFailure(failure);
                  else {
                    void projects.refetch();
                    if (project) void canvases.refetch();
                    if (canvasId && project) void document.refetch();
                  }
                }}
              />
            )}
            {create.error && <Failure error={create.error} />}{" "}
            {rename.error && (
              <Failure
                error={rename.error}
                retry={
                  rename.error instanceof ApiError &&
                  rename.error.status === 409
                    ? () => void recoverDocument()
                    : undefined
                }
                retryLabel="读取最新画布（保留名称草稿）"
              />
            )}{" "}
            {signout.error && <Failure error={signout.error} />}
            {!projects.isPending && !projects.error && !items.length && (
              <p role="status">
                当前账号没有可访问项目，请联系管理员创建或授权。
              </p>
            )}
            {projectId && !uuid.test(projectId) && (
              <p role="alert">
                请选择正式项目。样例标识不能用于保存服务端画布。
              </p>
            )}
            {projectId &&
              !project &&
              !projects.isPending &&
              !projects.hasNextPage &&
              !projects.error && <p role="alert">当前账号无法访问此项目。</p>}
            {project && !canvasId && !canvases.isPending && !canvases.error && (
              <p role="status">
                {canvases.data?.items.length
                  ? "选择一个画布继续创作。"
                  : "此项目暂无画布，创建后即可开始。"}
              </p>
            )}
            {canvasId && !uuid.test(canvasId) && (
              <p role="alert">画布标识无效，请选择已有画布。</p>
            )}
            {project && uuid.test(canvasId) && document.isPending && (
              <p role="status">正在读取画布…</p>
            )}
            {document.data &&
              project &&
              document.data.projectId === projectId &&
              !document.error && (
                <>
                  <div className="flex flex-wrap items-end justify-between gap-4">
                    <form
                      className="flex items-end gap-2"
                      onSubmit={(event) => {
                        event.preventDefault();
                        rename.mutate();
                      }}
                    >
                      <Field>
                        <FieldLabel htmlFor="rename-canvas">
                          画布名称
                        </FieldLabel>
                        <Input
                          id="rename-canvas"
                          value={renameName}
                          onChange={(event) =>
                            setRenameName(event.target.value)
                          }
                          required
                          maxLength={128}
                          disabled={readonly || pending || busy || dirty}
                        />
                      </Field>
                      <Button
                        variant="secondary"
                        type="submit"
                        disabled={
                          !nameDirty ||
                          !renameName.trim() ||
                          readonly ||
                          pending ||
                          busy ||
                          dirty
                        }
                      >
                        重命名画布
                      </Button>
                    </form>
                    <Button
                      variant="ghost"
                      disabled={readonly || pending || busy}
                      onClick={() => requestLeave(() => setDeleting(true))}
                    >
                      删除整个画布
                    </Button>
                  </div>
                  <Editor
                    key={`${document.data.id}:${epoch}`}
                    document={document.data}
                    readOnly={readonly}
                    onAuthFailure={authFailure}
                    onDirtyChange={setDirty}
                    onBusyChange={setBusy}
                    reload={() =>
                      cache.fetchQuery({
                        queryKey: ["canvas", "document", canvasId],
                        queryFn: () => getCanvas(canvasId),
                        staleTime: 0,
                      })
                    }
                    save={async (revision, commands, token) => {
                      const saved = await saveCanvasCommands(
                        canvasId,
                        revision,
                        commands,
                        token,
                      );
                      cache.setQueryData(
                        ["canvas", "document", canvasId],
                        saved,
                      );
                      return saved;
                    }}
                  />
                </>
              )}
            {document.data &&
              project &&
              document.data.projectId !== projectId && (
                <p role="alert">画布不属于所选项目，请重新选择。</p>
              )}
          </>
        )}
      </div>
      <Dialog
        open={Boolean(leave)}
        onOpenChange={(open) => {
          if (!open) setLeave(undefined);
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>放弃未保存修改？</DialogTitle>
            <DialogDescription>
              本页仍有草稿、失败操作或等待确认的请求。继续后这些修改不会自动保存。
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="secondary" onClick={() => setLeave(undefined)}>
              继续编辑
            </Button>
            <Button
              variant="destructive"
              disabled={busy || pending}
              onClick={() => {
                const action = leave;
                setLeave(undefined);
                setDirty(false);
                setRenameName("");
                dirtyRef.current = false;
                action?.run();
              }}
            >
              放弃并继续
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
      <Dialog open={deleting} onOpenChange={setDeleting}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>删除整个画布？</DialogTitle>
            <DialogDescription>
              “{document.data?.name}
              ”及全部节点和连接将被删除。项目媒体仍保留。此操作不能使用画布撤销恢复。
            </DialogDescription>
          </DialogHeader>
          {remove.error && (
            <Failure
              error={remove.error}
              retry={
                remove.error instanceof ApiError && remove.error.status === 409
                  ? () => void recoverDocument()
                  : undefined
              }
              retryLabel="读取最新并重新确认删除"
            />
          )}
          <DialogFooter>
            <Button
              variant="secondary"
              disabled={remove.isPending}
              onClick={() => setDeleting(false)}
            >
              取消
            </Button>
            <Button
              variant="destructive"
              disabled={
                remove.isPending ||
                !document.data ||
                (remove.error instanceof ApiError &&
                  remove.error.status === 409)
              }
              onClick={() => remove.mutate()}
            >
              {remove.isPending ? "正在删除…" : "确认删除画布"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </main>
  );
}
