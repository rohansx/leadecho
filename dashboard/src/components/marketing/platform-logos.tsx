// Monochrome brand marks (currentColor). Recognizable simplified glyphs.
// Well-known brand paths (X, LinkedIn, Hacker News, Dev.to, Quora, Reddit) are
// standard single-path silhouettes; Lobsters/Indie Hackers are simple lettermarks.

import type { ReactNode } from "react";

export type LogoProps = { className?: string };

function Svg({ className, children }: LogoProps & { children: ReactNode }) {
  return (
    <svg
      viewBox="0 0 24 24"
      fill="currentColor"
      aria-hidden="true"
      className={className}
    >
      {children}
    </svg>
  );
}

export function RedditIcon({ className }: LogoProps) {
  return (
    <Svg className={className}>
      <path
        fillRule="evenodd"
        clipRule="evenodd"
        d="M12 0A12 12 0 0 0 0 12a12 12 0 0 0 12 12 12 12 0 0 0 12-12A12 12 0 0 0 12 0zm5.01 4.744c.688 0 1.25.561 1.25 1.249a1.25 1.25 0 0 1-2.498.056l-2.597-.547-.8 3.747c1.824.07 3.48.632 4.674 1.488.308-.309.73-.491 1.207-.491.968 0 1.754.786 1.754 1.754 0 .716-.435 1.333-1.01 1.614a3.111 3.111 0 0 1 .042.52c0 2.694-3.13 4.87-7.004 4.87-3.874 0-7.004-2.176-7.004-4.87 0-.183.015-.366.043-.534A1.748 1.748 0 0 1 4.028 12c0-.968.786-1.754 1.754-1.754.463 0 .898.196 1.207.49 1.207-.883 2.878-1.43 4.744-1.487l.885-4.182a.342.342 0 0 1 .14-.197.35.35 0 0 1 .238-.042l2.906.617a1.214 1.214 0 0 1 1.108-.701zM9.25 12C8.561 12 8 12.562 8 13.25c0 .687.561 1.248 1.25 1.248.687 0 1.248-.561 1.248-1.249 0-.688-.561-1.249-1.249-1.249zm5.5 0c-.687 0-1.248.561-1.248 1.25 0 .687.561 1.248 1.249 1.248.688 0 1.249-.561 1.249-1.249 0-.687-.562-1.249-1.25-1.249zm-5.466 3.99a.327.327 0 0 0-.231.094.33.33 0 0 0 0 .463c.842.842 2.484.913 2.961.913.477 0 2.105-.056 2.961-.913a.361.361 0 0 0 .029-.463.33.33 0 0 0-.464 0c-.547.533-1.684.73-2.512.73-.828 0-1.979-.196-2.512-.73a.326.326 0 0 0-.232-.095z"
      />
    </Svg>
  );
}

export function XIcon({ className }: LogoProps) {
  return (
    <Svg className={className}>
      <path d="M18.244 2.25h3.308l-7.227 8.26 8.502 11.24H16.17l-5.214-6.817L4.99 21.75H1.68l7.73-8.835L1.254 2.25H8.08l4.713 6.231zm-1.161 17.52h1.833L7.084 4.126H5.117z" />
    </Svg>
  );
}

export function LinkedInIcon({ className }: LogoProps) {
  return (
    <Svg className={className}>
      <path d="M20.447 20.452h-3.554v-5.569c0-1.328-.027-3.037-1.852-3.037-1.853 0-2.136 1.445-2.136 2.939v5.667H9.351V9h3.414v1.561h.046c.477-.9 1.637-1.85 3.37-1.85 3.601 0 4.267 2.37 4.267 5.455v6.286zM5.337 7.433a2.062 2.062 0 01-2.063-2.065 2.064 2.064 0 112.063 2.065zm1.782 13.019H3.555V9h3.564v11.452zM22.225 0H1.771C.792 0 0 .774 0 1.729v20.542C0 23.227.792 24 1.771 24h20.451C23.2 24 24 23.227 24 22.271V1.729C24 .774 23.2 0 22.222 0h.003z" />
    </Svg>
  );
}

