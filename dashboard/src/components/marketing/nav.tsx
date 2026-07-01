import { Link } from "@tanstack/react-router";
import { motion } from "motion/react";
import { Moon, Sun, ArrowRight } from "lucide-react";
import { useTheme } from "@/providers/theme-provider";
import { Logo } from "./logo";

const links = [
  { href: "#how", label: "How it works" },
  { href: "#person360", label: "Person360" },
  { href: "#pricing", label: "Pricing" },
  { href: "#faq", label: "FAQ" },
];

export function MarketingNav() {
  const { theme, toggleTheme } = useTheme();

  return (
    <motion.header
      initial={{ y: -20, opacity: 0 }}
      animate={{ y: 0, opacity: 1 }}
      transition={{ duration: 0.5, ease: [0.16, 1, 0.3, 1] }}
      className="sticky top-0 z-50 border-b border-border/70 bg-background/80 backdrop-blur-md"
    >
      <div className="max-w-6xl mx-auto px-6 h-16 flex items-center justify-between">
        <Logo />

        <nav className="hidden md:flex items-center gap-7">
          {links.map((l) => (
            <a
              key={l.href}
              href={l.href}
              className="text-sm text-foreground-soft hover:text-foreground transition-colors"
            >
              {l.label}
            </a>
          ))}
          <a
            href="https://github.com/your-org/leadecho"
            target="_blank"
            rel="noreferrer"
            className="text-sm text-foreground-soft hover:text-foreground transition-colors inline-flex items-center gap-1"
          >
            GitHub <ArrowRight className="h-3 w-3 -rotate-45" />
          </a>
        </nav>

        <div className="flex items-center gap-3">
          <button
            onClick={toggleTheme}
            aria-label="Toggle theme"
            className="w-9 h-9 rounded-lg border border-border bg-card shadow-xs hover:bg-accent transition-colors flex items-center justify-center cursor-pointer"
          >
            {theme === "dark" ? (
              <Sun className="h-4 w-4" />
            ) : (
              <Moon className="h-4 w-4" />
            )}
          </button>
          <Link
            to="/login"
            className="hidden sm:inline text-sm text-foreground-soft hover:text-foreground transition-colors"
          >
            Sign in
          </Link>
          <Link
            to="/inbox"
            className="inline-flex items-center gap-1.5 rounded-lg bg-primary text-primary-foreground px-4 py-2 text-sm font-medium shadow-sm hover:bg-primary-hover transition-colors"
          >
            Sign up
          </Link>
        </div>
      </div>
    </motion.header>
  );
}
