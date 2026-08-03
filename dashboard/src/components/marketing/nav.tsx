import { useState } from "react";
import { Link } from "@tanstack/react-router";
import { motion, useReducedMotion } from "motion/react";
import { Moon, Sun, ArrowRight } from "lucide-react";
import { useTheme } from "@/providers/theme-provider";
import { cn } from "@/lib/utils";
import { Logo } from "./logo";
import { GITHUB_URL } from "@/lib/constants";

const links = [
  { href: "#how", label: "How it works" },
  { href: "#pricing", label: "Pricing" },
  { href: "#faq", label: "FAQ" },
  { href: GITHUB_URL, label: "GitHub", external: true },
];

export function MarketingNav() {
  const { theme, toggleTheme } = useTheme();
  const reduceMotion = useReducedMotion();
  const [hovered, setHovered] = useState<number | null>(null);

  return (
    <motion.header
      initial={{ y: -20, opacity: 0 }}
      animate={{ y: 0, opacity: 1 }}
      transition={{ duration: 0.5, ease: [0.16, 1, 0.3, 1] }}
      className="sticky top-3 z-40 px-4"
    >
      <div className="glass-strong mx-auto flex h-14 max-w-4xl items-center justify-between gap-3 rounded-full px-3 pl-5">
        <Logo />

        <nav
          className="absolute left-1/2 hidden -translate-x-1/2 items-center md:flex"
          onMouseLeave={() => setHovered(null)}
        >
          {links.map((l, i) => (
            <a
              key={l.href}
              href={l.href}
              {...(l.external
                ? { target: "_blank", rel: "noreferrer" }
                : {})}
              onMouseEnter={() => setHovered(i)}
              className="relative inline-flex items-center gap-1 rounded-full px-3.5 py-1.5 text-sm text-foreground-soft transition-colors hover:text-foreground"
            >
              {hovered === i && (
                <motion.span
                  layoutId="nav-hover"
                  aria-hidden="true"
                  className="absolute inset-0 -z-10 rounded-full bg-primary/10"
                  transition={
                    reduceMotion
                      ? { duration: 0 }
                      : { type: "spring", stiffness: 380, damping: 32 }
                  }
                />
              )}
              <span className="relative">{l.label}</span>
              {l.external && (
                <ArrowRight
                  aria-hidden="true"
                  className="h-3 w-3 -rotate-45"
                />
              )}
            </a>
          ))}
        </nav>

        <div className="flex items-center gap-2">
          <button
            onClick={toggleTheme}
            aria-label="Toggle theme"
            className="flex h-9 w-9 cursor-pointer items-center justify-center rounded-full border border-border/60 transition-colors hover:bg-accent"
          >
            {theme === "dark" ? (
              <Sun className="h-4 w-4" />
            ) : (
              <Moon className="h-4 w-4" />
            )}
          </button>
          <Link
            to="/login"
            className="hidden text-sm text-foreground-soft transition-colors hover:text-foreground sm:inline"
          >
            Sign in
          </Link>
          <Link
            to="/register"
            className={cn(
              "inline-flex items-center gap-1.5 rounded-full bg-primary px-4 py-2 text-sm font-medium text-primary-foreground shadow-sm transition-colors hover:bg-primary-hover",
            )}
          >
            Get started
          </Link>
        </div>
      </div>
    </motion.header>
  );
}
