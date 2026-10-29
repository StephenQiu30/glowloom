"use client";

import dynamic from "next/dynamic";
import { useEffect, useRef, useState } from "react";
import { VoiceFilters, emptyVoiceTags, type VoiceTags } from "./voice-filters";
import { Star, Pencil, Trash2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import { Input } from "@/components/ui/input";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Dialog,
  DialogContent,
  DialogTitle,
  DialogDescription,
} from "@/components/ui/dialog";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";
import {
  ProjectMediaPicker,
  ProjectMediaPreview,
} from "@/components/media/project-media";
import { readErrorMessage } from "@/lib/read-error";
import { RequestError } from "@/lib/request";
import {
  changeVoice,
  favoriteSystem,
  readSystemVoices,
  readVoices,
  saveVoice,
  type ReferenceVoice,
  type SystemVoice,
  type VoiceSelection,
} from "./voice-library-api";

function voiceError(error: unknown) {
  if (error instanceof RequestError) {
    if (error.code === "voice_revision_conflict")
      return "音色已在另一窗口更新，请重新读取。";
    if (error.code === "invalid_voice_reference")
      return "请选择不超过 30 秒、10 MB 的就绪音频，并确认声音使用权限。";
    return readErrorMessage(error);
  }
  return error instanceof Error ? error.message : "音色操作未完成，请重试。";
}

const UploadDialog = dynamic(() =>
  import("@/components/media/upload-dialog").then((m) => m.UploadDialog),
);
const voiceTouchTargets =
  "max-sm:[&_button:not([role=checkbox])]:min-h-11 max-sm:[&_button:not([role=checkbox])]:min-w-11 max-sm:[&_input]:min-h-11";
