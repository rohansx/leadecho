import type { ReactNode } from "react";
import { motion, useReducedMotion } from "motion/react";

const EASE = [0.2, 0.7, 0.2, 1] as const;

/** Fades content up once as it scrolls into view. */
export function Reveal({ children, delay = 0, className }: { children: ReactNode; delay?: number; className?: string }) {
  const reduce = useReducedMotion();
  return (
    <motion.div
      className={className}
      initial={reduce ? false : { opacity: 0, y: 14 }}
      whileInView={{ opacity: 1, y: 0 }}
      viewport={{ once: true, margin: "-60px" }}
      transition={{ duration: 0.6, delay, ease: EASE }}
    >
      {children}
    </motion.div>
  );
}

/** A bar at `pct`. Static: it must read correctly on first paint and in a screenshot. */
export function Bar({ pct, className }: { pct: number; className?: string }) {
  return (
    <div className="h-1.5 flex-1 overflow-hidden rounded-full bg-muted">
      <div className={className ?? "h-full rounded-full bg-foreground"} style={{ width: `${pct}%` }} />
    </div>
  );
}
