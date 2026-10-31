"use client";

import { useState, useSyncExternalStore, type ReactNode } from "react";
import useSWR from "swr";
import { getScopeGeneration, getSessionGeneration } from "@/lib/request";
import { UsersRound } from "lucide-react";
import { listTeamMembers, teamMemberAvatar } from "@/api/identity";
import { getMyAvatar } from "@/api/media";
import { useOptionalSession } from "@/components/auth/session";
import { useSpace } from "@/components/spaces/space-context";
import {
  Avatar,
  AvatarImage,
  AvatarFallback,
  AvatarGroup,
  AvatarGroupCount,
  AvatarBadge,
} from "@/components/ui/avatar";
import {
  HoverCard,
  HoverCardTrigger,
  HoverCardContent,
} from "@/components/ui/hover-card";
import {
  Popover,
  PopoverTrigger,
  PopoverContent,
  PopoverTitle,
} from "@/components/ui/popover";
import { Button } from "@/components/ui/button";
import type { CanvasYjsLiveSession, YjsPeer } from "./yjs-live-session";

/** Presence is a list of connections; the toolbar represents people. */
export function participantUserIDs(
  actorID: string,
  peers: readonly Pick<YjsPeer, "actorId">[],
) {
  return [
    actorID,
    ...Array.from(
      new Set(
        peers.map((peer) => peer.actorId).filter((id) => id && id !== actorID),
      ),
    ).sort(),
  ];
}

type Context = {
  actor: API.MeView;
  scopeID: string;
  sessionID: string;
  requestGeneration: string;
  teamID?: string;
  userIDs: string[];
};
type Person = { id: string; name: string; assetID: string };

function useParticipant(id: string, context: Context) {
  const self = id === context.actor.id;
  const profile = useSWR(
    context.teamID
      ? [
          "canvas-participants",
          context.actor.id,
          context.sessionID,
          context.requestGeneration,
          context.scopeID,
          context.teamID,
          context.userIDs.join(","),
        ]
      : null,
    async () => {
      const wanted = new Set(context.userIDs);
      const people: Record<string, Person> = {};
      let cursor: string | undefined;
      const visited = new Set<string>();
      do {
        const page = await listTeamMembers({
          id: context.teamID!,
          limit: 100,
          status: "active",
          cursor,
        });
        for (const member of page.items ?? []) {
          if (wanted.delete(member.user_id))
            people[member.user_id] = {
              id: member.user_id,
              name: member.display_name,
              assetID: member.avatar_asset_id,
            };
        }
        cursor = page.next_cursor || undefined;
        if (cursor && visited.has(cursor))
          throw new Error("参与者列表暂时不可用");
        if (cursor) visited.add(cursor);
      } while (cursor && wanted.size);
      return people;
    },
    {
      refreshInterval: 60_000,
      shouldRetryOnError: false,
      keepPreviousData: false,
    },
  );
  const person: Person = self
    ? {
        id,
        name: context.actor.display_name,
        assetID: context.actor.avatar_asset_id,
      }
    : (!profile.error && profile.data?.[id]) || { id, name: "", assetID: "" };
  const avatar = useSWR(
    person.assetID
      ? [
          "canvas-participant-avatar",
          context.actor.id,
          context.sessionID,
          context.requestGeneration,
          context.scopeID,
          context.teamID,
          id,
          person.assetID,
        ]
      : null,
    () =>
      self ? getMyAvatar() : teamMemberAvatar({ id: context.teamID!, uid: id }),
    {
      refreshInterval: (data) => {
        const expires = Date.parse(data?.expires_at ?? "");
        return Number.isFinite(expires)
          ? Math.max(5_000, expires - Date.now() - 10_000)
          : 45_000;
      },
      shouldRetryOnError: false,
      keepPreviousData: false,
    },
  );
  return {
    name: person.name.trim() || (self ? "我" : "协作者"),
    self,
    url: !avatar.error ? avatar.data?.url : undefined,
    loading: !self && profile.isLoading,
    unavailable:
      !self &&
      (!context.teamID ||
        !!profile.error ||
        (!!profile.data && !profile.data[id])),
    avatarError: !!avatar.error,
    retry: () => {
      void profile.mutate();
      void avatar.mutate();
    },
  };
}
type Profile = ReturnType<typeof useParticipant>;

