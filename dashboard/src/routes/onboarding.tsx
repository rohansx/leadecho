import { createFileRoute, useNavigate } from "@tanstack/react-router";
import { useEffect, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Button } from "@/components/ui/button";
import { useAuth } from "@/lib/auth";
import { completeOnboardingWithListeners, getOnboardingStatus, saveBrandProfile } from "@/lib/api";
import { INBOX_QUEUES } from "@/lib/constants";
import { BackfillStep } from "@/components/onboarding/backfill-step";
import { BrandProfileCard } from "@/components/onboarding/brand-profile-card";
import { ListenerBuilder } from "@/components/onboarding/listener-builder";
import { UrlStep } from "@/components/onboarding/url-step";
import type { AnalysisResult, BrandProfile, FirstValue, Goal, Proposal } from "@/components/onboarding/types";

export const Route = createFileRoute("/onboarding")({
  component: OnboardingPage,
});

type Step = "url" | "profile" | "listeners" | "backfill";
const PROGRESS: Record<Step, number> = { url: 10, profile: 35, listeners: 65, backfill: 100 };

const EMPTY_PROFILE: BrandProfile = {
  overview: "",
  product: "",
  positioning: "",
  customer_segments: [],
  competitors: [],
  pain_points: [],
  keywords: { brand: [], product: [], category: [], problem: [] },
  features: [],
  target_audience: "",
  source_url: "",
};

function OnboardingPage() {
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const { user, loading } = useAuth();
  const [step, setStep] = useState<Step>("url");
  const [goals, setGoals] = useState<Goal[]>(["find_buyers"]);
  const [platforms, setPlatforms] = useState<string[]>(["reddit", "hackernews", "devto"]);
  const [profile, setProfile] = useState<BrandProfile>(EMPTY_PROFILE);
  const [subreddits, setSubreddits] = useState<string[]>([]);

  const { data: onboarding } = useQuery({
    queryKey: ["onboarding"],
    queryFn: getOnboardingStatus,
    enabled: !!user,
    retry: false,
  });

  useEffect(() => {
    // Only bounce finished users who arrive here; not ourselves mid-backfill.
    if (onboarding?.completed && step === "url") navigate({ to: "/inbox" });
  }, [onboarding, step, navigate]);

  useEffect(() => {
    if (!loading && !user) navigate({ to: "/login" });
  }, [loading, user, navigate]);

  const saveProfile = useMutation({
    mutationFn: () => saveBrandProfile(profile),
    onSuccess: (saved) => {
      setProfile(saved);
      setStep("listeners");
    },
  });

  const complete = useMutation({
    mutationFn: (listeners: Proposal[]) => completeOnboardingWithListeners({ brand_profile: profile, listeners, subreddits }),
    // Do not invalidate ["onboarding"] yet: the dashboard guard would redirect
    // before the backfill step is shown.
    onSuccess: () => setStep("backfill"),
  });

  function onAnalyzed(r: AnalysisResult) {
    setProfile({ ...EMPTY_PROFILE, ...r.brand_profile });
    setSubreddits(r.analysis.suggested_subreddits ?? []);
    setStep("profile");
  }

  function openInbox(fv: FirstValue | undefined) {
    queryClient.setQueryData(["onboarding"], { ...onboarding, completed: true });
    void queryClient.invalidateQueries({ queryKey: ["onboarding"] });
    navigate({
      to: "/inbox",
      search: {
        queue: INBOX_QUEUES.ALL,
        id: fv?.mentions[0]?.id,
        loosened: fv?.loosened ? "1" : undefined,
      },
    });
  }

  if (loading) return null;

  return (
    <div className="fixed inset-0 z-[9999] flex flex-col items-center justify-center p-6 bg-background overflow-hidden">
      <div className="relative z-10 w-full max-w-[680px] bg-card border border-border shadow-lg p-8 max-h-[92vh] overflow-y-auto">
        <div className="flex items-center gap-2.5 mb-6">
          <div className="w-9 h-9 bg-primary border border-border shadow-xs flex items-center justify-center font-[family-name:var(--font-head)] text-primary-foreground text-sm font-bold">
            LE
          </div>
          <span className="font-[family-name:var(--font-head)] text-xl">LeadEcho</span>
        </div>
        <div className="h-1 bg-muted border border-border mb-6 overflow-hidden">
          <div className="h-full bg-primary transition-all duration-500 ease-out" style={{ width: `${PROGRESS[step]}%` }} />
        </div>

        {step === "url" && (
          <>
            <UrlStep goals={goals} onGoalsChange={setGoals} platforms={platforms} onPlatformsChange={setPlatforms} onAnalyzed={onAnalyzed} />
            <button
              onClick={() => setStep("profile")}
              className="block mx-auto mt-4 text-sm text-muted-foreground hover:text-foreground underline underline-offset-2 cursor-pointer"
            >
              Can't reach your site? Fill in the profile manually
            </button>
          </>
        )}

        {step === "profile" && (
          <div>
            <p className="text-xs font-semibold text-primary uppercase tracking-wider mb-1">Step 2 of 4</p>
            <h1 className="font-[family-name:var(--font-head)] text-2xl mb-4">Your brand profile</h1>
            <BrandProfileCard profile={profile} onChange={setProfile} />
            {saveProfile.isError && <p className="text-destructive text-sm mt-3">{saveProfile.error.message}</p>}
            <div className="flex gap-2 mt-4">
              <Button variant="outline" onClick={() => setStep("url")}>
                Back
              </Button>
              <Button
                className="flex-1"
                disabled={!profile.product.trim() || goals.length === 0 || saveProfile.isPending}
                onClick={() => saveProfile.mutate()}
              >
                {saveProfile.isPending ? "Saving..." : "Build my listeners →"}
              </Button>
            </div>
          </div>
        )}

        {step === "listeners" && (
          <ListenerBuilder
            goals={goals}
            platforms={platforms}
            creating={complete.isPending}
            createError={complete.isError ? complete.error.message : undefined}
            onCreate={(listeners) => complete.mutate(listeners)}
          />
        )}

        {step === "backfill" && <BackfillStep onOpenInbox={openInbox} />}
      </div>
    </div>
  );
}
