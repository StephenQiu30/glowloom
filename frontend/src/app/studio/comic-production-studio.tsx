"use client";

import {
  AlertCircle,
  ArrowRight,
  Box,
  Check,
  ImagePlus,
  Info,
  LockKeyhole,
  MapPin,
  Mic2,
  Palette,
  Plus,
  Save,
  Search,
  Shirt,
  Upload,
  Users,
  WandSparkles,
} from "lucide-react";
import Image from "next/image";
import { useMemo, useState } from "react";
import { Tabs } from "radix-ui";

import { StudioShell } from "@/components/studio/studio-shell";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { cn } from "@/lib/class-names";

const assetTypes = [
  { id: "character", label: "角色", count: 4, icon: Users },
  { id: "scene", label: "场景", count: 6, icon: MapPin },
  { id: "prop", label: "道具", count: 8, icon: Box },
  { id: "costume", label: "服装", count: 5, icon: Shirt },
  { id: "voice", label: "声音", count: 4, icon: Mic2 },
  { id: "style", label: "风格", count: 2, icon: Palette },
] as const;

type AssetType = (typeof assetTypes)[number]["id"];

type Character = {
  id: string;
  name: string;
  role: string;
  style: string;
  version: number;
  locked: boolean;
  description: string;
  image: string;
  sheet?: string;
  faceReferences: number;
  costumeReferences: number;
  shotReferences: number;
  blockers: number;
};

const characters: Character[] = [
  {
    id: "gu-qinghe",
    name: "顾清禾",
    role: "女主角",
    style: "水墨幻想",
    version: 3,
    locked: true,
    description: "清冷疏离，乌发高髻，青灰色长衫，右眼下方一颗泪痣。",
    image: "/assets/lanverse-studio/gu-qinghe-portrait.png",
    sheet: "/assets/lanverse-studio/gu-qinghe-model-sheet.png",
    faceReferences: 3,
    costumeReferences: 2,
    shotReferences: 18,
    blockers: 2,
  },
  {
    id: "lu-chenzhou",
    name: "陆沉舟",
    role: "男主角",
    style: "水墨幻想",
    version: 2,
    locked: true,
    description: "沉静克制，深色束发，墨灰长袍，左手常持旧剑。",
    image: "/assets/lanverse-studio/lu-chenzhou-portrait.png",
    faceReferences: 2,
    costumeReferences: 2,
    shotReferences: 14,
    blockers: 0,
  },
  {
    id: "a-ning",
    name: "阿宁",
    role: "重要配角",
    style: "水墨幻想",
    version: 1,
    locked: false,
    description: "明朗机敏，红色窄袖衣，肩背药箱，发间系红绳。",
    image: "/assets/lanverse-studio/a-ning-portrait.png",
    faceReferences: 2,
    costumeReferences: 1,
    shotReferences: 8,
    blockers: 3,
  },
  {
    id: "painting-spirit",
    name: "画灵",
    role: "神秘角色",
    style: "水墨幻想",
    version: 2,
    locked: false,
    description: "银白长发，半透明衣袂，轮廓带水墨晕染与淡青微光。",
    image: "/assets/lanverse-studio/painting-spirit-portrait.png",
    faceReferences: 2,
    costumeReferences: 2,
    shotReferences: 6,
    blockers: 1,
  },
];

const genericAssets: Record<Exclude<AssetType, "character">, { name: string; meta: string }[]> = {
  scene: [
    { name: "顾府画阁", meta: "内景 · 夜 · v2" },
    { name: "长安雨巷", meta: "外景 · 夜 · v3" },
    { name: "无相画境", meta: "幻境 · v1" },
    { name: "西市茶楼", meta: "内景 · 日 · v2" },
  ],
  prop: [
    { name: "青玉画轴", meta: "核心道具 · 已锁定" },
    { name: "陆氏旧剑", meta: "武器 · 已锁定" },
    { name: "朱砂伞", meta: "随身道具 · 待确认" },
    { name: "铜制药箱", meta: "随身道具 · v1" },
  ],
  costume: [
    { name: "顾清禾 · 青灰常服", meta: "角色服装 · v3" },
    { name: "顾清禾 · 画境礼服", meta: "角色服装 · v1" },
    { name: "陆沉舟 · 夜行装", meta: "角色服装 · v2" },
  ],
  voice: [
    { name: "顾清禾 · 清冷女声", meta: "普通话 · 已授权" },
    { name: "陆沉舟 · 青年男声", meta: "普通话 · 已授权" },
    { name: "旁白 · 沉浸叙事", meta: "普通话 · 已授权" },
  ],
  style: [
    { name: "水墨幻想", meta: "主视觉 · 已锁定" },
    { name: "雨夜霓虹", meta: "场景变体 · v1" },
  ],
};

