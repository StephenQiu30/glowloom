"use client";
import { useRef, useState } from "react";
import {
  Dialog,
  DialogContent,
  DialogTitle,
  DialogDescription,
} from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  createAdminUser,
  updateAdminUser,
  disableAdminUser,
  enableAdminUser,
  unlockAdminUser,
  resetAdminPassword,
  listAdminUsers,
} from "@/components/creation/generated/creation";
import { AuthError } from "@/components/auth/request";
import {
  requireUserMutation,
  requireUserPage,
  userError,
  type User,
} from "./contracts";
export type UserAction =
  "create" | "edit" | "disable" | "enable" | "password" | "unlock";
const labels = {
  create: "创建用户",
  edit: "编辑用户",
  disable: "停用用户",
  enable: "启用用户",
  password: "重置密码",
  unlock: "解除登录锁定",
};
export function UserDialog({
  action,
  user,
  self,
  onClose,
  onSaved,
}: {
  action: UserAction;
  user?: User;
  self: boolean;
  onClose: () => void;
  onSaved: (message: string) => void;
}) {
  const [login, setLogin] = useState(""),
    [name, setName] = useState(user?.display_name ?? ""),
    [role, setRole] = useState<"creator" | "admin">(user?.role ?? "creator"),
    [password, setPassword] = useState(""),
    [confirmation, setConfirmation] = useState(""),
    [error, setError] = useState(""),
    [pending, setPending] = useState(false),
    [uncertain, setUncertain] = useState(false),
    [latest, setLatest] = useState<User | null>(null),
    [checked, setChecked] = useState(false),
    [leave, setLeave] = useState(false);
  const busy = useRef(false),
    requestKey = useRef(crypto.randomUUID());
  const passwordNeeded = action === "create" || action === "password";
  const validPassword =
    Array.from(password).length >= 10 &&
    new TextEncoder().encode(password).length <= 72 &&
    /[A-Za-z]/.test(password) &&
    /[0-9]/.test(password) &&
    password === confirmation;
  async function submit() {
    if (busy.current || uncertain) return;
    busy.current = true;
    setPending(true);
    setError("");
    try {
      const opts = { headers: { "Idempotency-Key": requestKey.current } };
      let r: CreationAPI.AdminMutationReceipt;
      if (action === "create")
        r = await createAdminUser(
          {
            login_name: login.trim(),
            display_name: name.trim(),
            role,
            initial_password: password,
          },
          opts,
        );
      else if (action === "edit")
        r = await updateAdminUser(
          { id: user!.id },
          {
            expected_revision: user!.revision,
            display_name: name.trim(),
            role,
          },
          opts,
        );
      else if (action === "password")
        r = await resetAdminPassword(
          { id: user!.id },
          { expected_revision: user!.revision, new_password: password },
          opts,
        );
      else
        r = await (
          action === "disable"
            ? disableAdminUser
            : action === "unlock"
              ? unlockAdminUser
              : enableAdminUser
        )({ id: user!.id }, { expected_revision: user!.revision }, opts);
      requireUserMutation(r, user?.id, user?.revision);
      if (
        (action === "create" &&
          (r.status !== "active" || r.must_change_password !== true)) ||
        (action === "password" && r.must_change_password !== true) ||
        (action === "disable" && r.status !== "disabled") ||
        (action === "enable" && r.status !== "active") ||
        (action === "unlock" &&
          (r.login_unlocked !== true || r.status !== user?.status)) ||
        (action === "edit" &&
          (r.role !== role || r.display_name !== name.trim()))
      )
        throw new AuthError("invalid_receipt");
      setPassword("");
      setConfirmation("");
      onSaved(
        action === "create"
          ? "用户已创建。请通过安全渠道告知初始密码，首次登录时必须修改。"
          : action === "password"
            ? "密码已重置，原有会话已失效，用户下次登录需修改密码。"
            : labels[action] + "成功。",
      );
    } catch (e) {
      setError(userError(e));
      setUncertain(
        !(e instanceof AuthError && e.status >= 400 && e.status < 500),
      );
    } finally {
      busy.current = false;
      setPending(false);
    }
  }
  async function reconcile() {
    if (busy.current) return;
    busy.current = true;
    setPending(true);
    setError("");
    try {
      let cursor: string | undefined;
      const seen = new Set<string>();
      let found: User | undefined;
      do {
        const p = requireUserPage(await listAdminUsers({ limit: 200, cursor }));
        found = p.items.find((u) =>
          user
            ? u.id === user.id
            : u.login_name.toLowerCase() === login.trim().toLowerCase(),
        );
        cursor = p.next_cursor;
        if (cursor) {
          if (seen.has(cursor)) throw new AuthError("invalid_receipt");
          seen.add(cursor);
        }
      } while (!found && cursor);
      setLatest(found ?? null);
      setChecked(true);
    } catch (e) {
      setError(userError(e));
    } finally {
      busy.current = false;
      setPending(false);
    }
  }
  const changed =
    action === "create"
      ? !!(name || login || password)
      : name !== user?.display_name || role !== user?.role || !!password;
  function close() {
    if (pending) return;
    if (uncertain || changed) setLeave(true);
    else onClose();
  }
  return (
    <>
      <Dialog
        open
        onOpenChange={(v) => {
          if (!v) close();
        }}
      >
        <DialogContent
          className="max-h-[90dvh] overflow-y-auto sm:max-w-[560px]"
          onInteractOutside={(e) => e.preventDefault()}
        >
          <DialogTitle>{labels[action]}</DialogTitle>
          <DialogDescription>
            {action === "disable"
              ? "停用后用户无法继续登录，现有会话将失效。"
              : action === "enable"
                ? "启用后用户可重新登录，旧会话不会恢复。"
                : action === "unlock"
                  ? "只解除登录失败锁定，不启用已停用账号，不修改密码或恢复旧会话。"
                  : passwordNeeded
                    ? "设置临时密码，用户首次登录时必须修改。"
                    : "修改显示名或角色，权限将以服务端最新状态为准。"}
          </DialogDescription>
          <form
            onSubmit={(e) => {
              e.preventDefault();
              void submit();
            }}
          >
            <fieldset disabled={pending || uncertain}>
              {user && (
                <div className="user-dialog-identity">
                  <span>{user.display_name.slice(0, 1)}</span>
                  <div>
                    <strong>{user.display_name}</strong>
                    <small>{user.login_name}</small>
                  </div>
                </div>
              )}
              {action === "create" && (
                <label className="form-label">
                  登录名
                  <Input
                    aria-label="用户登录名"
                    autoFocus
                    autoComplete="off"
                    value={login}
                    onChange={(e) => setLogin(e.target.value)}
                    required
                    minLength={1}
                    maxLength={64}
                    pattern="[A-Za-z0-9_.-]+"
                    placeholder="1–64 位字母、数字或 _ . -"
                  />
                </label>
              )}
              {(action === "create" || action === "edit") && (
                <>
                  <label className="form-label">
                    显示名
                    <Input
                      aria-label="用户显示名"
                      value={name}
                      onChange={(e) => setName(e.target.value)}
                      required
                      maxLength={32}
                    />
                  </label>
                  <label className="form-label">
                    角色
                    <select
                      aria-label="用户角色"
                      value={role}
                      disabled={self}
                      onChange={(e) =>
                        setRole(e.target.value as "creator" | "admin")
                      }
                    >
                      <option value="creator">创作者</option>
                      <option value="admin">管理员</option>
                    </select>
                    {role === "admin" && (
                      <small>管理员可管理本组织的用户与配置。</small>
                    )}
                  </label>
                </>
              )}
              {passwordNeeded && (
                <>
                  <label className="form-label">
                    临时密码
                    <Input
                      aria-label="临时密码"
                      type="password"
                      autoComplete="new-password"
                      value={password}
                      onChange={(e) => setPassword(e.target.value)}
                      minLength={10}
                      maxLength={72}
                      required
                    />
                    <small>至少 10 个字符，包含字母和数字，最多 72 字节</small>
                  </label>
                  <label className="form-label">
                    确认临时密码
                    <Input
                      aria-label="确认临时密码"
                      type="password"
                      autoComplete="new-password"
                      value={confirmation}
                      onChange={(e) => setConfirmation(e.target.value)}
                      required
                    />
                    {confirmation && confirmation !== password && (
                      <small className="text-red-300">
                        两次输入的密码不一致
                      </small>
                    )}
                  </label>
                </>
              )}
            </fieldset>
            {error && (
              <p role="alert" className="flow-error">
                {error}
              </p>
            )}
            {uncertain && (
              <div className="quote-card">
                <p>
                  操作可能已经生效，请先读取最新用户信息。
                  {action === "password" &&
                    "目录无法证明密码重置结果，请通过安全渠道确认。"}
                </p>
                <Button
                  type="button"
                  variant="outline"
                  disabled={pending}
                  onClick={() => void reconcile()}
                >
                  核对最新用户信息
                </Button>
                {checked && (
                  <p role="status">
                    {latest
                      ? `当前用户：${latest.display_name} · ${latest.role === "admin" ? "管理员" : "创作者"} · ${latest.status === "active" ? "已启用" : "已停用"} · ${latest.login_locked ? "登录已锁定" : "未锁定"} · 版本 ${latest.revision}`
                      : "当前目录中未找到该用户。请稍后刷新核对。"}
                  </p>
                )}
              </div>
            )}
            <div className="flow-actions">
              <Button
                type="button"
                variant="outline"
                disabled={pending}
                onClick={close}
              >
                {uncertain ? "关闭并返回列表" : "取消"}
              </Button>
              <Button
                variant={action === "disable" ? "destructive" : "default"}
                disabled={
                  pending ||
                  uncertain ||
                  (passwordNeeded && !validPassword) ||
                  ((action === "create" || action === "edit") &&
                    !name.trim()) ||
                  (action === "edit" &&
                    name.trim() === user?.display_name &&
                    role === user.role)
                }
              >
                {pending ? "正在处理…" : labels[action]}
              </Button>
            </div>
          </form>
        </DialogContent>
      </Dialog>
      {leave && (
        <Dialog open onOpenChange={setLeave}>
          <DialogContent role="alertdialog">
            <DialogTitle>关闭{labels[action]}？</DialogTitle>
            <DialogDescription>
              {uncertain
                ? "结果尚未确认，返回列表后请核对实际状态再操作。"
                : "未保存的输入将被清除。"}
            </DialogDescription>
            <div className="flow-actions">
              <Button variant="outline" onClick={() => setLeave(false)}>
                继续查看
              </Button>
              <Button variant="destructive" onClick={onClose}>
                确认关闭
              </Button>
            </div>
          </DialogContent>
        </Dialog>
      )}
    </>
  );
}
