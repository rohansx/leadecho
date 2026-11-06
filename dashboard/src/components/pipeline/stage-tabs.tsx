import { STAGES } from "@/lib/pipeline";
import type { LeadStage } from "@/lib/types";
import { cn } from "@/lib/utils";

interface StageTabsProps {
  value: LeadStage;
  counts: Map<string, number>;
  onChange: (stage: LeadStage) => void;
}

export function StageTabs({ value, counts, onChange }: StageTabsProps) {
  return (
    <div role="tablist" className="flex flex-wrap gap-1">
      {STAGES.map((s) => (
        <button
          key={s.key}
          role="tab"
          aria-selected={value === s.key}
          onClick={() => onChange(s.key)}
          className={cn(
            "h-8 px-3 rounded-lg text-[13px] font-medium inline-flex items-center gap-1.5 cursor-pointer",
            value === s.key ? "bg-foreground text-background" : "text-muted-foreground hover:bg-accent",
          )}
        >
          {s.label}
          <span className="tabular-nums opacity-70">{counts.get(s.key) ?? 0}</span>
        </button>
      ))}
    </div>
  );
}