function CharacterList({ selectedId, onSelect }: { selectedId: string; onSelect: (id: string) => void }) {
  const [query, setQuery] = useState("");
  const visibleCharacters = characters.filter((character) => character.name.includes(query));

  return (
    <section className="min-w-0 border-r border-slate-200/80 bg-white p-3" aria-label="角色列表">
      <div className="flex items-center justify-between gap-2 px-1 py-1">
        <h2 className="text-sm font-semibold">角色列表</h2>
        <Button aria-label="添加角色" size="sm" variant="outline"><Plus aria-hidden="true" />添加</Button>
      </div>
      <div className="relative mt-3">
        <Search className="pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2 text-slate-400" aria-hidden="true" />
        <Input className="pl-9" aria-label="搜索角色" placeholder="搜索角色" value={query} onChange={(event) => setQuery(event.target.value)} />
      </div>
      <div className="mt-3 grid gap-2">
        {visibleCharacters.map((character) => (
          <button
            aria-pressed={selectedId === character.id}
            className={cn(
              "group flex min-w-0 items-center gap-3 rounded-xl border p-2 text-left transition-all",
              selectedId === character.id
                ? "border-[#75ccda] bg-[#f2fbfc] shadow-sm shadow-cyan-700/5"
                : "border-slate-200 bg-white hover:border-slate-300 hover:bg-slate-50",
            )}
            key={character.id}
            onClick={() => onSelect(character.id)}
            type="button"
          >
            <span className="relative size-16 shrink-0 overflow-hidden rounded-lg bg-slate-100">
              <Image alt={`${character.name}头像`} fill sizes="64px" src={character.image} className="object-cover" />
            </span>
            <span className="min-w-0 flex-1">
              <span className="flex items-center gap-1.5 truncate text-sm font-medium">
                {character.name}
                {character.locked ? <LockKeyhole className="size-3 text-[#079db3]" aria-label="已锁定" /> : null}
              </span>
              <span className="mt-1 block truncate text-xs text-slate-500">{character.role} · v{character.version}</span>
            </span>
          </button>
        ))}
      </div>
    </section>
  );
}

function ReferenceRow({ label, count, image }: { label: string; count: number; image: string }) {
  return (
    <div>
      <div className="mb-2 flex items-center gap-2 text-sm font-medium">
        {label}<span className="font-normal text-slate-400">{count} 张</span>
      </div>
      <div className="flex gap-2">
        {Array.from({ length: count }).map((_, index) => (
          <span className="relative size-12 overflow-hidden rounded-lg border border-slate-200 bg-slate-100" key={index}>
            <Image alt={`${label} ${index + 1}`} fill sizes="48px" src={image} className={cn("object-cover", index === 1 && "scale-110 object-[55%_35%]", index === 2 && "scale-125 object-[45%_30%]")} />
          </span>
        ))}
        <button aria-label={`添加${label}`} className="grid size-12 place-items-center rounded-lg border border-dashed border-slate-300 text-slate-500 transition-colors hover:border-[#079db3] hover:text-[#079db3]" type="button">
          <Plus className="size-4" aria-hidden="true" />
        </button>
      </div>
    </div>
  );
}

