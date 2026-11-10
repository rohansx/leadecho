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
    <section id={id} className={cn("border-t border-border py-24 scroll-mt-16", className)}>
      <div className="mx-auto max-w-6xl px-4 sm:px-6">{children}</div>
    </section>
  );
}

export function SectionHead({ title, children }: { title: ReactNode; children?: ReactNode }) {
  return (
    <div className="grid gap-4 lg:grid-cols-[1fr_1fr] lg:gap-16 lg:items-end">
      <h2 className="text-[2.5rem] leading-[1.05] font-bold tracking-[-0.03em] text-balance">{title}</h2>
      {children && <p className="text-foreground-soft max-w-xl lg:pb-1">{children}</p>}
    </div>
  );
}
