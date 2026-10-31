"use client";

import { useRef, useState, type FormEvent, type ComponentProps } from "react";
import { ArrowLeft } from "lucide-react";
import * as api from "@/components/canvas/runtime/api/account-bff";
import { unwrap } from "@/components/canvas/runtime/lib/request";
import { useAccountCommand } from "../hooks/use-account-command";
import { useAccount } from "../account-provider";
import { AccountError, AccountField, AccountForm } from "../account-fields";
import { Button } from "@/components/ui/button";
import { Spinner } from "@/components/ui/spinner";
import { FieldGroup } from "@/components/ui/field";
import { Tabs, TabsList, TabsTrigger, TabsContent } from "@/components/ui/tabs";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
} from "@/components/ui/dialog";
import { toast } from "sonner";
import { CaptchaField } from "./captcha-field";
import type { AuthMode } from "./auth-dialog-provider";

type Credentials = {
  userAccount: string;
  userPassword: string;
  checkPassword: string;
};

export function AuthDialog({
  mode,
  onModeChange,
  onClose,
  onCloseAutoFocus,
  onAuthenticated,
}: {
  mode: AuthMode;
  onModeChange: (mode: AuthMode) => void;
  onClose: () => void;
  onCloseAutoFocus: ComponentProps<typeof DialogContent>["onCloseAutoFocus"];
  onAuthenticated: (session: CanvasContract.BrowserSessionVO) => void;
}) {
  const register = mode === "register";
  const withCommand = useAccountCommand();
  const auth = useAccount();
  const inFlight = useRef(false);
  const returnFocus = useRef(false);
  const [pending, setPending] = useState(false);
  const [intent, setIntent] = useState<Credentials>();
  const [account, setAccount] = useState("");
  const [password, setPassword] = useState("");
  const [confirm, setConfirm] = useState("");
  const [error, setError] = useState<Error>();

  function switchMode(value: AuthMode) {
    setError(undefined);
    setIntent(undefined);
    setPassword("");
    setConfirm("");
    onModeChange(value);
  }

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (inFlight.current || intent) return;
    if (!event.currentTarget.reportValidity()) return;
    if (register && password !== confirm) {
      setError(new Error("两次输入的密码不一致"));
      return;
    }
    setError(undefined);
    inFlight.current = true;
    setPending(true);
    const prepared = await auth.refresh();
    inFlight.current = false;
    setPending(false);
    if (!prepared || prepared.account) {
      return;
    }
    setIntent({
      userAccount: account,
      userPassword: password,
      checkPassword: confirm,
    });
  }

  async function verified(ticket: CanvasContract.CaptchaTicketVO) {
    if (inFlight.current || !intent) return;
    if (!ticket.captchaToken || Number(ticket.expiresAt) <= Date.now()) {
      setError(new Error("验证凭据已失效，请重新验证"));
      setIntent(undefined);
      return;
    }
    inFlight.current = true;
    setPending(true);
    setError(undefined);
    try {
      if (register) {
        unwrap(
          await api.registerAccount({
            ...withCommand("registerAccount", intent),
            captchaToken: ticket.captchaToken,
          }),
        );
        toast.success("注册成功，请登录");
        switchMode("login");
      } else {
        onAuthenticated(
          unwrap(
            await api.loginAccount({
              userAccount: intent.userAccount,
              userPassword: intent.userPassword,
              captchaToken: ticket.captchaToken,
            }),
          ),
        );
      }
    } catch (cause) {
      setError(cause instanceof Error ? cause : new Error("请求失败"));
      setIntent(undefined);
      returnFocus.current = true;
      await auth.refresh();
    } finally {
      inFlight.current = false;
      setPending(false);
    }
  }

  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open && !inFlight.current) onClose();
      }}
    >
      <DialogContent
        className="dialog-auth max-h-dialog overflow-y-auto"
        onCloseAutoFocus={onCloseAutoFocus}
        showCloseButton={!pending}
      >
        <DialogHeader>
          <DialogTitle
            ref={(node) => {
              if (node && intent) node.focus();
            }}
            tabIndex={-1}
          >
            {intent
              ? "完成安全验证"
              : register
                ? "创建织光账号"
                : "欢迎回到织光"}
          </DialogTitle>
          <DialogDescription>
            {intent
              ? "拖动滑块完成拼图，也可使用方向键移动并按 Enter 确认。"
              : register
                ? "开始你的第一部作品。"
                : "继续你的创作。"}
          </DialogDescription>
        </DialogHeader>
        {intent && (
          <Button
            type="button"
            variant="ghost"
            className="w-fit"
            disabled={pending}
            onClick={() => {
              returnFocus.current = true;
              setIntent(undefined);
              setError(undefined);
            }}
          >
            <ArrowLeft aria-hidden="true" data-icon="inline-start" />
            {register ? "返回注册" : "返回登录"}
          </Button>
        )}
        <AccountError error={error} />
        {intent ? (
          <CaptchaField
            scene={register ? "REGISTER" : "LOGIN"}
            disabled={pending}
            onVerified={verified}
          />
        ) : (
          <Tabs
            className="gap-4"
            value={mode}
            onValueChange={(value) => {
              if (
                !inFlight.current &&
                (value === "login" || value === "register")
              )
                switchMode(value);
            }}
          >
            <TabsList
              className="group-data-horizontal/tabs:h-control w-52 max-w-full"
              aria-label="账户登录与注册"
            >
              <TabsTrigger value="login" disabled={pending}>
                登录
              </TabsTrigger>
              <TabsTrigger value="register" disabled={pending}>
                注册
              </TabsTrigger>
            </TabsList>
            {(["login", "register"] as const).map((tab) => (
              <TabsContent key={tab} value={tab}>
                {mode === tab && (
                  <AccountForm className="auth-form" onSubmit={submit}>
                    <FieldGroup>
                      <AccountField
                        name="account"
                        label="账号"
                        autoComplete="username"
                        required
                        minLength={4}
                        maxLength={32}
                        pattern="[A-Za-z0-9_]+"
                        description="4–32 位字母、数字或下划线"
                        placeholder="请输入账号"
                        value={account}
                        ref={(node) => {
                          if (node && returnFocus.current && !pending) {
                            returnFocus.current = false;
                            node.focus();
                          }
                        }}
                        onChange={(event) => setAccount(event.target.value)}
                        disabled={pending}
                      />
                      <AccountField
                        name="password"
                        label="密码"
                        type="password"
                        autoComplete={
                          register ? "new-password" : "current-password"
                        }
                        required
                        minLength={8}
                        maxLength={64}
                        placeholder={register ? "至少 8 位字符" : "请输入密码"}
                        value={password}
                        onChange={(event) => setPassword(event.target.value)}
                        disabled={pending}
                      />
                      {register && (
                        <AccountField
                          name="confirm"
                          label="确认密码"
                          type="password"
                          autoComplete="new-password"
                          required
                          minLength={8}
                          maxLength={64}
                          placeholder="再次输入密码"
                          value={confirm}
                          onChange={(event) => setConfirm(event.target.value)}
                          disabled={pending}
                        />
                      )}
                      <Button
                        type="submit"
                        className="auth-submit w-full"
                        disabled={pending}
                      >
                        {pending && <Spinner />}
                        {register ? "注册" : "登录"}
                      </Button>
                    </FieldGroup>
                    <p className="account-caption">
                      {register
                        ? "注册成功后返回登录；不会自动开始生成任务。"
                        : "登录后返回当前内容，未提交的创作输入会保留。"}
                    </p>
                    {!register && (
                      <p className="account-caption">遇到问题？联系管理员</p>
                    )}
                  </AccountForm>
                )}
              </TabsContent>
            ))}
          </Tabs>
        )}
      </DialogContent>
    </Dialog>
  );
}