function CharacterInspector({
  character,
  onSaved,
}: {
  character: Character;
  onSaved: (version: number) => void;
}) {
  const [description, setDescription] = useState(character.description);
  const [saved, setSaved] = useState(false);

  function saveVersion() {
    setSaved(true);
    onSaved(character.version + 1);
    window.setTimeout(() => setSaved(false), 2600);
  }

  return (
    <aside className="min-w-0 bg-white p-5" aria-label="角色资产信息">
      <div className="flex items-start justify-between gap-4">
        <div>
          <h2 className="text-2xl font-semibold tracking-tight">{character.name}</h2>
          <p className="mt-1 text-sm text-slate-500">{character.role} · {character.style}</p>
        </div>
        <Badge className="border-[#bee8ee] bg-[#effbfc] text-[#087f91]" variant="outline">版本 v{character.version}</Badge>
      </div>

      <div className="mt-6 flex items-center gap-2 text-sm text-slate-600">
        {character.locked ? <LockKeyhole className="size-4 text-[#079db3]" aria-hidden="true" /> : <AlertCircle className="size-4 text-amber-500" aria-hidden="true" />}
        {character.locked ? "已锁定为分镜默认版本" : "未锁定，生成前需确认"}
      </div>

      <div className="mt-6">
        <div className="flex items-center justify-between">
          <Label htmlFor="consistencyDescription">一致性描述</Label>
          <span className="text-xs text-slate-400">{description.length}/120</span>
        </div>
        <textarea
          className="mt-2 min-h-28 w-full resize-none rounded-xl border border-slate-200 bg-white px-3 py-3 text-sm leading-6 outline-none transition focus:border-[#079db3] focus:ring-3 focus:ring-cyan-500/10"
          id="consistencyDescription"
          maxLength={120}
          value={description}
          onChange={(event) => setDescription(event.target.value)}
        />
      </div>

      <div className="mt-6 grid gap-5">
        <ReferenceRow label="面部参考" count={character.faceReferences} image={character.image} />
        <ReferenceRow label="服装参考" count={character.costumeReferences} image={character.sheet ?? character.image} />
      </div>

      <div className="mt-7 grid gap-2">
        <Button className="h-10 bg-[#079db3] text-white hover:bg-[#078da0]" onClick={saveVersion}>
          {saved ? <Check aria-hidden="true" /> : <Save aria-hidden="true" />}
          {saved ? "版本已保存" : "保存新版本"}
        </Button>
        <Button className="h-10 text-[#078fa5]" variant="outline">
          <WandSparkles aria-hidden="true" />生成更多参考
        </Button>
      </div>

      <div className="mt-6 rounded-xl border border-amber-200/80 bg-amber-50/80 p-4">
        <p className="text-sm font-semibold">影响范围</p>
        <div className="mt-3 grid gap-3 text-sm">
          <button className="flex items-center gap-2 text-left" type="button">
            <Info className="size-4 shrink-0 text-[#079db3]" aria-hidden="true" />
            <span className="flex-1">{character.shotReferences} 个分镜引用此版本</span>
            <span className="text-[#078fa5]">查看影响</span>
          </button>
          {character.blockers > 0 ? (
            <button className="flex items-center gap-2 text-left" type="button">
              <AlertCircle className="size-4 shrink-0 text-amber-500" aria-hidden="true" />
              <span className="flex-1">{character.blockers} 个镜头需重新确认</span>
              <span className="text-[#078fa5]">去处理</span>
            </button>
          ) : (
            <div className="flex items-center gap-2 text-emerald-700"><Check className="size-4" aria-hidden="true" />没有待处理的镜头</div>
          )}
        </div>
      </div>
    </aside>
  );
}

function GenericAssetWorkspace({ type }: { type: Exclude<AssetType, "character"> }) {
  const assetType = assetTypes.find((item) => item.id === type)!;
  const Icon = assetType.icon;
  const items = genericAssets[type];

  return (
    <div className="grid min-h-[560px] grid-cols-[240px_minmax(0,1fr)] overflow-hidden rounded-2xl border border-slate-200 bg-white">
      <section className="border-r border-slate-200 p-4">
        <div className="flex items-center justify-between">
          <h2 className="text-sm font-semibold">{assetType.label}列表</h2>
          <Button aria-label={`添加${assetType.label}`} size="icon-sm" variant="outline"><Plus aria-hidden="true" /></Button>
        </div>
        <div className="mt-4 grid gap-2">
          {items.map((item, index) => (
            <button className={cn("rounded-xl border p-3 text-left", index === 0 ? "border-[#75ccda] bg-[#f2fbfc]" : "border-slate-200 hover:bg-slate-50")} key={item.name} type="button">
              <span className="block text-sm font-medium">{item.name}</span>
              <span className="mt-1 block text-xs text-slate-500">{item.meta}</span>
            </button>
          ))}
        </div>
      </section>
      <section className="grid place-items-center p-10 text-center">
        <div className="max-w-sm">
          <span className="mx-auto grid size-16 place-items-center rounded-2xl bg-slate-100 text-[#079db3]"><Icon className="size-7" aria-hidden="true" /></span>
          <h2 className="mt-5 text-xl font-semibold">{items[0].name}</h2>
          <p className="mt-2 text-sm leading-6 text-slate-500">在这里管理{assetType.label}规格、参考媒体、授权状态和不可变版本；锁定后自动成为新分镜的默认引用。</p>
          <div className="mt-6 flex justify-center gap-2">
            <Button variant="outline"><Upload aria-hidden="true" />添加参考</Button>
            <Button className="bg-[#079db3] text-white hover:bg-[#078da0]"><Save aria-hidden="true" />保存新版本</Button>
          </div>
        </div>
      </section>
    </div>
  );
}

