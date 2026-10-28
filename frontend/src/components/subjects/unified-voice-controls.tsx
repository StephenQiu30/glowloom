"use client";
import { useId, useState } from "react";
import Link from "next/link";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from "@/components/ui/dialog";
import { Field, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Spinner } from "@/components/ui/spinner";
import type { useSubjectVoiceJob } from "./unified-voice-job";

type Owner = ReturnType<typeof useSubjectVoiceJob>;
function VoiceSelection({ owner }: { owner: Owner }) {
  const id = useId();
  const [release, setRelease] = useState(
    owner.options?.models.length === 1
      ? owner.options.models[0].release_id
      : "",
  );
  const [voice, setVoice] = useState("");
  const [name, setName] = useState("智能配音1");
  const [text, setText] = useState("你好，很高兴与你一起创作。");
  const model = owner.options?.models.find(
    (model) => model.release_id === release,
  );
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open) owner.reset();
      }}
    >
      <DialogContent className="unified-subject-source">
        <DialogTitle>选择配音模型与音色</DialogTitle>
        <DialogDescription>
          使用已接入的豆包语音配置合成试听音频。
        </DialogDescription>
        {owner.options && owner.options.models.length > 1 ? (
          <Field>
            <FieldLabel htmlFor={`${id}-model`}>模型</FieldLabel>
            <Select
              value={release}
              onValueChange={(value) => {
                setRelease(value);
                setVoice(
                  owner.options!.models.find(
                    (model) => model.release_id === value,
                  )?.default_voice_key ?? "",
                );
              }}
            >
              <SelectTrigger id={`${id}-model`} aria-label="配音模型">
                <SelectValue placeholder="选择模型" />
              </SelectTrigger>
              <SelectContent>
                {owner.options.models.map((model) => (
                  <SelectItem key={model.release_id} value={model.release_id}>
                    {model.display_name}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </Field>
        ) : (
          <p className="text-sm">{model?.display_name}</p>
        )}
        <Field>
          <FieldLabel htmlFor={`${id}-voice`}>音色</FieldLabel>
          <Select value={voice} onValueChange={setVoice}>
            <SelectTrigger
              id={`${id}-voice`}
              aria-label="配音音色"
              disabled={!model}
            >
              <SelectValue placeholder="选择音色" />
            </SelectTrigger>
            <SelectContent>
              {model?.voices.map((voice) => (
                <SelectItem key={voice.key} value={voice.key}>
                  {voice.name}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </Field>
        <Field>
          <FieldLabel htmlFor={`${id}-name`}>音色名称</FieldLabel>
          <Input
            id={`${id}-name`}
            value={name}
            onChange={(event) => setName(event.target.value)}
            maxLength={20}
          />
        </Field>
        <Field>
          <FieldLabel htmlFor={`${id}-text`}>试听文本</FieldLabel>
          <Textarea
            id={`${id}-text`}
            value={text}
            onChange={(event) => setText(event.target.value)}
            maxLength={500}
            rows={2}
          />
        </Field>
        {owner.error ? (
          <p role="alert" className="text-sm text-destructive">
            {owner.error}
          </p>
        ) : null}
        <DialogFooter>
          <Button variant="outline" onClick={owner.reset}>
            取消
          </Button>
          <Button
            disabled={!model || !voice || !name.trim() || !text.trim()}
            onClick={() => void owner.generate(release, voice, name, text)}
          >
            生成音色
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
export function UnifiedSubjectVoiceControls({ owner }: { owner: Owner }) {
  if (owner.phase === "choosing") return <VoiceSelection owner={owner} />;
  if (owner.phase === "idle" || owner.phase === "ready") return null;
  const recovering = ["unknown", "read_error", "unrecorded"].includes(
    owner.phase,
  );
  const loading = ["options", "submitting", "waiting"].includes(owner.phase);
  return (
    <div className="flex min-w-0 flex-1 flex-col gap-2">
      {loading ? (
        <Button type="button" variant="outline" disabled className="w-40 gap-2">
          <Spinner aria-hidden="true" />
          音色加载中
        </Button>
      ) : null}
      {owner.error ? (
        <div role="alert" className="flex flex-wrap items-center gap-2 text-sm">
          <span className="text-destructive">{owner.error}</span>
          {recovering ? (
            <Button
              type="button"
              variant="ghost"
              onClick={() => void owner.recover()}
            >
              恢复读取
            </Button>
          ) : null}
          {owner.phase === "unrecorded" ? (
            <Button
              type="button"
              variant="outline"
              onClick={() => void owner.retryOriginal()}
            >
              重试原生成请求
            </Button>
          ) : null}
          {owner.phase === "import_pending" ? (
            <Button
              type="button"
              variant="outline"
              onClick={() => void owner.retryOriginal()}
            >
              重试保存音频
            </Button>
          ) : null}
          {owner.phase === "failed" ? (
            <Button
              type="button"
              variant="ghost"
              onClick={() => void owner.begin()}
            >
              重新生成
            </Button>
          ) : null}
          {owner.phase === "unavailable" ? (
            <>
              <Button
                type="button"
                variant="ghost"
                onClick={() => void owner.begin()}
              >
                重新读取配置
              </Button>
              <Button type="button" variant="link" asChild>
                <Link href="/providers">接入语音模型</Link>
              </Button>
            </>
          ) : null}
        </div>
      ) : null}
      {owner.active && owner.phase !== "options" ? (
        <p className="text-xs text-muted-foreground">
          关闭设置不会取消已提交的配音任务。
        </p>
      ) : null}
    </div>
  );
}
