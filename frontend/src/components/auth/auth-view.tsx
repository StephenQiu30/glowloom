"use client";

import Image from "next/image";
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
import { Check, Circle, Eye, EyeOff, ShieldAlert } from "lucide-react";
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
import { Spinner } from "@/components/ui/spinner";
import { screenHref } from "@/components/layout/routes";

function PasswordInput({
  id,
  value,
  onChange,
  invalid = false,
  autoComplete = "new-password",
  disabled = false,
  describedBy,
  placeholder = "输入密码",
}: {
  id: string;
  value: string;
  onChange: (value: string) => void;
  invalid?: boolean;
  autoComplete?: string;
  disabled?: boolean;
  describedBy?: string;
  placeholder?: string;
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
        disabled={disabled}
        aria-describedby={describedBy}
        placeholder={placeholder}
      />
      <InputGroupAddon align="inline-end">
        <InputGroupButton
          aria-label={visible ? "隐藏密码" : "显示密码"}
          aria-pressed={visible}
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

function AuthArtwork() {
  return (
    <aside className="auth-art">
      <Brand wordmark />
      <div className="auth-art-content">
        <p>一份灵感，多种可能</p>
        <div className="auth-previews">
          <div className="auth-preview">
            <div>
              <Image
                src="/design/perfume-77c55e0b.png"
                alt="暖金色的海边香氛创作"
                fill
                sizes="280px"
                preload
              />
            </div>
            <p>图片 · 暖金时刻</p>
          </div>
          <div className="auth-preview">
            <div>
              <Image
                src="/design/butterfly-a86a055c.jpeg"
                alt="蓝色蝴蝶翅膀的微观光影"
                fill
                sizes="248px"
              />
            </div>
            <p>参考 · 微观光影</p>
          </div>
        </div>
        <h2>让灵感，成为作品。</h2>
        <p className="auth-caption">
          从图片、声音到短片，在一张画布里继续创作。
        </p>
      </div>
    </aside>
  );
}

export function AuthView({
  mode,
  embedded = false,
  onAuthenticated,
  onContinueGuest,
  onPendingChange,
}: {
  mode: "login" | "register" | "reset-password";
  embedded?: boolean;
  onAuthenticated?: (session: AuthAPI.SessionView) => void;
  onContinueGuest?: () => void;
  onPendingChange?: (pending: boolean) => void;
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
  const [serverField, setServerField] = useState<string | null>(null);
  const [session, setSession] = useState<AuthAPI.SessionView | null>(null);
  const [availability, setAvailability] = useState("");
  const form = useRef<HTMLFormElement>(null);
  const errorAlert = useRef<HTMLDivElement>(null);
  const submitting = useRef(false);
  const requestAbort = useRef<AbortController | null>(null);
  const availabilityAbort = useRef<AbortController | null>(null);
  const reset = mode === "reset-password";
  const register = mode === "register";
  const Surface = embedded ? "div" : "main";
  useEffect(() => {
    const controller = new AbortController();
    if (reset)
      getSession({ signal: controller.signal })
        .then((value) => {
          if (!controller.signal.aborted) setSession(requireSession(value));
        })
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
      requestAbort.current?.abort();
      availabilityAbort.current?.abort();
    };
  }, [reset, router]);
  useEffect(() => {
    if (!error || pending) return;
    if (serverField)
      form.current
        ?.querySelector<HTMLInputElement>('[id="' + serverField + '"]')
        ?.focus();
    else errorAlert.current?.focus();
  }, [error, serverField, pending]);
  const validPassword =
    Array.from(password).length >= 10 &&
    /\p{L}/u.test(password) &&
    /\p{Nd}/u.test(password) &&
    new TextEncoder().encode(password).length <= 72;
  const mismatch = confirmation.length > 0 && confirmation !== password;
  const errors: Record<string, string> = {};
  if (!reset) {
    if (!username.trim()) errors["login-name"] = "请输入登录名";
    else if (register && !/^[a-zA-Z0-9._-]{1,64}$/.test(username.trim()))
      errors["login-name"] = "登录名需为 1～64 位英文、数字或 . _ -";
  }
  if (register && !displayName.trim()) errors["display-name"] = "请输入显示名";
  else if (register && Array.from(displayName.trim()).length > 32)
    errors["display-name"] = "显示名不能超过 32 个字符";
  if (reset && !currentPassword) errors["current-password"] = "请输入当前密码";
  if (!password) errors.password = "请输入密码";
  else if (mode !== "login" && !validPassword)
    errors.password = "密码需至少 10 个字符，包含字母和数字，且不超过 72 字节";
  else if (reset && password === currentPassword)
    errors.password = "新密码不能与当前密码相同";
  if (register || reset) {
    if (!confirmation) errors["confirm-password"] = "请再次输入密码";
    else if (confirmation !== password)
      errors["confirm-password"] = "两次输入的密码不一致";
  }
  const valid =
    Object.keys(errors).length === 0 && (!reset || Boolean(session));
  function fieldError(id: string) {
    if (serverField === id) return error;
    return submitted || (id === "confirm-password" && mismatch)
      ? errors[id]
      : undefined;
  }
  function clearServerError() {
    setError("");
    setServerField(null);
  }
  async function submit() {
    setSubmitted(true);
    if (submitting.current) return;
    if (!valid) {
      const first = Object.keys(errors)[0];
      if (first)
        form.current
          ?.querySelector<HTMLInputElement>('[id="' + first + '"]')
          ?.focus();
      return;
    }
    submitting.current = true;
    setPending(true);
    onPendingChange?.(true);
    clearServerError();
    const controller = new AbortController();
    requestAbort.current = controller;
    availabilityAbort.current?.abort();
    try {
      const options = { signal: controller.signal };
      const value =
        reset && session
          ? await changePassword(
              {
                expected_credential_revision: session.actor.credential_revision,
                current_password: currentPassword,
                new_password: password,
                confirm_password: confirmation,
              },
              options,
            )
          : register
            ? await registerAccount(
                {
                  login_name: username.trim(),
                  display_name: displayName.trim(),
                  password,
                  confirm_password: confirmation,
                },
                options,
              )
            : await createSession(
                { login_name: username.trim(), password, persistent: remember },
                options,
              );
      if (controller.signal.aborted) return;
      const result = requireSession(value);
      if (onAuthenticated && !result.actor.must_change_password) {
        onAuthenticated(result);
        return;
      }
      router.push(
        screenHref(
          result.actor.must_change_password ? "reset-password" : "home",
        ),
      );
    } catch (reason) {
      if (controller.signal.aborted) return;
      const field =
        reason instanceof AuthError
          ? reason.code === "login_name_taken"
            ? "login-name"
            : reason.code === "weak_password" ||
                reason.code === "password_reused"
              ? "password"
              : reason.code === "current_password_invalid"
                ? "current-password"
                : null
          : null;
      setServerField(field);
      setError(
        reason instanceof Error ? reason.message : "请求结果未确认，请重试。",
      );
      if (field)
        form.current
          ?.querySelector<HTMLInputElement>('[id="' + field + '"]')
          ?.focus();
    } finally {
      submitting.current = false;
      onPendingChange?.(false);
      if (!controller.signal.aborted) setPending(false);
    }
  }
  async function logout() {
    if (!session || pending) return;
    const controller = new AbortController();
    requestAbort.current = controller;
    setPending(true);
    clearServerError();
    try {
      await deleteSession(
        { session_id: session.session_id },
        { signal: controller.signal },
      );
      if (!controller.signal.aborted) router.replace(screenHref("login"));
    } catch (reason) {
      if (!controller.signal.aborted)
        setError(reason instanceof Error ? reason.message : "退出结果未确认。");
    } finally {
      if (!controller.signal.aborted) setPending(false);
    }
  }
  async function checkAvailability() {
    if (!register || pending || !/^[a-zA-Z0-9._-]{1,64}$/.test(username.trim()))
      return;
    availabilityAbort.current?.abort();
    const controller = new AbortController();
    availabilityAbort.current = controller;
    setAvailability("正在检查登录名…");
    try {
      const result = await getLoginAvailability(
        { login_name: username.trim() },
        { signal: controller.signal },
      );
      if (typeof result?.available !== "boolean")
        throw new AuthError("invalid_receipt");
      if (!controller.signal.aborted)
        setAvailability(
          result.available
            ? "登录名可以使用"
            : "登录名已被使用，提交时将再次校验。",
        );
    } catch {
      if (!controller.signal.aborted)
        setAvailability("暂时无法检查，提交时将再次校验。");
    }
  }
  return (
    <Surface
      className={cn(
        embedded ? "auth-embedded" : "auth-shell",
        reset && "auth-reset",
      )}
    >
      {!reset && !embedded ? <AuthArtwork /> : null}
      <div className="auth-form-panel">
        <form
          ref={form}
          noValidate
          aria-label={register ? "注册表单" : reset ? "改密表单" : "登录表单"}
          aria-busy={pending}
          className={cn(
            "my-auto flex w-full shrink-0 flex-col gap-4",
            reset ? "max-w-[400px]" : "max-w-[360px]",
          )}
          onSubmit={(event) => {
            event.preventDefault();
            void submit();
          }}
        >
          {!embedded && (
            <div className={cn(reset ? "mb-2 text-center" : "mb-1")}>
              <h1 className="text-[28px] leading-9 font-semibold tracking-tight">
                {reset ? "设置新密码" : register ? "创建账号" : "登录织光"}
              </h1>
              <p className="mt-2 text-sm text-muted-foreground">
                {reset
                  ? session?.actor.must_change_password
                    ? "首次登录或密码被重置后，需要先修改密码才能继续使用。"
                    : "修改后保留当前设备的会话，其他设备需要重新登录。"
                  : register
                    ? "登录名创建后不可修改"
                    : "登录后继续你的创作与素材。"}
              </p>
            </div>
          )}
          <FieldGroup>
            {!reset ? (
              <Field
                data-invalid={Boolean(fieldError("login-name"))}
                data-disabled={pending}
              >
                <FieldLabel htmlFor="login-name">登录名</FieldLabel>
                <Input
                  id="login-name"
                  value={username}
                  disabled={pending}
                  onChange={(e) => {
                    setUsername(e.target.value);
                    availabilityAbort.current?.abort();
                    setAvailability("");
                    clearServerError();
                  }}
                  onBlur={() => void checkAvailability()}
                  maxLength={64}
                  className="h-11"
                  placeholder="输入登录名"
                  autoComplete="username"
                  autoCapitalize="none"
                  spellCheck={false}
                  aria-invalid={Boolean(fieldError("login-name"))}
                  aria-describedby={
                    fieldError("login-name")
                      ? "login-name-error"
                      : register
                        ? "login-name-description"
                        : undefined
                  }
                />
                {fieldError("login-name") ? (
                  <FieldError id="login-name-error">
                    {fieldError("login-name")}
                  </FieldError>
                ) : register ? (
                  <FieldDescription
                    id="login-name-description"
                    aria-live="polite"
                  >
                    {availability || "支持英文字母、数字、点、下划线和短横线。"}
                  </FieldDescription>
                ) : null}
              </Field>
            ) : (
              <Field
                data-invalid={Boolean(fieldError("current-password"))}
                data-disabled={pending}
              >
                <FieldLabel htmlFor="current-password">当前密码</FieldLabel>
                <PasswordInput
                  id="current-password"
                  value={currentPassword}
                  disabled={pending}
                  onChange={(value) => {
                    setCurrentPassword(value);
                    clearServerError();
                  }}
                  invalid={Boolean(fieldError("current-password"))}
                  autoComplete="current-password"
                  describedBy={
                    fieldError("current-password")
                      ? "current-password-error"
                      : undefined
                  }
                />
                {fieldError("current-password") ? (
                  <FieldError id="current-password-error">
                    {fieldError("current-password")}
                  </FieldError>
                ) : null}
              </Field>
            )}
            {register ? (
              <Field
                data-invalid={Boolean(fieldError("display-name"))}
                data-disabled={pending}
              >
                <FieldLabel htmlFor="display-name">显示名</FieldLabel>
                <Input
                  id="display-name"
                  value={displayName}
                  disabled={pending}
                  onChange={(e) => {
                    setDisplayName(e.target.value);
                    clearServerError();
                  }}
                  className="h-11"
                  placeholder="你希望大家如何称呼你"
                  autoComplete="nickname"
                  aria-invalid={Boolean(fieldError("display-name"))}
                  aria-describedby={
                    fieldError("display-name")
                      ? "display-name-error"
                      : undefined
                  }
                />
                {fieldError("display-name") ? (
                  <FieldError id="display-name-error">
                    {fieldError("display-name")}
                  </FieldError>
                ) : null}
              </Field>
            ) : null}
            <Field
              data-invalid={Boolean(fieldError("password"))}
              data-disabled={pending}
            >
              <FieldLabel htmlFor="password">
                {reset ? "新密码" : "密码"}
              </FieldLabel>
              <PasswordInput
                id="password"
                value={password}
                disabled={pending}
                onChange={(value) => {
                  setPassword(value);
                  clearServerError();
                }}
                invalid={Boolean(fieldError("password"))}
                autoComplete={
                  mode === "login" ? "current-password" : "new-password"
                }
                describedBy={
                  fieldError("password")
                    ? "password-error"
                    : register
                      ? "password-description"
                      : undefined
                }
              />
              {fieldError("password") ? (
                <FieldError id="password-error">
                  {fieldError("password")}
                </FieldError>
              ) : register ? (
                <FieldDescription id="password-description">
                  至少 10 位，含字母和数字，不超过 72 字节。
                </FieldDescription>
              ) : null}
              {reset || (register && !embedded) ? (
                <PasswordRules value={password} />
              ) : null}
            </Field>
            {register || reset ? (
              <Field
                data-invalid={Boolean(fieldError("confirm-password"))}
                data-disabled={pending}
              >
                <FieldLabel htmlFor="confirm-password">
                  确认{reset ? "新" : ""}密码
                </FieldLabel>
                <PasswordInput
                  id="confirm-password"
                  placeholder="再次输入密码"
                  value={confirmation}
                  disabled={pending}
                  onChange={(value) => {
                    setConfirmation(value);
                    clearServerError();
                  }}
                  invalid={Boolean(fieldError("confirm-password"))}
                  describedBy={
                    fieldError("confirm-password")
                      ? "confirm-password-error"
                      : undefined
                  }
                />
                {fieldError("confirm-password") ? (
                  <FieldError id="confirm-password-error">
                    {fieldError("confirm-password")}
                  </FieldError>
                ) : null}
              </Field>
            ) : (
              <Field orientation="horizontal" data-disabled={pending}>
                <Checkbox
                  id="remember"
                  checked={remember}
                  disabled={pending}
                  onCheckedChange={(value) => setRemember(value === true)}
                />
                <FieldLabel htmlFor="remember">在此设备保持登录</FieldLabel>
              </Field>
            )}
          </FieldGroup>
          {error && !serverField ? (
            <Alert variant="danger" ref={errorAlert} tabIndex={-1}>
              <ShieldAlert />
              <AlertDescription>{error}</AlertDescription>
            </Alert>
          ) : null}
          <div className="auth-actions">
            <Button
              size="lg"
              type="submit"
              className="h-11 w-full"
              disabled={pending || (reset && !session)}
            >
              {pending ? (
                <Spinner data-icon="inline-start" aria-label="正在提交" />
              ) : null}
              {pending
                ? register
                  ? "正在创建账号…"
                  : reset
                    ? "正在保存…"
                    : "正在登录…"
                : reset
                  ? "保存并继续"
                  : register
                    ? "创建账号"
                    : "登录"}
            </Button>
            {embedded && onContinueGuest ? (
              <Button type="button" variant="ghost" onClick={onContinueGuest}>
                暂不登录，继续浏览
              </Button>
            ) : null}
            {mode === "login" ? (
              <p className="auth-help">忘记密码？联系管理员协助找回。</p>
            ) : null}
          </div>
          {reset && (
            <Button
              type="button"
              variant="ghost"
              onClick={() => void logout()}
              disabled={!session || pending}
            >
              退出登录
            </Button>
          )}
          {reset ? (
            <p className="text-center text-xs text-muted-foreground">
              修改后保留当前设备并轮换会话，其他设备上的登录立即失效。
            </p>
          ) : !embedded ? (
            <p className="text-xs text-muted-foreground">
              {register ? "已有账号？" : "还没有账号？"}
              <Link
                className="text-foreground underline underline-offset-4"
                href={screenHref(register ? "login" : "register")}
              >
                {register ? "登录" : "注册"}
              </Link>
            </p>
          ) : null}
        </form>
      </div>
    </Surface>
  );
}
