"use client";
import { PageHeader } from "@/components/layout/page-header";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Item, ItemContent, ItemGroup, ItemTitle } from "@/components/ui/item";
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
  const teams = spaces.filter((space) => space.kind === "team");
  return (
    <div className="grid w-full max-w-3xl gap-(--layout-gap)">
      <PageHeader
        title="我的团队"
        description="团队角色仅在对应团队内生效。选择团队空间后，创作归属于该空间。"
      />
      {teams.length ? (
        <ItemGroup aria-label="已加入的团队">
          {teams.map((team) => (
            <div key={team.id} role="listitem">
              <Item asChild variant="muted">
                <Link href={`/teams/${team.team_id}`}>
                  <ItemContent className="min-w-0">
                    <ItemTitle>{team.name}</ItemTitle>
                  </ItemContent>
                  <span className="text-muted-foreground">
                    {team.role}
                    {!team.writable ? " · 只读" : ""}
                  </span>
                </Link>
              </Item>
            </div>
          ))}
        </ItemGroup>
      ) : (
        <ContentState
          title="尚未加入团队"
          description="创建团队，或接受邀请后开始协作。"
        />
      )}
      <Card>
        <CardHeader>
          <CardTitle>
            <h2>创建团队</h2>
          </CardTitle>
        </CardHeader>
        <CardContent>
          <form
            id="team-create"
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
        </CardContent>
      </Card>
    </div>
  );
}
