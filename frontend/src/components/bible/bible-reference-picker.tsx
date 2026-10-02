"use client";
import { useId, useRef, useState } from "react";
import Image from "next/image";
import Link from "next/link";
import { useInfiniteQuery, useQuery } from "@tanstack/react-query";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import { useReleaseMediaSource } from "@/components/canvas/nodes/media-lifecycle";
import {
  bibleScopeKey,
  getBibleMediaCandidates,
  getBiblePreview,
} from "./bible-queries";
import {
  imageRoles,
  imageRoleLabels,
  referenceInputSchema,
  type BibleIdentity,
  type BibleReference,
} from "./bible-model";

export function BiblePreview({
  identity,
  assetId,
  kind,
  revision,
}: {
  identity: BibleIdentity;
  assetId: string;
  kind: "image" | "audio";
  revision?: number;
}) {
  const audio = useRef<HTMLAudioElement>(null);
  const preview = useQuery({
    queryKey: [...bibleScopeKey(identity), "preview", kind, assetId, revision],
    queryFn: ({ signal }) =>
      getBiblePreview(identity, assetId, kind, revision, signal),
    staleTime: 0,
    gcTime: 0,
    retry: false,
    refetchOnWindowFocus: false,
  });
  const proven = preview.isFetchedAfterMount && preview.isSuccess;
  useReleaseMediaSource(audio, proven ? preview.data.url : "");
  if (preview.isError)
    return (
      <div>
        <p role="alert">当前授权或原媒体版本不匹配，私有预览无法读取。</p>
        <Button variant="outline" onClick={() => void preview.refetch()}>
          重新核验原媒体
        </Button>
      </div>
    );
  if (!proven) return <p role="status">正在核验项目和原媒体版本…</p>;
  return kind === "image" ? (
    <Image
      unoptimized
      width={preview.data.asset.width ?? 640}
      height={preview.data.asset.height ?? 480}
      src={preview.data.url}
      alt={preview.data.asset.file_name}
      className="max-h-96 w-full object-contain"
    />
  ) : (
    <audio
      ref={audio}
      controls
      preload="metadata"
      src={preview.data.url}
      className="w-full"
    />
  );
}
export function BibleAssetSelect({
  identity,
  kind,
  value,
  label,
  locked,
  onChange,
}: {
  identity: BibleIdentity;
  kind: "image" | "audio";
  value: string;
  label: string;
  locked: boolean;
  onChange: (id: string) => void;
}) {
  const id = useId(),
    [preview, setPreview] = useState<string>();
  const candidates = useInfiniteQuery({
    queryKey: [...bibleScopeKey(identity), "media", kind],
    initialPageParam: undefined as string | undefined,
    queryFn: ({ signal, pageParam }) =>
      getBibleMediaCandidates(identity, kind, pageParam, signal),
    getNextPageParam: (page, pages) =>
      page.next_cursor &&
      !pages
        .slice(0, -1)
        .some((prior) => prior.next_cursor === page.next_cursor)
        ? page.next_cursor
        : undefined,
    staleTime: 0,
    gcTime: 0,
    retry: false,
  });
  const items = candidates.data?.pages.flatMap((page) => page.items) ?? [];
  const current = items.some((item) => item.id === value);
  return (
    <div className="space-y-2">
      <Label htmlFor={id}>{label}</Label>
      <select
        id={id}
        disabled={locked || candidates.isPending}
        value={value}
        onChange={(event) => {
          setPreview(undefined);
          onChange(event.target.value);
        }}
        className="h-10 w-full rounded-md bg-muted px-2"
      >
        <option value="">不绑定</option>
        {value && !current && (
          <option value={value}>当前原素材 · {value}</option>
        )}
        {items.map((item) => (
          <option key={item.id} value={item.id}>
            {item.file_name} · {item.id}
          </option>
        ))}
      </select>
      {candidates.isError && (
        <div>
          <p role="alert">当前项目正式素材读取未完成。</p>
          <Button
            variant="outline"
            disabled={locked}
            onClick={() => void candidates.refetch()}
          >
            重试正式素材列表
          </Button>
        </div>
      )}
      {candidates.hasNextPage && (
        <Button
          variant="outline"
          disabled={locked || candidates.isFetchingNextPage}
          onClick={() => void candidates.fetchNextPage()}
        >
          读取更多正式{kind === "image" ? "图片" : "音频"}
        </Button>
      )}
      {value && (
        <Button
          type="button"
          variant="outline"
          disabled={locked}
          onClick={() => setPreview(value)}
        >
          预览{label}
        </Button>
      )}
      {preview && (
        <BiblePreview identity={identity} assetId={preview} kind={kind} />
      )}
    </div>
  );
}
export function BibleReferencePicker({
  identity,
  initial,
  locked,
  onDirty,
  onSave,
}: {
  identity: BibleIdentity;
  initial: BibleReference[];
  locked: boolean;
  onDirty: () => void;
  onSave: (
    references: { role: (typeof imageRoles)[number]; asset_id: string }[],
  ) => void | Promise<void>;
}) {
  const [values, setValues] = useState(() =>
      Object.fromEntries(
        imageRoles.map((role) => [
          role,
          initial.find((ref) => ref.role === role)?.media.asset_id ?? "",
        ]),
      ),
    ),
    [error, setError] = useState<string>();
  return (
    <div className="space-y-4">
      <p>
        每张正式项目图片对应一个明确角色。保存时服务端冻结原件和预览版本，当前选择不会修改历史造型。
      </p>
      <div className="grid gap-4 sm:grid-cols-2">
        {imageRoles.map((role) => (
          <BibleAssetSelect
            key={role}
            identity={identity}
            kind="image"
            value={values[role]}
            label={imageRoleLabels[role]}
            locked={locked}
            onChange={(value) => {
              setValues((previous) => ({ ...previous, [role]: value }));
              setError(undefined);
              onDirty();
            }}
          />
        ))}
      </div>
      {locked ? (
        <p>当前原意图未完成，项目素材入口暂时锁定。</p>
      ) : (
        <Link href={`/assets?scope=project&project_id=${identity.projectId}`}>
          在当前项目素材库上传新图片
        </Link>
      )}
      <Button
        disabled={locked}
        onClick={async () => {
          try {
            const order = [
              ...initial.map((item) => item.role),
              ...imageRoles.filter(
                (role) => !initial.some((item) => item.role === role),
              ),
            ];
            const refs = order
              .filter((role) => values[role])
              .map((role) =>
                referenceInputSchema.parse({ role, asset_id: values[role] }),
              );
            await onSave(refs);
          } catch {
            setError("参考图输入无效，请保留原草稿并核对正式素材。");
          }
        }}
      >
        保存六类参考图
      </Button>
      {error && <p role="alert">{error}</p>}
    </div>
  );
}
