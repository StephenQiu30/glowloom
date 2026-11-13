import { ExternalLink } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";

export const pricingUnits: Record<string, string> = {
  per_request: "每次请求",
  per_image: "每张图片",
  per_second: "每秒",
  per_1k_tokens: "每千 Token",
  per_1k_chars: "每千字符",
};

export const billingStates: Record<string, string> = {
  billable: "可计费",
  price_pending: "待补价",
  usage_pending: "待验证或发布",
  inactive: "未开放",
};

const priceSources: Record<string, { name: string; href: string }> = {
  volcengine_ark: {
    name: "火山方舟",
    href: "https://docs.volcengine.com/docs/ark/model-pricing?lang=zh",
  },
  volcengine_speech: {
    name: "豆包语音",
    href: "https://docs.volcengine.com/docs/doubao-voice/billing-instructions?lang=zh",
  },
  deepseek: {
    name: "DeepSeek",
    href: "https://api-docs.deepseek.com/zh-cn/quick_start/pricing/",
  },
  openai: {
    name: "OpenAI",
    href: "https://developers.openai.com/api/docs/pricing",
  },
  openrouter: {
    name: "OpenRouter",
    href: "https://openrouter.ai/models",
  },
};

export function isProcurementPrice(price: API.AdminPrice) {
  return !!(
    price.id &&
    price.source_url &&
    price.reviewed_at &&
    !price.rule?.procurement_price_version_id &&
    price.rule?.discount_factor === undefined &&
    price.rule?.markup_multiplier === undefined
  );
}

export function PricingSources({ adapters }: { adapters: string[] }) {
  const links = [...new Set(adapters)].flatMap((adapter) => {
    const source = priceSources[adapter];
    return source ? [source] : [];
  });
  return (
    <Popover>
      <PopoverTrigger asChild>
        <Button size="toolbar" variant="outline">
          供应商价目
          <ExternalLink data-icon="inline-end" />
        </Button>
      </PopoverTrigger>
      <PopoverContent align="end" className="w-56 gap-1 p-2">
        {links.length ? (
          links.map(({ name, href }) => (
            <Button
              key={href}
              asChild
              variant="ghost"
              className="justify-between"
            >
              <a href={href} target="_blank" rel="noreferrer">
                {name}
                <ExternalLink data-icon="inline-end" />
              </a>
            </Button>
          ))
        ) : (
          <p className="p-2 text-sm text-muted-foreground">
            请查阅当前连接的供应商价目。
          </p>
        )}
      </PopoverContent>
    </Popover>
  );
}
