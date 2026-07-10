import { createFileRoute } from "@tanstack/react-router";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { useEffect, useState } from "react";
import { Text } from "@/components/ui/text";
import {
  Card,
  CardHeader,
  CardTitle,
  CardDescription,
  CardContent,
} from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Settings as SettingsIcon, Sparkles, Puzzle, Copy, Check, AlertTriangle, Router, KeyRound, Activity } from "lucide-react";
import {
  getExtensionToken,
  rotateExtensionToken,
  revokeExtensionToken,
  getLLMConfig,
  getLLMUsage,
  saveLLMConfig,
  saveLLMProviderKey,
  verifyLLMProvider,
  type LLMConfigResponse,
  type LLMModelTarget,
  type LLMProviderStatus,
} from "@/lib/api";

export const Route = createFileRoute("/_dashboard/settings")({
  component: SettingsPage,
});


const modelSlots = [
  { key: "cheap", label: "Cheap", description: "Filtering, classification, and reply pre-checks" },
  { key: "strong", label: "Strong", description: "Reply drafting and product analysis" },
  { key: "embedding", label: "Embedding", description: "Mention and profile semantic matching" },
];

const routeRows = [
  ["filter", "Spam filter"],
  ["classify", "Intent classification"],
  ["pre_filter_reply", "Reply pre-filter"],
  ["draft_reply", "Reply drafting"],
  ["analyze_product", "Product analysis"],
  ["embed_mentions", "Mention embeddings"],
  ["embed_profiles", "Profile embeddings"],
  ["embed_documents", "Document embeddings"],
];

const CUSTOM_MODEL = "__custom";

function cloneModels(config?: LLMConfigResponse): Record<string, LLMModelTarget> {
  return {
    cheap: { provider: config?.models?.cheap?.provider ?? "", model: config?.models?.cheap?.model ?? "" },
    strong: { provider: config?.models?.strong?.provider ?? "", model: config?.models?.strong?.model ?? "" },
    embedding: { provider: config?.models?.embedding?.provider ?? "voyage", model: config?.models?.embedding?.model ?? "voyage-3" },
  };
}

