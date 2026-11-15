import { useEffect, useRef, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Loader2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import { getFirstValue, getSetupStatus } from "@/lib/api";
import { setupPollMs } from "@/lib/setup-status";
import { NextSteps } from "./next-steps";
import { SetupChecklist } from "./setup-checklist";
import type { FirstValue } from "./types";

interface BackfillStepProps {
  onOpenInbox: (fv: FirstValue | undefined) => void;
}

const POLL_MS = 3000;
const AUTO_OPEN_AFTER_IDLE_MS = 4000;

/** Step 4: what the first scan has done so far; opens the inbox once leads are in. */
export function BackfillStep({ onOpenInbox }: BackfillStepProps) {
  const { data } = useQuery({ queryKey: ["onboarding-first-value"], queryFn: getFirstValue, refetchInterval: POLL_MS });
  const { data: setup } = useQuery({ queryKey: ["setup-status"], queryFn: getSetupStatus, refetchInterval: (query) => setupPollMs(query.state.data, POLL_MS) });
  const [startedAt] = useState(() => Date.now());
  const opened = useRef(false);

  const found = data?.mentions.length ?? 0;

  // Land on the inbox automatically once the scan finished with results.
  useEffect(() => {
    if (opened.current || !data || data.scanning || found === 0) return;
    if (Date.now() - startedAt < AUTO_OPEN_AFTER_IDLE_MS) return;
    opened.current = true;
    onOpenInbox(data);
  }, [data, found, startedAt, onOpenInbox]);

  return (
    <div>
      <p className="text-xs font-semibold text-primary uppercase tracking-wider mb-1">Step 4 of 4</p>
      <h1 className="font-[family-name:var(--font-head)] text-2xl mb-2">Finding your first leads</h1>
      <div className="mb-5">
        {setup ? (
          <SetupChecklist status={setup} />
        ) : (
          <div className="flex items-center gap-2 text-sm">
            <Loader2 className="size-4 animate-spin text-primary" />
            <span>Checking progress...</span>
          </div>
        )}
      </div>
      <Button className="w-full mb-6" variant={found > 0 ? "default" : "outline"} onClick={() => onOpenInbox(data)}>
        {found > 0 ? "Open inbox →" : "Open inbox now"}
      </Button>
      <NextSteps />
    </div>
  );
}
