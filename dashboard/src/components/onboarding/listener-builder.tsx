import { useEffect, useState } from "react";
import { Button } from "@/components/ui/button";
import { sendSampleFeedback } from "@/lib/api";
import { ListenerCard } from "./listener-card";
import { streamSSE } from "./sse";
import type { Goal, ListenerState, Proposal, QueryTested, Sample } from "./types";

interface ListenerBuilderProps {
  goals: Goal[];
  platforms: string[];
  creating: boolean;
  createError?: string;
  onCreate: (listeners: Proposal[]) => void;
}

const initial = (goals: Goal[]): Record<string, ListenerState> =>
  Object.fromEntries(goals.map((g) => [g, { goal: g, steps: [], tests: [], samples: [], feedback: {}, included: true }]));

/** Step 3: consumes the listener-builder SSE stream, one card per goal. */
export function ListenerBuilder({ goals, platforms, creating, createError, onCreate }: ListenerBuilderProps) {
  const [states, setStates] = useState<Record<string, ListenerState>>(() => initial(goals));
  const [done, setDone] = useState(false);
  const [error, setError] = useState("");

  useEffect(() => {
    const ctrl = new AbortController();
    const patch = (goal: string, fn: (s: ListenerState) => ListenerState) =>
      setStates((prev) => (prev[goal] ? { ...prev, [goal]: fn(prev[goal]) } : prev));

    streamSSE(
      "/onboarding/listeners/build",
      { goals, platforms },
      (event, data) => {
        const d = data as Record<string, unknown>;
        const goal = String(d.goal ?? "");
        if (event === "step" && goal) patch(goal, (s) => ({ ...s, steps: [...s.steps, String(d.message)] }));
        if (event === "query_tested") patch(goal, (s) => ({ ...s, tests: [...s.tests, data as QueryTested] }));
        if (event === "samples") patch(goal, (s) => ({ ...s, samples: (d.posts as Sample[]) ?? [] }));
        if (event === "listener_proposal") patch(goal, (s) => ({ ...s, proposal: data as Proposal }));
        if (event === "done") setDone(true);
      },
      ctrl.signal,
    )
      .catch((err: unknown) => {
        if (!ctrl.signal.aborted) setError(err instanceof Error ? err.message : "Listener builder failed");
      })
      .finally(() => {
        if (!ctrl.signal.aborted) setDone(true);
      });
    return () => ctrl.abort();
  }, [goals, platforms]);

  function feedback(goal: Goal, s: Sample, label: "up" | "down") {
    setStates((prev) => ({ ...prev, [goal]: { ...prev[goal], feedback: { ...prev[goal].feedback, [s.url]: label } } }));
    sendSampleFeedback({ goal, url: s.url, text: s.text, platform: s.platform, label }).catch(() => {
      // Calibration is best-effort; the listener still works without it.
    });
  }

  const chosen = Object.values(states)
    .filter((s) => s.included && s.proposal && s.proposal.keywords.length > 0)
    .map((s) => s.proposal as Proposal);

  return (
    <div>
      <p className="text-xs font-semibold text-primary uppercase tracking-wider mb-1">Step 3 of 4</p>
      <h1 className="font-[family-name:var(--font-head)] text-2xl mb-2">Your listeners</h1>
      <p className="text-muted-foreground text-sm mb-5">
        Each listener was tested live on public sources. Rate the samples, tweak what it looks for, then create.
      </p>
      <div className="space-y-3 mb-5">
        {goals.map((g) => (
          <ListenerCard
            key={g}
            state={states[g]}
            onToggle={() => setStates((prev) => ({ ...prev, [g]: { ...prev[g], included: !prev[g].included } }))}
            onProposalChange={(p) => setStates((prev) => ({ ...prev, [g]: { ...prev[g], proposal: p } }))}
            onFeedback={(s, l) => feedback(g, s, l)}
          />
        ))}
      </div>
      {error && <p className="text-destructive text-sm mb-3">{error}</p>}
      {createError && <p className="text-destructive text-sm mb-3">{createError}</p>}
      <Button className="w-full" disabled={!done || creating || chosen.length === 0} onClick={() => onCreate(chosen)}>
        {!done ? "Testing keywords..." : creating ? "Creating..." : `Create ${chosen.length} listener${chosen.length === 1 ? "" : "s"} & scan last 7 days`}
      </Button>
    </div>
  );
}
