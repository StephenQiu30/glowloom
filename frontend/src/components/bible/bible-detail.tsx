"use client";
import { useState } from "react";
import dynamic from "next/dynamic";
import { Button } from "@/components/ui/button";
import {
  definitionFields,
  imageRoleLabels,
  kindLabels,
  visuallyReady,
  type BibleDetail,
  type BibleIdentity,
  type BibleVersion,
} from "./bible-model";
import type { BibleActionFrame } from "./bible-action-dialog";
import type { BibleLookFrame } from "./bible-look-dialog";
const Preview = dynamic(
  () =>
    import("./bible-reference-picker").then((module) => module.BiblePreview),
  { ssr: false },
);
export function BibleVersionBody({
  identity,
  version,
}: {
  identity: BibleIdentity;
  version: BibleVersion;
}) {
  const [preview, setPreview] = useState<{
    id: string;
    kind: "image" | "audio";
    revision: number;
  }>();
  const content =
    version.kind === "character"
      ? version.character
      : version.kind === "location"
        ? version.location
        : version.prop;
  return (
    <div className="min-w-0 space-y-4 wrap-anywhere">
      <h3 className="text-lg font-medium whitespace-pre-wrap">
        {content.name}
      </h3>
      <dl className="space-y-2">
        <dt>别名</dt>
        <dd className="whitespace-pre-wrap">
          {content.aliases?.join("\n") || "无"}
        </dd>
        <dt>说明</dt>
        <dd className="whitespace-pre-wrap">{content.description || "无"}</dd>
      </dl>
      {version.kind === "character" ? (
        <>
          <dl className="space-y-2">
            {definitionFields.map(([key, label]) => (
              <div key={key}>
                <dt className="text-muted-foreground">{label}</dt>
                <dd className="whitespace-pre-wrap">
                  {version.character.definition[key] || "未填写"}
                </dd>
              </div>
            ))}
          </dl>
          <section aria-label="完整角色造型" className="space-y-5">
            {version.character.looks.map((look) => (
              <div
                key={look.id}
                className="space-y-2 rounded-lg bg-muted/40 p-3"
              >
                <h4 className="font-medium whitespace-pre-wrap">
                  {look.name}
                  {look.default ? " · 默认造型" : ""}
                </h4>
                <p className="text-xs break-all">
                  造型身份 {look.id} ·{" "}
                  {visuallyReady(look.references ?? [])
                    ? "已有正式三视图用途"
                    : "三视图用途尚未齐全"}
                </p>
                <p className="whitespace-pre-wrap">{look.description}</p>
                {look.applies_to?.length ? (
                  <details>
                    <summary>{look.applies_to.length}个正式适用范围</summary>
                    <ul className="space-y-1 text-xs break-all">
                      {look.applies_to.map((scope) => (
                        <li
                          key={`${scope.episode_id}:${scope.scene_key ?? ""}`}
                        >
                          分集 {scope.episode_id}
                          {scope.scene_key
                            ? ` · 场景 ${scope.scene_key}`
                            : " · 整集"}
                        </li>
                      ))}
                    </ul>
                  </details>
                ) : (
                  <p>角色通用造型</p>
                )}
                <ul className="space-y-2">
                  {look.references?.map((reference) => (
                    <li key={reference.role} className="text-xs break-all">
                      <p>
                        {imageRoleLabels[reference.role]} · 原件{" "}
                        {reference.media.asset_id} · rev
                        {reference.media.revision} · SHA{" "}
                        {reference.media.sha256}
                      </p>
                      <p>
                        正式预览版本 {reference.media.rendition_id} · SHA{" "}
                        {reference.media.rendition_sha256}
                      </p>
                      <Button
                        variant="outline"
                        onClick={() =>
                          setPreview({
                            id: reference.media.asset_id,
                            kind: "image",
                            revision: reference.media.revision,
                          })
                        }
                      >
                        预览 {look.name} {imageRoleLabels[reference.role]}
                      </Button>
                    </li>
                  ))}
                </ul>
              </div>
            ))}
          </section>
          <section aria-label="当前角色声音" className="space-y-2">
            <h4 className="font-medium">声音绑定</h4>
            {version.character.voice ? (
              version.character.voice.kind === "sample" ? (
                <>
                  <p className="whitespace-pre-wrap">
                    样本 {version.character.voice.sample.name} ·{" "}
                    {version.character.voice.instructions}
                  </p>
                  <p className="text-xs break-all">
                    音频原件 {version.character.voice.sample.media.asset_id} ·
                    rev{version.character.voice.sample.media.revision} · SHA{" "}
                    {version.character.voice.sample.media.sha256}
                  </p>
                  <Button
                    variant="outline"
                    onClick={() => {
                      const voice = version.character.voice;
                      if (voice?.kind === "sample")
                        setPreview({
                          id: voice.sample.media.asset_id,
                          kind: "audio",
                          revision: voice.sample.media.revision,
                        });
                    }}
                  >
                    播放正式声音样本
                  </Button>
                </>
              ) : (
                <>
                  <p>
                    {version.character.voice.catalog.model_key} · v
                    {version.character.voice.catalog.model_version} ·{" "}
                    {version.character.voice.catalog.voice_key}
                  </p>
                  <p className="text-xs break-all">
                    模型版本 {version.character.voice.catalog.model_version_id}{" "}
                    · 安全参数SHA{" "}
                    {version.character.voice.catalog.param_schema_sha256}
                  </p>
                  <pre className="whitespace-pre-wrap">
                    {JSON.stringify(
                      version.character.voice.catalog.params,
                      null,
                      2,
                    )}
                  </pre>
                  <p className="whitespace-pre-wrap">
                    {version.character.voice.instructions}
                  </p>
                </>
              )
            ) : (
              <p>未绑定正式声音</p>
            )}
          </section>
        </>
      ) : (
        <dl>
          <dt>生成约束</dt>
          <dd className="whitespace-pre-wrap">
            {("prompt" in content && content.prompt) || "未填写"}
          </dd>
        </dl>
      )}
      <p className="text-xs break-all">
        不变版本 {version.id} · v{version.number} · SHA {version.content_sha256}{" "}
        · 创建者 {version.actor_id} · {version.created_at}
      </p>
      <p>
        来源：{version.origin === "manual" ? "手工设定" : "已采纳正式生成结果"}
      </p>
      {version.result && (
        <p className="text-xs break-all">
          原任务 {version.result.operation_id} · 原输出{" "}
          {version.result.output_id} · 输入SHA {version.result.input_sha256} ·
          输出SHA {version.result.output_sha256}
        </p>
      )}
      {preview && (
        <section aria-label="私有正式媒体预览">
          <Button variant="ghost" onClick={() => setPreview(undefined)}>
            关闭原媒体预览
          </Button>
          <Preview
            key={`${preview.id}:${preview.revision}`}
            identity={identity}
            assetId={preview.id}
            kind={preview.kind}
            revision={preview.revision}
          />
        </section>
      )}
    </div>
  );
}
export function BibleDetail({
  identity,
  detail,
  locked,
  readOnly = false,
  onEdit,
  onAction,
  onLook,
  onVoice,
  onHistory,
  onResult,
  onResolved,
}: {
  identity: BibleIdentity;
  detail: BibleDetail;
  locked: boolean;
  readOnly?: boolean;
  onEdit: () => void;
  onAction: (frame: BibleActionFrame) => void;
  onLook: (frame: BibleLookFrame) => void;
  onVoice: () => void;
  onHistory: () => void;
  onResult: () => void;
  onResolved: (id: string) => void;
}) {
  const active = !detail.head.deleted && !detail.head.redirect_id;
  return (
    <section aria-label="设定详情" className="min-w-0 space-y-4">
      <div className="flex flex-wrap gap-2">
        <Button variant="outline" disabled={locked} onClick={onHistory}>
          不可变版本历史
        </Button>
        {active ? (
          <>
            <Button disabled={locked || readOnly} onClick={onEdit}>
              编辑完整{kindLabels[detail.head.kind]}
            </Button>
            <Button
              variant="outline"
              disabled={
                locked ||
                readOnly ||
                detail.head.confirmed_version_id === detail.current.id
              }
              onClick={() => onAction({ action: "confirm", detail })}
            >
              审核并确认当前版本
            </Button>
            <Button
              variant="outline"
              disabled={locked || readOnly}
              onClick={onResult}
            >
              采纳正式生成结果
            </Button>
            <Button
              variant="outline"
              disabled={locked || readOnly}
              onClick={() => onAction({ action: "delete", detail })}
            >
              回收设定
            </Button>
          </>
        ) : detail.head.deleted ? (
          <Button
            disabled={locked || readOnly}
            onClick={() => onAction({ action: "restore", detail })}
          >
            恢复原设定身份
          </Button>
        ) : (
          <Button
            variant="outline"
            disabled={locked}
            onClick={() => onResolved(detail.resolved_id)}
          >
            查看合并后的当前身份
          </Button>
        )}
      </div>
      <p className="text-xs break-all">
        原身份 {detail.head.id} · CAS版本 {detail.head.revision} ·
        已确认不变版本 {detail.head.confirmed_version_id ?? "尚未确认"}
        {detail.head.redirect_id
          ? ` · 合并重定向 ${detail.head.redirect_id}`
          : ""}
        {detail.head.deleted ? " · 已回收" : ""}
      </p>
      <BibleVersionBody
        key={detail.current.id}
        identity={identity}
        version={detail.current}
      />
      {active && detail.current.kind === "character" && (
        <div className="space-y-4">
          <div className="flex flex-wrap gap-2">
            <Button
              variant="outline"
              disabled={locked || readOnly}
              onClick={() =>
                onLook({
                  action: "look_create",
                  id: detail.head.id,
                  revision: detail.head.revision,
                })
              }
            >
              新增角色造型
            </Button>
            <Button
              variant="outline"
              disabled={locked || readOnly}
              onClick={onVoice}
            >
              绑定角色声音
            </Button>
            <Button
              variant="outline"
              disabled={locked || readOnly || !detail.current.character.voice}
              onClick={() => onAction({ action: "voice_unbind", detail })}
            >
              解除当前声音
            </Button>
            <Button
              variant="outline"
              disabled={
                locked ||
                readOnly ||
                detail.head.confirmed_version_id !== detail.current.id
              }
              onClick={() => onAction({ action: "merge", detail })}
            >
              合并到另一确认角色
            </Button>
          </div>
          <ul className="space-y-3">
            {detail.current.character.looks.map((look) => (
              <li key={look.id} className="flex flex-wrap items-center gap-2">
                <span className="whitespace-pre-wrap">{look.name}</span>
                <Button
                  variant="outline"
                  disabled={locked || readOnly}
                  onClick={() =>
                    onLook({
                      action: "look_update",
                      id: detail.head.id,
                      revision: detail.head.revision,
                      look,
                    })
                  }
                >
                  编辑 {look.name}
                </Button>
                <Button
                  variant="outline"
                  disabled={locked || readOnly}
                  onClick={() =>
                    onLook({
                      action: "references",
                      id: detail.head.id,
                      revision: detail.head.revision,
                      look,
                    })
                  }
                >
                  参考图 {look.name}
                </Button>
                <Button
                  variant="outline"
                  disabled={locked || readOnly || look.default}
                  onClick={() =>
                    onAction({
                      action: "look_default",
                      detail,
                      lookId: look.id,
                    })
                  }
                >
                  设为默认 {look.name}
                </Button>
                <Button
                  variant="outline"
                  disabled={locked || readOnly || look.default}
                  onClick={() =>
                    onAction({ action: "look_delete", detail, lookId: look.id })
                  }
                >
                  删除造型 {look.name}
                </Button>
              </li>
            ))}
          </ul>
          <p className="text-xs text-muted-foreground">
            默认造型删除前需明确设定另一个默认；删除、合并与已确认内容修改必须读取真实下游影响。
          </p>
        </div>
      )}
    </section>
  );
}
