"use client";
import { FeedbackToast } from "@/components/feedback/feedback-toast";
import { pendingInvitation } from "./invitation-continuity";
import Image from "next/image";
import Link from "next/link";
import { cn } from "cn";
import { useEffect, useRef, useState, useId } from "react";
import { useRouter } from "next/navigation";
import { Eye, EyeOff } from "lucide-react";
import { Brand } from "@/components/brand/brand";
import {
  getAuthPolicy,
  createSession,
  getSession,
  changePassword,
  startRegistration,
  resendRegistration,
  verifyRegistration,
  completeRegistration,
  startPasswordReset,
  completePasswordReset,
} from "@/api/identity";
import {
  RequestError,
  setCSRFToken,
  subscribeSessionInvalidation,
} from "@/lib/request";
import { clearPrivateSession, revokeSession } from "./session";
import { branchJournal } from "@/components/generation/draft-save";
import {
  MIN_PASSWORD_LENGTH,
  MAX_PASSWORD_LENGTH,
  PASSWORD_LENGTH_HINT,
  requireSession,
  authErrorMessage,
  requireAccepted,
} from "./contracts";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Field,
  FieldGroup,
  FieldLabel,
  FieldDescription,
} from "@/components/ui/field";
import {
  Card,
  CardHeader,
  CardTitle,
  CardDescription,
  CardContent,
  CardFooter,
} from "@/components/ui/card";
import { LoadingState } from "@/components/layout/loading-state";
import { Spinner } from "@/components/ui/spinner";
import {
  InputGroup,
  InputGroupInput,
  InputGroupAddon,
  InputGroupButton,
} from "@/components/ui/input-group";
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
    <InputGroup className="h-10 max-md:h-11">
      <InputGroupInput
        id={id}
        className="h-full"
        type={visible ? "text" : "password"}
        value={value}
        onChange={(e) => onChange(e.target.value)}
        aria-invalid={invalid}
        autoComplete={autoComplete}
        disabled={disabled}
        aria-describedby={describedBy}
        placeholder={placeholder}
      />
      <InputGroupAddon align="inline-end" className="py-0">
        <InputGroupButton
          size="icon-sm"
          className="max-md:size-11"
          disabled={disabled}
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
  mode: "login" | "register" | "reset-password" | "recover";
  embedded?: boolean;
  onAuthenticated?: (session: API.EmailSessionView) => void;
  onContinueGuest?: () => void;
  onPendingChange?: (pending: boolean) => void;
}) {
  const router = useRouter();
  const [policyStatus, setPolicyStatus] = useState<
    "loading" | "error" | "ready"
  >("loading");
  const [policyRetry, setPolicyRetry] = useState(0);
  const [policyError, setPolicyError] = useState("");
  const errorID = useId();
  const [invalidField, setInvalidField] = useState("");
  const recoveryProof = useRef<string | null>(null);
  const [policy, setPolicy] = useState<API.AuthPolicyView | null>(null);
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [confirmation, setConfirmation] = useState("");
  const [currentPassword, setCurrentPassword] = useState("");
  const [displayName, setDisplayName] = useState("");
  const [code, setCode] = useState("");
  const [operation, setOperation] = useState("");
  const [grant, setGrant] = useState("");
  const [accepted, setAccepted] = useState(false);
  const [remember, setRemember] = useState(false);
  const [recoveryToken, setRecoveryToken] = useState("");
  const [session, setSession] = useState<API.EmailSessionView | null>(null);
  const [pending, setPending] = useState(false);
  const [error, setError] = useState("");
  const [feedbackAttempt, setFeedbackAttempt] = useState(0);
  const [notice, setNotice] = useState("");
  const [retryAt, setRetryAt] = useState(0);
  const [now, setNow] = useState(0);
  const flight = useRef<AbortController | null>(null);
  const form = useRef<HTMLFormElement>(null);
  const errorField = useRef("");
  const lock = useRef(false);
  const change = mode === "reset-password" && !recoveryToken;
  const securityPage =
    !embedded && (mode === "reset-password" || mode === "recover");
  const register = mode === "register";
  const invited = Boolean(pendingInvitation());
  const registrationAllowed = Boolean(
    policy?.registration_enabled || (invited && policy?.terms_version),
  );
  const recover = mode === "recover" || Boolean(recoveryToken);
  useEffect(() => {
    const controller = new AbortController();
    if (
      recoveryProof.current === null &&
      (mode === "recover" || mode === "reset-password")
    ) {
      recoveryProof.current =
        new URLSearchParams(window.location.hash.slice(1)).get("token") ?? "";
      if (recoveryProof.current)
        window.history.replaceState(
          null,
          "",
          window.location.pathname + window.location.search,
        );
    }
    const incomingToken = recoveryProof.current ?? "";
    if (change && !incomingToken)
      getSession({ signal: controller.signal })
        .then(requireSession)
        .then((value) => {
          if (!controller.signal.aborted) {
            setCSRFToken(value.csrf_token);
            setSession(value);
          }
        })
        .catch((reason) => {
          if (!controller.signal.aborted) setError(authErrorMessage(reason));
        });
    if (recover || (mode === "reset-password" && incomingToken)) {
      const token = incomingToken ?? "";
      if (token) {
        Promise.resolve().then(() => {
          if (!controller.signal.aborted) setRecoveryToken(token);
        });
      }
    }
    return () => {
      controller.abort();
      flight.current?.abort();
      onPendingChange?.(false);
    };
  }, [change, recover, register, mode, onPendingChange]);
  useEffect(() => {
    if (!register) return;
    const controller = new AbortController();
    getAuthPolicy({ signal: controller.signal })
      .then((value) => {
        if (controller.signal.aborted) return;
        if (
          !value ||
          typeof value.registration_enabled !== "boolean" ||
          typeof value.terms_version !== "string" ||
          typeof value.available_terms_version !== "string" ||
          !["/terms"].includes(value.terms_url) ||
          value.privacy_url !== "/privacy"
        )
          throw new RequestError("invalid_receipt");
        setPolicy(value);
        setPolicyStatus("ready");
        setPolicyError("");
      })
      .catch((reason) => {
        if (!controller.signal.aborted) {
          setPolicyStatus("error");
          setPolicyError(authErrorMessage(reason));
        }
      });
    return () => controller.abort();
  }, [register, policyRetry]);
  useEffect(() => {
    if (!change) return;
    return subscribeSessionInvalidation(() => {
      setSession(null);
      setPassword("");
      setCurrentPassword("");
      void clearPrivateSession();
      router.replace("/login");
    });
  }, [change, router]);
  useEffect(() => {
    if (!retryAt) return;
    const timer = window.setInterval(() => setNow(Date.now()), 1000);
    return () => clearInterval(timer);
  }, [retryAt]);
  useEffect(() => {
    if (error) {
      const input = errorField.current
        ? form.current?.querySelector<HTMLInputElement>(
            `[id="${errorField.current}"]`,
          )
        : null;
      if (input) input.focus();
    }
  }, [error, feedbackAttempt]);
  const remaining = Math.max(0, Math.ceil((retryAt - now) / 1000));
  async function switchAccount() {
    if (!session || lock.current) return;
    lock.current = true;
    setPending(true);
    setError("");
    setInvalidField("");
    onPendingChange?.(true);
    const controller = new AbortController();
    flight.current = controller;
    try {
      await revokeSession(session);
      if (!controller.signal.aborted) router.replace("/login");
    } catch (reason) {
      if (!controller.signal.aborted) setError(authErrorMessage(reason));
    } finally {
      lock.current = false;
      if (!controller.signal.aborted) {
        setPending(false);
        onPendingChange?.(false);
      }
    }
  }
  async function perform(action: "submit" | "resend" = "submit") {
    if (
      lock.current ||
      (register && (policyStatus !== "ready" || !registrationAllowed))
    )
      return;
    setFeedbackAttempt((value) => value + 1);
    setError("");
    errorField.current = "";
    setInvalidField("");
    setNotice("");
    const needsPassword =
      mode === "login" ||
      change ||
      Boolean(grant) ||
      (recover && Boolean(recoveryToken));
    if (
      (!change &&
        !recoveryToken &&
        (!/^[\x21-\x7e]+$/.test(email.trim()) ||
          !/^[^@]+@[^@]+$/.test(email.trim()))) ||
      email.trim().length > 254
    ) {
      errorField.current = "email";
      setInvalidField("email");
      setError("请输入有效的邮箱地址。");
      return;
    }
    if (needsPassword && !password) {
      errorField.current = "password";
      setInvalidField("password");
      setError("请输入密码。");
      return;
    }
    if (
      needsPassword &&
      mode !== "login" &&
      (Array.from(password).length < MIN_PASSWORD_LENGTH ||
        Array.from(password).length > MAX_PASSWORD_LENGTH ||
        password !== confirmation)
    ) {
      const field =
        Array.from(password).length < MIN_PASSWORD_LENGTH ||
        Array.from(password).length > MAX_PASSWORD_LENGTH
          ? "password"
          : "confirm-password";
      errorField.current = field;
      setInvalidField(field);
      setError(
        field === "confirm-password"
          ? "两次输入的密码不一致，请重新确认。"
          : `新密码需为 ${PASSWORD_LENGTH_HINT}。`,
      );
      return;
    }
    if (change && (!session || !currentPassword)) {
      errorField.current = "current-password";
      setInvalidField("current-password");
      setError("请先加载会话并输入当前密码。");
      return;
    }
    lock.current = true;
    setPending(true);
    onPendingChange?.(true);
    const controller = new AbortController();
    flight.current = controller;
    const options = { signal: controller.signal };
    try {
      if (register) {
        if (!operation) {
          const result = requireAccepted(
            await startRegistration(
              { email: email.trim(), invitation_token: pendingInvitation() },
              options,
            ),
          );
          if (controller.signal.aborted) return;
          setOperation(result.operation_id);
          setRetryAt(Date.now() + result.retry_after_seconds * 1000);
          setNow(Date.now());
          setNotice(
            "请求已受理。邮件提交结果尚未确认，请检查邮箱。验证码 10 分钟有效。",
          );
        } else if (action === "resend") {
          const result = requireAccepted(
            await resendRegistration({ id: operation }, options),
          );
          if (controller.signal.aborted) return;
          setRetryAt(Date.now() + result.retry_after_seconds * 1000);
          setNow(Date.now());
          setNotice("重发请求已受理。");
        } else if (!grant) {
          const result = await verifyRegistration(
            { id: operation },
            { code },
            options,
          );
          if (controller.signal.aborted) return;
          if (
            typeof result.registration_grant !== "string" ||
            !result.registration_grant
          )
            throw new RequestError("proof_invalid");
          setGrant(result.registration_grant);
          setNotice("邮箱验证成功，请设置账号。");
        } else {
          if (!accepted) {
            setError("请先阅读并同意协议。");
            return;
          }
          await completeRegistration(
            { id: operation },
            {
              registration_grant: grant,
              display_name: displayName.trim(),
              password,
              confirm_password: confirmation,
              terms_version: policy?.terms_version ?? "",
              terms_accepted: accepted,
            },
            options,
          );
          if (!controller.signal.aborted) {
            setGrant("");
            setOperation("");
            router.replace("/login");
          }
        }
      } else if (recover) {
        if (!recoveryToken) {
          const result = requireAccepted(
            await startPasswordReset({ email: email.trim() }, options),
          );
          if (controller.signal.aborted) return;
          setRetryAt(Date.now() + result.retry_after_seconds * 1000);
          setNow(Date.now());
          setNotice(
            "如果该邮箱可以恢复账号，你会收到恢复链接。请求已受理，不代表邮件已送达。",
          );
        } else {
          await completePasswordReset(
            { token: recoveryToken, password, confirm_password: confirmation },
            options,
          );
          if (!controller.signal.aborted) {
            setCSRFToken(null);
            await branchJournal.clear().catch(() => {});
            router.replace("/login");
          }
        }
      } else {
        const value =
          change && session
            ? await changePassword(
                {
                  expected_credential_epoch: session.credential_epoch,
                  current_password: currentPassword,
                  new_password: password,
                  confirm_password: confirmation,
                },
                options,
              )
            : await createSession(
                { email: email.trim(), password, persistent: remember },
                options,
              );
        if (controller.signal.aborted) return;
        const result = requireSession(value);
        setCSRFToken(result.csrf_token);
        if (onAuthenticated) onAuthenticated(result);
        else router.push(pendingInvitation() ? "/invitations" : "/");
        if (onAuthenticated && pendingInvitation()) router.push("/invitations");
      }
    } catch (reason) {
      if (!controller.signal.aborted) setError(authErrorMessage(reason));
    } finally {
      lock.current = false;
      if (!controller.signal.aborted) {
        setPending(false);
        onPendingChange?.(false);
      }
    }
  }
  const passwordStage =
    mode === "login" ||
    change ||
    Boolean(grant) ||
    (recover && Boolean(recoveryToken));
  const title = change
    ? "更新登录密码"
    : register
      ? "创建账号"
      : recover
        ? recoveryToken
          ? "设置新密码"
          : "找回密码"
        : "登录织光";
  function clearFieldError(field: string) {
    if (field === invalidField) {
      setInvalidField("");
      setError("");
    }
  }
  const formContent = (
    <form
      ref={form}
      noValidate
      aria-label={title + "表单"}
      aria-busy={pending}
      className={cn(
        "flex w-full min-w-0 flex-col gap-5",
        !securityPage && "my-auto max-w-[400px]",
      )}
      onSubmit={(event) => {
        event.preventDefault();
        void perform();
      }}
    >
      {!embedded && !securityPage ? (
        <h1 className="text-[28px] font-semibold">{title}</h1>
      ) : null}
      {change && !session && !error ? (
        <LoadingState>正在确认账号状态…</LoadingState>
      ) : null}
      <FieldGroup>
        {register ? (
          <p className="text-sm text-muted-foreground">
            {policyStatus === "loading"
              ? "正在读取注册策略…"
              : policyStatus === "error"
                ? null
                : invited && registrationAllowed
                  ? "受邀注册：请验证受邀邮箱，并阅读服务条款与隐私政策。"
                  : policy?.registration_enabled
                    ? `协议版本 ${policy.terms_version}`
                    : "注册暂未开放。请稍后再试，您仍可查阅服务条款与隐私政策。"}
          </p>
        ) : null}
        {register && policyStatus === "error" ? (
          <FeedbackToast
            message={policyError}
            action={{
              label: "重新读取注册策略",
              onClick: () => {
                setPolicyStatus("loading");
                setPolicy(null);
                setPolicyError("");
                setPolicyRetry((n) => n + 1);
              },
            }}
          />
        ) : null}
        {!change && !recoveryToken ? (
          <Field
            data-invalid={
              Boolean(error && invalidField === "email") || undefined
            }
          >
            <FieldLabel htmlFor="email">邮箱</FieldLabel>
            <Input
              id="email"
              className={securityPage ? "h-10 max-md:h-11" : undefined}
              aria-invalid={Boolean(error && invalidField === "email")}
              aria-describedby={
                error && invalidField === "email" ? errorID : undefined
              }
              type="email"
              autoComplete="email"
              value={email}
              maxLength={254}
              disabled={pending || Boolean(operation)}
              onChange={(e) => {
                setEmail(e.target.value);
                clearFieldError("email");
              }}
            />
          </Field>
        ) : null}
        {register ? (
          <p className="text-sm">
            <Link href={policy?.terms_url ?? "/terms"} target="_blank">
              服务条款
            </Link>{" "}
            ·{" "}
            <Link href={policy?.privacy_url ?? "/privacy"} target="_blank">
              隐私政策
            </Link>
          </p>
        ) : null}
        {register && operation && !grant ? (
          <Field>
            <FieldLabel htmlFor="verification-code">邮箱验证码</FieldLabel>
            <Input
              id="verification-code"
              inputMode="numeric"
              autoComplete="one-time-code"
              maxLength={6}
              value={code}
              onChange={(e) => setCode(e.target.value)}
              disabled={pending}
            />
            <Button
              type="button"
              variant="outline"
              disabled={pending || remaining > 0}
              onClick={() => void perform("resend")}
            >
              {remaining ? `${remaining} 秒后重发` : "重发验证码"}
            </Button>
          </Field>
        ) : null}
        {register && grant ? (
          <Field>
            <FieldLabel htmlFor="display-name">显示名</FieldLabel>
            <Input
              id="display-name"
              value={displayName}
              onChange={(e) => setDisplayName(e.target.value)}
              disabled={pending}
            />
          </Field>
        ) : null}
        {change ? (
          <Field
            data-invalid={
              Boolean(error && invalidField === "current-password") || undefined
            }
          >
            <FieldLabel htmlFor="current-password">当前密码</FieldLabel>
            <PasswordInput
              id="current-password"
              invalid={Boolean(error && invalidField === "current-password")}
              describedBy={
                error && invalidField === "current-password"
                  ? errorID
                  : undefined
              }
              value={currentPassword}
              onChange={(value) => {
                setCurrentPassword(value);
                clearFieldError("current-password");
              }}
              disabled={pending}
              autoComplete="current-password"
              placeholder="输入当前登录密码"
            />
          </Field>
        ) : null}
        {passwordStage ? (
          <>
            <Field
              data-invalid={
                Boolean(error && invalidField === "password") || undefined
              }
            >
              <FieldLabel htmlFor="password">
                {mode === "login" ? "密码" : "新密码"}
              </FieldLabel>
              <PasswordInput
                id="password"
                invalid={Boolean(error && invalidField === "password")}
                describedBy={
                  error && invalidField === "password"
                    ? errorID
                    : mode !== "login"
                      ? "password-help"
                      : undefined
                }
                value={password}
                onChange={(value) => {
                  setPassword(value);
                  clearFieldError("password");
                }}
                disabled={pending}
                autoComplete={
                  mode === "login" ? "current-password" : "new-password"
                }
                placeholder={
                  mode === "login"
                    ? "输入密码"
                    : `设置至少 ${MIN_PASSWORD_LENGTH} 个字符的新密码`
                }
              />
              {mode !== "login" ? (
                <FieldDescription id="password-help">
                  使用 {PASSWORD_LENGTH_HINT}，支持空格和中文。
                </FieldDescription>
              ) : null}
            </Field>
            {mode !== "login" ? (
              <>
                <Field
                  data-invalid={
                    Boolean(error && invalidField === "confirm-password") ||
                    undefined
                  }
                >
                  <FieldLabel htmlFor="confirm-password">确认新密码</FieldLabel>
                  <PasswordInput
                    id="confirm-password"
                    invalid={Boolean(
                      error && invalidField === "confirm-password",
                    )}
                    describedBy={
                      error && invalidField === "confirm-password"
                        ? errorID
                        : undefined
                    }
                    value={confirmation}
                    onChange={(value) => {
                      setConfirmation(value);
                      clearFieldError("confirm-password");
                    }}
                    disabled={pending}
                    placeholder="再次输入新密码"
                  />
                </Field>
              </>
            ) : (
              <Field orientation="horizontal">
                <Checkbox
                  id="remember-session"
                  checked={remember}
                  disabled={pending}
                  onCheckedChange={(value) => setRemember(value === true)}
                />
                <FieldLabel htmlFor="remember-session">
                  在此设备保持登录
                </FieldLabel>
              </Field>
            )}
          </>
        ) : null}
        {register && grant ? (
          <Field orientation="horizontal">
            <Checkbox
              id="accept-terms"
              checked={accepted}
              disabled={pending}
              onCheckedChange={(value) => setAccepted(value === true)}
            />
            <FieldLabel htmlFor="accept-terms">
              阅读并同意{" "}
              <Link href="/terms" target="_blank">
                服务条款
              </Link>{" "}
              和{" "}
              <Link href="/privacy" target="_blank">
                隐私政策
              </Link>
              （版本 {policy?.terms_version}）
            </FieldLabel>
          </Field>
        ) : null}
      </FieldGroup>
      {error ? (
        <>
          <FeedbackToast message={error} attempt={feedbackAttempt} />
          <span id={errorID} className="sr-only">
            {error}
          </span>
        </>
      ) : null}
      {notice ? <FeedbackToast message={notice} tone="info" /> : null}
      <Button
        type="submit"
        className={securityPage ? "h-10 max-md:h-11" : undefined}
        disabled={
          pending ||
          (change && !session) ||
          (recover && !recoveryToken && remaining > 0) ||
          (register && (policyStatus !== "ready" || !registrationAllowed))
        }
      >
        {pending ? (
          <Spinner data-icon="inline-start" aria-hidden="true" />
        ) : null}
        {pending
          ? "正在提交…"
          : register
            ? grant
              ? "创建账号"
              : operation
                ? "验证邮箱"
                : "发送验证码"
            : recover
              ? recoveryToken
                ? "保存新密码"
                : remaining
                  ? `${remaining} 秒后可重试`
                  : "发送恢复链接"
              : change
                ? "保存新密码"
                : "登录"}
      </Button>
      {change && session ? (
        <Button
          type="button"
          variant="link"
          className="h-10 max-md:h-11"
          disabled={pending}
          onClick={() => void switchAccount()}
        >
          使用其他账号登录
        </Button>
      ) : null}
      <Button
        asChild
        variant="link"
        className={securityPage ? "h-10 max-md:h-11" : undefined}
      >
        {mode === "login" ? (
          <Link href="/recover">忘记密码</Link>
        ) : (
          <Link href={change && session ? "/account" : "/login"}>
            {change && session ? "返回账号设置" : "返回登录"}
          </Link>
        )}
      </Button>
      {embedded && onContinueGuest ? (
        <Button type="button" variant="ghost" onClick={onContinueGuest}>
          暂不登录，继续浏览
        </Button>
      ) : null}
    </form>
  );
  if (securityPage)
    return (
      <main className="auth-security">
        <div className="auth-security-content">
          <Brand wordmark />
          <Card className="w-full [--card-spacing:--spacing(6)]">
            <CardHeader>
              <CardTitle>
                <h1>{title}</h1>
              </CardTitle>
              <CardDescription>
                {change
                  ? "验证当前密码，为账号设置新的登录密码。"
                  : recoveryToken
                    ? "设置新密码后，使用邮箱重新登录。"
                    : "输入注册邮箱，获取密码恢复链接。"}
              </CardDescription>
            </CardHeader>
            <CardContent>{formContent}</CardContent>
            {change ? (
              <CardFooter>
                <p className="text-xs text-muted-foreground">
                  保存成功后将退出其他设备，并更新此设备的登录会话。
                </p>
              </CardFooter>
            ) : null}
          </Card>
        </div>
      </main>
    );
  return (
    <main className={embedded ? "auth-embedded" : "auth-shell"}>
      {!embedded ? <AuthArtwork /> : null}
      <div className="auth-form-panel">{formContent}</div>
    </main>
  );
}
