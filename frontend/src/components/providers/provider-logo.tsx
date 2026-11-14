"use client";

import Image from "next/image";
import useSWR from "swr";
import { getProviderLogo } from "@/api/media";
import { useOptionalSession } from "@/components/auth/session";
import { Avatar, AvatarImage, AvatarFallback } from "@/components/ui/avatar";
import { Globe } from "lucide-react";
import { cn } from "cn";

const logos: Record<string, string> = {
  openrouter: "/brand/providers/openrouter.svg",
  openai: "/brand/providers/openai.svg",
  volcengine_ark: "/brand/providers/volcengine.svg",
  volcengine_music: "/brand/providers/volcengine.svg",
  volcengine_avatar: "/brand/providers/jimeng.svg",
  volcengine_motion: "/brand/providers/jimeng.svg",
  volcengine_speech: "/brand/providers/doubao.svg",
  byteplus: "/brand/providers/byteplus.svg",
  deepseek: "/brand/providers/deepseek.svg",
  vercel: "/brand/providers/vercel.svg",
};

function BuiltinProviderLogo({
  adapterKey,
  metadata,
  className,
  iconOnly = false,
}: {
  adapterKey?: string;
  metadata?: API.AdapterMetadata;
  className?: string;
  iconOnly?: boolean;
}) {
  const logoKey = metadata?.logo_key ?? adapterKey;
  const source =
    logoKey && Object.hasOwn(logos, logoKey) ? logos[logoKey] : undefined;
  if (!source)
    return (
      <span className="inline-flex min-w-0 items-center gap-1">
        <Globe
          aria-hidden="true"
          className={cn("size-5 shrink-0", className)}
        />
        {!iconOnly && (
          <span className="truncate">
            {metadata?.display_name ?? adapterKey ?? "其他服务"}
          </span>
        )}
      </span>
    );

  if (
    logoKey === "openrouter" ||
    logoKey === "openai" ||
    logoKey === "vercel"
  ) {
    return (
      <span
        aria-hidden="true"
        className={cn("size-5 shrink-0 bg-foreground", className)}
        style={{
          maskImage: `url('${source}')`,
          maskSize: "contain",
          maskPosition: "center",
          maskRepeat: "no-repeat",
        }}
      />
    );
  }

  return (
    <Image
      src={source}
      alt=""
      width={20}
      height={20}
      unoptimized
      className={cn("size-5 shrink-0 object-contain", className)}
    />
  );
}

type ProviderLogoProps = Parameters<typeof BuiltinProviderLogo>[0] & {
  logoAssetID?: string | null;
};

function UploadedProviderLogo({
  logoAssetID,
  ...props
}: ProviderLogoProps & { logoAssetID: string }) {
  const session = useOptionalSession();
  const { data, error } = useSWR(
    session
      ? ["provider-logo", session.actor.id, session.session_id, logoAssetID]
      : null,
    () => getProviderLogo({ id: logoAssetID }),
    {
      shouldRetryOnError: false,
      refreshInterval: (value) =>
        value?.expires_at
          ? Math.max(
              1000,
              new Date(value.expires_at).getTime() - Date.now() - 30_000,
            )
          : 0,
    },
  );
  if (!session || !data?.url || error)
    return <BuiltinProviderLogo {...props} />;
  return (
    <Avatar className={cn("size-5 shrink-0 rounded-none", props.className)}>
      <AvatarImage src={data.url} alt="" className="object-contain" />
      <AvatarFallback className="rounded-none bg-transparent">
        <BuiltinProviderLogo {...props} iconOnly />
      </AvatarFallback>
    </Avatar>
  );
}

export function ProviderLogo({ logoAssetID, ...props }: ProviderLogoProps) {
  return logoAssetID ? (
    <UploadedProviderLogo
      key={logoAssetID}
      logoAssetID={logoAssetID}
      {...props}
    />
  ) : (
    <BuiltinProviderLogo {...props} />
  );
}
