import type { CSSProperties } from "react";
import { spring } from "remotion";
import { loadFont as loadGeist } from "@remotion/google-fonts/Geist";
import { loadFont as loadGeistMono } from "@remotion/google-fonts/GeistMono";

const sans = loadGeist("normal", { weights: ["400", "500", "600"], subsets: ["latin"] });
const monoFont = loadGeistMono("normal", { weights: ["500", "600"], subsets: ["latin"] });

export const FONT = `${sans.fontFamily}, system-ui, sans-serif`;
export const MONO = `${monoFont.fontFamily}, monospace`;

// The dashboard's .dark tokens (dashboard/src/globals.css). Keep in sync by hand.
export const C = {
  page: "#0b0b0c",
  bg: "#09090b",
  card: "#111113",
  secondary: "#18181b",
  border: "#27272a",
  fg: "#fafafa",
  soft: "#d4d4d8",
  muted: "#a1a1aa",
  gold: "#f0b429",
  goldFg: "#1c1405",
  marker: "#ffe58f",
  markerFg: "#1c1a14",
  success: "#4ade80",
};

export const FPS = 30;
export const FADE = 10;

// Scene start frames. Each scene runs until the next start, plus FADE of crossfade.
export const T = { s1: 0, s2: 125, s3: 245, s4: 400, s5: 510, s6: 680, end: 790 };

// The app's main pane inside the browser window, in composition pixels.
export const MAIN = { left: 261, top: 81, width: 1278, height: 774 };

/** 0 -> 1 entrance spring starting at `delay`. */
export function enter(frame: number, delay = 0): number {
  return spring({ frame: frame - delay, fps: FPS, config: { damping: 200 } });
}

export function rise(p: number, dy = 14): CSSProperties {
  return { opacity: p, transform: `translateY(${(1 - p) * dy}px)` };
}

export const num = (n: number): string => Math.round(n).toLocaleString("en-US");

export const card: CSSProperties = {
  background: C.card,
  border: `1px solid ${C.border}`,
  borderRadius: 16,
};

export const chip: CSSProperties = {
  fontFamily: MONO,
  fontWeight: 600,
  fontVariantNumeric: "tabular-nums",
  background: C.gold,
  color: C.goldFg,
  borderRadius: 999,
  padding: "2px 12px",
};
