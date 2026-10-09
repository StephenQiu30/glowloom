"use client";

import { Brand } from "@/components/layout/brand";
import {
  changePassword,
  createSession,
  deleteSession,
  getLoginAvailability,
  getSession,
  registerAccount,
} from "./generated/auth";
import { AuthError, requireSession } from "./request";
import { cn } from "cn";
import Link from "next/link";
import { useEffect, useRef, useState } from "react";
import { useRouter } from "next/navigation";
import {
  ArrowRight,
  Check,
  Circle,
  Eye,
  EyeOff,
  ShieldAlert,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Field,
  FieldLabel,
  FieldGroup,
  FieldDescription,
  FieldError,
} from "@/components/ui/field";
import {
  InputGroup,
  InputGroupInput,
  InputGroupAddon,
  InputGroupButton,
} from "@/components/ui/input-group";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { screenHref } from "@/components/layout/routes";

function PasswordInput({
  id,
  value,
  onChange,
  invalid = false,
  autoComplete = "new-password",
}: {
  id: string;
  value: string;
  onChange: (value: string) => void;
  invalid?: boolean;
  autoComplete?: string;
}) {
  const [visible, setVisible] = useState(false);
  return (
    <InputGroup className="h-11">
      <InputGroupInput
        id={id}
        type={visible ? "text" : "password"}
        value={value}
        onChange={(e) => onChange(e.target.value)}
        aria-invalid={invalid}
        autoComplete={autoComplete}
      />
      <InputGroupAddon align="inline-end">
        <InputGroupButton
          aria-label={visible ? "隐藏密码" : "显示密码"}
          onClick={() => setVisible(!visible)}
        >
          {visible ? <EyeOff /> : <Eye />}
        </InputGroupButton>
      </InputGroupAddon>
    </InputGroup>
  );
}

function PasswordRules({ value }: { value: string }) {
  return (
    <ul className="flex flex-col gap-1 text-xs text-muted-foreground">
      {[
        [
          Array.from(value).length >= 10,
          Array.from(value).length >= 10
            ? "至少 10 位"
            : "至少 10 位（还差 " + (10 - Array.from(value).length) + " 位）",
        ],
        [/\p{L}/u.test(value) && /\p{Nd}/u.test(value), "同时包含字母与数字"],
        [new TextEncoder().encode(value).length <= 72, "不超过 72 字节"],
      ].map(([valid, text]) => (
        <li className="flex items-center gap-2" key={String(text)}>
          {valid ? <Check className="size-3" /> : <Circle className="size-3" />}
          {String(text)}
        </li>
      ))}
    </ul>
  );
}

function AuthArtwork({ register }: { register: boolean }) {
  return (
    <aside className="relative hidden min-h-0 overflow-hidden bg-auth-art lg:block">
      <div
        className="absolute top-[clamp(96px,calc(50dvh-280px),216px)] left-[7%] h-[370px] w-[720px] origin-top-left [@media(max-height:600px)]:hidden [@media(max-height:760px)]:scale-85"
        aria-hidden="true"
      >
        <svg className="absolute inset-0 size-full" viewBox="0 0 720 370">
          <path
            d="M170 75 C240 75 240 146 320 146 M170 299 C250 299 242 175 320 175 M548 154 C605 154 594 86 658 86"
            fill="none"
            stroke="var(--border)"
          />
          <circle
            cx="320"
            cy="146"
            r="4"
            fill="var(--surface-1)"
            stroke="var(--muted-foreground)"
          />
          <circle
            cx="320"
            cy="175"
            r="4"
            fill="var(--surface-1)"
            stroke="var(--muted-foreground)"
          />
        </svg>
        <div className="absolute top-8 left-0 w-44 -rotate-2">
          <p className="mb-2 text-[10px] text-muted-foreground">角色 · 林舟</p>
          <div className="h-56 rounded-lg bg-surface-2" />
          <p className="mt-2 text-[10px] text-muted-foreground">
            场景 · 旧码头 夜
          </p>
          <div className="mt-2 h-28 rounded-lg bg-surface-1" />
        </div>
        <div className="absolute top-0 left-[318px] w-[228px]">
          <div className="mb-2 flex justify-between text-[10px] text-muted-foreground">
            <span>镜头 S03-02-04</span>
            <Badge variant="warning">生成中</Badge>
          </div>
          <div className="rounded-xl border border-primary bg-surface-1 p-2 shadow-2xl">
            <div className="relative h-[280px] rounded-lg bg-surface-3">
              <div className="absolute right-1 bottom-1 left-1 h-1 rounded-full bg-surface-4">
                <div className="h-full w-3/5 rounded-full bg-primary" />
              </div>
            </div>
            <p className="px-1 py-2 text-[10px] text-muted-foreground">
              中景推进，林舟在雨中抬头回望。
            </p>
          </div>
        </div>
        <div className="absolute top-12 left-[612px] h-64 w-40 rotate-3 rounded-lg bg-surface-2" />
      </div>
      <div className="absolute right-12 bottom-12 left-10">
        <p className="font-serif text-[40px] leading-[1.35] tracking-wide">
          浮光跃金，
          <br />
          静影沉璧。
        </p>
        <p className="mt-5 max-w-lg text-sm leading-7 text-muted-foreground">
          {register
            ? "一帧成诗，万帧光影。创建账号，把故事放进画布。"
            : "从剧本到成片的无限画布。每一个镜头、角色与声音，都在同一张画布上生长。"}
        </p>
      </div>
    </aside>
  );
}

