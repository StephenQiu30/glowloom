"use client";

import { AlertCircle, ArrowLeft, ArrowRight, CheckCircle2, Mail } from "lucide-react";
import { Spinner } from "@/components/ui/spinner";
import { FieldGroup, Field, FieldLabel } from "@/components/ui/field";
import { useRouter } from "next/navigation";
import { type FormEvent, useEffect, useState } from "react";

import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Input } from "@/components/ui/input";
import { setAccessToken } from "@/lib/auth-session";
import { appApiErrorMessage } from "@/lib/server-state";
import {
  useConfirmRegistrationVerificationMutation,
  useRegisterMutation,
  useRequestRegistrationVerificationMutation,
} from "@/features/identity/endpoints";

type RegistrationStep = "email" | "verification" | "profile";

function ErrorAlert({ message }: { message: string }) {
  return (
    <Alert variant="destructive">
      <AlertCircle aria-hidden="true" />
      <AlertTitle>暂时无法继续</AlertTitle>
      <AlertDescription>{message}</AlertDescription>
    </Alert>
  );
}

export function RegistrationForm({ hydrated }: { hydrated: boolean }) {
  const router = useRouter();
  const [requestVerification, requestState] = useRequestRegistrationVerificationMutation();
  const [confirmVerification, confirmState] = useConfirmRegistrationVerificationMutation();
  const [register, registerState] = useRegisterMutation();
  const [step, setStep] = useState<RegistrationStep>("email");
  const [email, setEmail] = useState("");
  const [code, setCode] = useState("");
  const [registrationTicket, setRegistrationTicket] = useState<string | null>(null);
  const [retryAfter, setRetryAfter] = useState(0);
  const [emailSent, setEmailSent] = useState<boolean | null>(null);
  const [agreed, setAgreed] = useState(false);
  const [errorMessage, setErrorMessage] = useState<string | null>(null);

  useEffect(() => {
    if (step !== "verification" || retryAfter <= 0) return;
    const timer = window.setInterval(() => {
      setRetryAfter((current) => Math.max(0, current - 1));
    }, 1000);
    return () => window.clearInterval(timer);
  }, [retryAfter, step]);

  const requesting = requestState.isLoading;
  const confirming = confirmState.isLoading;
  const registering = registerState.isLoading;

  async function sendCode() {
    setErrorMessage(null);
    const normalizedEmail = email.trim();
    try {
      const response = await requestVerification({ email: normalizedEmail }).unwrap();
      setEmail(normalizedEmail);
      setCode("");
      setEmailSent(response.email_sent);
      setRetryAfter(response.retry_after_seconds);
      setStep("verification");
    } catch (error: unknown) {
      setErrorMessage(appApiErrorMessage(error));
    }
  }

  async function handleEmailSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    await sendCode();
  }

  async function handleVerificationSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setErrorMessage(null);
    try {
      const response = await confirmVerification({ email, code }).unwrap();
      setRegistrationTicket(response.registration_ticket);
      setStep("profile");
    } catch (error: unknown) {
      setErrorMessage(appApiErrorMessage(error));
    }
  }

  async function handleRegistrationSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!registrationTicket || !agreed) return;
    setErrorMessage(null);
    const values = new FormData(event.currentTarget);
    try {
      const response = await register({
        display_name: String(values.get("displayName") ?? "").trim(),
        password: String(values.get("password") ?? ""),
        registration_ticket: registrationTicket,
      }).unwrap();
      setAccessToken(response.access_token);
      router.replace("/projects");
    } catch (error: unknown) {
      setErrorMessage(appApiErrorMessage(error));
    }
  }

  function editEmail() {
    setStep("email");
    setCode("");
    setRegistrationTicket(null);
    setRetryAfter(0);
    setEmailSent(null);
    setErrorMessage(null);
  }

  if (step === "email") {
    return (
      <form key="email" onSubmit={handleEmailSubmit}>
        <FieldGroup className="mt-8 grid gap-5">
          <p className="text-xs font-medium text-muted-foreground">步骤 1 / 3 · 验证邮箱</p>
          <Field data-disabled={!hydrated || requesting}>
            <FieldLabel htmlFor="registration-email">邮箱</FieldLabel>
            <Input
              autoComplete="email"
              disabled={!hydrated || requesting}
              id="registration-email"
              onChange={(event) => setEmail(event.target.value)}
              placeholder="creator@example.com"
              required
              type="email"
              value={email}
            />
          </Field>
          {errorMessage ? <ErrorAlert message={errorMessage} /> : null}
          <Button size="lg" disabled={!hydrated || requesting} type="submit">
            {requesting ? (
              <Spinner data-icon="inline-start" aria-hidden="true" />
            ) : (
              <Mail data-icon="inline-start" aria-hidden="true" />
            )}
            发送验证码
          </Button>
        </FieldGroup>
      </form>
    );
  }

  if (step === "verification") {
    return (
      <form key="verification" onSubmit={handleVerificationSubmit}>
        <FieldGroup className="mt-8 grid gap-5">
          <div className="flex items-center justify-between gap-4">
            <p className="text-xs font-medium text-muted-foreground">步骤 2 / 3 · 输入验证码</p>
            <Button onClick={editEmail} size="sm" type="button" variant="ghost">
              <ArrowLeft data-icon="inline-start" aria-hidden="true" />
              修改邮箱
            </Button>
          </div>
          <Alert>
            {emailSent ? <Mail aria-hidden="true" /> : <AlertCircle aria-hidden="true" />}
            <AlertTitle>{emailSent ? "检查你的邮箱" : "未发送验证码"}</AlertTitle>
            <AlertDescription>
              {emailSent
                ? `验证码已经发送至 ${email}。`
                : "本次未发送验证码，请检查邮箱是否可用于注册，或直接登录。"}
            </AlertDescription>
          </Alert>
          <Field data-disabled={!hydrated || confirming}>
            <FieldLabel htmlFor="registration-code">验证码</FieldLabel>
            <Input
              autoComplete="one-time-code"
              disabled={!hydrated || confirming}
              id="registration-code"
              inputMode="numeric"
              maxLength={6}
              onChange={(event) => setCode(event.target.value.replace(/\D/g, "").slice(0, 6))}
              pattern="\d{6}"
              placeholder="6 位数字"
              required
              value={code}
            />
          </Field>
          {errorMessage ? <ErrorAlert message={errorMessage} /> : null}
          <Button size="lg" disabled={!hydrated || confirming || code.length !== 6} type="submit">
            {confirming ? <Spinner data-icon="inline-start" aria-hidden="true" /> : null}
            确认验证码
            <ArrowRight data-icon="inline-start" aria-hidden="true" />
          </Button>
          <Button
            disabled={!hydrated || requesting || retryAfter > 0}
            onClick={sendCode}
            type="button"
            variant="ghost"
          >
            <span aria-live="polite">
              {retryAfter > 0 ? `${retryAfter} 秒后可重新发送` : "重新发送验证码"}
            </span>
          </Button>
        </FieldGroup>
      </form>
    );
  }

  return (
    <form key="profile" onSubmit={handleRegistrationSubmit}>
      <FieldGroup className="mt-8 grid gap-5">
        <p className="text-xs font-medium text-muted-foreground">步骤 3 / 3 · 创建账号</p>
        <Alert>
          <CheckCircle2 aria-hidden="true" />
          <AlertTitle>邮箱已验证</AlertTitle>
          <AlertDescription>{email}</AlertDescription>
        </Alert>
        <Field data-disabled={!hydrated || registering}>
          <FieldLabel htmlFor="displayName">显示名称</FieldLabel>
          <Input
            autoComplete="name"
            disabled={!hydrated || registering}
            id="displayName"
            name="displayName"
            placeholder="你的创作署名"
            required
          />
        </Field>
        <Field data-disabled={!hydrated || registering}>
          <FieldLabel htmlFor="registration-password">密码</FieldLabel>
          <Input
            autoComplete="new-password"
            disabled={!hydrated || registering}
            id="registration-password"
            minLength={12}
            name="password"
            placeholder="输入账户密码"
            required
            type="password"
          />
          <p className="text-xs text-muted-foreground">至少 12 个字符，建议包含数字与符号。</p>
        </Field>
        <Field
          data-disabled={!hydrated || registering}
          orientation="horizontal"
          className="flex items-start gap-2"
        >
          <Checkbox
            checked={agreed}
            disabled={!hydrated || registering}
            id="registration-agreement"
            onCheckedChange={(checked) => setAgreed(checked === true)}
          />
          <FieldLabel
            className="pt-0.5 leading-5 font-normal text-muted-foreground"
            htmlFor="registration-agreement"
          >
            我已阅读并同意服务协议与隐私政策
          </FieldLabel>
        </Field>
        {errorMessage ? <ErrorAlert message={errorMessage} /> : null}
        <Button size="lg" disabled={!hydrated || registering || !agreed} type="submit">
          {registering ? <Spinner data-icon="inline-start" aria-hidden="true" /> : null}
          注册并开始创作
          <ArrowRight data-icon="inline-start" aria-hidden="true" />
        </Button>
      </FieldGroup>
    </form>
  );
}
