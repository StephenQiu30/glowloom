"use client";

import Link from "next/link";
import { useEffect, useState, type FormEvent } from "react";
import { useRouter, useSearchParams } from "next/navigation";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  Field,
  FieldLabel,
  FieldDescription,
  FieldGroup,
} from "@/components/ui/field";
import { ThemeToggle } from "@/components/theme-toggle";
import { ApiError } from "@/lib/request";
import { loginDestination } from "./navigation";
import {
  CURRENT_SESSION_KEY,
  changePassword,
  getCurrentSession,
  login,
  type CurrentSession,
} from "./queries";

function Failure({ error }: { error: unknown }) {
  if (!error) return null;
  return (
    <div
      role="alert"
      className="rounded-lg bg-destructive/10 p-4 text-sm text-destructive"
    >
      <p>{error instanceof Error ? error.message : "操作未完成，请重试。"}</p>
      {error instanceof ApiError && error.requestId && (
        <p className="mt-2 text-xs">请求编号：{error.requestId}</p>
      )}
    </div>
  );
}
export function LoginScreen() {
  const router = useRouter();
  const params = useSearchParams();
  const destination = loginDestination(params.get("returnTo"));
  const client = useQueryClient();
  const session = useQuery({
    queryKey: CURRENT_SESSION_KEY,
    queryFn: ({ signal }) => getCurrentSession(signal),
    retry: false,
  });
  const passwordRequired = Boolean(
    session.data?.must_change_password ||
    session.data?.user.must_change_password,
  );
  useEffect(() => {
    if (
      session.isSuccess &&
      !session.isFetching &&
      session.data &&
      !passwordRequired
    )
      router.replace(destination);
  }, [
    destination,
    passwordRequired,
    router,
    session.data,
    session.isSuccess,
    session.isFetching,
  ]);
  async function authenticated(value: CurrentSession) {
    await client.cancelQueries();
    client.removeQueries({
      predicate: (query) =>
        query.queryKey.length !== 2 ||
        query.queryKey[0] !== CURRENT_SESSION_KEY[0] ||
        query.queryKey[1] !== CURRENT_SESSION_KEY[1],
    });
    client.setQueryData(CURRENT_SESSION_KEY, value);
  }
  async function changed() {
    await client.cancelQueries();
    client.removeQueries({
      predicate: (query) =>
        query.queryKey.length !== 2 ||
        query.queryKey[0] !== CURRENT_SESSION_KEY[0] ||
        query.queryKey[1] !== CURRENT_SESSION_KEY[1],
    });
    const value = await getCurrentSession();
    client.setQueryData(CURRENT_SESSION_KEY, value);
  }
  const unauthenticated =
    session.error instanceof ApiError && session.error.status === 401;
  return (
    <main className="flex min-h-screen flex-col bg-background text-foreground">
      <header className="flex items-center justify-between px-8 py-6">
        <Link
          href="/projects"
          className="rounded text-lg font-semibold tracking-tight focus-visible:ring-2"
        >
          Lanverse
        </Link>
        <ThemeToggle />
      </header>
      <div className="mx-auto grid w-full max-w-5xl flex-1 items-center gap-14 px-8 py-16 lg:grid-cols-2">
        <section>
          <p className="font-mono text-xs tracking-widest text-muted-foreground">
            LANVERSE / CREATIVE WORKSPACE
          </p>
          <h1 className="mt-5 text-5xl leading-tight font-semibold tracking-tighter">
            让每个故事，
            <br />
            都有自己的画面。
          </h1>
          <p className="mt-6 max-w-sm text-sm leading-8 text-muted-foreground">
            从故事、角色到镜头和声音。回到你的工作台，继续创作。
          </p>
        </section>
        <section className="w-full max-w-md rounded-2xl bg-muted/40 p-8">
          {session.isPending ? (
            <p role="status">正在检查会话…</p>
          ) : unauthenticated ? (
            <LoginForm authenticated={authenticated} />
          ) : session.error ? (
            <>
              <Failure error={session.error} />
              <Button className="mt-5" onClick={() => void session.refetch()}>
                重试连接
              </Button>
            </>
          ) : passwordRequired ? (
            <PasswordForm
              changed={changed}
              recheck={() => void session.refetch()}
              checking={session.isFetching}
            />
          ) : (
            <p role="status">正在打开工作区…</p>
          )}
        </section>
      </div>
    </main>
  );
}
function LoginForm({
  authenticated,
}: {
  authenticated: (session: CurrentSession) => Promise<void>;
}) {
  const [name, setName] = useState("");
  const [password, setPassword] = useState("");
  const mutation = useMutation({
    mutationFn: () => login(name.trim(), password),
    onSuccess: authenticated,
  });
  function submit(event: FormEvent) {
    event.preventDefault();
    if (!mutation.isPending) mutation.mutate();
  }
  return (
    <>
      <h2 className="text-2xl font-semibold">登录工作台</h2>
      <p className="mt-3 text-sm leading-7 text-muted-foreground">
        使用你的工作区账号继续创作。
      </p>
      <form className="mt-7 space-y-6" onSubmit={submit}>
        <FieldGroup>
          <Field>
            <FieldLabel htmlFor="login-name">账号</FieldLabel>
            <Input
              id="login-name"
              name="username"
              value={name}
              onChange={(event) => setName(event.target.value)}
              autoComplete="username"
              required
              maxLength={128}
              disabled={mutation.isPending}
            />
          </Field>
          <Field>
            <FieldLabel htmlFor="login-password">密码</FieldLabel>
            <Input
              id="login-password"
              name="password"
              type="password"
              value={password}
              onChange={(event) => setPassword(event.target.value)}
              autoComplete="current-password"
              required
              maxLength={1024}
              disabled={mutation.isPending}
            />
          </Field>
        </FieldGroup>
        <Failure error={mutation.error} />
        <Button type="submit" className="w-full" disabled={mutation.isPending}>
          {mutation.isPending ? "正在登录…" : "登录"}
        </Button>
      </form>
    </>
  );
}
function PasswordForm({
  changed,
  recheck,
  checking,
}: {
  changed: () => Promise<void>;
  recheck: () => void;
  checking: boolean;
}) {
  const [current, setCurrent] = useState("");
  const [password, setPassword] = useState("");
  const [confirmation, setConfirmation] = useState("");
  const [error, setError] = useState("");
  const mutation = useMutation({
    mutationFn: () => changePassword(current, password, crypto.randomUUID()),
    onSuccess: changed,
  });
  function submit(event: FormEvent) {
    event.preventDefault();
    if (mutation.isPending) return;
    if (password !== confirmation) {
      setError("两次输入的新密码不一致。");
      return;
    }
    if (password === current) {
      setError("新密码应与当前密码不同。");
      return;
    }
    if (
      new TextEncoder().encode(password).length > 72 ||
      [...password].length < 10 ||
      !/\p{L}/u.test(password) ||
      !/\p{Nd}/u.test(password)
    ) {
      setError("新密码至少 10 个字符，包含字母和数字；过长时请缩短。");
      return;
    }
    setError("");
    mutation.mutate();
  }
  return (
    <>
      <h2 className="text-2xl font-semibold">修改初始密码</h2>
      <p className="mt-3 text-sm leading-7 text-muted-foreground">
        首次登录，请先设置新的密码。
      </p>
      <form className="mt-7 space-y-6" onSubmit={submit}>
        <FieldGroup>
          <Field>
            <FieldLabel htmlFor="current-password">当前密码</FieldLabel>
            <Input
              id="current-password"
              name="current-password"
              type="password"
              value={current}
              onChange={(event) => setCurrent(event.target.value)}
              autoComplete="current-password"
              required
              maxLength={1024}
              disabled={mutation.isPending}
            />
          </Field>
          <Field>
            <FieldLabel htmlFor="new-password">新密码</FieldLabel>
            <Input
              id="new-password"
              name="new-password"
              type="password"
              value={password}
              onChange={(event) => setPassword(event.target.value)}
              autoComplete="new-password"
              required
              minLength={10}
              maxLength={72}
              disabled={mutation.isPending}
            />
            <FieldDescription>
              至少 10 个字符，包含字母和数字。
            </FieldDescription>
          </Field>
          <Field>
            <FieldLabel htmlFor="confirm-password">再次输入新密码</FieldLabel>
            <Input
              id="confirm-password"
              name="confirm-password"
              type="password"
              value={confirmation}
              onChange={(event) => setConfirmation(event.target.value)}
              autoComplete="new-password"
              required
              maxLength={72}
              disabled={mutation.isPending}
            />
          </Field>
        </FieldGroup>
        {error && (
          <p role="alert" className="text-sm text-destructive">
            {error}
          </p>
        )}
        <Failure error={mutation.error} />
        {mutation.isError && (
          <Button
            type="button"
            variant="secondary"
            className="w-full"
            disabled={checking || mutation.isPending}
            onClick={recheck}
          >
            重新检查会话
          </Button>
        )}
        <Button type="submit" className="w-full" disabled={mutation.isPending}>
          {mutation.isPending ? "正在更新…" : "更新密码"}
        </Button>
      </form>
    </>
  );
}
