import { useEffect, useRef, useState, type ReactNode } from "react";
import { animate, motion, useInView, useReducedMotion } from "motion/react";

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

/** Counts from 0 to `to` the first time it is seen. */
export function CountUp({ to, prefix = "" }: { to: number; prefix?: string }) {
  const ref = useRef<HTMLSpanElement>(null);
  const seen = useInView(ref, { once: true, margin: "-40px" });
  const reduce = useReducedMotion();
  const [n, setN] = useState(reduce ? to : 0);

  useEffect(() => {
    if (!seen || reduce) return;
    const a = animate(0, to, { duration: 1.2, ease: EASE, onUpdate: (v) => setN(Math.round(v)) });
    return () => a.stop();
  }, [seen, reduce, to]);

  return (
    <span ref={ref}>
      {prefix}
      {n.toLocaleString()}
    </span>
  );
}

/** A bar that grows to `pct` the first time it is seen. */
export function Bar({ pct, className }: { pct: number; className?: string }) {
  const reduce = useReducedMotion();
  return (
    <div className="h-1.5 flex-1 overflow-hidden rounded-full bg-muted">
      <motion.div
        className={className ?? "h-full rounded-full bg-foreground"}
        initial={reduce ? false : { width: 0 }}
        whileInView={{ width: `${pct}%` }}
        viewport={{ once: true }}
        transition={{ duration: 1.1, ease: EASE }}
        style={reduce ? { width: `${pct}%` } : undefined}
      />
    </div>
  );
}
