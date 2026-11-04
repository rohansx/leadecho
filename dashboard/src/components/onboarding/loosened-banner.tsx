import { useState } from "react";
import { X } from "lucide-react";

/** Shown on the inbox after onboarding when no high-fit leads exist yet. */
export function LoosenedBanner() {
  const [open, setOpen] = useState(true);
  if (!open) return null;
  return (
    <div className="fixed bottom-4 left-1/2 -translate-x-1/2 z-50 max-w-md flex items-start gap-2 px-4 py-2.5 bg-card border border-border shadow-lg rounded-lg text-sm">
      <span>
        No high-fit leads yet, so we loosened the filter and are showing the closest matches. Rate them to tune your
        listeners.
      </span>
      <button type="button" aria-label="Dismiss" onClick={() => setOpen(false)} className="text-muted-foreground hover:text-foreground cursor-pointer">
        <X className="size-4" />
      </button>
    </div>
  );
}
