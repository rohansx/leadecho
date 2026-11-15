import { Link } from "@tanstack/react-router";
import { cn } from "@/lib/utils";

/** The LeadEcho mark (brand yellow: #e0a800 light, #ffd43b dark): a lead (dot) inside a listening ring, with its echo slipping out.
 *  Same geometry as public/favicon.svg, logo.svg and the extension icons. */
export function LogoMark({ className }: { className?: string }) {
  return (
    <svg
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth={2.4}
      strokeLinecap="round"
      className={cn("text-[#e0a800] dark:text-[#ffd43b]", className)}
      aria-hidden="true"
    >
      <circle cx="12" cy="12" r="3.6" fill="currentColor" stroke="none" />
      <path d="M4.12 13.39A8 8 0 1 1 10.61 19.88" />
      <circle cx="4.9" cy="19.1" r="1.7" fill="currentColor" stroke="none" />
    </svg>
  );
}

/** Mark + wordmark. The one brand lockup used everywhere in the app and site. */
export function Logo({ className, to = "/" }: { className?: string; to?: "/" | "/inbox" }) {
  return (
    <Link
      to={to}
      aria-label="LeadEcho home"
      className={cn("flex items-center gap-2 font-[family-name:var(--font-head)] text-lg", className)}
    >
      <LogoMark className="h-6 w-6" />
      <span className="font-semibold tracking-[-0.02em]">LeadEcho</span>
    </Link>
  );
}
