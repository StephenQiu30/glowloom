"use client";

import { Clapperboard, LoaderCircle } from "lucide-react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { type FormEvent, useState } from "react";

import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  appApiErrorMessage,
  useLoginMutation,
  useRegisterMutation,
} from "@/lib/app-api";
import { setAccessToken } from "@/lib/auth-session";

type AuthMode = "login" | "register";

export function AuthForm({ mode }: { mode: AuthMode }) {
  const router = useRouter();
  const [login, loginState] = useLoginMutation();
  const [register, registerState] = useRegisterMutation();
  const [errorMessage, setErrorMessage] = useState<string | null>(null);
  const isRegister = mode === "register";
  const isSubmitting = loginState.isLoading || registerState.isLoading;

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setErrorMessage(null);
    const form = new FormData(event.currentTarget);
    const email = String(form.get("email"));
    const password = String(form.get("password"));

    try {
      const auth = isRegister
        ? await register({
            email,
            password,
            display_name: String(form.get("displayName")),
          }).unwrap()
        : await login({ email, password }).unwrap();
      setAccessToken(auth.access_token);
      router.replace("/projects");
    } catch (error: unknown) {
      setErrorMessage(appApiErrorMessage(error));
    }
  }

  return (
    <main className="grid min-h-screen place-items-center bg-muted/40 px-5 py-12">
      <div className="w-full max-w-md">
        <Link className="mb-8 flex items-center justify-center gap-2 font-semibold" href="/">
          <span className="grid size-9 place-items-center rounded-xl bg-primary text-primary-foreground">
            <Clapperboard className="size-5" aria-hidden="true" />
          </span>
          Lanverse
        </Link>
        <Card>
          <CardHeader>
            <CardTitle className="text-2xl" role="heading" aria-level={1}>
              {isRegister ? "创建账号" : "登录 Lanverse"}
            </CardTitle>
            <CardDescription>
              {isRegister
                ? "创建个人工作空间，开始管理你的短剧项目。"
                : "继续进入你的 AI 短剧制作工作台。"}
            </CardDescription>
          </CardHeader>
          <CardContent>
            <form className="grid gap-5" onSubmit={handleSubmit}>
              {isRegister && (
                <div className="grid gap-2">
                  <Label htmlFor="displayName">显示名称</Label>
                  <Input id="displayName" name="displayName" required maxLength={80} />
                </div>
              )}
              <div className="grid gap-2">
                <Label htmlFor="email">邮箱</Label>
                <Input id="email" name="email" type="email" autoComplete="email" required />
              </div>
              <div className="grid gap-2">
                <Label htmlFor="password">密码</Label>
                <Input
                  id="password"
                  name="password"
                  type="password"
                  autoComplete={isRegister ? "new-password" : "current-password"}
                  minLength={isRegister ? 12 : undefined}
                  maxLength={128}
                  required
                />
                {isRegister && (
                  <p className="text-xs text-muted-foreground">使用 12 至 128 个字符。</p>
                )}
              </div>
              {errorMessage && (
                <Alert variant="destructive" role="alert">
                  <AlertTitle>无法继续</AlertTitle>
                  <AlertDescription>{errorMessage}</AlertDescription>
                </Alert>
              )}
              <Button className="w-full" size="lg" disabled={isSubmitting} type="submit">
                {isSubmitting && <LoaderCircle className="animate-spin" aria-hidden="true" />}
                {isRegister ? "注册并开始创作" : "登录"}
              </Button>
            </form>
            <p className="mt-6 text-center text-sm text-muted-foreground">
              {isRegister ? "已有账号？" : "还没有账号？"}
              <Button asChild variant="link" className="px-1">
                <Link href={isRegister ? "/login" : "/register"}>
                  {isRegister ? "直接登录" : "创建账号"}
                </Link>
              </Button>
            </p>
          </CardContent>
        </Card>
      </div>
    </main>
  );
}
