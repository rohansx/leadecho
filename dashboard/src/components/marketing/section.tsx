import type { ReactNode } from "react";
import { cn } from "@/lib/utils";

/** One block in the main column. The id is what the table of contents links to. */
export function Section({ id, children, className }: { id: string; children: ReactNode; className?: string }) {
  return (
    <section id={id} className={cn("scroll-mt-24 border-t border-border pt-16 pb-20 first:border-t-0 first:pt-4", className)}>
      {children}
    </section>
  );
}

export function Label({ children }: { children: ReactNode }) {
  return (
    <p className="font-[family-name:var(--font-plexmono)] text-[12px] uppercase tracking-[0.08em] text-muted-foreground">
      {children}
    </p>
  );
}

/** Two-tone heading: the statement in ink, its consequence in a muted tone. */
export function SectionHead({
  label,
  title,
  muted,
  children,
}: {
  label: string;
  title: string;
  muted?: string;
  children?: ReactNode;
}) {
  return (
    <div className="max-w-2xl">
      <Label>{label}</Label>
      <h2 className="mt-3 text-[clamp(2rem,3.6vw,2.6rem)] leading-[1.12] font-light tracking-[-0.025em] text-balance">
        {title}
        {muted && <span className="text-muted-head"> {muted}</span>}
      </h2>
      {children && <p className="mt-5 text-foreground-soft">{children}</p>}
    </div>
  );
}