function LLMRouterCard() {
  const queryClient = useQueryClient();
  const [keyDrafts, setKeyDrafts] = useState<Record<string, string>>({});
  const [models, setModels] = useState<Record<string, LLMModelTarget>>({});
  const [routing, setRouting] = useState<Record<string, string>>({});
  const [message, setMessage] = useState<string | null>(null);

  const { data: config, isLoading } = useQuery({
    queryKey: ["llm-config"],
    queryFn: getLLMConfig,
  });
  const { data: usage } = useQuery({
    queryKey: ["llm-usage"],
    queryFn: getLLMUsage,
  });

  useEffect(() => {
    if (!config) return;
    setModels(cloneModels(config));
    setRouting(config.routing ?? {});
  }, [config]);

  const saveKey = useMutation({
    mutationFn: ({ provider, apiKey }: { provider: string; apiKey: string }) =>
      saveLLMProviderKey(provider, apiKey),
    onSuccess: (_, vars) => {
      setKeyDrafts((prev) => ({ ...prev, [vars.provider]: "" }));
      setMessage(`${vars.provider} key saved`);
      queryClient.invalidateQueries({ queryKey: ["llm-config"] });
    },
    onError: (err) => setMessage(err instanceof Error ? err.message : "Failed to save key"),
  });

  const verify = useMutation({
    mutationFn: (provider: string) => verifyLLMProvider(provider),
    onSuccess: (_, provider) => setMessage(`${provider} verified`),
    onError: (err) => setMessage(err instanceof Error ? err.message : "Verification failed"),
  });

  const saveRoutes = useMutation({
    mutationFn: () => saveLLMConfig({ models, routing }),
    onSuccess: () => {
      setMessage("AI Router config saved");
      queryClient.invalidateQueries({ queryKey: ["llm-config"] });
    },
    onError: (err) => setMessage(err instanceof Error ? err.message : "Failed to save routing"),
  });

  const providers: LLMProviderStatus[] = config?.providers ?? [];
  const providerByID = new Map(providers.map((provider) => [provider.provider, provider]));
  const chatProviders = providers.filter((p) => p.capabilities.includes("chat"));
  const embeddingProviders = providers.filter((p) => p.capabilities.includes("embedding"));
  const defaultModelFor = (provider: string) => providerByID.get(provider)?.default_model ?? "";
  const modelOptionsFor = (provider: string) => providerByID.get(provider)?.recommended_models ?? [];

  function updateModel(slot: string, patch: Partial<LLMModelTarget>) {
    setModels((prev) => {
      const next = { ...(prev[slot] ?? { provider: "", model: "" }), ...patch };
      if (patch.provider && !next.model) next.model = defaultModelFor(patch.provider);
      return { ...prev, [slot]: next };
    });
  }

  return (
    <Card>
      <CardHeader>
        <div className="flex items-center gap-3">
          <div className="w-10 h-10 rounded bg-primary border-2 border-border shadow-xs flex items-center justify-center">
            <Router className="h-5 w-5 text-primary-foreground" />
          </div>
          <div>
            <CardTitle>AI Router</CardTitle>
            <CardDescription>
              Connect BYOK providers, choose cheap/strong/embedding models, and verify usage.
            </CardDescription>
          </div>
        </div>
      </CardHeader>
      <CardContent className="space-y-6">
        <div className="flex flex-wrap items-center gap-2 p-3 rounded border-2 border-border bg-background">
          <Badge variant={config?.health.ready ? "surface" : "outline"} size="sm">
            {isLoading ? "Loading" : config?.health.ready ? "Ready" : "Needs setup"}
          </Badge>
          <Text as="p" className="text-sm text-muted-foreground">
            Chat {config?.health.chat_ok ? "OK" : "missing"} · Strong {config?.health.strong_ok ? "OK" : "missing"} · Embedding {config?.health.embed_ok ? "OK" : "missing"}
          </Text>
          {message && <Text as="p" className="text-sm text-primary-ink">{message}</Text>}
        </div>

        <div className="space-y-3">
          <div className="flex items-center gap-2">
            <KeyRound className="h-4 w-4" />
            <Text as="p" className="font-medium">Providers</Text>
          </div>
          <div className="grid gap-3 md:grid-cols-2">
            {providers.map((provider) => (
              <div key={provider.provider} className="rounded border-2 border-border bg-background p-3 space-y-3">
                <div className="flex items-center justify-between gap-2">
                  <div>
                    <Text as="p" className="font-medium">{provider.display_name}</Text>
                    <Text as="p" className="text-xs text-muted-foreground">
                      {provider.is_set ? `${provider.masked_key} · ${provider.key_source}` : provider.capabilities.join(", ")}
                    </Text>
                  </div>
                  <Badge variant={provider.is_set ? "surface" : "outline"} size="sm">
                    {provider.is_set ? "Configured" : "No key"}
                  </Badge>
                </div>
                <div className="flex gap-2">
                  <input
                    type="password"
                    placeholder={`Paste ${provider.provider} key`}
                    value={keyDrafts[provider.provider] ?? ""}
                    onChange={(e) => setKeyDrafts((prev) => ({ ...prev, [provider.provider]: e.target.value }))}
                    className="flex-1 px-2 py-1.5 text-sm rounded border-2 border-border bg-background text-foreground"
                  />
                  <button
                    onClick={() => saveKey.mutate({ provider: provider.provider, apiKey: keyDrafts[provider.provider] ?? "" })}
                    disabled={!keyDrafts[provider.provider] || saveKey.isPending}
                    className="px-3 py-1.5 text-sm font-medium rounded border-2 border-border bg-background hover:bg-accent disabled:opacity-50"
                  >
                    Save
                  </button>
                  <button
                    onClick={() => verify.mutate(provider.provider)}
                    disabled={!provider.is_set || verify.isPending}
                    className="px-3 py-1.5 text-sm font-medium rounded border-2 border-border bg-background hover:bg-accent disabled:opacity-50"
                  >
                    Verify
                  </button>
                </div>
              </div>
            ))}
          </div>
        </div>

        <div className="space-y-3">
          <Text as="p" className="font-medium">Model slots</Text>
          <div className="grid gap-3 md:grid-cols-3">
            {modelSlots.map((slot) => {
              const options = slot.key === "embedding" ? embeddingProviders : chatProviders;
              const selectedProvider = models[slot.key]?.provider ?? "";
              const selectedModel = models[slot.key]?.model ?? "";
              const modelOptions = modelOptionsFor(selectedProvider);
              const isCustomModel = !!selectedModel && !modelOptions.includes(selectedModel);
              return (
                <div key={slot.key} className="rounded border-2 border-border bg-background p-3 space-y-3">
                  <div>
                    <Text as="p" className="font-medium">{slot.label}</Text>
                    <Text as="p" className="text-xs text-muted-foreground">{slot.description}</Text>
                  </div>

                  <label className="space-y-1 block">
                    <span className="text-xs font-medium text-muted-foreground">Provider</span>
                    <select
                      value={selectedProvider}
                      onChange={(e) => updateModel(slot.key, { provider: e.target.value, model: defaultModelFor(e.target.value) })}
                      className="w-full px-2 py-1.5 text-sm rounded border-2 border-border bg-background"
                    >
                      <option value="">Auto choose configured provider</option>
                      {options.map((p) => <option key={p.provider} value={p.provider}>{p.display_name}</option>)}
                    </select>
                  </label>

                  <label className="space-y-1 block">
                    <span className="text-xs font-medium text-muted-foreground">Model</span>
                    <select
                      value={!selectedProvider ? "" : isCustomModel ? CUSTOM_MODEL : selectedModel}
                      disabled={!selectedProvider}
                      onChange={(e) => updateModel(slot.key, { model: e.target.value === CUSTOM_MODEL ? "" : e.target.value })}
                      className="w-full px-2 py-1.5 text-sm rounded border-2 border-border bg-background disabled:opacity-50"
                    >
                      <option value="">Select a model</option>
                      {modelOptions.map((model) => <option key={model} value={model}>{model}</option>)}
                      {selectedProvider && <option value={CUSTOM_MODEL}>Custom model...</option>}
                    </select>
                  </label>

                  {selectedProvider && (!modelOptions.length || isCustomModel || selectedModel === "") && (
                    <input
                      value={selectedModel}
                      onChange={(e) => updateModel(slot.key, { model: e.target.value })}
                      placeholder="Enter custom model id"
                      className="w-full px-2 py-1.5 text-sm rounded border-2 border-border bg-background"
                    />
                  )}
                </div>
              );
            })}
          </div>
        </div>

        <div className="space-y-3">
          <Text as="p" className="font-medium">Task routing</Text>
          <div className="rounded border-2 border-border overflow-hidden">
            {routeRows.map(([task, label]) => (
              <div key={task} className="flex items-center justify-between gap-3 p-3 border-b last:border-b-0 border-border bg-background">
                <Text as="p" className="text-sm">{label}</Text>
                <select
                  value={routing[task] ?? (task.startsWith("embed") ? "embedding" : "cheap")}
                  onChange={(e) => setRouting((prev) => ({ ...prev, [task]: e.target.value }))}
                  className="px-2 py-1.5 text-sm rounded border-2 border-border bg-background"
                >
                  <option value="cheap">Cheap</option>
                  <option value="strong">Strong</option>
                  <option value="embedding">Embedding</option>
                </select>
              </div>
            ))}
          </div>
          <button
            onClick={() => saveRoutes.mutate()}
            disabled={saveRoutes.isPending}
            className="px-4 py-2 text-sm font-medium rounded border-2 border-border bg-primary text-primary-foreground hover:bg-primary-hover disabled:opacity-50"
          >
            {saveRoutes.isPending ? "Saving..." : "Save routing"}
          </button>
        </div>

        <div className="space-y-3">
          <div className="flex items-center gap-2">
            <Activity className="h-4 w-4" />
            <Text as="p" className="font-medium">30-day usage</Text>
          </div>
          <div className="rounded border-2 border-border overflow-hidden">
            {(usage ?? []).length === 0 ? (
              <div className="p-3 bg-background text-sm text-muted-foreground">No LLM calls recorded yet.</div>
            ) : usage?.slice(0, 6).map((row) => (
              <div key={`${row.task}-${row.provider}-${row.model}`} className="grid grid-cols-4 gap-2 p-3 border-b last:border-b-0 border-border bg-background text-sm">
                <span>{row.task}</span>
                <span>{row.provider}</span>
                <span className="truncate">{row.model}</span>
                <span>{row.calls} calls · {row.errors} errors</span>
              </div>
            ))}
          </div>
        </div>
      </CardContent>
    </Card>
  );
}

