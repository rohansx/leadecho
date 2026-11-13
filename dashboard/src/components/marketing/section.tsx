import type { ReactNode } from "react";
import { cn } from "@/lib/utils";

export function Section({
  id,
  children,
  className,
}: {
  id?: string;
  children: ReactNode;
  className?: string;
}) {
  return (
    <section id={id} className={cn("scroll-mt-20 py-16 sm:py-20", className)}>
      <div className="mx-auto w-full max-w-[1180px] px-5 sm:px-8">{children}</div>
    </section>
  );
}

export function Eyebrow({ children }: { children: ReactNode }) {
  return (
    <p className="inline-flex items-center gap-2 font-[family-name:var(--font-mono)] text-[12px] text-muted-foreground">
      <span className="size-1.5 rounded-full bg-dot" aria-hidden="true" />
      {children}
    </p>
  );
}

export function H2({ children, className }: { children: ReactNode; className?: string }) {
  return (
    <h2
      className={cn(
        "text-[clamp(2.1rem,4vw,3.1rem)] leading-[1.04] font-semibold tracking-[-0.035em] text-balance [font-stretch:92%]",
        className,
      )}
    >
      {children}
    </h2>
  );
}
