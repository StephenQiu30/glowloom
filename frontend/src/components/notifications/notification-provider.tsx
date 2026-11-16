"use client";

import {
  createContext,
  useContext,
  useEffect,
  useEffectEvent,
  useRef,
  useState,
  type ReactNode,
} from "react";
import useSWR from "swr";
import { useOptionalSession } from "@/components/auth/session";
import {
  getNotificationSummary,
  getNotificationDisplay,
  getPublicAnnouncements,
  ackNotificationDisplay,
  readAllNotifications,
} from "@/api/notifications";
import { toast } from "@/components/feedback/notification";
import { RequestError, subscribeContextChange } from "@/lib/request";
import {
  mergeNotificationSummary,
  requireNotificationSummary,
  requireDisplayAnnouncement,
  type NotificationSummary,
  type DisplayAnnouncement,
} from "./contracts";
import { GlobalAnnouncement, AnnouncementDetail } from "./global-announcement";
import { NotificationDetail } from "./notification-detail";
import { AutomaticAnnouncement } from "./automatic-announcement";

type NotificationCenter = {
  identity: string;
  signedIn: boolean;
  summary: NotificationSummary | undefined;
  summaryError: unknown;
  readAllPending: boolean;
  readAllError: boolean;
  revision: number;
  synchronize: (summary: API.NotificationSummary) => void;
  readAll: () => Promise<void>;
  refresh: () => Promise<void>;
  view: (announcement: DisplayAnnouncement) => void;
};
const NotificationContext = createContext<NotificationCenter | null>(null);
export const useNotificationCenter = () => useContext(NotificationContext);

export function NotificationProvider({ children }: { children: ReactNode }) {
  const session = useOptionalSession();
  const identity = session
    ? `${session.actor.id}:${session.session_id}`
    : "guest";
  return (
    <NotificationSession
      key={identity}
      identity={identity}
      signedIn={Boolean(session)}
    >
      {children}
    </NotificationSession>
  );
}