export function VoiceLibrary({
  projectID,
  ownerKey,
  onClose,
  onSelect,
  onRestoreFocus,
}: {
  projectID: string;
  ownerKey: string;
  onClose: () => void;
  onSelect: (voice: VoiceSelection) => Promise<void> | void;
  onRestoreFocus?: () => void;
}) {
  const [voices, setVoices] = useState<ReferenceVoice[]>([]),
    [system, setSystem] = useState<SystemVoice[]>([]);
  const [filters, setFilters] = useState<VoiceTags>({
    gender: "all",
    age: "all",
    language: "all",
    style: "all",
  });
  const [tab, setTab] = useState("all"),
    [search, setSearch] = useState(""),
    [epoch, setEpoch] = useState(0),
    [loading, setLoading] = useState(true),
    [error, setError] = useState("");
  const [creating, setCreating] = useState(false),
    [editing, setEditing] = useState<ReferenceVoice | null>(null),
    [deleting, setDeleting] = useState<ReferenceVoice | null>(null),
    [name, setName] = useState("");
  const [busy, setBusy] = useState(false);
  const lock = useRef(false);
  const createButton = useRef<HTMLButtonElement>(null);
  const managementButton = useRef<HTMLButtonElement | null>(null);
  function restoreManagementFocus(event: Event) {
    event.preventDefault();
    (managementButton.current?.isConnected
      ? managementButton.current
      : createButton.current
    )?.focus();
  }
  useEffect(() => {
    const c = new AbortController();
    Promise.all([
      readVoices(projectID, c.signal),
      readSystemVoices(projectID, c.signal),
    ])
      .then(([mine, publicVoices]) => {
        if (!c.signal.aborted) {
          setVoices(mine);
          setSystem(publicVoices);
        }
      })
      .catch((e) => {
        if (!c.signal.aborted) setError(voiceError(e));
      })
      .finally(() => {
        if (!c.signal.aborted) setLoading(false);
      });
    return () => c.abort();
  }, [projectID, epoch]);
  async function run(work: () => Promise<void> | void) {
    if (lock.current) return;
    lock.current = true;
    setBusy(true);
    setError("");
    try {
      await work();
    } catch (e) {
      setError(voiceError(e));
    } finally {
      lock.current = false;
      setBusy(false);
    }
  }
  const match = (v: { name: string; favorite: boolean } & Partial<VoiceTags>) =>
    v.name.toLocaleLowerCase().includes(search.toLocaleLowerCase()) &&
    (tab !== "favorites" || v.favorite) &&
    (Object.keys(filters) as (keyof VoiceTags)[]).every(
      (key) => filters[key] === "all" || filters[key] === (v[key] ?? "未标注"),
    );
  const mine = voices.filter(match),
    publicVoices = tab === "mine" ? [] : system.filter(match);
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open && !busy) onClose();
      }}
    >
      <DialogContent
        className={`max-h-[88dvh] overflow-y-auto sm:max-w-3xl ${voiceTouchTargets}`}
        onCloseAutoFocus={(event) => {
          if (onRestoreFocus) {
            event.preventDefault();
            onRestoreFocus();
          }
        }}
      >
        <DialogTitle>音色库</DialogTitle>
        <DialogDescription>
          系统音色用于文字配音，参考音色用于全能配音。
        </DialogDescription>
        <div className="flex flex-wrap items-center justify-between gap-2">
          <Tabs value={tab} onValueChange={setTab}>
            <TabsList className="max-sm:h-auto">
              <TabsTrigger value="all">全部</TabsTrigger>
              <TabsTrigger value="mine">我的音色</TabsTrigger>
              <TabsTrigger value="favorites">收藏</TabsTrigger>
            </TabsList>
          </Tabs>
          <Button
            ref={createButton}
            onClick={() => setCreating(true)}
            disabled={busy}
          >
            创建音色
          </Button>
        </div>
        <Input
          aria-label="搜索音色"
          placeholder="搜索音色名称"
          value={search}
          onChange={(e) => setSearch(e.target.value)}
        />
        <VoiceFilters value={filters} onChange={setFilters} filter />
        {loading ? (
          <p role="status">正在读取音色…</p>
        ) : (
          <div className="grid gap-3 sm:grid-cols-2">
            {publicVoices.map((voice) => (
              <article key={voice.id} className="rounded-xl border p-4">
                <div className="flex items-center justify-between">
                  <h3 className="font-medium">{voice.name}</h3>
                  <Button
                    size="icon-sm"
                    variant="ghost"
                    aria-label={`${voice.favorite ? "取消收藏" : "收藏"}${voice.name}`}
                    aria-pressed={voice.favorite}
                    disabled={busy}
                    onClick={() =>
                      void run(async () => {
                        await favoriteSystem(projectID, voice);
                        setSystem((items) =>
                          items.map((v) =>
                            v.id === voice.id
                              ? { ...v, favorite: !v.favorite }
                              : v,
                          ),
                        );
                      })
                    }
                  >
                    <Star className={voice.favorite ? "fill-current" : ""} />
                  </Button>
                </div>
                <p className="my-3 text-xs text-muted-foreground">
                  系统音色 · 文字配音
                </p>
                <Button
                  variant="outline"
                  disabled={busy}
                  onClick={() =>
                    void run(() => onSelect({ kind: "system", voice }))
                  }
                >
                  使用音色
                </Button>
              </article>
            ))}
            {mine.map((voice) => (
              <article key={voice.id} className="min-w-0 rounded-xl border p-4">
                <div className="flex items-center justify-between gap-2">
                  <h3 className="truncate font-medium">{voice.name}</h3>
                  <Button
                    size="icon-sm"
                    variant="ghost"
                    aria-label={`${voice.favorite ? "取消收藏" : "收藏"}${voice.name}`}
                    aria-pressed={voice.favorite}
                    disabled={busy}
                    onClick={() =>
                      void run(async () => {
                        const updated = await changeVoice(projectID, voice, {
                          favorite: !voice.favorite,
                        });
                        setVoices((items) =>
                          items.map((v) => (v.id === updated.id ? updated : v)),
                        );
                      })
                    }
                  >
                    <Star className={voice.favorite ? "fill-current" : ""} />
                  </Button>
                </div>
                <p className="my-2 text-xs text-muted-foreground">
                  我的音色 · 参考音频
                </p>
                <div className="my-3 overflow-hidden [&_audio]:w-full">
                  <ProjectMediaPreview
                    projectID={projectID}
                    id={voice.asset_id}
                    kind="audio"
                    large
                  />
                </div>
                <div className="flex gap-2">
                  <Button
                    variant="outline"
                    disabled={busy}
                    onClick={() =>
                      void run(() => onSelect({ kind: "reference", voice }))
                    }
                  >
                    使用音色
                  </Button>
                  <Button
                    variant="ghost"
                    size="icon"
                    aria-label={`重命名${voice.name}`}
                    disabled={busy}
                    onClick={(event) => {
                      managementButton.current = event.currentTarget;
                      setEditing(voice);
                      setName(voice.name);
                    }}
                  >
                    <Pencil />
                  </Button>
                  <Button
                    variant="ghost"
                    size="icon"
                    aria-label={`删除${voice.name}`}
                    disabled={busy}
                    onClick={(event) => {
                      managementButton.current = event.currentTarget;
                      setDeleting(voice);
                    }}
                  >
                    <Trash2 />
                  </Button>
                </div>
              </article>
            ))}
            {!mine.length && !publicVoices.length && !error && (
              <p className="py-6 text-sm text-muted-foreground">
                没有匹配的音色
              </p>
            )}
          </div>
        )}
        {error && (
          <div role="alert">
            <p>{error}</p>
            <Button
              variant="outline"
              onClick={() => {
                setLoading(true);
                setError("");
                setEpoch((n) => n + 1);
              }}
            >
              重新读取
            </Button>
          </div>
        )}
        {creating && (
          <CreateVoice
            projectID={projectID}
            ownerKey={ownerKey}
            onClose={() => setCreating(false)}
            onRestoreFocus={() => createButton.current?.focus()}
            onSaved={(voice) => {
              setVoices((items) => [
                voice,
                ...items.filter((v) => v.id !== voice.id),
              ]);
              setCreating(false);
              setTab("mine");
            }}
          />
        )}
        {editing && (
          <Dialog
            open
            onOpenChange={(open) => {
              if (!open && !busy) setEditing(null);
            }}
          >
            <DialogContent
              className={voiceTouchTargets}
              onCloseAutoFocus={restoreManagementFocus}
            >
              <DialogTitle>重命名音色</DialogTitle>
              <DialogDescription>更新音色库中的显示名称。</DialogDescription>
              <Input
                aria-label="音色名称"
                maxLength={60}
                value={name}
                onChange={(e) => setName(e.target.value)}
              />
              <Button
                disabled={busy || !name.trim()}
                onClick={() =>
                  void run(async () => {
                    const updated = await changeVoice(projectID, editing, {
                      name: name.trim(),
                    });
                    setVoices((items) =>
                      items.map((v) => (v.id === updated.id ? updated : v)),
                    );
                    setEditing(null);
                  })
                }
              >
                保存名称
              </Button>
              {error && <p role="alert">{error}</p>}
            </DialogContent>
          </Dialog>
        )}
        {deleting && (
          <Dialog
            open
            onOpenChange={(open) => {
              if (!open && !busy) setDeleting(null);
            }}
          >
            <DialogContent
              className={voiceTouchTargets}
              onCloseAutoFocus={restoreManagementFocus}
            >
              <DialogTitle>删除“{deleting.name}”？</DialogTitle>
              <DialogDescription>
                从音色库移除，原音频和已有创作引用会保留。
              </DialogDescription>
              <Button
                variant="destructive"
                disabled={busy}
                onClick={() =>
                  void run(async () => {
                    await changeVoice(projectID, deleting, { delete: true });
                    setVoices((items) =>
                      items.filter((v) => v.id !== deleting.id),
                    );
                    setDeleting(null);
                  })
                }
              >
                删除音色
              </Button>
              {error && <p role="alert">{error}</p>}
            </DialogContent>
          </Dialog>
        )}
      </DialogContent>
    </Dialog>
  );
}

