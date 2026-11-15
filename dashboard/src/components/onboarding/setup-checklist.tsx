import { useQuery } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { Check, Circle, Loader2, Minus } from "lucide-react";
import { listKeywords } from "@/lib/api";
import { keywordResult, leaveLine, setupLines, type SetupStatus, type StageState } from "@/lib/setup-status";

function StateIcon({ state }: { state: StageState }) {
  const cls = "size-4 mt-0.5 shrink-0";
  if (state === "done") return <Check className={`${cls} text-primary`} aria-label="Done" />;
  if (state === "running") return <Loader2 className={`${cls} animate-spin text-primary`} aria-label="Running" />;
  if (state === "not_included") return <Minus className={`${cls} text-muted-foreground`} aria-label="Not included" />;
  return <Circle className={`${cls} text-muted-foreground`} aria-label="Waiting" />;
}

/** What a workspace without a lead is waiting for. Used on onboarding step 4 and in the inbox. */
export function SetupChecklist({ status }: { status: SetupStatus }) {
  // Same key as the inbox and the keywords page: one request between them.
  const { data: keywords } = useQuery({ queryKey: ["keywords"], queryFn: listKeywords, enabled: status.finished_empty });
  const active = (keywords ?? []).filter((k) => k.is_active);

  return (
    <div className="text-left text-sm">
      <ul className="space-y-1.5">
        {setupLines(status).map((line) => (
          <li key={line.key} className="flex gap-2">
            <StateIcon state={line.state} />
            <span className={line.state === "not_included" ? "text-muted-foreground" : undefined}>{line.text}</span>
          </li>
        ))}
      </ul>

      {status.finished_empty && (
        <div className="mt-3 border-t border-border pt-3">
          <p className="font-medium">No buyers were found for these keywords so far.</p>
          {active.length > 0 && (
            <ul className="mt-1.5 space-y-0.5 text-xs text-muted-foreground">
              {active.map((k) => (
                <li key={k.id}>{keywordResult(k.term, k.yield)}</li>
              ))}
            </ul>
          )}
          <Link to="/keywords" className="mt-2 inline-block text-xs text-primary underline underline-offset-2">
            Adjust your keywords
          </Link>
        </div>
      )}

      <p className="mt-3 text-xs text-muted-foreground">{leaveLine(status)}</p>
    </div>
  );
}