function ChromeExtensionCard() {
  const queryClient = useQueryClient();
  const [newToken, setNewToken] = useState<string | null>(null);
  const [copied, setCopied] = useState(false);

  const { data: tokenInfo, isLoading } = useQuery({
    queryKey: ["extension-token"],
    queryFn: getExtensionToken,
  });

  const rotate = useMutation({
    mutationFn: () => rotateExtensionToken(),
    onSuccess: (data) => {
      setNewToken(data.token);
      queryClient.invalidateQueries({ queryKey: ["extension-token"] });
    },
  });

  const revoke = useMutation({
    mutationFn: revokeExtensionToken,
    onSuccess: () => {
      setNewToken(null);
      queryClient.invalidateQueries({ queryKey: ["extension-token"] });
    },
  });

  function copyToken() {
    if (!newToken) return;
    navigator.clipboard.writeText(newToken).then(() => {
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    });
  }

  const hasToken = tokenInfo?.has_token;

  return (
    <Card>
      <CardHeader>
        <div className="flex items-center gap-3">
          <div className="w-10 h-10 rounded bg-primary border-2 border-border shadow-xs flex items-center justify-center">
            <Puzzle className="h-5 w-5 text-primary-foreground" />
          </div>
          <div>
            <CardTitle>Chrome Extension</CardTitle>
            <CardDescription>
              Passively capture signals while you browse Reddit, X, LinkedIn, and HN.
            </CardDescription>
          </div>
        </div>
      </CardHeader>
      <CardContent className="space-y-4">
        {/* Token status */}
        <div className="flex items-center justify-between p-3 rounded border-2 border-border bg-background">
          <div className="flex items-center gap-2">
            <Badge variant={hasToken ? "surface" : "outline"} size="sm">
              {isLoading ? "Loading..." : hasToken ? "Active" : "No key"}
            </Badge>
            {hasToken && tokenInfo?.masked_token && (
              <Text as="p" className="text-sm font-mono text-muted-foreground">
                {tokenInfo.masked_token}
              </Text>
            )}
            {hasToken && tokenInfo?.last_used_at && (
              <Text as="p" className="text-xs text-muted-foreground">
                Last used {new Date(tokenInfo.last_used_at).toLocaleDateString()}
              </Text>
            )}
          </div>
          <div className="flex items-center gap-2">
            <button
              onClick={() => rotate.mutate()}
              disabled={rotate.isPending}
              className="px-3 py-1.5 text-sm font-medium rounded border-2 border-border bg-background hover:bg-accent transition-colors disabled:opacity-50"
            >
              {rotate.isPending ? "Generating..." : hasToken ? "Rotate key" : "Generate key"}
            </button>
            {hasToken && (
              <button
                onClick={() => revoke.mutate()}
                disabled={revoke.isPending}
                className="px-3 py-1.5 text-sm font-medium rounded border-2 border-border bg-background text-destructive hover:bg-destructive/10 transition-colors disabled:opacity-50"
              >
                Revoke
              </button>
            )}
          </div>
        </div>

        {/* New token reveal — shown once after generation */}
        {newToken && (
          <div className="p-3 rounded border-2 border-amber-500/50 bg-amber-500/5 space-y-2">
            <div className="flex items-center gap-2 text-amber-600 dark:text-amber-400">
              <AlertTriangle className="h-4 w-4 shrink-0" />
              <Text as="p" className="text-sm font-medium">
                Copy this key now — it won't be shown again.
              </Text>
            </div>
            <div className="flex items-center gap-2">
              <input
                readOnly
                value={newToken}
                className="flex-1 px-2 py-1.5 text-sm font-mono rounded border-2 border-border bg-background text-foreground"
              />
              <button
                onClick={copyToken}
                className="flex items-center gap-1.5 px-3 py-1.5 text-sm font-medium rounded border-2 border-border bg-background hover:bg-accent transition-colors"
              >
                {copied ? <Check className="h-4 w-4 text-green-500" /> : <Copy className="h-4 w-4" />}
                {copied ? "Copied" : "Copy"}
              </button>
            </div>
          </div>
        )}

        {/* Setup instructions */}
        <div className="space-y-2 pt-1">
          <Text as="p" className="text-sm font-medium">Setup</Text>
          <ol className="space-y-1 text-sm text-muted-foreground list-decimal list-inside">
            <li>Install the LeadEcho extension from the Chrome Web Store</li>
            <li>Click the extension icon → enter your backend URL</li>
            <li>Generate a key above and paste it into the extension popup</li>
            <li>Browse Reddit, X, LinkedIn, or HN — signals are captured automatically</li>
          </ol>
        </div>
      </CardContent>
    </Card>
  );
}

