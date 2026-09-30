"use client";

import dynamic from "next/dynamic";
import Link from "next/link";
import { useRouter, useSearchParams } from "next/navigation";
import { useState, useRef, type FormEvent } from "react";
import {
  useInfiniteQuery,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  Field,
  FieldDescription,
  FieldGroup,
  FieldLabel,
} from "@/components/ui/field";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { ThemeToggle } from "@/components/theme-toggle";
import { ApiError } from "@/lib/request";
import {
  changePassword,
  createCanvas,
  getCanvas,
  getCurrentUser,
  listCanvases,
  listProjects,
  login,
  logout,
  saveCanvasCommands,
  type Session,
} from "./live-queries";

const LiveEditor = dynamic(
  () => import("./live-editor").then((module) => module.LiveEditor),
  {
    ssr: false,
    loading: () => <p role="status">正在加载备注编辑器…</p>,
  },
);
const sessionKey = ["auth", "me"] as const;

function Failure({ error, retry }: { error: unknown; retry?: () => void }) {
  const failure = error instanceof Error ? error : new Error("读取未完成。");
  return (
    <div
      role="alert"
      className="space-y-3 rounded-lg bg-destructive/10 p-4 text-sm"
    >
      <p>{failure.message}</p>
      {error instanceof ApiError && error.requestId && (
        <p className="text-xs">请求编号：{error.requestId}</p>
      )}
      {retry && (
        <Button variant="secondary" onClick={retry}>
          重试读取
        </Button>
      )}
    </div>
  );
}
export function LiveWorkspace() {
  const queryClient = useQueryClient();
  const session = useQuery({
    queryKey: sessionKey,
    queryFn: ({ signal }) => getCurrentUser(signal),
    retry: false,
    refetchOnWindowFocus: false,
  });
  function authenticated(value: Session) {
    queryClient.setQueryData(sessionKey, value);
  }
  function clearPrivateQueries() {
    queryClient.removeQueries({ queryKey: ["live"] });
  }
  function authFailure(error: ApiError) {
    if (error.status === 401) {
      clearPrivateQueries();
      void session.refetch();
    }
  }
  const signOut = useMutation({
    mutationFn: () => logout(crypto.randomUUID()),
    onSuccess: () => {
      clearPrivateQueries();
      queryClient.resetQueries({ queryKey: sessionKey });
    },
  });
  const unauthenticated =
    session.error instanceof ApiError && session.error.status === 401;
  return (
    <main className="min-h-screen bg-background px-5 py-6 text-foreground lg:px-10">
      <a
        href="#canvas-main"
        className="sr-only rounded bg-background p-3 focus:not-sr-only focus:absolute focus:z-50"
      >
        跳转到主要内容
      </a>
      <header className="mx-auto flex max-w-7xl flex-wrap items-center justify-between gap-4 pb-10">
        <Link
          href="/projects"
          className="rounded text-lg font-semibold focus-visible:ring-2 focus-visible:ring-blue-500"
        >
          Lanverse
        </Link>
        <div className="flex items-center gap-3">
          <Link
            href="/poc/canvas"
            className="rounded text-sm underline focus-visible:ring-2 focus-visible:ring-blue-500"
          >
            性能 PoC
          </Link>
          <ThemeToggle />
          {session.data && (
            <Button
              variant="ghost"
              disabled={signOut.isPending}
              onClick={() => signOut.mutate()}
            >
              退出登录
            </Button>
          )}
        </div>
      </header>
      <div id="canvas-main" className="mx-auto max-w-7xl space-y-8">
        <section>
          <p className="font-mono text-xs tracking-widest text-muted-foreground">
            LANVERSE / CANVAS
          </p>
          <h1 className="mt-3 text-3xl font-semibold tracking-tight">
            真实备注画布
          </h1>
          <p className="mt-3 max-w-3xl text-sm leading-7 text-muted-foreground">
            在你的项目中保存文字备注、布局和注释连接。页面刷新后从服务端恢复。
          </p>
        </section>
        {signOut.error && <Failure error={signOut.error} />}
        {session.isPending ? (
          <p role="status">正在检查登录会话…</p>
        ) : unauthenticated ? (
          <LoginForm onSuccess={authenticated} />
        ) : session.error ? (
          <Failure error={session.error} retry={() => void session.refetch()} />
        ) : session.data?.user.must_change_password ? (
          <PasswordForm onSuccess={() => void session.refetch()} />
        ) : session.data ? (
          <AuthenticatedWorkspace
            key={session.data.user.id}
            session={session.data}
            onAuthFailure={authFailure}
          />
        ) : null}
      </div>
    </main>
  );
}

