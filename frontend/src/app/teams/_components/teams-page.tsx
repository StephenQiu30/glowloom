"use client";
import { ContentState } from "@/components/layout/content-state";
import { FeedbackToast } from "@/components/feedback/feedback-toast";
import { Field, FieldGroup, FieldLabel } from "@/components/ui/field";
import { useId, useRef, useState } from "react";
import Link from "next/link";
import { createTeam } from "@/api/identity";
import { useSpace } from "@/components/spaces/space-context";
import { authErrorMessage } from "@/components/auth/contracts";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
export function TeamsPage() {
  const nameID = useId();
  const { spaces, refresh } = useSpace();
  const [name, setName] = useState("");
  const [pending, setPending] = useState(false);
  const [error, setError] = useState("");
  const [created, setCreated] = useState("");
  const lock = useRef(false);
  const creationKey = useRef(crypto.randomUUID());
  return (
    <div className="mx-auto grid w-full max-w-3xl gap-6 p-4">
      <h1 className="text-2xl">我的团队</h1>
      <p>团队角色仅在对应团队内生效。选择团队空间后，创作归属于该空间。</p>
      <div className="grid gap-3">
        {spaces
          .filter((s) => s.kind === "team")
          .map((s) => (
            <Link
              className="rounded-xl border p-4 underline"
              key={s.id}
              href={`/teams/${s.team_id}`}
            >
              {s.name} · {s.role}
              {!s.writable ? " · 只读" : ""}
            </Link>
          ))}
        {!spaces.some((s) => s.kind === "team") ? (
          <ContentState
            title="尚未加入团队"
            description="创建团队，或接受邀请后开始协作。"
          />
        ) : null}
      </div>
      <form
        className="rounded-xl border p-4"
        onSubmit={async (e) => {
          e.preventDefault();
          if (lock.current) return;
          lock.current = true;
          setPending(true);
          setError("");
          try {
            const team = await createTeam(
              { name: name.trim() },
              { headers: { "Idempotency-Key": creationKey.current } },
            );
            creationKey.current = crypto.randomUUID();
            setCreated(team.id);
            setName("");
            await refresh();
          } catch (e) {
            setError(authErrorMessage(e));
          } finally {
            lock.current = false;
            setPending(false);
          }
        }}
      >
        <FieldGroup>
          <h2>创建团队</h2>
          <Field>
            <FieldLabel htmlFor={nameID}>团队名称</FieldLabel>
            <Input
              id={nameID}
              required
              minLength={2}
              maxLength={50}
              value={name}
              onChange={(e) => {
                setName(e.target.value);
                creationKey.current = crypto.randomUUID();
              }}
              disabled={pending}
            />
          </Field>
          <p className="text-sm text-muted-foreground">
            创建后，你将成为团队管理员。
          </p>
          <Button disabled={pending || !name.trim()}>
            {pending ? "正在创建…" : "创建团队"}
          </Button>
        </FieldGroup>
        {error ? <FeedbackToast message={error} /> : null}
        {created ? (
          <Link href={`/teams/${created}`} className="underline">
            管理新团队
          </Link>
        ) : null}
      </form>
    </div>
  );
}
