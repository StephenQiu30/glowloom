"use client";
import Link from "next/link";
import { useState } from "react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  Field,
  FieldLabel,
  FieldDescription,
  FieldGroup,
} from "@/components/ui/field";
import { ThemeToggle } from "@/components/theme-toggle";
export default function LoginPage() {
  const [notice, setNotice] = useState("");
  return (
    <main className="flex min-h-screen flex-col bg-background">
      <header className="flex items-center justify-between px-8 py-6">
        <Link href="/projects" className="text-lg font-semibold tracking-tight">
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
          <h2 className="text-2xl font-semibold">登录工作台</h2>
          <p className="mt-3 text-sm leading-7 text-muted-foreground">
            前端 PoC · 登录服务尚未接入。无需填写真实账号，直接进入样例即可。
          </p>
          <form
            className="mt-7 space-y-6"
            onSubmit={(e) => {
              e.preventDefault();
              setNotice(
                "登录服务尚未接入，表单内容没有发送。请进入样例工作台。",
              );
            }}
          >
            <FieldGroup>
              <Field>
                <FieldLabel htmlFor="login-email">邮箱</FieldLabel>
                <Input
                  id="login-email"
                  type="email"
                  placeholder="creator@example.test"
                  autoComplete="off"
                />
                <FieldDescription>
                  仅表单展示，请使用演示信息。
                </FieldDescription>
              </Field>
              <Field>
                <FieldLabel htmlFor="login-password">密码</FieldLabel>
                <Input
                  id="login-password"
                  type="password"
                  autoComplete="off"
                  placeholder="演示密码"
                />
              </Field>
            </FieldGroup>
            <Button type="submit" variant="secondary" className="w-full">
              检查登录入口
            </Button>
            {notice && (
              <p role="status" className="text-sm leading-7">
                {notice}
              </p>
            )}
            <Button asChild className="w-full">
              <Link href="/projects">进入样例工作台</Link>
            </Button>
          </form>
        </section>
      </div>
    </main>
  );
}
