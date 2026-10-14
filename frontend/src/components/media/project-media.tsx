"use client";
import Image from "next/image";
import { useEffect, useState } from "react";
import { ImageIcon, Upload, RotateCw } from "lucide-react";
import { getMediaPreview, listMediaAssets } from "@/api/media";
import { isProjectID } from "@/components/projects/project";
import { RequestError } from "@/lib/request";
import { flowError } from "@/components/generation/contracts";
import {
  Dialog,
  DialogContent,
  DialogTitle,
  DialogDescription,
} from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { UploadDialog } from "./upload-dialog";
type PreviewProps = {
  projectID: string;
  id: string;
  kind?: string;
  large?: boolean;
};
export function ProjectMediaPreview(props: PreviewProps) {
  return (
    <MediaPreview
      key={props.projectID + ":" + props.id + ":" + props.kind}
      {...props}
    />
  );
}
function MediaPreview({
  projectID,
  id,
  kind = "image",
  large = false,
}: {
  projectID: string;
  id: string;
  kind?: string;
  large?: boolean;
}) {
  const [url, setURL] = useState(""),
    [failed, setFailed] = useState(false),
    [retry, setRetry] = useState(0);
  useEffect(() => {
    const c = new AbortController();
    getMediaPreview({ pid: projectID, asset_id: id }, { signal: c.signal })
      .then((r) => {
        if (
          r.asset?.id !== id ||
          r.asset?.project_id !== projectID ||
          !r.url ||
          !/^https?:\/\//.test(r.url) ||
          !Number.isFinite(Date.parse(r.expires_at ?? "")) ||
          Date.parse(r.expires_at ?? "") <= Date.now()
        )
          throw new RequestError("invalid_receipt");
        if (!c.signal.aborted) {
          setURL(r.url);
          setFailed(false);
        }
      })
      .catch(() => {
        if (!c.signal.aborted) setFailed(true);
      });
    return () => c.abort();
  }, [projectID, id, retry]);
  if (failed)
    return (
      <span className="media-unavailable">
        <ImageIcon />
        <span>预览暂时不可用</span>
        {large && (
          <Button
            variant="ghost"
            onClick={() => {
              setFailed(false);
              setRetry((n) => n + 1);
            }}
          >
            <RotateCw />
            重试
          </Button>
        )}
      </span>
    );
  if (!url)
    return (
      <span className="media-unavailable" role="status">
        正在读取预览…
      </span>
    );
  if (kind === "video")
    return (
      <video
        src={url}
        controls={large}
        preload="metadata"
        className="size-full object-contain"
        onError={() => setFailed(true)}
      />
    );
  if (kind === "audio")
    return (
      <audio
        src={url}
        controls
        preload="metadata"
        onError={() => setFailed(true)}
      />
    );
  return (
    <Image
      src={url}
      alt="素材预览"
      fill
      sizes={large ? "80vw" : "280px"}
      unoptimized
      className={large ? "object-contain" : "object-cover"}
      onError={() => setFailed(true)}
    />
  );
}
export function ProjectMediaPicker({
  projectID,
  onSelect,
  onClose,
  kind = "image",
}: {
  projectID: string;
  kind?: string;
  onSelect: (asset: API.AssetSummary) => void;
  onClose: () => void;
}) {
  const [items, setItems] = useState<API.AssetSummary[]>([]),
    [cursor, setCursor] = useState<string | undefined>(),
    [error, setError] = useState(""),
    [loading, setLoading] = useState(true),
    [upload, setUpload] = useState(false),
    [retry, setRetry] = useState(0);
  function validate(p: API.AssetPage) {
    if (
      !Array.isArray(p.items) ||
      p.items.some(
        (i) =>
          !isProjectID(i.id) || i.project_id !== projectID || i.kind !== kind,
      )
    )
      throw new RequestError("invalid_receipt");
    return p;
  }
  useEffect(() => {
    const c = new AbortController();
    listMediaAssets({ pid: projectID, kind, limit: 40 }, { signal: c.signal })
      .then(validate)
      .then((p) => {
        if (!c.signal.aborted) {
          setItems(p.items!);
          setCursor(p.next_cursor);
          setError("");
        }
      })
      .catch((e) => {
        if (!c.signal.aborted) setError(flowError(e));
      })
      .finally(() => {
        if (!c.signal.aborted) setLoading(false);
      });
    return () => c.abort();
  }, [projectID, kind, retry]); // eslint-disable-line react-hooks/exhaustive-deps
  async function more() {
    setLoading(true);
    try {
      const p = validate(
        await listMediaAssets({ pid: projectID, kind, limit: 40, cursor }),
      );
      setItems((v) => [...v, ...p.items!]);
      setCursor(p.next_cursor);
    } catch (e) {
      setError(flowError(e));
    } finally {
      setLoading(false);
    }
  }
  return (
    <>
      <Dialog
        open
        onOpenChange={(v) => {
          if (!v) onClose();
        }}
      >
        <DialogContent className="sm:max-w-3xl">
          <DialogTitle>选择参考素材</DialogTitle>
          <DialogDescription>
            选择当前项目中可用的
            {kind === "image" ? "图片" : kind === "video" ? "视频" : "音频"}
            ，或上传新的文件。
          </DialogDescription>
          <Button variant="outline" onClick={() => setUpload(true)}>
            <Upload />
            从本地添加
          </Button>
          {error && (
            <div role="alert">
              {error}
              <Button
                variant="ghost"
                onClick={() => {
                  setLoading(true);
                  setRetry((n) => n + 1);
                }}
              >
                重新读取
              </Button>
            </div>
          )}
          <div className="media-picker-grid">
            {items.map((a) => (
              <button key={a.id} onClick={() => onSelect(a)}>
                <span className="media-picker-image">
                  <ProjectMediaPreview
                    projectID={projectID}
                    id={a.id!}
                    kind={a.kind}
                  />
                </span>
                <span>{a.file_name}</span>
              </button>
            ))}
          </div>
          {loading ? (
            <p role="status">正在读取素材…</p>
          ) : !error && items.length === 0 ? (
            <p>当前项目还没有可用素材。上传完成后可以继续选择。</p>
          ) : null}
          {cursor && (
            <Button
              disabled={loading}
              variant="ghost"
              onClick={() => void more()}
            >
              加载更多
            </Button>
          )}
        </DialogContent>
      </Dialog>
      {upload && (
        <UploadDialog
          projectID={projectID}
          onClose={() => setUpload(false)}
          onUploaded={(a) => {
            setUpload(false);
            if (a?.kind === kind && isProjectID(a.id)) {
              onSelect(a);
            } else {
              setLoading(true);
              setRetry((n) => n + 1);
            }
          }}
        />
      )}
    </>
  );
}