function Face({
  profile,
  badge = false,
}: {
  profile: Profile;
  badge?: boolean;
}) {
  return (
    <>
      <AvatarImage src={profile.url} alt="" />
      <AvatarFallback>{Array.from(profile.name)[0]}</AvatarFallback>
      {badge && (
        <AvatarBadge
          className="bg-(--canvas-presence-online)"
          aria-hidden="true"
        />
      )}
    </>
  );
}
function Details({ profile }: { profile: Profile }) {
  return (
    <div className="flex min-h-11 items-center gap-3">
      <Avatar>
        <Face profile={profile} badge />
      </Avatar>
      <div className="grid min-w-0 flex-1 gap-1">
        <p className="font-medium break-words">
          {profile.name}
          {profile.self ? "（你）" : ""}
        </p>
        <p className="text-xs text-muted-foreground">在线</p>
        {profile.loading && (
          <p role="status" className="text-xs text-muted-foreground">
            正在读取参与者信息…
          </p>
        )}
        {(profile.unavailable || profile.avatarError) && (
          <div
            role="alert"
            className="grid gap-1 text-xs text-muted-foreground"
          >
            <p>
              {profile.unavailable
                ? "暂时无法读取参与者信息"
                : "头像暂时无法加载，已显示默认头像"}
            </p>
            <Button
              variant="ghost"
              size="sm"
              className="min-h-11"
              onClick={profile.retry}
            >
              重新读取
            </Button>
          </div>
        )}
      </div>
    </div>
  );
}
function Participant({ id, context }: { id: string; context: Context }) {
  const profile = useParticipant(id, context);
  const [open, setOpen] = useState(false);
  const [hover, setHover] = useState(false);
  return (
    <HoverCard open={hover && !open} onOpenChange={setHover} openDelay={250}>
      <Popover
        open={open}
        onOpenChange={(next) => {
          setOpen(next);
          setHover(false);
        }}
      >
        <HoverCardTrigger asChild>
          <PopoverTrigger asChild>
            <Avatar asChild>
              <Button
                type="button"
                variant="ghost"
                size="icon"
                data-slot="avatar"
                aria-label={`查看参与者 ${profile.name}${profile.self ? "（你）" : ""}`}
                className="size-8 min-h-8 rounded-full border-0 p-0"
              >
                <Face profile={profile} />
              </Button>
            </Avatar>
          </PopoverTrigger>
        </HoverCardTrigger>
        <HoverCardContent
          align="end"
          side="bottom"
          sideOffset={8}
          data-canvas-no-zoom
        >
          <Details profile={profile} />
        </HoverCardContent>
        <PopoverContent
          align="end"
          side="bottom"
          sideOffset={8}
          aria-label="参与者信息"
          data-canvas-no-zoom
        >
          <Details profile={profile} />
        </PopoverContent>
      </Popover>
    </HoverCard>
  );
}
function ParticipantRow({ id, context }: { id: string; context: Context }) {
  const profile = useParticipant(id, context);
  return (
    <li>
      <Details profile={profile} />
    </li>
  );
}
function ParticipantList({
  ids,
  context,
  children,
}: {
  ids: string[];
  context: Context;
  children: ReactNode;
}) {
  return (
    <Popover>
      <PopoverTrigger asChild>{children}</PopoverTrigger>
      <PopoverContent
        align="end"
        sideOffset={8}
        aria-label="在线参与者"
        data-canvas-no-zoom
      >
        <PopoverTitle>在线参与者 · {ids.length} 人</PopoverTitle>
        <ul className="grid max-h-72 gap-3 overflow-y-auto">
          {ids.map((id) => (
            <ParticipantRow key={id} id={id} context={context} />
          ))}
        </ul>
      </PopoverContent>
    </Popover>
  );
}
function Participants({ ids, context }: { ids: string[]; context: Context }) {
  // SWR 2.5.1 disposes a local provider during Activity/Suspense hiding,
  // while React retains its cache ref. Use SWR's application cache lifecycle;
  // both queries partition private data by actor, session, scope and generation.
  return (
    <div
      className="flex shrink-0 items-center px-1"
      aria-label={`${ids.length} 位参与者在线`}
    >
      <AvatarGroup className="hidden items-center [@media(pointer:fine)]:flex">
        {ids.slice(0, 3).map((id) => (
          <Participant key={id} id={id} context={context} />
        ))}
        {ids.length > 3 && (
          <ParticipantList ids={ids} context={context}>
            <Button
              type="button"
              aria-label={`查看全部 ${ids.length} 位参与者`}
              variant="ghost"
              size="icon"
              className="size-8 min-h-8 shrink-0 rounded-full border-0 p-0"
            >
              <AvatarGroupCount>+{ids.length - 3}</AvatarGroupCount>
            </Button>
          </ParticipantList>
        )}
      </AvatarGroup>
      <ParticipantList ids={ids} context={context}>
        <Button
          variant="ghost"
          className="h-11 min-w-11 [@media(pointer:fine)]:hidden"
          aria-label={`查看全部 ${ids.length} 位参与者`}
        >
          <UsersRound />
          {ids.length}
        </Button>
      </ParticipantList>
    </div>
  );
}
export function CanvasParticipants({
  client,
  actorID,
  scopeID,
  connected,
}: {
  client: CanvasYjsLiveSession;
  actorID: string;
  scopeID: string;
  connected: boolean;
}) {
  const peers = useSyncExternalStore(
    client.subscribePresence,
    client.getPresence,
    client.getPresence,
  );
  const session = useOptionalSession();
  const { spaces } = useSpace();
  if (!connected)
    return (
      <span role="status" className="text-xs text-muted-foreground">
        协作连接已断开，在线状态未知
      </span>
    );
  if (!session || session.actor.id !== actorID) return null;
  const space = spaces.find((item) => item.id === scopeID);
  return (
    <Participants
      key={`${session.session_id}:${scopeID}`}
      ids={participantUserIDs(actorID, peers)}
      context={{
        actor: session.actor,
        sessionID: session.session_id,
        requestGeneration: `${getSessionGeneration()}:${getScopeGeneration()}`,
        scopeID,
        teamID: space?.team_id || undefined,
        userIDs: participantUserIDs(actorID, peers),
      }}
    />
  );
}
