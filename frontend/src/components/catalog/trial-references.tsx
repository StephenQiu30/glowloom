"use client";
import { useState } from "react";
import { ProjectMediaPicker } from "@/components/media/project-media";
import { referenceRoleLabel } from "@/components/generation/reference-source";
import { Button } from "@/components/ui/button";
export type TrialRole = {
  role: string;
  types: string[];
  min_count?: number;
  max_count?: number;
  max_ms?: number;
  max_bytes?: number;
};
export function trialRoles(
  version: API.AdminModelVersion | undefined,
  mode: string,
): TrialRole[] {
  const roles = version?.limits?.roles;
  if (!roles || typeof roles !== "object") return [];
  return Object.entries(roles).flatMap(([role, value]) => {
    if (!value || typeof value !== "object" || role.startsWith("_")) return [];
    const r = value as {
      types?: string[];
      min_count?: number;
      max_count?: number;
      for_modes?: string[];
      max_ms?: number;
      max_bytes?: number;
    };
    if (!Array.isArray(r.types) || (r.for_modes && !r.for_modes.includes(mode)))
      return [];
    return [
      {
        role,
        types: r.types,
        min_count: r.min_count,
        max_count: r.max_count,
        max_ms: r.max_ms,
        max_bytes: r.max_bytes,
      },
    ];
  });
}
export function TrialReferences({
  projectID,
  roles,
  value,
  disabled,
  onChange,
}: {
  projectID?: string;
  roles: TrialRole[];
  value: API.TrialMediaInput[];
  disabled: boolean;
  onChange: (v: API.TrialMediaInput[]) => void;
}) {
  const [picker, setPicker] = useState<TrialRole | null>(null);
  const [error, setError] = useState("");
  return (
    <div className="flex min-w-0 flex-col gap-3">
      {roles.map((role) => (
        <div className="flex min-w-0 flex-col gap-2" key={role.role}>
          <div className="flex flex-wrap items-center justify-between gap-2">
            <span className="text-sm">
              {referenceRoleLabel(role.role)}
              {role.min_count ? "（必选）" : ""}
            </span>
            <Button
              type="button"
              variant="outline"
              className="max-md:h-11"
              disabled={
                disabled ||
                !projectID ||
                value.filter((r) => r.role === role.role).length >=
                  (role.max_count ?? 1)
              }
              onClick={() => {
                setError("");
                setPicker(role);
              }}
            >
              选择素材
            </Button>
          </div>
          {value
            .filter((r) => r.role === role.role)
            .map((r) => (
              <div
                key={r.media_asset_id}
                className="flex min-w-0 items-center justify-between gap-2"
              >
                <span className="truncate text-xs text-muted-foreground">
                  已选择 {referenceRoleLabel(role.role)}
                </span>
                <Button
                  type="button"
                  variant="ghost"
                  className="max-md:h-11"
                  disabled={disabled}
                  onClick={() => onChange(value.filter((v) => v !== r))}
                >
                  移除
                </Button>
              </div>
            ))}
        </div>
      ))}
      {error ? (
        <p role="alert" className="text-xs text-destructive">
          {error}
        </p>
      ) : null}
      {picker && projectID ? (
        <ProjectMediaPicker
          key={`${projectID}:${picker.role}`}
          projectID={projectID}
          kind={picker.types[0]}
          onClose={() => setPicker(null)}
          onSelect={(asset) => {
            if (
              !asset.id ||
              asset.project_id !== projectID ||
              !picker.types.includes(asset.kind ?? "") ||
              (picker.max_ms !== undefined &&
                (asset.duration_ms ?? Infinity) > picker.max_ms) ||
              (picker.max_bytes !== undefined &&
                (asset.byte_size ?? Infinity) > picker.max_bytes)
            ) {
              setError("素材类型、时长、大小或项目归属不符合当前模型要求。");
              setPicker(null);
              return;
            }
            if (
              !value.some(
                (r) => r.role === picker.role && r.media_asset_id === asset.id,
              )
            )
              onChange([
                ...value,
                { role: picker.role, media_asset_id: asset.id },
              ]);
            setPicker(null);
          }}
        />
      ) : null}
    </div>
  );
}