export function ComicProductionStudio() {
  const [assetType, setAssetType] = useState<AssetType>("character");
  const [selectedCharacterId, setSelectedCharacterId] = useState("gu-qinghe");
  const [currentStep, setCurrentStep] = useState(1);
  const [notice, setNotice] = useState<string | null>(null);
  const selectedCharacter = useMemo(
    () => characters.find((character) => character.id === selectedCharacterId) ?? characters[0],
    [selectedCharacterId],
  );

  function showNotice(message: string) {
    setNotice(message);
    window.setTimeout(() => setNotice(null), 3200);
  }

  return (
    <StudioShell
      active="assets"
      currentStep={currentStep}
      projectName="她从画中来"
      topAction={(
        <Button className="ml-auto h-10 bg-[#079db3] px-4 text-white hover:bg-[#078da0]" onClick={() => { setCurrentStep(2); showNotice("已进入分镜阶段，2 个受影响镜头等待确认"); }}>
          继续制作<ArrowRight aria-hidden="true" />
        </Button>
      )}
    >
      {notice ? (
        <div className="fixed top-24 right-6 z-50 flex items-center gap-2 rounded-xl border border-emerald-200 bg-white px-4 py-3 text-sm shadow-lg shadow-slate-950/10" role="status">
          <Check className="size-4 text-emerald-600" aria-hidden="true" />{notice}
        </div>
      ) : null}

      <div className="mx-auto max-w-[1440px] px-5 py-7 md:px-8">
          <div className="flex flex-wrap items-end justify-between gap-4">
            <div>
              <h1 className="text-3xl font-semibold tracking-[-0.03em]">资产库</h1>
              <p className="mt-2 text-sm text-slate-500">锁定角色与世界观，让每个镜头保持一致</p>
            </div>
            <div className="flex items-center gap-2 text-xs text-slate-500"><LockKeyhole className="size-4 text-[#079db3]" aria-hidden="true" />资产版本只追加，历史引用不会漂移</div>
          </div>

          <Tabs.Root className="mt-6" value={assetType} onValueChange={(value) => setAssetType(value as AssetType)}>
            <Tabs.List className="flex gap-7 overflow-x-auto border-b border-slate-200" aria-label="资产类型">
              {assetTypes.map((type) => (
                <Tabs.Trigger className="group relative flex shrink-0 items-center gap-2 pb-3 text-sm text-slate-500 outline-none data-[state=active]:font-medium data-[state=active]:text-[#078fa5]" key={type.id} value={type.id}>
                  <type.icon className="size-4" strokeWidth={1.8} aria-hidden="true" />
                  {type.label}<span className="text-xs text-slate-400">{type.count}</span>
                  <span className="absolute inset-x-0 bottom-0 h-0.5 rounded-full bg-[#079db3] opacity-0 group-data-[state=active]:opacity-100" />
                </Tabs.Trigger>
              ))}
            </Tabs.List>

            <Tabs.Content className="mt-0 overflow-x-auto outline-none" value="character">
              <div className="grid min-h-[650px] grid-cols-[220px_minmax(420px,1fr)_330px] overflow-hidden rounded-b-2xl border-x border-b border-slate-200 bg-white 2xl:grid-cols-[240px_minmax(520px,1fr)_350px]" data-testid="character-asset-workspace">
                <CharacterList selectedId={selectedCharacterId} onSelect={setSelectedCharacterId} />
                <section className="relative min-w-0 bg-[#fafbfb] p-4" aria-label="角色设定图">
                  <div className="relative h-[590px] overflow-hidden rounded-2xl border border-slate-200 bg-[#f4f5f4] 2xl:h-[640px]">
                    <Image
                      alt={selectedCharacter.sheet ? `${selectedCharacter.name}角色设定图` : `${selectedCharacter.name}角色参考图`}
                      fill
                      loading="eager"
                      sizes="(min-width: 1280px) 55vw, 50vw"
                      src={selectedCharacter.sheet ?? selectedCharacter.image}
                      className={selectedCharacter.sheet ? "scale-[1.3] object-contain 2xl:scale-[1.36]" : "object-contain p-16"}
                    />
                    {!selectedCharacter.sheet ? (
                      <div className="absolute right-4 bottom-4 left-4 flex items-center justify-between rounded-xl border border-slate-200 bg-white/95 px-4 py-3 text-sm shadow-sm">
                        <span className="flex items-center gap-2 text-slate-600"><ImagePlus className="size-4 text-[#079db3]" aria-hidden="true" />角色设定图尚未补齐</span>
                        <Button size="sm" variant="outline">生成设定图</Button>
                      </div>
                    ) : null}
                  </div>
                  <div className="absolute inset-x-4 bottom-4 flex items-center justify-center gap-2 text-xs text-slate-500"><Info className="size-4 text-[#079db3]" aria-hidden="true" />锁定后将作为后续分镜与生成的默认参考</div>
                </section>
                <CharacterInspector key={selectedCharacter.id} character={selectedCharacter} onSaved={(version) => showNotice(`${selectedCharacter.name} v${version} 已保存，旧版本仍可追溯`)} />
              </div>
            </Tabs.Content>
            {assetTypes.filter((type) => type.id !== "character").map((type) => (
              <Tabs.Content className="mt-0 outline-none" key={type.id} value={type.id}>
                <GenericAssetWorkspace type={type.id as Exclude<AssetType, "character">} />
              </Tabs.Content>
            ))}
          </Tabs.Root>
      </div>
    </StudioShell>
  );
}
