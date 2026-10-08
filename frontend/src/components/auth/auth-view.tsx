"use client";

import { Brand } from "@/components/layout/brand";
import { demoNotice } from "@/components/feedback/demo-notice";
import { cn } from "cn";
import Link from "next/link";
import { useState } from "react";
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
}: {
  id: string;
  value: string;
  onChange: (value: string) => void;
  invalid?: boolean;
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
        autoComplete="off"
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
          value.length >= 10,
          value.length >= 10
            ? "至少 10 位"
            : "至少 10 位（还差 " + (10 - value.length) + " 位）",
        ],
        [/[a-zA-Z]/.test(value) && /\d/.test(value), "同时包含字母与数字"],
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
    <aside className="relative hidden min-h-[900px] overflow-hidden bg-auth-art lg:block">
      <div
        className="absolute top-[216px] left-[7%] h-[370px] w-[720px]"
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
  const [username, setUsername] = useState("chendao");
  const [displayName, setDisplayName] = useState("陈导");
  const [password, setPassword] = useState("");
  const [confirmation, setConfirmation] = useState("");
  const [currentPassword, setCurrentPassword] = useState("");
  const [submitted, setSubmitted] = useState(false);
  const [remember, setRemember] = useState(false);
  const reset = mode === "reset-password";
  const register = mode === "register";
  const validPassword =
    password.length >= 10 && /[a-zA-Z]/.test(password) && /\d/.test(password);
  const mismatch = confirmation.length > 0 && confirmation !== password;
  const valid = register
    ? Boolean(
        displayName.trim() &&
        username.trim() &&
        validPassword &&
        confirmation === password,
      )
    : reset
      ? Boolean(
          currentPassword &&
          validPassword &&
          password !== currentPassword &&
          confirmation === password,
        )
      : Boolean(username.trim() && password);
  return (
    <main className="relative min-h-svh [&_[data-slot=field-group]]:gap-4">
      <header className="absolute inset-x-0 top-0 z-10 flex items-center justify-between px-6 py-7 lg:px-10">
        <Brand wordmark />
        <Button variant={reset ? "ghost" : "outline"} size="sm" asChild>
          <Link href={screenHref(register || reset ? "login" : "register")}>
            {reset ? "退出登录" : register ? "登录" : "注册"}
          </Link>
        </Button>
      </header>
      <div
        className={cn(
          reset
            ? "flex min-h-svh items-center justify-center px-6 py-28"
            : "grid min-h-svh lg:grid-cols-[55%_45%]",
        )}
      >
        {!reset ? <AuthArtwork register={register} /> : null}
        <div
          className={cn(
            reset
              ? "w-full max-w-[400px]"
              : "flex min-h-svh items-center justify-center px-6 py-28",
            !reset &&
              (register
                ? "lg:items-start lg:pt-[164px]"
                : "lg:items-start lg:pt-[230px]"),
          )}
        >
          <form
            className={cn(
              reset
                ? "flex w-full max-w-[400px] flex-col gap-5"
                : "flex w-full max-w-[360px] flex-col gap-5",
            )}
            onSubmit={(event) => {
              event.preventDefault();
              setSubmitted(true);
              if (valid) {
                demoNotice(
                  reset
                    ? "密码演示已完成"
                    : register
                      ? "演示账号已创建"
                      : "已进入演示工作台",
                );
                router.push(screenHref("home"));
              }
            }}
          >
            <div className={cn(reset ? "mb-2 text-center" : "mb-1")}>
              <h1 className="text-[28px] leading-9 font-semibold tracking-tight">
                {reset ? "设置新密码" : register ? "创建账号" : "欢迎回来"}
              </h1>
              <p className="mt-2 text-sm text-muted-foreground">
                {reset
                  ? "首次登录或密码被重置后，需要先修改密码才能继续使用。"
                  : register
                    ? "登录名创建后不可修改"
                    : "登录后继续你的创作项目"}
              </p>
            </div>
            {!register && !reset ? (
              <Alert variant="danger">
                <ShieldAlert />
                <AlertDescription>
                  登录名或密码不正确。连续错误 5 次将锁定 15 分钟。
                </AlertDescription>
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
                    maxLength={32}
                  />
                </Field>
              ) : null}
              {!reset ? (
                <Field data-invalid={submitted && !username.trim()}>
                  <FieldLabel htmlFor="login-name">登录名</FieldLabel>
                  <Input
                    id="login-name"
                    value={username}
                    onChange={(e) => setUsername(e.target.value)}
                    className="h-11"
                    autoComplete="off"
                    aria-invalid={submitted && !username.trim()}
                  />
                  {register ? (
                    <FieldDescription>✓ 可以使用</FieldDescription>
                  ) : null}
                </Field>
              ) : (
                <Field>
                  <FieldLabel htmlFor="current-password">当前密码</FieldLabel>
                  <PasswordInput
                    id="current-password"
                    value={currentPassword}
                    onChange={setCurrentPassword}
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
                    <Button variant="link" size="xs" asChild>
                      <Link href={screenHref("reset-password")}>
                        忘记密码请联系管理员
                      </Link>
                    </Button>
                  ) : null}
                </div>
                <PasswordInput
                  id="password"
                  value={password}
                  onChange={setPassword}
                  invalid={submitted && !password}
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
              disabled={(register || reset) && !valid}
            >
              {reset ? "保存并继续" : register ? "创建账号" : "登录"}
              {!register && !reset ? (
                <ArrowRight data-icon="inline-end" />
              ) : null}
            </Button>
            {reset ? (
              <p className="text-center text-xs text-muted-foreground">
                修改后，除共享终端设备上的登录会立即失效。
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
