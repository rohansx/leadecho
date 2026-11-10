import { cn } from "@/lib/utils";

const btn =
  "inline-flex h-11 items-center justify-center gap-2 rounded-full px-5 text-[15px] font-semibold transition-[background-color,opacity,transform] active:scale-[0.98]";
export const btnPrimary = cn(btn, "bg-primary-bg text-primary-fg hover:opacity-90");
export const btnSecondary = cn(btn, "border border-border bg-card hover:bg-accent");

export const container = "mx-auto w-full max-w-[1180px] px-5 sm:px-8";
