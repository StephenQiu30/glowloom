"use client";

import { useState } from "react";
import Link from "next/link";
import { Monitor, UserRound } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Avatar, AvatarFallback } from "@/components/ui/avatar";
import {
  Card,
  CardHeader,
  CardTitle,
  CardDescription,
  CardContent,
  CardFooter,
} from "@/components/ui/card";
import {
  Field,
  FieldGroup,
  FieldLabel,
  FieldDescription,
} from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";
import { ProductShell, PageHeading, demoNotice, LocalDialog } from "./shared";
import { screenHref } from "./screens";

export function AccountPage() {
  const [name, setName] = useState("陈导");
  const [theme, setTheme] = useState("深色");
  const [prompt, setPrompt] = useState(
    "电影感光线，35mm 胶片质感，避免文字水印。",
  );
  const [remote, setRemote] = useState(true);
  const [avatar, setAvatar] = useState(false);
  return (
    <ProductShell screen="account" contextual>
      <PageHeading title="账号设置" />
      <div className="flex flex-col gap-5">
        <Card variant="panel" id="profile">
          <CardHeader>
            <CardTitle>个人资料</CardTitle>
            <CardDescription>
              显示名会出现在审计记录和协作标识中。
            </CardDescription>
          </CardHeader>
          <CardContent className="flex flex-col gap-5">
            <div className="flex items-center gap-4">
              <Avatar className="size-16">
                <AvatarFallback>{name[0] || "陈"}</AvatarFallback>
              </Avatar>
              <Button variant="outline" onClick={() => setAvatar(true)}>
                更换头像
              </Button>
            </div>
            <FieldGroup>
              <div className="grid gap-6 sm:grid-cols-2">
                <Field>
                  <FieldLabel htmlFor="profile-name">显示名</FieldLabel>
                  <Input
                    id="profile-name"
                    value={name}
                    onChange={(e) => setName(e.target.value)}
                    maxLength={32}
                  />
                </Field>
                <Field>
                  <FieldLabel htmlFor="profile-login">登录名</FieldLabel>
                  <Input id="profile-login" value="chendao" readOnly disabled />
                  <FieldDescription>创建后不可更改</FieldDescription>
                </Field>
              </div>
              <Field orientation="horizontal" className="w-auto">
                <FieldLabel>角色</FieldLabel>
                <Badge variant="secondary">制作者</Badge>
              </Field>
            </FieldGroup>
          </CardContent>
          <CardFooter className="justify-between">
            <span className="text-xs text-muted-foreground">
              最多 32 个字符
            </span>
            <Button
              onClick={() => demoNotice("个人资料已保存")}
              disabled={!name.trim()}
            >
              保存
            </Button>
          </CardFooter>
        </Card>
        <Card variant="panel" id="sessions">
          <CardHeader>
            <CardTitle>密码与会话</CardTitle>
            <CardDescription>
              会话空闲 12 小时或满 7 天后自动失效。
            </CardDescription>
          </CardHeader>
          <CardContent className="flex flex-col gap-4">
            <div className="flex items-center gap-3 rounded-md bg-surface-2 p-3">
              <Monitor className="size-4" />
              <div className="flex-1 text-sm">
                macOS · Chrome
                <p className="mt-1 text-xs text-muted-foreground">当前会话</p>
              </div>
              <Badge>本机</Badge>
            </div>
            {remote ? (
              <div className="flex items-center gap-3 p-3">
                <Monitor className="size-4" />
                <div className="flex-1 text-sm">
                  Windows · Edge
                  <p className="mt-1 text-xs text-muted-foreground">
                    最近活动时间
                  </p>
                </div>
                <Button
                  variant="outline"
                  size="sm"
                  onClick={() => {
                    setRemote(false);
                    demoNotice("演示设备已退出");
                  }}
                >
                  退出该设备
                </Button>
              </div>
            ) : null}
          </CardContent>
          <CardFooter className="justify-between gap-4">
            <p className="text-xs text-muted-foreground">
              修改密码会让其他设备立即退出。
            </p>
            <Button variant="outline" asChild>
              <Link href={screenHref("reset-password")}>修改密码</Link>
            </Button>
          </CardFooter>
        </Card>
        <Card variant="panel" id="preferences">
          <CardHeader>
            <CardTitle>偏好</CardTitle>
            <CardDescription>只影响自己的界面与默认提示词。</CardDescription>
          </CardHeader>
          <CardContent>
            <FieldGroup className="gap-5">
              <Field orientation="horizontal">
                <FieldLabel>主题</FieldLabel>
                <ToggleGroup
                  type="single"
                  value={theme}
                  onValueChange={(value) => {
                    if (value) {
                      setTheme(value);
                      demoNotice(`${value}偏好已选择，当前画板保持深色对照`);
                    }
                  }}
                  aria-label="主题偏好"
                  className="ml-auto"
                >
                  {["深色", "浅色", "跟随系统"].map((value) => (
                    <ToggleGroupItem key={value} value={value}>
                      {value}
                    </ToggleGroupItem>
                  ))}
                </ToggleGroup>
              </Field>
              <Field>
                <FieldLabel htmlFor="default-prompt">默认提示词后缀</FieldLabel>
                <Textarea
                  id="default-prompt"
                  value={prompt}
                  onChange={(e) => setPrompt(e.target.value)}
                  rows={3}
                />
              </Field>
            </FieldGroup>
          </CardContent>
          <CardFooter className="justify-end">
            <Button onClick={() => demoNotice("偏好已保存")}>保存</Button>
          </CardFooter>
        </Card>
        <Card variant="panel">
          <CardContent className="flex items-center justify-between gap-4">
            <div>
              <p className="text-sm font-medium">退出登录</p>
              <p className="mt-1 text-xs text-muted-foreground">
                只退出当前设备。
              </p>
            </div>
            <Button variant="destructive" asChild>
              <Link href={screenHref("login")}>退出登录</Link>
            </Button>
          </CardContent>
        </Card>
        <details id="licenses" className="text-xs text-muted-foreground">
          <summary className="cursor-pointer">模型来源</summary>
          <p className="py-3">
            演示模型配置来自 Figma 占位内容，未连接供应商。
          </p>
        </details>
      </div>
      <LocalDialog
        open={avatar}
        onOpenChange={setAvatar}
        title="更换头像"
        onConfirm={() => demoNotice("头像已更新")}
      >
        <div className="flex items-center justify-center py-8">
          <Avatar className="size-20">
            <AvatarFallback>
              <UserRound className="size-8" />
            </AvatarFallback>
          </Avatar>
        </div>
        <p className="text-center text-sm text-muted-foreground">
          当前演示使用显示名首字作为头像。
        </p>
      </LocalDialog>
    </ProductShell>
  );
}
