import { ChevronRight, Search } from "lucide-react";
import { ProviderLogo } from "@/components/providers/provider-logo";
import { Badge } from "@/components/ui/badge";
import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
} from "@/components/ui/command";
import {
  Empty,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from "@/components/ui/empty";
import {
  modelPreset,
  type AiServiceConfiguration,
  type GatewayPreset,
} from "@/lib/ai-service-admin";

export type SetupModel = {
  id: string;
  title: string;
  upstream: string;
  preset?: GatewayPreset;
  candidateID?: string;
};

export type SetupProvider = {
  id: string;
  name: string;
  models: SetupModel[];
};

const preferredProviders = [
  "volcengine_ark",
  "deepseek",
  "openai",
  "openrouter",
];

export function setupProviders(
  configuration: AiServiceConfiguration,
  scope?: string,
) {
  const presets = configuration.model_presets ?? [];
  const providers = new Map<string, SetupProvider>();
  function add(adapter: string, name: string, model: SetupModel) {
    const provider = providers.get(adapter) ?? {
      id: adapter,
      name,
      models: [],
    };
    provider.models.push(model);
    providers.set(adapter, provider);
  }

  const preparedPresetKeys = new Set<string>();
  for (const review of configuration.onboarding) {
    if (
      review.state !== "prepared" ||
      (scope && review.scope_id && review.scope_id !== scope)
    ) {
      continue;
    }
    const model = configuration.models.find(
      ({ model }) => model.id === review.model_id,
    )?.model;
    const preset =
      (model &&
        modelPreset(
          presets,
          model,
          review.provider_key,
          review.upstream_model,
        )) ??
      presets.find(
        ({ adapter, upstream }) =>
          adapter === review.provider_key && upstream === review.upstream_model,
      );
    const adapter = preset?.adapter ?? review.provider_key ?? "other";
    if (preset) preparedPresetKeys.add(preset.key);

    add(
      adapter,
      preset?.provider ??
        configuration.media_drivers.find(({ key }) => key === adapter)?.name ??
        review.provider_key ??
        "其他供应商",
      {
        id: review.id,
        title:
          model?.name && model.name !== review.upstream_model
            ? model.name
            : (preset?.name ?? review.upstream_model),
        upstream: review.upstream_model,
        preset,
        candidateID: review.id,
      },
    );
  }

  for (const preset of presets) {
    if (!preparedPresetKeys.has(preset.key)) {
      add(preset.adapter, preset.provider, {
        id: preset.key,
        title: preset.name,
        upstream: preset.upstream,
        preset,
      });
    }
  }

  const rank = (id: string) => {
    const index = preferredProviders.indexOf(id);
    return index === -1 ? preferredProviders.length : index;
  };
  return [...providers.values()].sort(
    (a, b) => rank(a.id) - rank(b.id) || a.name.localeCompare(b.name),
  );
}

export function SetupModelCatalog({
  provider,
  selected,
  disabled,
  onSelect,
}: {
  provider: SetupProvider;
  selected?: string;
  disabled: boolean;
  onSelect: (model: SetupModel) => void;
}) {
  return (
    <section
      aria-label={`${provider.name} 的模型`}
      className="flex min-w-0 flex-col gap-4"
    >
      <div className="flex min-w-0 items-center gap-3">
        <span className="flex size-10 shrink-0 items-center justify-center rounded-lg bg-muted">
          <ProviderLogo adapterKey={provider.id} iconOnly />
        </span>
        <div className="min-w-0">
          <h3 className="text-base font-semibold">{provider.name}</h3>
          <p className="text-sm text-muted-foreground">
            {provider.models.length} 个型号
          </p>
        </div>
      </div>

      <Command
        label="搜索模型"
        className="min-w-0 gap-2 p-0 max-md:[&_[data-slot=input-group]]:h-11!"
      >
        <CommandInput
          placeholder="搜索模型名称或上游型号"
          className="max-md:text-base"
          disabled={disabled}
        />
        <CommandList
          label="模型列表"
          className="max-h-[min(44dvh,26rem)] overscroll-contain"
        >
          <CommandEmpty className="p-0">
            <Empty className="py-8">
              <EmptyHeader>
                <EmptyMedia variant="icon">
                  <Search aria-hidden="true" />
                </EmptyMedia>
                <EmptyTitle>没有匹配的模型</EmptyTitle>
              </EmptyHeader>
            </Empty>
          </CommandEmpty>
          <CommandGroup>
            {provider.models.map((model) => (
              <CommandItem
                key={model.id}
                value={model.id}
                keywords={[model.title, model.upstream]}
                disabled={disabled}
                data-checked={selected === model.id}
                onSelect={() => onSelect(model)}
                className="min-h-12 cursor-pointer gap-3 px-3 py-2"
              >
                <span className="flex min-w-0 flex-1 flex-col gap-0.5">
                  <span className="truncate font-medium">{model.title}</span>
                  {model.title !== model.upstream && (
                    <span className="truncate text-xs text-muted-foreground">
                      {model.upstream}
                    </span>
                  )}
                </span>
                {model.candidateID ? (
                  <Badge variant="secondary">待验证</Badge>
                ) : model.preset?.quick_onboarding ? (
                  <Badge variant="outline">快速接入</Badge>
                ) : null}
                {selected !== model.id && <ChevronRight aria-hidden="true" />}
              </CommandItem>
            ))}
          </CommandGroup>
        </CommandList>
      </Command>
    </section>
  );
}
