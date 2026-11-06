"use client";

import dynamic from "next/dynamic";
import Link from "next/link";
import { useRef, useState } from "react";
import { cn } from "cn";
import {
  Box,
  Brush,
  Camera,
  Film,
  Image,
  ScanLine,
  Sparkles,
  Video,
} from "lucide-react";
import { useOptionalSession } from "@/components/auth/session";
import { useLoginDialog } from "@/components/layout/basic-layout";
import { PageLayout } from "@/components/layout/page-layout";
import { AssetCreateProject } from "@/components/projects/asset-create-project";
import { useSpace } from "@/components/spaces/space-context";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { categories, type Tool } from "./catalog";

const ToolImportDialog = dynamic(() =>
  import("./tool-import-dialog").then((module) => module.ToolImportDialog),
);
const categoryIcons = [
  [Brush, Sparkles, Film, ScanLine],
  [Video, Brush, Video, Film],
  [Image, Brush, Brush, Image, Brush, Image, ScanLine],
  [Camera, Sparkles, Image, Box, Video, Box],
];
const coverSurfaces = [
  "bg-muted",
  "bg-surface-2",
  "bg-surface-3",
  "bg-surface-4",
];

export function ToolsDirectory() {
  const session = useOptionalSession();
  const login = useLoginDialog();
  const { selected, pending } = useSpace();
  const [active, setActive] = useState("全部");
  const [opened, setOpened] = useState<Tool | null>(null);
  const [locked, setLocked] = useState(false);
  const [error, setError] = useState("");
  const opener = useRef<HTMLButtonElement>(null);
  const focusOpener = () => opener.current?.focus();

  function launch(tool: Tool, trigger: HTMLButtonElement) {
    setError("");
    if ("template" in tool) {
      if (!session) {
        login();
        return;
      }
      if (pending || !selected?.writable) {
        setError("请选择可写的创作空间后再打开 3D导演台。");
        return;
      }
    }
    opener.current = trigger;
    setOpened(tool);
  }

  return (
    <PageLayout screen="ai-tools" hideSearch fullBleed>
      <div className="mx-auto flex w-full max-w-[86rem] flex-col gap-7 px-4 py-8 md:px-9 md:py-10">
        <h1 className="text-2xl font-semibold tracking-tight md:text-center md:text-4xl">
          创作所需 一应俱全
        </h1>
        {error && (
          <Alert variant="destructive">
            <AlertDescription>{error}</AlertDescription>
          </Alert>
        )}
        <Tabs value={active} onValueChange={setActive} className="gap-6">
          <TabsList
            aria-label="分类"
            variant="pill"
            className="max-w-full flex-wrap gap-0 md:gap-2"
          >
            {["全部", ...categories.map(({ name }) => name)].map((name) => (
              <TabsTrigger key={name} value={name} className="px-3 md:px-4">
                {name}
              </TabsTrigger>
            ))}
          </TabsList>
          {["全部", ...categories.map(({ name }) => name)].map((tab) => (
            <TabsContent key={tab} value={tab} className="flex flex-col gap-7">
              {categories.map(({ name, tools }, groupIndex) =>
                tab === "全部" || tab === name ? (
                  <section
                    key={name}
                    aria-label={name}
                    className="flex flex-col gap-3"
                  >
                    <h2 className="font-medium">{name}</h2>
                    <ul className="grid grid-cols-2 gap-3 md:grid-cols-4 md:gap-4">
                      {tools.map((tool, i) => {
                        const Icon = categoryIcons[groupIndex][i];
                        const titleID = `ai-tool-${tool.id}`;
                        return (
                          <li key={tool.id} className="min-w-0">
                            <Card
                              variant="feature"
                              size="sm"
                              className="isolate"
                            >
                              <CardContent
                                className={cn(
                                  "grid aspect-video min-h-20 place-items-center p-0 text-[var(--navigation-accent)]",
                                  coverSurfaces[i % coverSurfaces.length],
                                )}
                              >
                                <Icon
                                  aria-hidden="true"
                                  className="size-9 stroke-[1.35] md:size-12"
                                />
                              </CardContent>
                              <CardHeader className="grow py-3">
                                <CardTitle id={titleID}>{tool.name}</CardTitle>
                                <CardDescription className="text-xs">
                                  {tool.summary}
                                </CardDescription>
                              </CardHeader>
                              {"href" in tool ? (
                                <Link
                                  href={tool.href}
                                  aria-labelledby={titleID}
                                  className="absolute inset-0 z-10 rounded-xl outline-none hover:bg-foreground/5 focus-visible:ring-1 focus-visible:ring-ring focus-visible:ring-inset"
                                />
                              ) : (
                                <Button
                                  variant="ghost"
                                  type="button"
                                  aria-haspopup="dialog"
                                  aria-labelledby={titleID}
                                  disabled={locked}
                                  onClick={(event) =>
                                    launch(tool, event.currentTarget)
                                  }
                                  className="absolute inset-0 z-10 h-auto w-auto cursor-pointer rounded-xl outline-none hover:bg-foreground/5 focus-visible:ring-1 focus-visible:ring-ring focus-visible:ring-inset disabled:cursor-wait"
                                />
                              )}
                            </Card>
                          </li>
                        );
                      })}
                    </ul>
                  </section>
                ) : null,
              )}
              {tab === "全部" && (
                <p className="text-xs text-muted-foreground">没有更多了</p>
              )}
            </TabsContent>
          ))}
        </Tabs>
      </div>
      {opened && "media" in opened && (
        <ToolImportDialog
          key={`${opened.id}/${session?.actor.id ?? "guest"}/${selected?.id ?? "none"}`}
          tool={opened}
          onClose={() => setOpened(null)}
          onCloseAutoFocus={focusOpener}
        />
      )}
      {opened && "template" in opened && (
        <AssetCreateProject
          kind="canvas"
          template={{ key: opened.template, name: opened.name }}
          onClose={() => setOpened(null)}
          onLock={setLocked}
        />
      )}
    </PageLayout>
  );
}
