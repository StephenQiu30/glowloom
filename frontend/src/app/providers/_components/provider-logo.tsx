import Image from "next/image";
import { Globe } from "lucide-react";

const logos: Record<string, string> = {
  openrouter: "/brand/providers/openrouter.svg",
  openai: "/brand/providers/openai.svg",
  volcengine_ark: "/brand/providers/volcengine.svg",
  volcengine_music: "/brand/providers/volcengine.svg",
  volcengine_avatar: "/brand/providers/jimeng.svg",
  volcengine_motion: "/brand/providers/jimeng.svg",
  volcengine_speech: "/brand/providers/doubao.svg",
  byteplus: "/brand/providers/byteplus.png",
  deepseek: "/brand/providers/deepseek.svg",
  vercel: "/brand/providers/vercel.svg",
};

export function ProviderLogo({
  adapterKey,
  metadata,
}: {
  adapterKey?: string;
  metadata?: API.AdapterMetadata;
}) {
  const logoKey = metadata?.logo_key ?? adapterKey;
  const source =
    logoKey && Object.hasOwn(logos, logoKey) ? logos[logoKey] : undefined;
  if (!source)
    return (
      <span className="inline-flex min-w-0 items-center gap-1">
        <Globe aria-hidden="true" className="size-5 shrink-0" />
        <span className="truncate">
          {metadata?.display_name ?? adapterKey ?? "其他服务"}
        </span>
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
        className="size-5 shrink-0 bg-foreground"
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
      className="size-5 shrink-0 object-contain"
    />
  );
}
