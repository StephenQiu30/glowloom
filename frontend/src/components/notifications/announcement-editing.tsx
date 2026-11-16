"use client";

import useSWR from "swr";
import { getPlatformNotification } from "@/api/notifications";
import { useSession } from "@/components/auth/session";
import { ContentState } from "@/components/layout/content-state";
import { Button } from "@/components/ui/button";
import { requireAnnouncement } from "./contracts";
import { AnnouncementEditor } from "./announcement-editor";

export function AnnouncementEditing({ id }: { id: string }) {
  const { actor } = useSession();
  const detail = useSWR(
    ["announcement-edit", actor.id, id],
    async () => requireAnnouncement(await getPlatformNotification({ id })),
    {
      shouldRetryOnError: false,
      revalidateOnFocus: false,
      revalidateOnReconnect: false,
    },
  );
  if (detail.error)
    return (
      <ContentState
        state="error"
        title="通知暂时无法读取"
        actions={
          <Button variant="outline" onClick={() => void detail.mutate()}>
            重新读取
          </Button>
        }
      />
    );
  if (!detail.data)
    return <ContentState state="loading" title="正在读取通知" />;
  return (
    <AnnouncementEditor
      key={`${id}:${detail.data.version}`}
      announcement={detail.data}
    />
  );
}
