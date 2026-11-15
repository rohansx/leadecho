import { interpolate, useCurrentFrame } from "remotion";
import { C, MONO, chip, enter, num, rise } from "./theme";

const clamp = { extrapolateLeft: "clamp", extrapolateRight: "clamp" } as const;

interface Post {
  tag: string;
  where: string;
  text: string;
  score?: string;
}

// Fictional sample posts, same set as the landing page feed. Most are noise.
const POSTS: Post[] = [
  { tag: "r", where: "r/webdev", text: "Is it just me or is CSS getting harder every year" },
  { tag: "Y", where: "Hacker News", text: "Show HN: I built a tiny static site generator in Rust" },
  { tag: "r", where: "r/SaaS", text: "Budget is $50–100/mo, we just need something that watches Reddit for our problem", score: "9.2" },
  { tag: "DEV", where: "Dev.to", text: "10 VS Code extensions I can't live without" },
  { tag: "L", where: "Lobsters", text: "A deep dive into Postgres vacuum internals" },
  { tag: "X", where: "X", text: "anyone have a tool that flags buying intent instead of brand mentions?", score: "8.8" },
  { tag: "r", where: "r/startups", text: "Unpopular opinion: most landing pages are too long" },
  { tag: "Y", where: "Hacker News", text: "Ask HN: What is your favourite terminal font?" },
  { tag: "in", where: "LinkedIn", text: "My team spends five hours a week scrolling Reddit for leads. Open to tools.", score: "9.0" },
  { tag: "IH", where: "Indie Hackers", text: "Month 3 revenue update and what I would do differently" },
  { tag: "X", where: "X", text: "hot take: tabs are fine, your editor is the problem" },
];

const ROW = 70;
const SPEED = 5; // px per frame
const FIRST = 9; // row index sitting at the top on frame 0

/** Scene 1: posts pour in at the top; each is judged as it moves down. */
export function Stream() {
  const f = useCurrentFrame();
  const rows = Array.from({ length: 26 }, (_, i) => i);
  const collected = 2184 - Math.max(0, Math.round(((130 - f) * SPEED) / ROW));
  return (
    <div style={{ padding: "28px 36px" }}>
      <div style={{ display: "flex", alignItems: "center", gap: 12, fontSize: 22, fontWeight: 600 }}>
        <span style={{ width: 9, height: 9, borderRadius: 9, background: C.fg, opacity: 0.55 + 0.45 * Math.sin(f / 5) }} />
        Incoming
        <span style={{ marginLeft: "auto", fontSize: 18, fontWeight: 400, color: C.muted }}>
          <span style={{ fontFamily: MONO, fontVariantNumeric: "tabular-nums", color: C.soft }}>{num(Math.min(2184, collected))}</span> collected today
        </span>
      </div>
      <div style={{ position: "relative", marginTop: 20, height: 660, overflow: "hidden", borderTop: `1px solid ${C.border}` }}>
        {rows.map((i) => {
          const y = (FIRST - i) * ROW + f * SPEED;
          if (y < -ROW || y > 680) return null;
          const p = POSTS[i % POSTS.length];
          const judged = interpolate(y, [70, 140], [0, 1], clamp);
          return (
            <div
              key={i}
              style={{
                position: "absolute",
                top: y,
                left: 0,
                right: 0,
                height: ROW,
                display: "flex",
                alignItems: "center",
                gap: 16,
                borderBottom: `1px solid ${C.border}`,
                opacity: p.score ? 1 : 1 - 0.62 * judged,
              }}
            >
              <span
                style={{
                  width: 44,
                  height: 30,
                  borderRadius: 8,
                  border: `1px solid ${C.border}`,
                  fontFamily: MONO,
                  fontSize: 14,
                  color: C.soft,
                  display: "flex",
                  alignItems: "center",
                  justifyContent: "center",
                  flexShrink: 0,
                }}
              >
                {p.tag}
              </span>
              <span style={{ width: 150, fontSize: 17, color: C.muted, flexShrink: 0 }}>{p.where}</span>
              <span style={{ flex: 1, fontSize: 20, whiteSpace: "nowrap", overflow: "hidden", textOverflow: "ellipsis" }}>{p.text}</span>
              <span style={{ width: 90, textAlign: "right", flexShrink: 0, opacity: judged }}>
                {p.score ? <span style={{ ...chip, fontSize: 18 }}>{p.score}</span> : <span style={{ fontSize: 16, color: C.muted }}>skipped</span>}
              </span>
            </div>
          );
        })}
        <div style={{ position: "absolute", left: 0, right: 0, bottom: 0, height: 120, background: `linear-gradient(transparent, ${C.bg})` }} />
      </div>
    </div>
  );
}

const STAGES = [
  { n: 2184, label: "collected", dropped: "" },
  { n: 966, label: "not spam", dropped: "1,218 spam removed" },
  { n: 459, label: "match your problem", dropped: "507 off topic" },
  { n: 73, label: "ready to buy", dropped: "386 not buying yet" },
];
const BAR_W = 1206;

/** Scene 2: the funnel. Each stage arrives showing the previous count and ticks down to its own. */
export function Funnel() {
  const f = useCurrentFrame();
  return (
    <div style={{ padding: "28px 36px" }}>
      <div style={{ fontSize: 22, fontWeight: 600 }}>Today</div>
      {STAGES.map((s, k) => {
        const start = 6 + k * 24;
        const p = enter(f, start);
        const prev = k === 0 ? s.n : STAGES[k - 1].n;
        const v = interpolate(f, [start + 4, start + 24], [prev, s.n], clamp);
        const last = k === STAGES.length - 1;
        return (
          <div key={s.label} style={{ marginTop: k === 0 ? 30 : 46, ...rise(p) }}>
            <div style={{ display: "flex", alignItems: "baseline", gap: 18 }}>
              <span style={{ fontFamily: MONO, fontWeight: 600, fontVariantNumeric: "tabular-nums", fontSize: last ? 88 : 64, lineHeight: 1, width: 260 }}>
                {num(v)}
              </span>
              <span style={{ fontSize: 26, color: last ? C.fg : C.soft, fontWeight: last ? 500 : 400 }}>{s.label}</span>
              <span style={{ marginLeft: "auto", fontSize: 18, color: C.muted, opacity: interpolate(f, [start + 20, start + 30], [0, 1], clamp) }}>
                {s.dropped}
              </span>
            </div>
            <div style={{ marginTop: 18, height: 18, borderRadius: 9, background: C.secondary, width: BAR_W }}>
              <div style={{ height: 18, borderRadius: 9, background: last ? C.fg : C.muted, width: Math.max(18, (v / 2184) * BAR_W) }} />
            </div>
          </div>
        );
      })}
    </div>
  );
}
