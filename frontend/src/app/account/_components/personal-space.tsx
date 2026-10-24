"use client";

import dynamic from "next/dynamic";
import Link from "next/link";
import { Suspense, useState } from "react";
import { useSession } from "@/components/auth/session";
import { getMyAvatar } from "@/api/media";
import { BoundAvatar } from "@/components/account/avatar-editor";
import { PageLayout } from "@/components/layout/page-layout";
import { PageHeading } from "@/components/layout/page-heading";
import { Button } from "@/components/ui/button";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import {
  Dialog,
  DialogContent,
  DialogTitle,
  DialogDescription,
} from "@/components/ui/dialog";
import { ContentState } from "@/components/layout/content-state";

const Settings = dynamic(() =>
  import("./account-page").then((m) => m.Settings),
);
const FavoriteAssets = dynamic(() =>
  import("@/components/account/favorite-assets").then((m) => m.FavoriteAssets),
);
const ProjectList = dynamic(() =>
  import("@/components/projects/project-list").then((m) => m.ProjectList),
);

export function PersonalSpace() {
  const { actor } = useSession();
  const [settings, setSettings] = useState(false);
  const [filter, setFilter] = useState<"active" | "archived" | "deleted">(
    "active",
  );
  const [sort, setSort] = useState<"recent_content" | "metadata">(
    "recent_content",
  );
  return (
    <PageLayout screen="account">
      <PageHeading
        title="个人空间"
        actions={
          <Button variant="ghost" onClick={() => setSettings(true)}>
            账号设置
          </Button>
        }
      />
      <div className="personal-space-layout">
        <aside className="personal-profile" aria-label="个人资料">
          <BoundAvatar
            assetID={actor.avatar_asset_id}
            load={(signal) => getMyAvatar({ signal })}
            label={actor.display_name}
          />
          <h2 className="text-xl font-medium">{actor.display_name}</h2>
          <p className="text-sm break-all text-muted-foreground">
            {actor.email}
          </p>
          <Button variant="secondary" onClick={() => setSettings(true)}>
            编辑资料
          </Button>
        </aside>
        <Tabs defaultValue="works" className="min-w-0 gap-6">
          <TabsList variant="navigation" aria-label="个人内容">
            <TabsTrigger value="works">作品</TabsTrigger>
            <TabsTrigger value="favorites">收藏</TabsTrigger>
            <TabsTrigger value="canvases">画布</TabsTrigger>
          </TabsList>
          <TabsContent value="works">
            <ContentState
              title="作品发布暂未开放"
              description="已保存的创作可在素材库和画布中继续编辑。"
              actions={
                <Button asChild variant="secondary">
                  <Link href="/">开始创作</Link>
                </Button>
              }
            />
          </TabsContent>
          <TabsContent value="favorites">
            <Suspense
              fallback={<ContentState state="loading" title="正在读取收藏" />}
            >
              <FavoriteAssets key={actor.id} />
            </Suspense>
          </TabsContent>
          <TabsContent value="canvases">
            <Suspense
              fallback={<ContentState state="loading" title="正在读取画布" />}
            >
              <ProjectList
                key={`${filter}/${sort}`}
                query=""
                view="grid"
                filter={filter}
                onFilterChange={setFilter}
                sort={sort}
                onSortChange={setSort}
              />
            </Suspense>
          </TabsContent>
        </Tabs>
      </div>
      <Dialog open={settings} onOpenChange={setSettings}>
        <DialogContent className="account-settings-dialog sm:max-w-3xl">
          <DialogTitle>账号设置</DialogTitle>
          <DialogDescription className="sr-only">
            管理个人资料、安全与创作偏好
          </DialogDescription>
          <Suspense
            fallback={
              <ContentState
                className="min-h-40"
                state="loading"
                title="正在读取设置"
              />
            }
          >
            <Settings embedded />
          </Suspense>
        </DialogContent>
      </Dialog>
    </PageLayout>
  );
}
