import { useRef, useState, useEffect } from "react";
import { Check, Loader2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { AIKeySetup } from "./ai-key-setup";
import { streamSSE } from "./sse";
import { GOALS, type AnalysisResult, type Goal } from "./types";
import { LockedChip, PaidPlatformsNote } from "@/components/billing/paid-platforms";
import { useLockedPlatforms } from "@/lib/use-locked-platforms";

// Platforms the listener builder can target. Testable ones are live-tested
// against free public APIs; the rest are kept on the listener for later.
export const LISTENER_PLATFORMS = [
  { value: "reddit", label: "Reddit" },
  { value: "hackernews", label: "Hacker News" },
  { value: "devto", label: "Dev.to" },
  { value: "bluesky", label: "Bluesky" }, // sources
  { value: "twitter", label: "Twitter / X", note: "paid data, once a day" },
  { value: "linkedin", label: "LinkedIn", note: "paid data, opt-in" },
  { value: "exa", label: "Web (Exa)", note: "needs EXA_API_KEY" },
];

interface UrlStepProps {
  goals: Goal[];
  onGoalsChange: (g: Goal[]) => void;
  platforms: string[];
  onPlatformsChange: (p: string[]) => void;
  onAnalyzed: (r: AnalysisResult) => void;
}

const toggle = <T,>(list: T[], v: T): T[] => (list.includes(v) ? list.filter((x) => x !== v) : [...list, v]);

/** Step 1: URL → streamed analysis; goal + platform chips while it runs. */
export function UrlStep({ goals, onGoalsChange, platforms, onPlatformsChange, onAnalyzed }: UrlStepProps) {
  const [url, setUrl] = useState("");
  const [steps, setSteps] = useState<string[]>([]);
  const [running, setRunning] = useState(false);
  const [error, setError] = useState("");
  const [result, setResult] = useState<AnalysisResult | null>(null);
  const abort = useRef<AbortController | null>(null);
  const isLocked = useLockedPlatforms();
  useEffect(() => () => abort.current?.abort(), []);

  async function analyze() {
    if (!url.trim() || running) return;
    abort.current?.abort();
    abort.current = new AbortController();
    setRunning(true);
    setError("");
    setSteps([]);
    setResult(null);
    try {
      await streamSSE(
        "/onboarding/analyze",
        { url },
        (event, data) => {
          const d = data as Record<string, unknown>;
          if (event === "step") setSteps((s) => [...s, String(d.message ?? "")]);
          if (event === "error") setError(String(d.error ?? "Analysis failed"));
          if (event === "result") setResult(data as AnalysisResult);
        },
        abort.current.signal,
      );
    } catch (err) {
      setError(err instanceof Error ? err.message : "Analysis failed");
    } finally {
      setRunning(false);
    }
  }

  const ready = !!result && goals.length > 0;

  return (
    <div>
      <p className="text-xs font-semibold text-primary uppercase tracking-wider mb-1">Step 1 of 4</p>
      <h1 className="font-[family-name:var(--font-head)] text-2xl mb-2">Enter your product URL</h1>
      <p className="text-muted-foreground text-sm mb-5">
        We read your site, build your brand profile and seed your knowledge base so replies are grounded.
      </p>
      <AIKeySetup />

      <div className="flex gap-2 mb-4">
        <Input
          placeholder="https://yourproduct.com"
          value={url}
          onChange={(e) => {
            setUrl(e.target.value);
            setResult(null);
          }}
          onKeyDown={(e) => e.key === "Enter" && analyze()}
          className="flex-1"
        />
        <Button onClick={analyze} disabled={!url.trim() || running}>
          {running ? <Loader2 className="size-4 animate-spin" /> : "Analyze"}
        </Button>
      </div>

      {(steps.length > 0 || running) && (
        <ul className="mb-5 space-y-1.5 text-sm">
          {steps.map((s, i) => (
            <li key={`${i}-${s}`} className="flex items-center gap-2">
              {i === steps.length - 1 && running ? (
                <Loader2 className="size-3.5 animate-spin text-primary" />
              ) : (
                <Check className="size-3.5 text-primary" />
              )}
              <span className={i === steps.length - 1 ? "text-foreground" : "text-muted-foreground"}>{s}</span>
            </li>
          ))}
        </ul>
      )}
      {error && <p className="text-destructive text-sm mb-4">{error}</p>}

      <p className="block text-xs text-muted-foreground uppercase tracking-wider mb-2 font-medium">What should we find?</p>
      <div className="grid grid-cols-2 gap-2 mb-5">
        {GOALS.map((g) => {
          const on = goals.includes(g.value);
          return (
            <button
              key={g.value}
              type="button"
              onClick={() => onGoalsChange(toggle(goals, g.value))}
              className={`text-left px-3 py-2 border border-border rounded-lg cursor-pointer transition-all ${
                on ? "bg-primary text-primary-foreground shadow-xs" : "bg-background hover:bg-muted"
              }`}
            >
              <span className="block text-sm font-semibold">{g.label}</span>
              <span className={`block text-xs ${on ? "opacity-80" : "text-muted-foreground"}`}>{g.hint}</span>
            </button>
          );
        })}
      </div>

      <p className="block text-xs text-muted-foreground uppercase tracking-wider mb-2 font-medium">Where?</p>
      <div className="flex flex-wrap gap-2 mb-6">
        {LISTENER_PLATFORMS.map((p) => isLocked(p.value) ? (
          <LockedChip key={p.value} label={p.label} />
        ) : (
          <button
            key={p.value}
            type="button"
            title={p.note ? `${p.label}: ${p.note}` : p.label}
            onClick={() => onPlatformsChange(toggle(platforms, p.value))}
            className={`px-3 py-1 border border-border text-sm rounded cursor-pointer transition-all ${
              platforms.includes(p.value) ? "bg-primary text-primary-foreground font-semibold" : "bg-background text-muted-foreground hover:bg-muted"
            }`}
          >
            {p.label}
            {p.note && <span className="ml-1 opacity-60 text-xs">*</span>}
          </button>
        ))}
      </div>
      {LISTENER_PLATFORMS.some((p) => isLocked(p.value)) && (
        <div className="-mt-4 mb-6">
          <PaidPlatformsNote />
        </div>
      )}

      <Button className="w-full" disabled={!ready} onClick={() => result && onAnalyzed(result)}>
        {!result ? (running ? "Reading your site..." : "Analyze your URL to continue") : goals.length === 0 ? "Pick at least one goal" : "Review brand profile →"}
      </Button>
    </div>
  );
}
