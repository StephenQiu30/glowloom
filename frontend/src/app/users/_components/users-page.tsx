"use client";

import { demoNotice } from "@/components/feedback/demo-notice";
import { ProductShell } from "@/components/layout/product-shell";
import { PageHeading } from "@/components/layout/page-heading";
import { SearchInput } from "@/components/forms/search-input";
import { Choice } from "@/components/forms/choice";
import { StatusBadge } from "@/components/feedback/status-badge";
import { MoreMenu } from "@/components/controls/more-menu";
import { EmptySearch } from "@/components/feedback/empty-search";
import { LocalDialog } from "@/components/controls/local-dialog";
import { useState } from "react";
import { Plus } from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import {
  Table,
  TableHeader,
  TableBody,
  TableRow,
  TableCell,
  TableHead,
} from "@/components/ui/table";
import { FieldGroup, Field, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { Avatar, AvatarFallback } from "@/components/ui/avatar";

type User = { login: string; name: string; role: string; status: string };

const initialUsers: User[] = [
  { login: "chendao", name: "陈导", role: "管理员", status: "启用" },
  { login: "shen.wan", name: "沈晚", role: "制作者", status: "启用" },
  { login: "lizhou", name: "李舟", role: "制作者", status: "需改密" },
  { login: "xiaoman", name: "小满", role: "制作者", status: "锁定中" },
  { login: "oldpost", name: "周驻", role: "制作者", status: "已停用" },
  { login: "ali", name: "阿梨", role: "管理员", status: "启用" },
];

export function UsersPage() {
  const [users, setUsers] = useState(initialUsers);
  const [search, setSearch] = useState("");
  const [role, setRole] = useState("角色：全部");
  const [status, setStatus] = useState("状态：全部");
  const [creating, setCreating] = useState(false);
  const [name, setName] = useState("");
  const [login, setLogin] = useState("");
  const [newRole, setNewRole] = useState("制作者");
  const visible = users.filter(
    (user) =>
      (user.name + user.login).includes(search) &&
      (role.endsWith("全部") || role.includes(user.role)) &&
      (status.endsWith("全部") || status.includes(user.status)),
  );
  const updateStatus = (target: string, value: string) => {
    const user = users.find((item) => item.login === target);
    if (
      value === "已停用" &&
      user?.role === "管理员" &&
      users.filter((item) => item.role === "管理员" && item.status === "启用")
        .length <= 1
    ) {
      toast.error("至少保留一个启用中的管理员");
      return;
    }
    setUsers((current) =>
      current.map((user) =>
        user.login === target ? { ...user, status: value } : user,
      ),
    );
    demoNotice("账号状态已更新");
  };
  return (
    <ProductShell screen="users" contextual>
      <PageHeading
        title="账号"
        description="账号不会被删除；禁用后其所有会话立即失效，历史记录保留。"
      >
        <Button onClick={() => setCreating(true)}>
          <Plus data-icon="inline-start" />
          创建账号
        </Button>
      </PageHeading>
      <div className="mb-5 flex flex-wrap gap-2">
        <SearchInput
          value={search}
          onChange={setSearch}
          placeholder="登录名或显示名"
          className="w-72"
        />
        <Choice
          value={role}
          onChange={setRole}
          label="角色筛选"
          options={["角色：全部", "角色：管理员", "角色：制作者"]}
        />
        <Choice
          value={status}
          onChange={setStatus}
          label="状态筛选"
          options={[
            "状态：全部",
            "状态：启用",
            "状态：需改密",
            "状态：锁定中",
            "状态：已停用",
          ]}
        />
      </div>
      <div className="overflow-hidden rounded-lg bg-surface-1">
        <Table>
          <TableHeader>
            <TableRow>
              {["登录名", "显示名", "角色", "状态", "最后登录", ""].map(
                (head, index) => (
                  <TableHead key={index}>{head}</TableHead>
                ),
              )}
            </TableRow>
          </TableHeader>
          <TableBody>
            {visible.map((user) => (
              <TableRow key={user.login}>
                <TableCell className="py-5 font-mono text-xs">
                  {user.login}
                </TableCell>
                <TableCell>
                  <span className="flex items-center gap-2">
                    <Avatar size="sm">
                      <AvatarFallback>{user.name[0]}</AvatarFallback>
                    </Avatar>
                    {user.name}
                  </span>
                </TableCell>
                <TableCell>{user.role}</TableCell>
                <TableCell>
                  <StatusBadge value={user.status} />
                </TableCell>
                <TableCell>
                  <span className="text-xs text-muted-foreground">[时间]</span>
                </TableCell>
                <TableCell>
                  <MoreMenu
                    label={`${user.name}的操作`}
                    items={[
                      {
                        label:
                          user.status === "已停用" ? "启用账号" : "停用账号",
                        action: () =>
                          updateStatus(
                            user.login,
                            user.status === "已停用" ? "启用" : "已停用",
                          ),
                      },
                      {
                        label: "重置密码",
                        action: () => updateStatus(user.login, "需改密"),
                      },
                      {
                        label: "解除锁定",
                        action: () => updateStatus(user.login, "启用"),
                      },
                    ]}
                  />
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
        {!visible.length ? <EmptySearch /> : null}
      </div>
      <p className="mt-4 text-xs text-muted-foreground">
        至少保留一个启用中的管理员。
      </p>
      <LocalDialog
        open={creating}
        onOpenChange={setCreating}
        title="创建账号"
        confirmLabel="创建账号"
        onConfirm={() => {
          if (!name.trim() || !login.trim()) {
            toast.error("请填写显示名与登录名");
            return false;
          }
          if (users.some((user) => user.login === login)) {
            toast.error("登录名已存在");
            return false;
          }
          setUsers([
            ...users,
            { name, login, role: newRole, status: "需改密" },
          ]);
          setName("");
          setLogin("");
          demoNotice("演示账号已创建");
        }}
      >
        <FieldGroup>
          <Field>
            <FieldLabel htmlFor="new-name">显示名</FieldLabel>
            <Input
              id="new-name"
              value={name}
              onChange={(e) => setName(e.target.value)}
            />
          </Field>
          <Field>
            <FieldLabel htmlFor="new-login">登录名</FieldLabel>
            <Input
              id="new-login"
              value={login}
              onChange={(e) => setLogin(e.target.value)}
            />
          </Field>
          <Field>
            <FieldLabel>角色</FieldLabel>
            <Choice
              value={newRole}
              onChange={setNewRole}
              label="新账号角色"
              options={["制作者", "管理员"]}
            />
          </Field>
        </FieldGroup>
      </LocalDialog>
    </ProductShell>
  );
}
