"use client";

import {
  useCallback,
  useEffect,
  useRef,
  useState,
  type ReactNode,
} from "react";
import { unstable_serialize, useSWRConfig } from "swr";
import useLiveQuery from "@/components/canvas/runtime/lib/use-live-query";
import { ApiError } from "@/components/canvas/runtime/lib/request";
import { notifyError } from "@/components/canvas/runtime/components/feedback/error-notification";
import {
  thumbnailQueue,
  type ThumbnailGrant,
  type ThumbnailQueue,
} from "@/components/canvas/runtime/components/assets/lib/thumbnail-queue";
import { PreviewImage } from "./preview-image";

const recoveryDelays = [250, 500, 1000];

type Props = {
  scope: string;
  fileId: string;
  version: string;
  fallback?: ReactNode;
  errorFallback?: ReactNode;
};

/** Fixed reference, private grant and byte budget shared by covers and library thumbnails. */
export function FixedThumbnail(props: Props) {
  return (
    <ThumbnailSession
      key={JSON.stringify([props.scope, props.fileId, props.version])}
      {...props}
    />
  );
}

function ThumbnailSession({
  scope,
  fileId,
  version,
  fallback,
  errorFallback = fallback,
}: Props) {
  const { cache, mutate } = useSWRConfig();
  const queue = thumbnailQueue(cache, scope);
  const key = [
    "asset-preview",
    scope,
    fileId,
    version,
    "thumbnail",
    "webp-frame-v2",
  ];
  const serialized = unstable_serialize(key);
  const [failedGrant, setFailedGrant] = useState<ThumbnailGrant>();
  const active = useRef(true);
  const attempts = useRef(0);
  const reported = useRef(false);
  const timer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);
  useEffect(() => {
    active.current = true;
    return () => {
      active.current = false;
      clearTimeout(timer.current);
    };
  }, []);
  useEffect(
    () =>
      queue.retain({ fileId, version, variant: "thumbnail" }, () => {
        void mutate(serialized, undefined, { revalidate: false })
          .catch(() => undefined)
          .then(() => cache.delete(serialized));
      }),
    [queue, fileId, version, serialized, cache, mutate],
  );
  const read = useLiveQuery(
    key,
    () => queue.request({ fileId, version, variant: "thumbnail" }),
    {
      shouldRetryOnError: false,
      revalidateOnFocus: false,
      revalidateOnMount: true,
      dedupingInterval: 0,
    },
  );
  const { mutate: renew, data: grant } = read;
  const recover = useCallback(() => {
    if (!active.current) return;
    setFailedGrant(grant);
    if (
      timer.current !== undefined ||
      attempts.current >= recoveryDelays.length
    )
      return;
    const delay = recoveryDelays[attempts.current++];
    timer.current = setTimeout(() => {
      timer.current = undefined;
      if (!active.current) return;
      void renew().catch(() => undefined);
    }, delay);
  }, [renew, grant]);
  // SWR callbacks belong to the original request owner, which may already have left.
  useEffect(() => {
    const error = read.error;
    if (!error || error?.name === "AbortError") return;
    if (!reported.current) {
      reported.current = true;
      queue.notifyOnce(error, (cause) =>
        notifyError(cause, "素材小图暂不可用"),
      );
    }
    // 429 already has a bounded Retry-After policy in the shared request layer.
    if (error instanceof ApiError && [50300, 50301, 50400].includes(error.code))
      recover();
    else {
      clearTimeout(timer.current);
      timer.current = undefined;
    }
  }, [read.error, queue, recover]);
  useEffect(() => {
    if (read.error || !read.data) return;
    clearTimeout(timer.current);
    timer.current = undefined;
    reported.current = false;
  }, [read.data, read.error]);
  const loaded = useCallback(() => {
    attempts.current = 0;
    clearTimeout(timer.current);
    timer.current = undefined;
    setFailedGrant(undefined);
  }, []);
  if (
    read.error ||
    (failedGrant && failedGrant === grant) ||
    grant?.status === "FAILED"
  )
    return errorFallback;
  return read.data?.status === "READY" && read.data.url ? (
    <ThumbnailBytes
      key={read.data.url}
      grant={read.data}
      queue={queue}
      onError={recover}
      onLoad={loaded}
    />
  ) : (
    fallback
  );
}

/** Hold the permit until the browser finishes bytes/decoding or this fixed grant is removed. */
function ThumbnailBytes({
  grant,
  queue,
  onError,
  onLoad,
}: {
  grant: ThumbnailGrant;
  queue: ThumbnailQueue;
  onError: () => void;
  onLoad: () => void;
}) {
  const [permitted, setPermitted] = useState(false);
  const release = useRef<(() => void) | undefined>(undefined);
  useEffect(() => {
    const abort = new AbortController();
    void queue
      .reserve(abort.signal)
      .then((done) => {
        if (abort.signal.aborted) return done();
        if (grant.expiresAt !== undefined && grant.expiresAt <= Date.now()) {
          done();
          onError();
          return;
        }
        release.current = done;
        setPermitted(true);
      })
      .catch(() => undefined);
    return () => {
      abort.abort();
      release.current?.();
    };
  }, [grant, queue, onError]);
  return permitted ? (
    <PreviewImage
      src={grant.url}
      alt=""
      loading="eager"
      className="absolute inset-0 size-full object-cover"
      onLoad={() => {
        release.current?.();
        onLoad();
      }}
      onError={() => {
        release.current?.();
        onError();
      }}
    />
  ) : null;
}
