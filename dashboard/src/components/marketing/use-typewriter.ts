import { useEffect, useRef, useState } from "react";
import { useInView, useReducedMotion } from "motion/react";

/** Types `text` out once it is seen. Returns the ref to attach and the text so far. */
export function useTypewriter(text: string, cps = 38) {
  const ref = useRef<HTMLDivElement>(null);
  const seen = useInView(ref, { once: true, margin: "-80px" });
  const reduce = useReducedMotion();
  const [len, setLen] = useState(reduce ? text.length : 0);

  useEffect(() => {
    if (!seen || reduce) return;
    const id = setInterval(() => {
      setLen((l) => {
        if (l >= text.length) clearInterval(id);
        return Math.min(l + 1, text.length);
      });
    }, 1000 / cps);
    return () => clearInterval(id);
  }, [seen, reduce, text, cps]);

  return { ref, typed: text.slice(0, len), done: len >= text.length };
}
