import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Button } from "@/components/ui/button";
import { getLLMConfig, saveLLMProviderKey, verifyLLMProvider } from "@/lib/api";
import type { LLMConfigResponse } from "@/lib/api";

/** Inline AI provider key form. Optional: onboarding works keyless, but
 *  the analysis and listeners are much better with a model. */
export function AIKeySetup() {
  const { data: llmConfig, refetch } = useQuery<LLMConfigResponse>({
    queryKey: ["llm-config"],
    queryFn: getLLMConfig,
    retry: false,
  });
  const [open, setOpen] = useState(false);
  const [provider, setProvider] = useState("openai");
  const [key, setKey] = useState("");
  const [message, setMessage] = useState("");
  const [busy, setBusy] = useState(false);

  if (llmConfig?.health?.chat_ok) {
    return (
      <div className="mb-4 p-2 rounded-lg border border-border bg-muted text-xs text-muted-foreground">
        ✓ AI provider configured: analysis and listener drafting are ready.
      </div>
    );
  }

  const chatProviders = (llmConfig?.providers ?? []).filter((p) => p.capabilities.includes("chat"));

  async function save() {
    if (!key.trim()) return;
    setBusy(true);
    setMessage("");
    try {
      await saveLLMProviderKey(provider, key.trim());
      try {
        await verifyLLMProvider(provider);
        setMessage(`${provider} key verified.`);
        setKey("");
        setOpen(false);
      } catch {
        setMessage("Key saved but verification failed: it may be invalid or out of credits.");
      }
      void refetch();
    } catch (err) {
      setMessage(err instanceof Error ? err.message : "Failed to save key");
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="mb-5 p-3 border border-dashed border-border rounded-lg space-y-2">
      <p className="text-sm text-muted-foreground">
        No AI provider yet. We'll still draft your profile from your site, but a key gives much better
        keywords and lead scoring.
      </p>
      {!open ? (
        <button onClick={() => setOpen(true)} className="text-sm text-primary underline underline-offset-2 cursor-pointer font-medium">
          + Add AI provider key
        </button>
      ) : (
        <div className="space-y-2">
          <select
            value={provider}
            onChange={(e) => setProvider(e.target.value)}
            className="w-full px-2 py-1.5 text-sm rounded-lg border border-border bg-background text-foreground"
          >
            {chatProviders.map((p) => (
              <option key={p.provider} value={p.provider}>
                {p.display_name}
              </option>
            ))}
          </select>
          <input
            type="password"
            placeholder={`Paste your ${provider} key`}
            value={key}
            onChange={(e) => setKey(e.target.value)}
            className="w-full px-2 py-1.5 text-sm rounded-lg border border-border bg-background text-foreground"
          />
          <div className="flex gap-2">
            <Button onClick={save} disabled={!key.trim() || busy} size="sm" className="flex-1">
              {busy ? "Saving & verifying..." : "Save & verify"}
            </Button>
            <Button onClick={() => setOpen(false)} variant="outline" size="sm">
              Cancel
            </Button>
          </div>
        </div>
      )}
      {message && <p className="text-sm text-muted-foreground">{message}</p>}
    </div>
  );
}
