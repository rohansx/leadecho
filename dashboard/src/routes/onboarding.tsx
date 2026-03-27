import { createFileRoute, useNavigate } from "@tanstack/react-router";
import { useState, useEffect } from "react";
import { useMutation } from "@tanstack/react-query";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { useAuth } from "@/lib/auth";
import { analyzeProductURL, completeOnboarding } from "@/lib/api";
import type { ProductAnalysis } from "@/lib/types";

export const Route = createFileRoute("/onboarding")({
  component: OnboardingPage,
});

const PLATFORMS = [
  { value: "reddit", label: "Reddit" },
  { value: "hackernews", label: "Hacker News" },
  { value: "twitter", label: "Twitter / X" },
  { value: "linkedin", label: "LinkedIn" },
  { value: "devto", label: "Dev.to" },
  { value: "lobsters", label: "Lobsters" },
  { value: "indiehackers", label: "Indie Hackers" },
];

const DEPLOY_STEPS = [
  "Creating monitoring profile...",
  "Generating pain-point embeddings...",
  "Setting up keyword monitors...",
  "Deploying agents to platforms...",
  "First scan starting...",
  "Your agents are live!",
];

function OnboardingPage() {
  const navigate = useNavigate();
  const { user, loading } = useAuth();
  const [step, setStep] = useState(1);

  // Step 1: URL input
  const [url, setUrl] = useState("");

  // Step 2: Review & edit (populated from AI analysis)
  const [productName, setProductName] = useState("");
  const [description, setDescription] = useState("");
  const [painPoints, setPainPoints] = useState<string[]>([]);
  const [keywords, setKeywords] = useState<string[]>([]);
  const [platforms, setPlatforms] = useState<string[]>(["reddit", "hackernews"]);
  const [subreddits, setSubreddits] = useState<string[]>([]);
  const [newPainPoint, setNewPainPoint] = useState("");
  const [newKeyword, setNewKeyword] = useState("");
  const [newSubreddit, setNewSubreddit] = useState("");

  // Step 3: Deploy animation
  const [deployStep, setDeployStep] = useState(0);

  useEffect(() => {
    if (!loading && !user) {
      navigate({ to: "/login" });
    }
  }, [loading, user, navigate]);

  const analyzeMutation = useMutation({
    mutationFn: (productUrl: string) => analyzeProductURL(productUrl),
    onSuccess: (data: ProductAnalysis) => {
      setProductName(data.product_name || "");
      setDescription(data.description || "");
      setPainPoints(data.pain_points || []);
      setKeywords(data.suggested_keywords || []);
      setPlatforms(data.suggested_platforms || ["reddit", "hackernews"]);
      setSubreddits(data.suggested_subreddits || []);
      setStep(2);
    },
  });

  const completeMutation = useMutation({
    mutationFn: () =>
      completeOnboarding({
        product_name: productName,
        description,
        pain_points: painPoints,
        keywords,
        platforms,
        subreddits,
      }),
    onSuccess: () => {
      // Start deploy animation
      setStep(3);
      let i = 0;
      const interval = setInterval(() => {
        i++;
        setDeployStep(i);
        if (i >= DEPLOY_STEPS.length - 1) {
          clearInterval(interval);
        }
      }, 800);
    },
  });

  function togglePlatform(p: string) {
    setPlatforms((prev) =>
      prev.includes(p) ? prev.filter((x) => x !== p) : [...prev, p],
    );
  }

  function removeChip(list: string[], setList: (v: string[]) => void, item: string) {
    setList(list.filter((x) => x !== item));
  }

  function addChip(value: string, list: string[], setList: (v: string[]) => void, setInput: (v: string) => void) {
    const trimmed = value.trim();
    if (trimmed && !list.includes(trimmed)) {
      setList([...list, trimmed]);
    }
    setInput("");
  }

  if (loading) return null;

  const progress = step === 1 ? 0 : step === 2 ? 50 : 100;

  return (
    <div className="fixed inset-0 z-[9999] flex flex-col items-center justify-center p-6 bg-background overflow-hidden">
      <style>{`
        @keyframes ob-grid-pan {
          0% { transform: translate(0, 0); }
          100% { transform: translate(60px, 60px); }
        }
        @keyframes ob-float {
          0%, 100% { transform: translateY(0); }
          50% { transform: translateY(-12px); }
        }
        @keyframes ob-pulse {
          0%, 100% { opacity: 0.4; }
          50% { opacity: 1; }
        }
        .ob-grid {
          position: absolute;
          inset: -60px;
          background-image:
            linear-gradient(hsl(var(--border) / 0.08) 1px, transparent 1px),
            linear-gradient(90deg, hsl(var(--border) / 0.08) 1px, transparent 1px);
          background-size: 60px 60px;
          animation: ob-grid-pan 40s linear infinite;
          pointer-events: none;
        }
        .ob-shape {
          position: absolute;
          border: 2px solid hsl(var(--border));
          pointer-events: none;
          opacity: 0.06;
        }
        .ob-shape-1 { width: 200px; height: 200px; top: 8%; right: 12%; animation: ob-float 8s ease-in-out infinite; }
        .ob-shape-2 { width: 120px; height: 120px; bottom: 15%; left: 8%; animation: ob-float 6s ease-in-out infinite 1s; }
        .ob-shape-3 { width: 80px; height: 80px; top: 60%; right: 25%; background: hsl(var(--primary)); border: none; opacity: 0.06; animation: ob-float 10s ease-in-out infinite 2s; }
        .ob-shape-4 { width: 150px; height: 150px; top: 20%; left: 15%; border: 2px solid hsl(var(--primary)); opacity: 0.05; animation: ob-float 12s ease-in-out infinite 3s; }
      `}</style>

      <div className="ob-grid" />
      <div className="ob-shape ob-shape-1" />
      <div className="ob-shape ob-shape-2" />
      <div className="ob-shape ob-shape-3" />
      <div className="ob-shape ob-shape-4" />

      <div className="relative z-10 w-full max-w-[580px] bg-card border-2 border-border shadow-lg p-10">
        {/* Logo */}
        <div className="flex items-center gap-2.5 mb-8">
          <div className="w-9 h-9 bg-primary border-2 border-border shadow-xs flex items-center justify-center font-[family-name:var(--font-head)] text-primary-foreground text-sm font-bold">
            LE
          </div>
          <span className="font-[family-name:var(--font-head)] text-xl">LeadEcho</span>
        </div>

        {/* Progress bar */}
        <div className="h-1 bg-muted border border-border mb-8 overflow-hidden">
          <div className="h-full bg-primary transition-all duration-500 ease-out" style={{ width: `${progress}%` }} />
        </div>

        {/* Step 1: URL Input */}
        {step === 1 && (
          <div>
            <p className="text-xs font-semibold text-primary uppercase tracking-wider mb-1">Step 1 of 3</p>
            <h1 className="font-[family-name:var(--font-head)] text-2xl mb-2">Enter your product URL</h1>
            <p className="text-muted-foreground text-sm mb-6">
              We'll analyze your website and automatically set up monitoring agents.
            </p>
            <Input
              placeholder="https://yourproduct.com"
              value={url}
              onChange={(e) => setUrl(e.target.value)}
              className="mb-4"
              onKeyDown={(e) => {
                if (e.key === "Enter" && url.trim()) analyzeMutation.mutate(url);
              }}
            />
            <Button
              onClick={() => analyzeMutation.mutate(url)}
              disabled={!url.trim() || analyzeMutation.isPending}
              className="w-full"
            >
              {analyzeMutation.isPending ? "Analyzing your product..." : "Analyze & Set Up"}
            </Button>
            {analyzeMutation.isError && (
              <p className="text-destructive text-sm mt-3">
                {analyzeMutation.error?.message || "Failed to analyze URL. Please try again."}
              </p>
            )}
            <button
              onClick={() => setStep(2)}
              className="block mx-auto mt-4 text-sm text-muted-foreground hover:text-foreground underline underline-offset-2 cursor-pointer"
            >
              Skip — set up manually
            </button>
          </div>
        )}

        {/* Step 2: Review & Edit */}
        {step === 2 && (
          <div className="max-h-[60vh] overflow-y-auto pr-1">
            <p className="text-xs font-semibold text-primary uppercase tracking-wider mb-1">Step 2 of 3</p>
            <h1 className="font-[family-name:var(--font-head)] text-2xl mb-6">Review & customize</h1>

            {/* Product name */}
            <label className="block text-xs text-muted-foreground uppercase tracking-wider mb-1.5 font-medium">
              Product name
            </label>
            <Input value={productName} onChange={(e) => setProductName(e.target.value)} className="mb-3" />

            {/* Description */}
            <label className="block text-xs text-muted-foreground uppercase tracking-wider mb-1.5 font-medium">
              Description
            </label>
            <Input value={description} onChange={(e) => setDescription(e.target.value)} className="mb-3" />

            {/* Pain Points */}
            <label className="block text-xs text-muted-foreground uppercase tracking-wider mb-1.5 font-medium">
              Pain points
            </label>
            <div className="flex flex-wrap gap-1.5 mb-1.5">
              {painPoints.map((pp) => (
                <span key={pp} className="inline-flex items-center gap-1 px-2 py-0.5 bg-muted border border-border text-xs">
                  {pp}
                  <button onClick={() => removeChip(painPoints, setPainPoints, pp)} className="text-muted-foreground hover:text-foreground cursor-pointer">&times;</button>
                </span>
              ))}
            </div>
            <div className="flex gap-1.5 mb-3">
              <Input
                placeholder="Add pain point..."
                value={newPainPoint}
                onChange={(e) => setNewPainPoint(e.target.value)}
                onKeyDown={(e) => { if (e.key === "Enter") addChip(newPainPoint, painPoints, setPainPoints, setNewPainPoint); }}
                className="flex-1"
              />
              <Button variant="outline" size="sm" onClick={() => addChip(newPainPoint, painPoints, setPainPoints, setNewPainPoint)}>Add</Button>
            </div>

            {/* Keywords */}
            <label className="block text-xs text-muted-foreground uppercase tracking-wider mb-1.5 font-medium">
              Monitoring keywords
            </label>
            <div className="flex flex-wrap gap-1.5 mb-1.5">
              {keywords.map((kw) => (
                <span key={kw} className="inline-flex items-center gap-1 px-2 py-0.5 bg-muted border border-border text-xs">
                  {kw}
                  <button onClick={() => removeChip(keywords, setKeywords, kw)} className="text-muted-foreground hover:text-foreground cursor-pointer">&times;</button>
                </span>
              ))}
            </div>
            <div className="flex gap-1.5 mb-3">
              <Input
                placeholder="Add keyword..."
                value={newKeyword}
                onChange={(e) => setNewKeyword(e.target.value)}
                onKeyDown={(e) => { if (e.key === "Enter") addChip(newKeyword, keywords, setKeywords, setNewKeyword); }}
                className="flex-1"
              />
              <Button variant="outline" size="sm" onClick={() => addChip(newKeyword, keywords, setKeywords, setNewKeyword)}>Add</Button>
            </div>

            {/* Platforms */}
            <label className="block text-xs text-muted-foreground uppercase tracking-wider mb-2 font-medium">
              Platforms
            </label>
            <div className="flex flex-wrap gap-2 mb-3">
              {PLATFORMS.map((p) => (
                <button
                  key={p.value}
                  onClick={() => togglePlatform(p.value)}
                  className={`px-4 py-1.5 border-2 border-border text-sm cursor-pointer font-[family-name:var(--font-sans)] transition-all ${
                    platforms.includes(p.value)
                      ? "bg-primary text-primary-foreground font-semibold shadow-xs"
                      : "bg-background text-muted-foreground hover:bg-muted"
                  }`}
                >
                  {p.label}
                </button>
              ))}
            </div>

            {/* Subreddits */}
            {platforms.includes("reddit") && (
              <>
                <label className="block text-xs text-muted-foreground uppercase tracking-wider mb-1.5 font-medium">
                  Subreddits
                </label>
                <div className="flex flex-wrap gap-1.5 mb-1.5">
                  {subreddits.map((sr) => (
                    <span key={sr} className="inline-flex items-center gap-1 px-2 py-0.5 bg-muted border border-border text-xs">
                      r/{sr}
                      <button onClick={() => removeChip(subreddits, setSubreddits, sr)} className="text-muted-foreground hover:text-foreground cursor-pointer">&times;</button>
                    </span>
                  ))}
                </div>
                <div className="flex gap-1.5 mb-3">
                  <Input
                    placeholder="Add subreddit..."
                    value={newSubreddit}
                    onChange={(e) => setNewSubreddit(e.target.value)}
                    onKeyDown={(e) => { if (e.key === "Enter") addChip(newSubreddit, subreddits, setSubreddits, setNewSubreddit); }}
                    className="flex-1"
                  />
                  <Button variant="outline" size="sm" onClick={() => addChip(newSubreddit, subreddits, setSubreddits, setNewSubreddit)}>Add</Button>
                </div>
              </>
            )}

            <Button
              onClick={() => completeMutation.mutate()}
              disabled={!productName.trim() || completeMutation.isPending}
              className="w-full mt-4"
            >
              {completeMutation.isPending ? "Deploying..." : "Deploy Agents"}
            </Button>
          </div>
        )}

        {/* Step 3: Agent Deployment Animation */}
        {step === 3 && (
          <div>
            <h1 className="font-[family-name:var(--font-head)] text-2xl mb-6">Deploying your agents</h1>
            <div className="space-y-3 mb-8">
              {DEPLOY_STEPS.map((label, i) => (
                <div
                  key={label}
                  className={`flex items-center gap-3 transition-all duration-300 ${
                    i <= deployStep ? "opacity-100" : "opacity-0 translate-y-2"
                  }`}
                >
                  <div
                    className={`w-5 h-5 border-2 border-border flex items-center justify-center text-xs transition-colors ${
                      i < deployStep
                        ? "bg-primary text-primary-foreground"
                        : i === deployStep
                          ? "bg-primary/20 text-primary"
                          : "bg-muted"
                    }`}
                    style={i === deployStep && i < DEPLOY_STEPS.length - 1 ? { animation: "ob-pulse 1.5s ease-in-out infinite" } : undefined}
                  >
                    {i < deployStep ? "\u2713" : i === deployStep ? "\u25CF" : ""}
                  </div>
                  <span className={`text-sm ${i <= deployStep ? "text-foreground" : "text-muted-foreground"}`}>
                    {label}
                  </span>
                </div>
              ))}
            </div>
            {deployStep >= DEPLOY_STEPS.length - 1 && (
              <Button onClick={() => navigate({ to: "/inbox" })} className="w-full">
                Open Inbox &rarr;
              </Button>
            )}
          </div>
        )}
      </div>
    </div>
  );
}
