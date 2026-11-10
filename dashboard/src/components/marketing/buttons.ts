import { cn } from "@/lib/utils";

const btn =
  "inline-flex h-11 items-center justify-center gap-2 rounded-lg px-5 text-[15px] font-semibold transition-colors";
export const btnPrimary = cn(btn, "bg-foreground text-background hover:bg-foreground/85");
export const btnSecondary = cn(btn, "border border-border bg-card hover:bg-accent");