function LoginForm({ onSuccess }: { onSuccess: (session: Session) => void }) {
  const [name, setName] = useState("");
  const [password, setPassword] = useState("");
  const mutation = useMutation({
    mutationFn: () => login(name, password),
    onSuccess: (session) => {
      setPassword("");
      onSuccess(session);
    },
  });
  return (
    <section className="max-w-md rounded-xl bg-muted/40 p-6">
      <h2 className="text-xl font-semibold">登录真实工作区</h2>
      <p className="mt-3 text-sm leading-7 text-muted-foreground">
        使用管理员已创建的账号。登录后只显示当前账号有权访问的项目。
      </p>
      <form
        className="mt-6 space-y-5"
        onSubmit={(event: FormEvent) => {
          event.preventDefault();
          if (!mutation.isPending) mutation.mutate();
        }}
      >
        <FieldGroup>
          <Field>
            <FieldLabel htmlFor="live-login-name">账号</FieldLabel>
            <Input
              id="live-login-name"
              autoComplete="username"
              required
              value={name}
              onChange={(event) => setName(event.target.value)}
              disabled={mutation.isPending}
            />
          </Field>
          <Field>
            <FieldLabel htmlFor="live-login-password">密码</FieldLabel>
            <Input
              id="live-login-password"
              type="password"
              autoComplete="current-password"
              required
              value={password}
              onChange={(event) => setPassword(event.target.value)}
              disabled={mutation.isPending}
            />
          </Field>
        </FieldGroup>
        {mutation.error && <Failure error={mutation.error} />}
        <Button type="submit" disabled={mutation.isPending}>
          {mutation.isPending ? "正在登录…" : "登录"}
        </Button>
      </form>
      <p className="mt-6 text-xs text-muted-foreground">
        <Link href="/projects" className="underline">
          前往固定样例工作台
        </Link>
        （样例修改只在本页保留）
      </p>
    </section>
  );
}
function PasswordForm({ onSuccess }: { onSuccess: () => void }) {
  const [current, setCurrent] = useState("");
  const [next, setNext] = useState("");
  const [confirmation, setConfirmation] = useState("");
  const [validation, setValidation] = useState("");
  const attempt = useRef<
    { current: string; next: string; key: string } | undefined
  >(undefined);
  const mutation = useMutation({
    mutationFn: () => {
      if (
        !attempt.current ||
        attempt.current.current !== current ||
        attempt.current.next !== next
      )
        attempt.current = { current, next, key: crypto.randomUUID() };
      return changePassword(current, next, attempt.current.key);
    },
    onSuccess: () => {
      setCurrent("");
      setNext("");
      setConfirmation("");
      attempt.current = undefined;
      onSuccess();
    },
  });
  return (
    <section className="max-w-md rounded-xl bg-muted/40 p-6">
      <h2 className="text-xl font-semibold">修改初始密码</h2>
      <p className="mt-3 text-sm leading-7 text-muted-foreground">
        首次登录需先修改初始密码，完成后才能访问项目与画布。
      </p>
      <form
        className="mt-6 space-y-5"
        onSubmit={(event) => {
          event.preventDefault();
          if (next !== confirmation) {
            setValidation("两次输入的新密码不一致。");
            return;
          }
          setValidation("");
          mutation.mutate();
        }}
      >
        <FieldGroup>
          <Field>
            <FieldLabel htmlFor="live-current-password">当前密码</FieldLabel>
            <Input
              id="live-current-password"
              type="password"
              autoComplete="current-password"
              required
              value={current}
              onChange={(event) => setCurrent(event.target.value)}
              disabled={mutation.isPending}
            />
          </Field>
          <Field>
            <FieldLabel htmlFor="live-new-password">新密码</FieldLabel>
            <Input
              id="live-new-password"
              type="password"
              autoComplete="new-password"
              required
              minLength={10}
              value={next}
              onChange={(event) => setNext(event.target.value)}
              disabled={mutation.isPending}
            />
            <FieldDescription>
              至少 10 个字符，包含字母与数字，且不同于当前密码。
            </FieldDescription>
          </Field>
          <Field>
            <FieldLabel htmlFor="live-confirm-password">
              再次输入新密码
            </FieldLabel>
            <Input
              id="live-confirm-password"
              type="password"
              autoComplete="new-password"
              required
              value={confirmation}
              onChange={(event) => setConfirmation(event.target.value)}
              disabled={mutation.isPending}
            />
          </Field>
        </FieldGroup>
        {validation && (
          <p role="alert" className="text-sm text-destructive">
            {validation}
          </p>
        )}
        {mutation.error && <Failure error={mutation.error} />}
        <Button type="submit" disabled={mutation.isPending}>
          {mutation.isPending ? "正在修改…" : "修改密码并继续"}
        </Button>
      </form>
    </section>
  );
}

