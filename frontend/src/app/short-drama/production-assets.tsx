"use client";
import { DEFAULT_PAGE_SIZE } from "@/lib/pagination";
import {
  DataPagination,
  type PageSize,
} from "@/components/media/data-pagination";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { ContentState } from "@/components/layout/content-state";
import { Item, ItemGroup, ItemTitle } from "@/components/ui/item";

import Link from "next/link";
import { useEffect, useState } from "react";
import useSWR from "swr";
import { createBibleEntry, listBibleEntries } from "@/api/bible";
import { listMediaAssets } from "@/api/media";
import { ProjectMediaPreview } from "@/components/media/project-media";
import { UploadDialog } from "@/components/media/upload-dialog";
import { isProjectID } from "@/components/projects/project";
import { RequestError } from "@/lib/request";
import { Field, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { useScriptWrite } from "./use-script-write";
import { Button } from "@/components/ui/button";
import { Card, CardHeader, CardTitle } from "@/components/ui/card";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";

const kinds = [
  { id: "character", label: "角色" },
  { id: "location", label: "场景" },
  { id: "prop", label: "道具" },
] as const;
type Kind = (typeof kinds)[number]["id"];

export function ProductionAssets({
  projectID,
  owner,
  writable,
  canvas,
  onLock,
}: {
  projectID: string;
  owner: string;
  writable: boolean;
  canvas: string;
  onLock: (locked: boolean) => void;
}) {
  const [pageSize, setPageSize] = useState<PageSize>(DEFAULT_PAGE_SIZE);
  const [cursors, setCursors] = useState<(string | undefined)[]>([undefined]);
  const cursor = cursors.at(-1);
  const [tab, setTab] = useState<Kind | "media">("media");
  const [upload, setUpload] = useState(false);
  const [preview, setPreview] = useState<string>();
  const [name, setName] = useState("");
  const write = useScriptWrite();
  useEffect(() => {
    onLock(write.locked);
    return () => onLock(false);
  }, [onLock, write.locked]);
  const media = useSWR(
    tab === "media"
      ? [owner, projectID, "production-media", pageSize, cursor]
      : null,
    () => listMediaAssets({ pid: projectID, limit: pageSize, cursor }),
  );
  const entities = useSWR(
    tab !== "media" ? [owner, projectID, "production-assets", tab] : null,
    () => listBibleEntries({ pid: projectID, kind: tab as Kind }),
  );
  function create(kind: Kind) {
    if (!name.trim() || !writable || write.locked) return;
    const key = crypto.randomUUID();
    const body = { expected_revision: 0, [kind]: { name: name.trim() } };
    void write.submit(async ({ signal }) => {
      const receipt = await createBibleEntry({ pid: projectID, kind }, body, {
        signal,
        headers: { "Idempotency-Key": key },
      });
      if (!isProjectID(receipt.entry_id) || receipt.kind !== kind)
        throw new RequestError("invalid_receipt");
      if (signal.aborted) return;
      setName("");
      void entities.mutate();
    });
  }

  return (
    <div className="flex flex-col gap-4">
      <header className="flex flex-wrap items-center justify-between gap-2">
        <h2 className="text-lg font-semibold">制作资产库</h2>
        <div className="flex flex-wrap gap-2">
          {writable && (
            <Button variant="outline" onClick={() => setUpload(true)}>
              上传项目素材
            </Button>
          )}
          <Button variant="outline" asChild>
            <Link href={canvas}>在画布使用素材</Link>
          </Button>
        </div>
      </header>
      <Tabs
        value={tab}
        onValueChange={(value) => {
          if (!write.locked) {
            setTab(value as Kind | "media");
            setName("");
          }
        }}
      >
        <TabsList variant="secondary" aria-label="制作资产分类">
          <TabsTrigger value="media" disabled={write.locked}>
            项目素材
          </TabsTrigger>
          {kinds.map(({ id, label }) => (
            <TabsTrigger key={id} value={id} disabled={write.locked}>
              {label}
            </TabsTrigger>
          ))}
        </TabsList>
        <TabsContent value="media">
          {media.error && (
            <Alert role="alert">
              <AlertDescription>
                无法读取项目素材。{" "}
                <Button onClick={() => void media.mutate()}>重试</Button>
              </AlertDescription>
            </Alert>
          )}
          {media.isLoading && (
            <ContentState
              state="loading"
              title="正在读取项目素材"
              loadingLayout="grid"
            />
          )}
          {media.data && !media.data.items?.length && (
            <ContentState title="暂无项目素材" />
          )}
          <div className="grid grid-cols-2 gap-3 md:grid-cols-4">
            {media.data?.items?.map(
              (asset) =>
                isProjectID(asset.id) &&
                asset.project_id === projectID && (
                  <Card key={asset.id} size="sm">
                    <CardHeader>
                      <CardTitle className="truncate" title={asset.file_name}>
                        {asset.file_name ?? "未命名素材"}
                      </CardTitle>
                      <p className="text-xs text-muted-foreground">
                        {asset.kind}
                      </p>
                      {["image", "video", "audio"].includes(
                        asset.kind ?? "",
                      ) && (
                        <Button
                          variant="outline"
                          size="sm"
                          onClick={() =>
                            setPreview(
                              preview === asset.id ? undefined : asset.id,
                            )
                          }
                        >
                          {preview === asset.id ? "关闭预览" : "预览"}
                        </Button>
                      )}
                      {preview === asset.id && (
                        <div className="aspect-video">
                          <ProjectMediaPreview
                            projectID={projectID}
                            id={asset.id}
                            kind={asset.kind}
                            large
                          />
                        </div>
                      )}
                    </CardHeader>
                  </Card>
                ),
            )}
          </div>
          <DataPagination
            label="项目素材分页"
            page={cursors.length}
            pageSize={pageSize}
            hasNext={Boolean(media.data?.next_cursor)}
            disabled={media.isLoading}
            onPageChange={(next) => {
              if (next < cursors.length) setCursors(cursors.slice(0, -1));
              else if (media.data?.next_cursor)
                setCursors([...cursors, media.data.next_cursor]);
            }}
            onPageSizeChange={(size) => {
              setPageSize(size);
              setCursors([undefined]);
            }}
          />
        </TabsContent>
        {kinds.map(({ id, label }) => (
          <TabsContent key={id} value={id}>
            {writable && (
              <form
                className="mb-4 flex flex-wrap items-end gap-2"
                onSubmit={(event) => {
                  event.preventDefault();
                  create(id);
                }}
              >
                <Field className="max-w-sm">
                  <FieldLabel htmlFor={`asset-name-${id}`}>
                    {label}名称
                  </FieldLabel>
                  <Input
                    id={`asset-name-${id}`}
                    maxLength={80}
                    value={name}
                    disabled={write.locked}
                    onChange={(event) => setName(event.target.value)}
                  />
                </Field>
                <Button disabled={write.locked || !name.trim()}>
                  新建{label}
                </Button>
              </form>
            )}
            {entities.isLoading && (
              <ContentState
                state="loading"
                title={`正在读取${label}`}
                loadingLayout="list"
              />
            )}
            {entities.error && (
              <Alert role="alert">
                <AlertDescription>
                  无法读取{label}。{" "}
                  <Button onClick={() => void entities.mutate()}>重试</Button>
                </AlertDescription>
              </Alert>
            )}
            {entities.data && !entities.data.entries?.length && (
              <ContentState title={`暂无${label}`} />
            )}
            {entities.data?.entries?.length ? (
              <ItemGroup aria-label={label}>
                {entities.data.entries.map((entry, index) => (
                  <Item
                    key={entry.head?.id ?? index}
                    role="listitem"
                    variant="muted"
                  >
                    <ItemTitle>{entry.name}</ItemTitle>
                  </Item>
                ))}
              </ItemGroup>
            ) : null}
          </TabsContent>
        ))}
      </Tabs>
      {write.error && (
        <Alert role="alert">
          <AlertDescription>
            {write.error}{" "}
            {write.unknown && (
              <Button
                disabled={write.pending}
                onClick={() => void write.submit()}
              >
                重试原操作
              </Button>
            )}
          </AlertDescription>
        </Alert>
      )}
      {upload && (
        <UploadDialog
          projectID={projectID}
          onClose={() => setUpload(false)}
          onUploaded={async () => {
            await media.mutate();
          }}
        />
      )}
    </div>
  );
}