function CreateVoice({
  projectID,
  ownerKey,
  onClose,
  onSaved,
  onRestoreFocus,
}: {
  projectID: string;
  ownerKey: string;
  onClose: () => void;
  onSaved: (voice: ReferenceVoice) => void;
  onRestoreFocus: () => void;
}) {
  const storageKey = `glowloom:pending-voice:${ownerKey}`;
  const [recovery] = useState(() => {
    try {
      const value = JSON.parse(localStorage.getItem(storageKey) ?? "null");
      if (
        value?.request?.request_id &&
        value.request.asset_id === value.asset?.id &&
        value.asset.project_id === projectID &&
        value.request.consent === true &&
        typeof value.request.name === "string"
      )
        return value as {
          request: Parameters<typeof saveVoice>[1];
          asset: API.AssetSummary;
        };
    } catch {
      /* A malformed local receipt must not authorize a request. */
    }
    return null;
  });
  const [tags, setTags] = useState<VoiceTags>(
    recovery
      ? {
          gender: recovery.request.gender,
          age: recovery.request.age,
          language: recovery.request.language,
          style: recovery.request.style,
        }
      : emptyVoiceTags,
  );
  const [name, setName] = useState(recovery?.request.name ?? ""),
    [asset, setAsset] = useState<API.AssetSummary | null>(
      recovery?.asset ?? null,
    ),
    [consent, setConsent] = useState(!!recovery),
    [picker, setPicker] = useState(false),
    [upload, setUpload] = useState(false),
    [busy, setBusy] = useState(false),
    [error, setError] = useState("");
  const [request, setRequest] = useState<
    Parameters<typeof saveVoice>[1] | null
  >(recovery?.request ?? null);
  const lock = useRef(false);
  function choose(value: API.AssetSummary) {
    if (
      value.kind !== "audio" ||
      !value.id ||
      !value.duration_ms ||
      value.duration_ms > 30000 ||
      !value.byte_size ||
      value.byte_size > 10 * 1024 * 1024
    ) {
      setError("请选择不超过 30 秒、10 MB 的就绪音频。");
      return;
    }
    setAsset(value);
    setError("");
    setPicker(false);
    setUpload(false);
  }
  async function save() {
    if (lock.current || !asset?.id) return;
    lock.current = true;
    setBusy(true);
    setError("");
    const input = request ?? {
      ...tags,
      request_id: crypto.randomUUID(),
      name: name.trim(),
      asset_id: asset.id,
      consent,
    };
    setRequest(input);
    try {
      // Retain the exact request through refreshes; storage failure happens before any write.
      localStorage.setItem(
        storageKey,
        JSON.stringify({ request: input, asset }),
      );
      const saved = await saveVoice(projectID, input);
      localStorage.removeItem(storageKey);
      onSaved(saved);
    } catch (e) {
      setError(voiceError(e));
      if (
        e instanceof RequestError &&
        ["invalid_voice_reference", "voice_not_found", "forbidden"].includes(
          e.code,
        )
      ) {
        setRequest(null);
        localStorage.removeItem(storageKey);
      }
    } finally {
      lock.current = false;
      setBusy(false);
    }
  }
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open && !busy) onClose();
      }}
    >
      <DialogContent
        className={`max-h-[85dvh] overflow-y-auto ${voiceTouchTargets}`}
        onCloseAutoFocus={(event) => {
          event.preventDefault();
          onRestoreFocus();
        }}
      >
        <DialogTitle>创建音色</DialogTitle>
        <DialogDescription>保存参考音频，用于全能配音。</DialogDescription>
        <Input
          aria-label="音色名称"
          placeholder="例如：故事旁白"
          maxLength={60}
          value={name}
          disabled={busy || !!request}
          onChange={(e) => setName(e.target.value)}
        />
        <div className="flex gap-2">
          <Button
            variant="outline"
            disabled={busy || !!request}
            onClick={() => setPicker(true)}
          >
            从项目选择音频
          </Button>
          <Button
            variant="outline"
            disabled={busy || !!request}
            onClick={() => setUpload(true)}
          >
            上传音频
          </Button>
        </div>
        <VoiceFilters
          value={tags}
          onChange={setTags}
          disabled={busy || !!request}
        />
        <p className="text-xs text-muted-foreground">
          不超过 30 秒、10 MB。使用清晰、单人说话的片段。
        </p>
        {asset?.id && (
          <>
            <p>{asset.file_name}</p>
            <ProjectMediaPreview
              projectID={projectID}
              id={asset.id}
              kind="audio"
              large
            />
          </>
        )}
        <Label className="flex items-start gap-2 text-sm max-sm:min-h-11">
          <Checkbox
            checked={consent}
            disabled={busy || !!request}
            onCheckedChange={(v) => setConsent(v === true)}
          />
          我有权使用该声音，并同意将所选音频用于生成。
        </Label>
        {error && <p role="alert">{error}</p>}
        <Button
          disabled={busy || !name.trim() || !asset || !consent}
          onClick={() => void save()}
        >
          {busy ? "正在保存…" : request ? "恢复保存结果" : "保存音色"}
        </Button>
        <p className="text-xs text-muted-foreground">
          保存为参考音色，不代表已训练供应商音色。
        </p>
        {picker && (
          <ProjectMediaPicker
            projectID={projectID}
            kind="audio"
            onClose={() => setPicker(false)}
            onSelect={choose}
          />
        )}
        {upload && (
          <UploadDialog
            projectID={projectID}
            onClose={() => setUpload(false)}
            onUploaded={(value) => {
              if (value) choose(value);
            }}
          />
        )}
      </DialogContent>
    </Dialog>
  );
}
