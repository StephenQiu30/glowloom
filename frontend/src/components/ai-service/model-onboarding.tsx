"use client";

import { useRef, useState, type ReactNode } from "react";
import { ArrowLeft, Settings2 } from "lucide-react";
import { ensureIndependentCreation } from "@/api/projects";
import { ProviderLogo } from "@/components/providers/provider-logo";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import {
  prepareQuickOnboarding,
  type AiServiceConfiguration,
  type OnboardingReview,
} from "@/lib/ai-service-admin";
import { SetupSelectField, type RunMutation } from "./admin-fields";
import { ModelSettings, RouteForm } from "./admin-forms";
import { OnboardingPanel } from "./onboarding-verification";
import {
  SetupModelCatalog,
  setupProviders,
  type SetupModel,
} from "./onboarding-catalog";

export type ModelOnboarding = { existing?: boolean; modelID?: string };

const customProvider = "__custom";

export function ModelOnboardingDialog({
  setup,
  configuration,
  scope,
  busy,
  error,
  run,
  refresh,
  allowExistingModel,
  onDirty,
  onChangeRequest,
  onClose,
  restoreFocus,
}: {
  setup: ModelOnboarding;
  configuration: AiServiceConfiguration;
  scope?: string;
  busy: boolean;
  error: ReactNode;
  run: RunMutation;
  refresh: () => void;
  allowExistingModel: boolean;
  onDirty: (dirty: boolean) => void;
  onChangeRequest: (action: () => void) => void;
  onClose: () => void;
  restoreFocus: () => void;
}) {
  const providers = setupProviders(configuration, scope);
  const [providerID, setProviderID] = useState(
    providers[0]?.id ?? customProvider,
  );
  const [existing, setExisting] = useState(Boolean(setup.existing));
  const [modelID, setModelID] = useState(setup.modelID);
  const [selected, setSelected] = useState<SetupModel>();
  const [candidate, setCandidate] = useState<{
    id: string;
    initial?: OnboardingReview;
  }>();
  const provider = providers.find(({ id }) => id === providerID);
  const activeProvider = provider?.id ?? customProvider;
  const model = configuration.models.find(
    ({ model }) => model.id === modelID,
  )?.model;
  const modelSearch = useRef<HTMLInputElement>(null);
  const modelHeading = useRef<HTMLHeadingElement>(null);
  const routeHeading = useRef<HTMLHeadingElement>(null);
  const verificationHeading = useRef<HTMLHeadingElement>(null);
  const catalogLocked = busy || Boolean(modelID);

  function showCandidate(id: string, initial?: OnboardingReview) {
    onDirty(false);
    setCandidate({ id, initial });
    requestAnimationFrame(() => verificationHeading.current?.focus());
  }

  function resetSelection() {
    setSelected(undefined);
    setCandidate(undefined);
  }

  function chooseProvider(id: string) {
    if (id === activeProvider || catalogLocked) return;
    onChangeRequest(() => {
      resetSelection();
      setProviderID(id);
    });
  }

  function preparePreset(choice: SetupModel) {
    if (!choice.preset?.quick_onboarding) return;
    const key = choice.preset.key;
    void run(async () => {
      if (!scope) throw new Error("请先选择可写的验证空间。");
      const project = await ensureIndependentCreation();
      if (!project.id) throw new Error("当前空间的独立创作容器不可用。");
      const review = await prepareQuickOnboarding({
        preset_key: key,
        scope_id: scope,
        project_id: project.id,
      });
      showCandidate(review.id, review);
    }, "");
  }

  function chooseModel(choice: SetupModel) {
    if (catalogLocked || choice.id === selected?.id) return;
    onChangeRequest(() => {
      setSelected(choice);
      setCandidate(undefined);
      if (choice.candidateID) {
        showCandidate(choice.candidateID);
      } else if (choice.preset?.quick_onboarding) {
        preparePreset(choice);
      } else {
        requestAnimationFrame(() => modelHeading.current?.focus());
      }
    });
  }

  const needsModel =
    !existing &&
    (!provider ||
      (selected?.preset &&
        !selected.preset.quick_onboarding &&
        !selected.candidateID));
  function returnToCatalog() {
    onChangeRequest(() => {
      resetSelection();
      requestAnimationFrame(() => modelSearch.current?.focus());
    });
  }

  const configurationPanel = (
    <>
      {error}
      {needsModel && (
        <section
          aria-labelledby="gateway-setup-model"
          className="flex flex-col gap-4"
          onChangeCapture={() => onDirty(true)}
        >
          <h3
            id="gateway-setup-model"
            ref={modelHeading}
            tabIndex={-1}
            className="text-base font-semibold"
          >
            {selected ? "销售配置" : "自定义模型"}
          </h3>
          {modelID ? (
            <p role="status" className="text-sm text-muted-foreground">
              {model ? "销售模型草稿已保存。" : "正在读取模型草稿…"}
            </p>
          ) : (
            <ModelSettings
              key={selected?.id ?? customProvider}
              preset={selected?.preset}
              driver={configuration.media_drivers.find(
                ({ key }) => key === selected?.preset?.adapter,
              )}
              busy={busy}
              run={run}
              onPrepared={(id) => {
                onDirty(false);
                setModelID(id);
                requestAnimationFrame(() => routeHeading.current?.focus());
              }}
            />
          )}
        </section>
      )}
      {!candidate && (existing || modelID) && (
        <section
          aria-labelledby="gateway-setup-route"
          className="flex flex-col gap-4"
          onChangeCapture={() => onDirty(true)}
        >
          <h3
            id="gateway-setup-route"
            ref={routeHeading}
            tabIndex={-1}
            className="text-base font-semibold"
          >
            供应商渠道
          </h3>
          {setup.existing && model && (
            <p className="text-sm text-muted-foreground">{model.name}</p>
          )}
          {(existing || model) && (
            <RouteForm
              configuration={configuration}
              busy={busy}
              run={run}
              initialModelID={modelID}
              fixedModel={Boolean(modelID)}
              defaultScope={scope}
              onPrepared={(review) => showCandidate(review.id, review)}
            />
          )}
        </section>
      )}
      {selected?.preset?.quick_onboarding &&
        !candidate &&
        (busy ? (
          <p role="status" className="text-sm text-muted-foreground">
            正在读取验证预算…
          </p>
        ) : (
          <Button
            type="button"
            variant="outline"
            className="w-fit"
            onClick={() => preparePreset(selected)}
          >
            重新读取验证预算
          </Button>
        ))}
      {candidate && (
        <OnboardingPanel
          key={candidate.id}
          id={candidate.id}
          initial={candidate.initial}
          models={configuration.models}
          headingRef={verificationHeading}
          run={run}
          busy={busy}
          refreshConfiguration={refresh}
          onKeyDirty={onDirty}
          retryRoute={(id) => {
            onDirty(false);
            resetSelection();
            setExisting(true);
            setModelID(id);
            requestAnimationFrame(() => routeHeading.current?.focus());
          }}
        />
      )}
    </>
  );

  return (
    <Dialog open onOpenChange={(open) => !open && onClose()}>
      <DialogContent
        className="flex max-h-[calc(100dvh-1rem)] flex-col gap-0 overflow-hidden p-0 motion-reduce:animate-none max-sm:max-w-[calc(100%-1rem)] sm:max-h-[calc(100dvh-3rem)] sm:max-w-4xl max-md:[&>[data-slot=dialog-close]]:size-11"
        overlayClassName="motion-reduce:animate-none"
        onCloseAutoFocus={(event) => {
          event.preventDefault();
          restoreFocus();
        }}
      >
        <DialogHeader className="gap-1 px-5 pt-5 pr-14 pb-3 text-left sm:px-6 sm:pr-14">
          <DialogTitle className="text-lg">
            {setup.existing ? "配置渠道" : "新增接入"}
          </DialogTitle>
          <DialogDescription className="sr-only">
            选择供应商和型号，配置价格并验证接入。
          </DialogDescription>
          {allowExistingModel && !setup.existing && !modelID && !candidate && (
            <Button
              type="button"
              variant="ghost"
              size="sm"
              className="mt-2 w-fit max-md:min-h-11"
              disabled={busy}
              onClick={() =>
                onChangeRequest(() => {
                  resetSelection();
                  setExisting(!existing);
                })
              }
            >
              {existing ? "配置新模型" : "使用已有模型"}
            </Button>
          )}
        </DialogHeader>

        {existing ? (
          <div className="flex min-h-0 min-w-0 flex-col gap-5 overflow-y-auto overscroll-contain px-5 py-5 sm:px-6">
            {configurationPanel}
          </div>
        ) : (
          <Tabs
            value={activeProvider}
            onValueChange={chooseProvider}
            orientation="vertical"
            className="grid min-h-0 min-w-0 gap-0 overflow-y-auto md:h-[min(31rem,75dvh)] md:grid-cols-[12rem_minmax(0,1fr)] md:overflow-hidden"
          >
            <div className="min-h-0 min-w-0 max-md:hidden md:overflow-y-auto md:overscroll-contain md:bg-muted/25 md:px-3 md:py-3">
              <h3 className="px-3 pb-2 text-sm font-medium text-muted-foreground">
                供应商
              </h3>
              <TabsList
                aria-label="供应商"
                variant="notification"
                className="h-auto w-full flex-col gap-1 bg-transparent p-0"
              >
                {providers.map(({ id, name }) => (
                  <TabsTrigger
                    key={id}
                    value={id}
                    disabled={catalogLocked}
                    className="min-h-11 px-3"
                  >
                    <ProviderLogo adapterKey={id} iconOnly />
                    <span className="truncate">{name}</span>
                  </TabsTrigger>
                ))}
                <TabsTrigger
                  value={customProvider}
                  disabled={catalogLocked}
                  className="min-h-11 px-3"
                >
                  <Settings2 aria-hidden="true" />
                  <span>自定义接入</span>
                </TabsTrigger>
              </TabsList>
            </div>
            {[...providers.map(({ id }) => id), customProvider].map((id) => (
              <TabsContent
                key={id}
                value={id}
                className="min-h-0 min-w-0 px-5 pt-3 pb-5 focus-visible:ring-1 focus-visible:ring-ring focus-visible:ring-inset md:overflow-y-auto md:overscroll-contain md:px-6"
              >
                {id === activeProvider && (
                  <div className="flex min-w-0 flex-col gap-5">
                    <div className="md:hidden">
                      <SetupSelectField
                        label="供应商"
                        value={activeProvider}
                        disabled={catalogLocked}
                        onValueChange={chooseProvider}
                        options={[
                          ...providers.map(
                            ({ id, name }) => [id, name] as const,
                          ),
                          [customProvider, "自定义接入"],
                        ]}
                      />
                    </div>
                    {provider && !selected && !modelID && (
                      <SetupModelCatalog
                        key={provider.id}
                        provider={provider}
                        searchRef={modelSearch}
                        disabled={catalogLocked}
                        onSelect={chooseModel}
                      />
                    )}
                    {selected && (
                      <div className="flex min-w-0 items-start justify-between gap-2">
                        <div className="min-w-0">
                          <h3 className="text-lg font-semibold wrap-anywhere">
                            {selected.title}
                          </h3>
                          {selected.title !== selected.upstream && (
                            <p className="text-xs wrap-anywhere text-muted-foreground">
                              {selected.upstream}
                            </p>
                          )}
                        </div>
                        {!modelID && (
                          <Button
                            type="button"
                            variant="ghost"
                            size="sm"
                            disabled={busy}
                            className="shrink-0 max-md:min-h-11"
                            onClick={returnToCatalog}
                          >
                            <ArrowLeft data-icon="inline-start" />
                            更换型号
                          </Button>
                        )}
                      </div>
                    )}
                    {configurationPanel}
                  </div>
                )}
              </TabsContent>
            ))}
          </Tabs>
        )}
      </DialogContent>
    </Dialog>
  );
}
