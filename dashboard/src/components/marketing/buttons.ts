import { cn } from "@/lib/utils";

const btn =
  "inline-flex h-11 items-center justify-center gap-2 rounded-full px-5 text-[15px] font-medium transition-[background-color,filter]";
export const btnPrimary = cn(btn, "border border-yellow-deep bg-yellow text-[#1c1a14] hover:brightness-95");
export const btnSecondary = cn(btn, "border border-border bg-card hover:bg-accent");