function SettingsPage() {
  return (
    <div className="space-y-6 max-w-3xl">
      <div>
        <Text as="h2">Settings</Text>
        <Text as="p" className="text-muted-foreground mt-1">
          Manage your account configuration.
        </Text>
      </div>

      <Card>
        <CardHeader>
          <div className="flex items-center gap-3">
            <div className="w-10 h-10 rounded bg-primary border-2 border-border shadow-xs flex items-center justify-center">
              <Sparkles className="h-5 w-5 text-primary-foreground" />
            </div>
            <div>
              <CardTitle>AI Features</CardTitle>
              <CardDescription>
                Intent classification and reply drafting are included for all
                users.
              </CardDescription>
            </div>
          </div>
        </CardHeader>
        <CardContent>
          <div className="flex items-center gap-2 p-3 rounded border-2 border-border bg-background">
            <Badge variant="surface" size="sm">Included</Badge>
            <Text as="p" className="text-sm text-muted-foreground">
              AI-powered classify and draft reply are available on every mention in your Inbox.
            </Text>
          </div>
        </CardContent>
      </Card>

      <LLMRouterCard />

      <ChromeExtensionCard />

      <Card>
        <CardHeader>
          <div className="flex items-center gap-3">
            <div className="w-10 h-10 rounded bg-primary border-2 border-border shadow-xs flex items-center justify-center">
              <SettingsIcon className="h-5 w-5 text-primary-foreground" />
            </div>
            <div>
              <CardTitle>Account</CardTitle>
              <CardDescription>
                Account settings and billing.
              </CardDescription>
            </div>
          </div>
        </CardHeader>
        <CardContent className="p-8 text-center">
          <Text as="p" className="text-muted-foreground">
            Billing, plan upgrades, and custom API keys coming in Pro plan.
          </Text>
        </CardContent>
      </Card>
    </div>
  );
}
