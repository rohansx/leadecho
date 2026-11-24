import { Link } from "@tanstack/react-router";
import { Lock } from "lucide-react";

/** A disabled platform chip with a lock; the label reads like the other chips. */
export function LockedChip({ label }: { label: string }) {
  return (
    <span
      title="Not on your plan"
      className="inline-flex cursor-not-allowed items-center gap-1 rounded-lg border border-dashed border-border px-2.5 py-1 text-xs font-medium text-muted-foreground"
    >
      <Lock className="h-3 w-3" aria-hidden="true" />
      {label}
    </span>
  );
}

/** One line under a platform picker explaining the locks, with the way out. */
export function PaidPlatformsNote() {
  return (
    <p className="text-xs text-muted-foreground">
      Reddit comes with Solo and up; X and LinkedIn with Starter and Pro.{" "}
      <Link to="/settings" search={{ tab: "billing" }} className="font-medium text-foreground underline underline-offset-2">
        Upgrade
      </Link>
    </p>
  );
}