function AuthenticatedWorkspace({
  session,
  onAuthFailure,
}: {
  session: Session;
  onAuthFailure: (error: ApiError) => void;
}) {
  const params = useSearchParams();
  const router = useRouter();
  const queryClient = useQueryClient();
  const projectId = params.get("project") ?? "";
  const canvasId = params.get("canvas") ?? "";
  const [name, setName] = useState("");
  const creationKey = useRef<
    | {
        projectId: string;
        name: string;
        key: string;
      }
    | undefined
  >(undefined);
  const projects = useInfiniteQuery({
    queryKey: ["live", "projects", session.user.id],
    initialPageParam: undefined as string | undefined,
    queryFn: ({ pageParam, signal }) => listProjects(pageParam, signal),
    getNextPageParam: (page) => page.next_cursor ?? undefined,
    retry: false,
    refetchOnWindowFocus: false,
  });
  const projectItems = projects.data?.pages.flatMap((page) => page.items) ?? [];
  const project = projectItems.find((item) => item.id === projectId);
  const canvases = useInfiniteQuery({
    queryKey: ["live", "canvases", projectId],
    initialPageParam: undefined as string | undefined,
    enabled: Boolean(project),
    queryFn: ({ pageParam, signal }) =>
      listCanvases(projectId, pageParam, signal),
    getNextPageParam: (page) => page.next_cursor ?? undefined,
    retry: false,
    refetchOnWindowFocus: false,
  });
  const document = useQuery({
    queryKey: ["live", "document", canvasId],
    queryFn: ({ signal }) => getCanvas(canvasId, signal),
    enabled: Boolean(project && canvasId),
    retry: false,
    refetchOnWindowFocus: false,
  });
  function choose(nextProject: string, nextCanvas = "") {
    const query = new URLSearchParams();
    if (nextProject) query.set("project", nextProject);
    if (nextCanvas) query.set("canvas", nextCanvas);
    router.replace(`/canvas${query.size ? `?${query}` : ""}`, {
      scroll: false,
    });
  }
  const create = useMutation({
    mutationFn: () => {
      const trimmed = name.trim();
      if (
        !creationKey.current ||
        creationKey.current.projectId !== projectId ||
        creationKey.current.name !== trimmed
      )
        creationKey.current = {
          projectId,
          name: trimmed,
          key: crypto.randomUUID(),
        };
      return createCanvas(projectId, trimmed, creationKey.current.key);
    },
    onSuccess: (created) => {
      setName("");
      creationKey.current = undefined;
      void queryClient.invalidateQueries({
        queryKey: ["live", "canvases", projectId],
      });
      queryClient.setQueryData(["live", "document", created.id], created);
      choose(projectId, created.id);
    },
    onError: (error) => {
      if (error instanceof ApiError) onAuthFailure(error);
    },
  });
  const canvasItems = canvases.data?.pages.flatMap((page) => page.items) ?? [];
  const readonly = project?.status === "archived";
  const failure = projects.error ?? canvases.error ?? document.error;
  return (
    <div className="space-y-7">
      <p className="text-sm text-muted-foreground">
        已登录：{session.user.display_name || session.user.login_name}
      </p>
      <div className="grid gap-6 lg:grid-cols-[minmax(200px,1fr)_minmax(200px,1fr)_minmax(250px,1fr)]">
        <Field>
          <FieldLabel htmlFor="live-project">项目</FieldLabel>
          <Select
            value={projectId || undefined}
            onValueChange={(value) => choose(value)}
            disabled={projects.isPending}
          >
            <SelectTrigger id="live-project" className="w-full">
              <SelectValue
                placeholder={
                  projects.isPending ? "正在读取项目…" : "选择有权项目"
                }
              />
            </SelectTrigger>
            <SelectContent>
              {projectItems.map((item) => (
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
          <FieldLabel htmlFor="live-canvas">画布</FieldLabel>
          <Select
            value={canvasId || undefined}
            onValueChange={(value) => choose(projectId, value)}
            disabled={!project || canvases.isPending}
          >
            <SelectTrigger id="live-canvas" className="w-full">
              <SelectValue
                placeholder={!project ? "先选择项目" : "选择已有画布"}
              />
            </SelectTrigger>
            <SelectContent>
              {canvasItems.map((item) => (
                <SelectItem key={item.id} value={item.id}>
                  {item.name}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          {canvases.hasNextPage && (
            <Button
              variant="ghost"
              disabled={canvases.isFetchingNextPage}
              onClick={() => void canvases.fetchNextPage()}
            >
              加载更多画布
            </Button>
          )}
        </Field>
        <form
          className="flex items-end gap-2"
          onSubmit={(event) => {
            event.preventDefault();
            if (!create.isPending) create.mutate();
          }}
        >
          <Field className="flex-1">
            <FieldLabel htmlFor="live-canvas-name">新画布名称</FieldLabel>
            <Input
              id="live-canvas-name"
              required
              maxLength={128}
              value={name}
              onChange={(event) => setName(event.target.value)}
              disabled={!project || readonly || create.isPending}
            />
          </Field>
          <Button
            type="submit"
            disabled={!project || readonly || !name.trim() || create.isPending}
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
              onAuthFailure(failure);
            else {
              void projects.refetch();
              if (project) void canvases.refetch();
              if (canvasId && project) void document.refetch();
            }
          }}
        />
      )}
      {create.error && <Failure error={create.error} />}
      {!projects.isPending && !projects.error && !projectItems.length && (
        <p role="status">当前账号没有可访问的项目，请联系管理员创建或授权。</p>
      )}
      {projectId &&
        !project &&
        !projects.isPending &&
        !projects.hasNextPage &&
        !projects.error && (
          <p role="alert">此项目不在当前账号的可访问列表中。请选择有权项目。</p>
        )}
      {project && !canvasId && !canvases.isPending && !canvases.error && (
        <p role="status">
          {canvasItems.length
            ? "选择一个已有画布继续编辑。"
            : "此项目还没有画布。创建后即可保存备注。"}
        </p>
      )}
      {project && canvasId && document.isPending && (
        <p role="status">正在读取服务端画布…</p>
      )}
      {document.data &&
        project &&
        document.data.project_id === projectId &&
        !document.error && (
          <LiveEditor
            key={document.data.id}
            document={document.data}
            readOnly={readonly}
            onAuthFailure={onAuthFailure}
            reload={() =>
              queryClient.fetchQuery({
                queryKey: ["live", "document", canvasId],
                queryFn: () => getCanvas(canvasId),
                staleTime: 0,
              })
            }
            save={async (revision, commands, key) => {
              const saved = await saveCanvasCommands(
                canvasId,
                revision,
                commands,
                key,
              );
              queryClient.setQueryData(["live", "document", canvasId], saved);
              return saved;
            }}
          />
        )}
      {document.data && project && document.data.project_id !== projectId && (
        <p role="alert">画布不属于所选项目，请重新选择。</p>
      )}
    </div>
  );
}