function NotificationSession({
  identity,
  signedIn,
  children,
}: {
  identity: string;
  signedIn: boolean;
  children: ReactNode;
}) {
  const [written, setWritten] = useState<NotificationSummary>();
  const [pending, setPending] = useState(false);
  const [readError, setReadError] = useState(false);
  const [revision, setRevision] = useState(0);
  const [activePopup, setActivePopup] = useState<DisplayAnnouncement>();
  const [detail, setDetail] = useState<DisplayAnnouncement>();
  const detailFocus = useRef<HTMLElement | null>(null);
  const shellFocus = useRef<HTMLDivElement>(null);
  const [dismissedBanner, setDismissedBanner] = useState<string>();
  const [observedAt, setObservedAt] = useState(() => Date.now());
  const [handled, setHandled] = useState(() => new Set<string>());
  const command = useRef<API.ReadAllNotificationsInput | undefined>(undefined);
  const flight = useRef(false);
  const alive = useRef(true);
  useEffect(() => {
    alive.current = true;
    return () => {
      alive.current = false;
    };
  }, []);
  const summaryRead = useSWR(
    signedIn ? ["notification-summary", identity] : null,
    async () => requireNotificationSummary(await getNotificationSummary()),
    {
      shouldRetryOnError: false,
    },
  );
  const display = useSWR(
    ["notification-display", identity],
    () =>
      signedIn
        ? getNotificationDisplay()
        : getPublicAnnouncements({ sessionRecovery: "never" }),
    {
      shouldRetryOnError: false,
      onSuccess: () => setObservedAt(Date.now()),
    },
  );
  const candidate = display.data?.popup;
  const popup =
    activePopup ??
    (candidate && !handled.has(`${candidate.id}:${candidate.revision}`)
      ? requireDisplayAnnouncement(candidate)
      : undefined);
  const summary = summaryRead.data
    ? mergeNotificationSummary(written, summaryRead.data)
    : written;
  const rawBanner = display.data?.banner;
  const banner =
    rawBanner &&
    `${rawBanner.id}:${rawBanner.revision}` !== dismissedBanner &&
    (!rawBanner.ends_at || new Date(rawBanner.ends_at).getTime() > observedAt)
      ? requireDisplayAnnouncement(rawBanner)
      : undefined;

  function synchronize(value: API.NotificationSummary) {
    setWritten((current) => mergeNotificationSummary(current, value));
    setRevision((current) => current + 1);
  }
  async function refresh() {
    const [value] = await Promise.all([summaryRead.mutate(), display.mutate()]);
    if (!alive.current) return;
    if (value)
      setWritten((current) => mergeNotificationSummary(current, value));
    setRevision((current) => current + 1);
  }
  const revalidate = useEffectEvent(() => {
    void refresh().catch(() => {});
  });
  useEffect(() => {
    // Scope cancellation also cancels these account-wide requests. Retry only
    // after the request layer has installed its next scope controller.
    let scopeTimer: ReturnType<typeof setTimeout>;
    const unsubscribe = subscribeContextChange(() => {
      clearTimeout(scopeTimer);
      scopeTimer = setTimeout(revalidate, 0);
    });
    // SWR's interval stops on cached errors; this interval must survive them.
    const interval = setInterval(() => {
      if (!document.hidden) revalidate();
    }, 30000);
    return () => {
      unsubscribe();
      clearTimeout(scopeTimer);
      clearInterval(interval);
    };
  }, []);
  async function readAll() {
    if (
      flight.current ||
      !summary ||
      (!command.current && summary.unread_count === 0)
    )
      return;
    flight.current = true;
    setPending(true);
    setReadError(false);
    command.current ??= {
      command_id: crypto.randomUUID(),
      data_epoch: summary.data_epoch,
      through_seq: summary.max_inbox_seq,
    };
    try {
      const next = await readAllNotifications(command.current);
      if (!alive.current) return;
      synchronize(next);
      command.current = undefined;
      toast.success("已全部标为已读");
    } catch {
      if (alive.current) {
        setReadError(true);
        toast.error("标记失败，请重试");
      }
    } finally {
      flight.current = false;
      if (alive.current) setPending(false);
    }
  }
  async function dismissBanner() {
    if (!banner) return;
    if (signedIn) {
      try {
        await ackNotificationDisplay({
          announcement_id: banner.id,
          revision: banner.revision,
          kind: "dismissed",
        });
      } catch {
        toast.error("关闭失败，请重试");
        return;
      }
    }
    setDismissedBanner(`${banner.id}:${banner.revision}`);
  }
  function closePopup() {
    if (popup)
      setHandled(
        (current) => new Set([...current, `${popup.id}:${popup.revision}`]),
      );
    setActivePopup(undefined);
  }
  function view(announcement: DisplayAnnouncement) {
    detailFocus.current =
      document.activeElement instanceof HTMLElement
        ? document.activeElement
        : null;
    setDetail(announcement);
  }

  return (
    <NotificationContext.Provider
      value={{
        identity,
        signedIn,
        summary,
        summaryError: summaryRead.error,
        readAllPending: pending,
        readAllError: readError,
        revision,
        synchronize,
        readAll,
        refresh,
        view,
      }}
    >
      <div
        ref={shellFocus}
        tabIndex={-1}
        className="notification-application"
        data-announcement={Boolean(banner) || undefined}
      >
        <GlobalAnnouncement
          announcement={banner}
          onExpire={() => {
            if (banner) setDismissedBanner(`${banner.id}:${banner.revision}`);
          }}
          onDismiss={() => void dismissBanner()}
          onView={() => {
            if (banner) view(banner);
          }}
        />
        {children}
        {popup ? (
          <AutomaticAnnouncement
            key={`${popup.id}:${popup.revision}`}
            announcement={popup}
            onClose={closePopup}
            onShown={async () => {
              setActivePopup(popup);
              try {
                await ackNotificationDisplay({
                  announcement_id: popup.id,
                  revision: popup.revision,
                  kind: "shown",
                });
                return alive.current;
              } catch (error) {
                if (
                  error instanceof RequestError &&
                  error.code !== "scope_switch_pending" &&
                  [404, 409, 422].includes(error.status)
                )
                  return false;
                throw error;
              }
            }}
          />
        ) : null}
        {detail?.notification_id ? (
          <NotificationDetail
            key={detail.notification_id}
            id={detail.notification_id}
            onClose={() => setDetail(undefined)}
            returnFocus={detailFocus}
            fallbackFocus={shellFocus}
          />
        ) : detail ? (
          <AnnouncementDetail
            announcement={detail}
            onClose={() => setDetail(undefined)}
          />
        ) : null}
      </div>
    </NotificationContext.Provider>
  );
}
