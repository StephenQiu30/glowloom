"use client";

import dynamic from "next/dynamic";
import Link from "next/link";
import { useRef, useState } from "react";
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
import { HeaderNavigation } from "@/components/layout/header-navigation";
import { PageHeader } from "@/components/canvas/runtime/components/layout/workspace/page-header";
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
import { categories, featuredToolIds, type Tool } from "./catalog";

const ToolImportDialog = dynamic(() =>
  import("./tool-import-dialog").then((module) => module.ToolImportDialog),
);
const categoryIcons = [
  [Brush, Sparkles, ScanLine],
  [Video, Brush, Video, Film, Film, Film],
  [Image, Brush, Brush, Image, Brush, Image, ScanLine],
  [Camera, Sparkles, Image, Box, ScanLine, Box],
];
const toolsById = new Map(
  categories.flatMap(({ tools }, group) =>
    tools.map(
      (tool, index) =>
        [tool.id, { tool, icon: categoryIcons[group][index] }] as const,
    ),
  ),
);
const featured = {
  name: "精选",
  tools: featuredToolIds.flatMap((id) => toolsById.get(id)?.tool ?? []),
};
const allSections = [
  featured,
  ...categories
    .map(({ name, tools }) => ({
      name,
      tools: tools.filter(({ id }) => !featuredToolIds.includes(id)),
    }))
    .filter(({ tools }) => tools.length),
];
const tabs = ["全部", ...categories.map(({ name }) => name)];

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
    <PageLayout screen="ai-tools" fullBleed>
      <div className="tool-directory">
        <PageHeader title="AI 工具" />
        {error && (
          <Alert variant="destructive">
            <AlertDescription>{error}</AlertDescription>
          </Alert>
        )}
        <Tabs value={active} onValueChange={setActive} className="gap-8">
          <HeaderNavigation label="工具分类" active={active}>
            <TabsList aria-label="分类" variant="workspace">
              {tabs.map((name) => (
                <TabsTrigger key={name} value={name}>
                  {name}
                </TabsTrigger>
              ))}
            </TabsList>
          </HeaderNavigation>
          {tabs.map((tab) => (
            <TabsContent key={tab} value={tab} className="flex flex-col gap-7">
              {(tab === "全部"
                ? allSections
                : tab === "精选"
                  ? [featured]
                  : categories.filter(({ name }) => tab === name)
              ).map(({ name, tools }) => (
                <section
                  key={name}
                  aria-label={name}
                  className="flex flex-col gap-3"
                >
                  <h2 className="text-base font-semibold">{name}</h2>
                  <ul className="tool-directory-grid">
                    {tools.map((tool) => {
                      const Icon = toolsById.get(tool.id)?.icon ?? Sparkles;
                      const titleID = `ai-tool-${tool.id}`;
                      return (
                        <li key={tool.id} className="min-w-0">
                          <Card
                            variant="flat"
                            size="sm"
                            className="tool-directory-card"
                          >
                            <CardContent className="tool-directory-icon">
                              <Icon
                                aria-hidden="true"
                                className="size-5 stroke-[1.65]"
                              />
                            </CardContent>
                            <CardHeader className="min-w-0 grow p-0">
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
              ))}
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