export function AuthView({
  mode,
}: {
  mode: "login" | "register" | "reset-password";
}) {
  const router = useRouter();
  const [username, setUsername] = useState("");
  const [displayName, setDisplayName] = useState("");
  const [password, setPassword] = useState("");
  const [confirmation, setConfirmation] = useState("");
  const [currentPassword, setCurrentPassword] = useState("");
  const [submitted, setSubmitted] = useState(false);
  const [remember, setRemember] = useState(false);
  const [pending, setPending] = useState(false);
  const [error, setError] = useState("");
  const [session, setSession] = useState<AuthAPI.SessionView | null>(null);
  const [availability, setAvailability] = useState("");
  const submitting = useRef(false);
  const availabilityAbort = useRef<AbortController | null>(null);
  const reset = mode === "reset-password";
  useEffect(() => {
    const controller = new AbortController();
    if (reset)
      getSession({ signal: controller.signal })
        .then((value) => setSession(requireSession(value)))
        .catch((reason) => {
          if (controller.signal.aborted) return;
          if (reason instanceof AuthError && reason.status === 401)
            router.replace(screenHref("login"));
          else
            setError(
              reason instanceof Error
                ? reason.message
                : "无法读取会话，请重试。",
            );
        });
    return () => {
      controller.abort();
      availabilityAbort.current?.abort();
    };
  }, [reset, router]);
  const register = mode === "register";
  const validPassword =
    Array.from(password).length >= 10 &&
    /\p{L}/u.test(password) &&
    /\p{Nd}/u.test(password) &&
    new TextEncoder().encode(password).length <= 72;
  const mismatch = confirmation.length > 0 && confirmation !== password;
  const valid = register
    ? Boolean(
        displayName.trim() &&
        Array.from(displayName.trim()).length <= 32 &&
        /^[a-zA-Z0-9._-]{1,64}$/.test(username.trim()) &&
        validPassword &&
        confirmation === password,
      )
    : reset
      ? Boolean(
          session &&
          currentPassword &&
          validPassword &&
          password !== currentPassword &&
          confirmation === password,
        )
      : Boolean(username.trim() && password);
  async function submit() {
    setSubmitted(true);
    if (!valid || submitting.current) return;
    submitting.current = true;
    setPending(true);
    setError("");
    try {
      const value =
        reset && session
          ? await changePassword({
              expected_credential_revision: session.actor.credential_revision,
              current_password: currentPassword,
              new_password: password,
              confirm_password: confirmation,
            })
          : register
            ? await registerAccount({
                login_name: username,
                display_name: displayName,
                password,
                confirm_password: confirmation,
              })
            : await createSession({
                login_name: username,
                password,
                persistent: remember,
              });
      const result = requireSession(value);
      router.push(
        screenHref(
          result.actor.must_change_password ? "reset-password" : "home",
        ),
      );
    } catch (reason) {
      setError(
        reason instanceof Error ? reason.message : "请求结果未确认，请重试。",
      );
    } finally {
      submitting.current = false;
      setPending(false);
    }
  }
  async function logout() {
    if (!session || pending) return;
    setPending(true);
    setError("");
    try {
      await deleteSession({ session_id: session.session_id });
      router.replace(screenHref("login"));
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : "退出结果未确认。");
    } finally {
      setPending(false);
    }
  }
  async function checkAvailability() {
    if (!register || !/^[a-zA-Z0-9._-]{1,64}$/.test(username.trim())) return;
    availabilityAbort.current?.abort();
    const controller = new AbortController();
    availabilityAbort.current = controller;
    setAvailability("正在检查登录名…");
    try {
      const result = await getLoginAvailability(
        { login_name: username },
        { signal: controller.signal },
      );
      if (typeof result?.available !== "boolean")
        throw new AuthError("invalid_receipt");
      if (!controller.signal.aborted)
        setAvailability(result.available ? "登录名可以使用" : "登录名已被使用");
    } catch {
      if (!controller.signal.aborted)
        setAvailability("暂时无法检查，提交时将再次校验。");
    }
  }
  return (
    <main className="relative h-dvh overflow-hidden [&_[data-slot=field-group]]:gap-4">
      <header className="absolute inset-x-0 top-0 z-10 flex items-center justify-between px-6 py-7 lg:px-10">
        <Brand wordmark />
        {reset ? (
          <Button
            variant="ghost"
            size="sm"
            onClick={logout}
            disabled={!session || pending}
          >
            退出登录
          </Button>
        ) : (
          <Button variant="outline" size="sm" asChild>
            <Link href={screenHref(register ? "login" : "register")}>
              {register ? "登录" : "注册"}
            </Link>
          </Button>
        )}
      </header>
      <div
        className={cn(
          "grid h-full min-h-0",
          !reset && "lg:grid-cols-[55%_45%]",
        )}
      >
        {!reset ? <AuthArtwork register={register} /> : null}
        <div className="mt-24 flex min-h-0 flex-col items-center overflow-y-auto px-6 pb-6">
          <form
            className={cn(
              "my-auto flex w-full shrink-0 flex-col gap-4",
              reset ? "max-w-[400px]" : "max-w-[360px]",
            )}
            onSubmit={(event) => {
              event.preventDefault();
              void submit();
            }}
          >
            <div className={cn(reset ? "mb-2 text-center" : "mb-1")}>
              <h1 className="text-[28px] leading-9 font-semibold tracking-tight">
                {reset ? "设置新密码" : register ? "创建账号" : "欢迎回来"}
              </h1>
              <p className="mt-2 text-sm text-muted-foreground">
                {reset
                  ? session?.actor.must_change_password
                    ? "首次登录或密码被重置后，需要先修改密码才能继续使用。"
                    : "修改后保留当前设备的会话，其他设备需要重新登录。"
                  : register
                    ? "登录名创建后不可修改"
                    : "登录后继续你的创作项目"}
              </p>
            </div>
            {error ? (
              <Alert variant="danger">
                <ShieldAlert />
                <AlertDescription>{error}</AlertDescription>
              </Alert>
            ) : null}
            <FieldGroup className="gap-5">
              {register ? (
                <Field>
                  <FieldLabel htmlFor="display-name">显示名</FieldLabel>
                  <Input
                    id="display-name"
                    value={displayName}
                    onChange={(e) => setDisplayName(e.target.value)}
                    className="h-11"
                  />
                </Field>
              ) : null}
              {!reset ? (
                <Field data-invalid={submitted && !username.trim()}>
                  <FieldLabel htmlFor="login-name">登录名</FieldLabel>
                  <Input
                    id="login-name"
                    value={username}
                    onChange={(e) => {
                      setUsername(e.target.value);
                      availabilityAbort.current?.abort();
                      setAvailability("");
                    }}
                    onBlur={() => void checkAvailability()}
                    maxLength={64}
                    className="h-11"
                    autoComplete="username"
                    aria-invalid={submitted && !username.trim()}
                  />
                  {register ? (
                    <FieldDescription>
                      {availability || "仅允许英文、数字及 . _ -，1～64字符"}
                    </FieldDescription>
                  ) : null}
                </Field>
              ) : (
                <Field>
                  <FieldLabel htmlFor="current-password">当前密码</FieldLabel>
                  <PasswordInput
                    id="current-password"
                    value={currentPassword}
                    onChange={setCurrentPassword}
                    autoComplete="current-password"
                  />
                </Field>
              )}
              <Field
                data-invalid={submitted && !validPassword && mode !== "login"}
              >
                <div className="flex items-center justify-between">
                  <FieldLabel htmlFor="password">
                    {reset ? "新密码" : "密码"}
                  </FieldLabel>
                  {mode === "login" ? (
                    <span className="text-xs text-muted-foreground">
                      忘记密码请联系管理员
                    </span>
                  ) : null}
                </div>
                <PasswordInput
                  id="password"
                  value={password}
                  onChange={setPassword}
                  invalid={submitted && !password}
                  autoComplete={
                    mode === "login" ? "current-password" : "new-password"
                  }
                />
                {register ? (
                  <div
                    className="grid grid-cols-3 gap-1"
                    aria-label={
                      validPassword ? "密码强度：满足规则" : "密码强度：待完善"
                    }
                  >
                    {[0, 1, 2].map((part) => (
                      <span
                        key={part}
                        className={cn(
                          "h-0.5 rounded-full",
                          validPassword ? "bg-primary" : "bg-surface-4",
                        )}
                      />
                    ))}
                  </div>
                ) : null}
                {register || reset ? <PasswordRules value={password} /> : null}
                {reset ? (
                  <p className="text-xs text-muted-foreground">
                    {password !== currentPassword && password ? "✓" : "○"}{" "}
                    与当前密码不同
                  </p>
                ) : null}
              </Field>
              {register || reset ? (
                <Field data-invalid={mismatch}>
                  <FieldLabel htmlFor="confirm-password">
                    确认{reset ? "新" : ""}密码
                  </FieldLabel>
                  <PasswordInput
                    id="confirm-password"
                    value={confirmation}
                    onChange={setConfirmation}
                    invalid={mismatch}
                  />
                  {mismatch ? (
                    <FieldError>两次输入的密码不一致</FieldError>
                  ) : null}
                </Field>
              ) : (
                <Field orientation="horizontal">
                  <Checkbox
                    id="remember"
                    checked={remember}
                    onCheckedChange={(value) => setRemember(value === true)}
                  />
                  <FieldLabel htmlFor="remember" className="text-sm">
                    在此设备保持登录
                  </FieldLabel>
                </Field>
              )}
            </FieldGroup>
            {submitted && !valid ? (
              <p role="alert" className="text-xs text-destructive">
                请完成以上信息后继续。
              </p>
            ) : null}
            <Button
              size="lg"
              type="submit"
              className="h-11 w-full"
              disabled={pending || ((register || reset) && !valid)}
            >
              {pending
                ? "正在提交…"
                : reset
                  ? "保存并继续"
                  : register
                    ? "创建账号"
                    : "登录"}
              {!register && !reset ? (
                <ArrowRight data-icon="inline-end" />
              ) : null}
            </Button>
            {reset ? (
              <p className="text-center text-xs text-muted-foreground">
                修改后保留当前设备并轮换会话，其他设备上的登录立即失效。
              </p>
            ) : (
              <p className="text-xs text-muted-foreground">
                {register ? "已有账号？" : "还没有账号？"}
                <Link
                  className="text-foreground underline underline-offset-4"
                  href={screenHref(register ? "login" : "register")}
                >
                  {register ? "登录" : "注册"}
                </Link>
              </p>
            )}
          </form>
        </div>
      </div>
    </main>
  );
}
