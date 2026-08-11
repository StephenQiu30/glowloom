"use client";

import {
  AlertCircle,
  ArrowLeft,
  ArrowRight,
  CheckCircle2,
  LoaderCircle,
  Mail,
} from "lucide-react";
import { useRouter } from "next/navigation";
import { type FormEvent, useEffect, useState } from "react";

import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { setAccessToken } from "@/lib/auth-session";
import {
  appApiErrorMessage,
  useConfirmRegistrationVerificationMutation,
  useRegisterMutation,
  useRequestRegistrationVerificationMutation,
} from "@/lib/server-state";

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
  const [requestVerification, requestState] =
    useRequestRegistrationVerificationMutation();
  const [confirmVerification, confirmState] =
    useConfirmRegistrationVerificationMutation();
  const [register, registerState] = useRegisterMutation();
  const [step, setStep] = useState<RegistrationStep>("email");
  const [email, setEmail] = useState("");
  const [code, setCode] = useState("");
  const [registrationTicket, setRegistrationTicket] = useState<string | null>(null);
  const [retryAfter, setRetryAfter] = useState(0);
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
    setErrorMessage(null);
  }

  if (step === "email") {
    return (
      <form className="mt-8 grid gap-5" key="email" onSubmit={handleEmailSubmit}>
        <p className="text-xs font-medium text-muted-foreground">步骤 1 / 3 · 验证邮箱</p>
        <div className="grid gap-2">
          <Label htmlFor="registration-email">邮箱</Label>
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
        </div>
        {errorMessage ? <ErrorAlert message={errorMessage} /> : null}
        <Button className="h-11" disabled={!hydrated || requesting} type="submit">
          {requesting ? (
            <LoaderCircle className="animate-spin" aria-hidden="true" />
          ) : (
            <Mail aria-hidden="true" />
          )}
          发送验证码
        </Button>
      </form>
    );
  }

  if (step === "verification") {
    return (
      <form
        className="mt-8 grid gap-5"
        key="verification"
        onSubmit={handleVerificationSubmit}
      >
        <div className="flex items-center justify-between gap-4">
          <p className="text-xs font-medium text-muted-foreground">
            步骤 2 / 3 · 输入验证码
          </p>
          <Button onClick={editEmail} size="sm" type="button" variant="ghost">
            <ArrowLeft aria-hidden="true" />
            修改邮箱
          </Button>
        </div>
        <Alert>
          <Mail aria-hidden="true" />
          <AlertTitle>检查你的邮箱</AlertTitle>
          <AlertDescription>
            如果该邮箱可用于注册，验证码已经发送至 {email}。
          </AlertDescription>
        </Alert>
        <div className="grid gap-2">
          <Label htmlFor="registration-code">验证码</Label>
          <Input
            autoComplete="one-time-code"
            disabled={!hydrated || confirming}
            id="registration-code"
            inputMode="numeric"
            maxLength={6}
            onChange={(event) =>
              setCode(event.target.value.replace(/\D/g, "").slice(0, 6))
            }
            pattern="\d{6}"
            placeholder="6 位数字"
            required
            value={code}
          />
        </div>
        {errorMessage ? <ErrorAlert message={errorMessage} /> : null}
        <Button
          className="h-11"
          disabled={!hydrated || confirming || code.length !== 6}
          type="submit"
        >
          {confirming ? (
            <LoaderCircle className="animate-spin" aria-hidden="true" />
          ) : null}
          确认验证码
          <ArrowRight aria-hidden="true" />
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
      </form>
    );
  }

  return (
    <form
      className="mt-8 grid gap-5"
      key="profile"
      onSubmit={handleRegistrationSubmit}
    >
      <p className="text-xs font-medium text-muted-foreground">步骤 3 / 3 · 创建账号</p>
      <Alert>
        <CheckCircle2 aria-hidden="true" />
        <AlertTitle>邮箱已验证</AlertTitle>
        <AlertDescription>{email}</AlertDescription>
      </Alert>
      <div className="grid gap-2">
        <Label htmlFor="displayName">显示名称</Label>
        <Input
          autoComplete="name"
          disabled={!hydrated || registering}
          id="displayName"
          name="displayName"
          placeholder="你的创作署名"
          required
        />
      </div>
      <div className="grid gap-2">
        <Label htmlFor="registration-password">密码</Label>
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
        <p className="text-xs text-muted-foreground">
          至少 12 个字符，建议包含数字与符号。
        </p>
      </div>
      <div className="flex items-start gap-2">
        <Checkbox
          checked={agreed}
          disabled={!hydrated || registering}
          id="registration-agreement"
          onCheckedChange={(checked) => setAgreed(checked === true)}
        />
        <Label
          className="pt-0.5 leading-5 font-normal text-muted-foreground"
          htmlFor="registration-agreement"
        >
          我已阅读并同意服务协议与隐私政策
        </Label>
      </div>
      {errorMessage ? <ErrorAlert message={errorMessage} /> : null}
      <Button
        className="h-11"
        disabled={!hydrated || registering || !agreed}
        type="submit"
      >
        {registering ? (
          <LoaderCircle className="animate-spin" aria-hidden="true" />
        ) : null}
        注册并开始创作
        <ArrowRight aria-hidden="true" />
      </Button>
    </form>
  );
}