export function HackerNewsIcon({ className }: LogoProps) {
  return (
    <Svg className={className}>
      <path d="M0 24V0h24v24H0zM6.951 5.896l4.9 8.876v5.283h1.53v-5.219l4.999-8.94H16.75l-3.129 5.855c-.117.24-.242.483-.354.775 0 0-.235-.564-.353-.775L9.783 5.896H6.951z" />
    </Svg>
  );
}

export function DevtoIcon({ className }: LogoProps) {
  return (
    <Svg className={className}>
      <path d="M7.42 10.05c-.18-.16-.46-.23-.84-.23H6l.02 2.44.04 2.45.56-.02c.41 0 .63-.07.83-.26.24-.24.26-.36.26-2.2 0-1.91-.02-1.96-.29-2.18zM0 4.94v14.12h24V4.94H0zM8.56 15.3c-.44.58-1.06.77-2.53.77H4.71V8.53h1.4c1.67 0 2.16.18 2.6.9.27.43.29.6.32 2.57.05 2.23-.02 2.73-.47 3.3zm5.09-5.47h-2.47v1.77h1.51v1.28l-.75.04-.76.03v1.77l1.22.03 1.23.04v1.28h-1.6c-1.53 0-1.61-.01-1.9-.3l-.3-.28v-3.16c0-3.02.01-3.18.25-3.48.23-.31.25-.31 1.88-.31h1.64v1.3zm4.68 5.45c-.17.43-.64.79-1 .79-.18 0-.45-.15-.67-.39-.32-.32-.45-.63-.82-2.08l-.9-3.39-.45-1.67h.76c.4 0 .75.02.75.05 0 .06 1.16 4.54 1.26 4.83.04.15.32-.7.73-2.3l.66-2.52.74-.04c.4-.02.73 0 .73.04 0 .14-1.67 6.38-1.8 6.68z" />
    </Svg>
  );
}

export function QuoraIcon({ className }: LogoProps) {
  return (
    <Svg className={className}>
      <path d="M12.738 18.701c-.831-1.635-1.805-3.287-3.708-3.287-.362 0-.727.06-1.06.209l-.646-1.291c.786-.674 2.058-1.207 3.688-1.207 2.544 0 3.858 1.225 4.911 2.766.628-.906.946-2.16.946-3.849 0-4.223-1.324-6.376-4.406-6.376-3.056 0-4.371 2.171-4.371 6.376 0 4.181 1.315 6.322 4.371 6.322.259 0 .506-.012.734-.036l-.35-.688-.109.061zm.848 1.671c-.502.086-1.043.132-1.622.132C7.539 20.604 3 17.294 3 12c0-5.35 4.539-8.66 8.965-8.66C16.512 3.34 21 6.616 21 12c0 3.008-1.412 5.481-3.588 7.011.685.949 1.406 1.579 2.437 1.579.911 0 1.28-.703 1.342-1.257h1.472c.086.795-.309 3.927-4.019 3.927-2.246 0-3.435-1.301-4.058-2.888z" />
    </Svg>
  );
}

// Simple lettermarks for platforms without a standard single-path silhouette.
export function LobstersIcon({ className }: LogoProps) {
  return (
    <Svg className={className}>
      <path d="M4 3h16v18H4z" fill="none" stroke="currentColor" strokeWidth="2" />
      <path d="M8.6 6.8h2.9v7.8h4.1v2.6H8.6z" />
    </Svg>
  );
}

export function IndieHackersIcon({ className }: LogoProps) {
  return (
    <Svg className={className}>
      <rect x="3" y="4" width="3.2" height="16" />
      <rect x="10" y="4" width="3.2" height="16" />
      <rect x="17.8" y="4" width="3.2" height="16" />
      <rect x="10" y="10.4" width="11" height="3.2" />
    </Svg>
  );
}
