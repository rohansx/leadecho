import { Easing, interpolate, useCurrentFrame } from "remotion";
import { C, MONO, card, enter, rise } from "./theme";
import { push } from "./scenes-b";

const clamp = { extrapolateLeft: "clamp", extrapolateRight: "clamp" } as const;

const DRAFT =
  "We hit the same wall. Keyword alerts only catch your brand name, so we match posts that describe the problem instead, and read through a queue so it stays under Reddit's API limits. It fits a $50–100 budget. Happy to show you how we set it up.";

const SOURCES = ["docs/how-matching-works.md", "docs/rate-limits.md", "docs/pricing.md"];

// Button centre inside the main pane; the cursor lands here.
const BTN = { x: 1160, y: 604 };
const CLICK = 122;

function Cursor({ x, y }: { x: number; y: number }) {
  return (
    <svg width="30" height="30" viewBox="0 0 24 24" style={{ position: "absolute", left: x, top: y }}>
      <path d="M4 2l15 9-6.5 1.5L10 20z" fill={C.fg} stroke={C.bg} strokeWidth="1.5" strokeLinejoin="round" />
    </svg>
  );
}

/** Scene 5: the reply types out, then the cursor approves it. */
export function Draft() {
  const f = useCurrentFrame();
  const chars = Math.floor(interpolate(f, [16, 96], [0, DRAFT.length], clamp));
  const move = interpolate(f, [92, CLICK - 4], [0, 1], { ...clamp, easing: Easing.bezier(0.4, 0, 0.2, 1) });
  const press = interpolate(f, [CLICK - 2, CLICK, CLICK + 4], [1, 0.96, 1], clamp);
  const posted = interpolate(f, [CLICK + 4, CLICK + 9], [0, 1], clamp);
  return (
    <div style={push(f, 60, 170, "50% 45%")}>
      <div style={{ position: "absolute", left: 36, top: 30, right: 36, fontSize: 19, color: C.muted }}>
        Replying to <span style={{ color: C.soft, fontWeight: 500 }}>u/build_in_public</span> in r/SaaS
      </div>
      <div style={{ ...card, position: "absolute", left: 36, top: 76, width: 1206, height: 560, ...rise(enter(f, 4)) }}>
        <div style={{ display: "flex", padding: "16px 24px", borderBottom: `1px solid ${C.border}`, fontSize: 19 }}>
          <span style={{ fontWeight: 600 }}>Draft reply</span>
          <span style={{ marginLeft: "auto", color: C.muted }}>written from your docs</span>
        </div>
        <p style={{ margin: 0, padding: "22px 24px", fontSize: 28, lineHeight: 1.55 }}>
          {DRAFT.slice(0, chars)}
          <span style={{ display: "inline-block", width: 2, height: 31, marginLeft: 2, verticalAlign: "-5px", background: C.fg, opacity: f % 30 < 15 && posted < 1 ? 1 : 0 }} />
        </p>
        <div style={{ position: "absolute", left: 24, right: 24, bottom: 100, display: "flex", gap: 10 }}>
          {SOURCES.map((s, i) => (
            <span
              key={s}
              style={{ fontFamily: MONO, fontSize: 15, color: C.soft, border: `1px solid ${C.border}`, borderRadius: 8, padding: "4px 10px", opacity: enter(f, 30 + i * 22) }}
            >
              {s}
            </span>
          ))}
        </div>
        <div style={{ position: "absolute", left: 0, right: 0, bottom: 0, height: 76, borderTop: `1px solid ${C.border}`, display: "flex", alignItems: "center", justifyContent: "flex-end", gap: 12, padding: "0 24px" }}>
          <span style={{ marginRight: "auto", fontSize: 18, color: C.success, opacity: posted }}>✓ Posted to r/SaaS</span>
          <span style={{ fontSize: 19, fontWeight: 500, color: C.soft, border: `1px solid ${C.border}`, borderRadius: 10, padding: "10px 20px" }}>Edit</span>
          <span
            style={{
              width: 212,
              textAlign: "center",
              fontSize: 19,
              fontWeight: 600,
              borderRadius: 10,
              padding: "11px 0",
              background: C.gold,
              color: C.goldFg,
              transform: `scale(${press})`,
              opacity: 1 - 0.5 * posted,
            }}
          >
            Approve and post
          </span>
        </div>
      </div>
      <Cursor x={interpolate(move, [0, 1], [760, BTN.x])} y={interpolate(move, [0, 1], [730, BTN.y])} />
    </div>
  );
}

const TILES = [
  { n: "61", label: "replies posted", sub: "each one approved by you" },
  { n: "14", label: "signed up", sub: "from those replies" },
  { n: "$4,872", label: "revenue added", sub: "from those signups" },
];

const PATH = [
  ["2,184", "posts collected"],
  ["73", "ready to buy"],
  ["61", "replies posted"],
  ["14", "signed up"],
];

/** Scene 6: what it added up to. Values are final from the first frame they are visible. */
export function Outcome() {
  const f = useCurrentFrame();
  return (
    <div style={{ padding: "28px 36px" }}>
      <div style={{ fontSize: 22, fontWeight: 600 }}>Results</div>
      <div style={{ display: "flex", gap: 20, marginTop: 26 }}>
        {TILES.map((t, i) => (
          <div key={t.label} style={{ ...card, flex: 1, padding: "36px 28px 40px", ...rise(enter(f, 6 + i * 10), 20) }}>
            <div style={{ fontSize: 21, color: C.soft }}>{t.label}</div>
            <div style={{ marginTop: 30, fontFamily: MONO, fontWeight: 600, fontSize: 88, lineHeight: 1, letterSpacing: "-0.03em", fontVariantNumeric: "tabular-nums" }}>{t.n}</div>
            <div style={{ marginTop: 30, fontSize: 19, color: C.muted }}>{t.sub}</div>
          </div>
        ))}
      </div>
      <div style={{ ...card, display: "flex", marginTop: 20, ...rise(enter(f, 38), 20) }}>
        {PATH.map(([n, label], i) => (
          <div key={label} style={{ flex: 1, padding: "26px 28px", borderLeft: i ? `1px solid ${C.border}` : undefined }}>
            <div style={{ fontFamily: MONO, fontWeight: 600, fontSize: 38, lineHeight: 1, fontVariantNumeric: "tabular-nums" }}>{n}</div>
            <div style={{ marginTop: 12, fontSize: 19, color: C.muted }}>{label}</div>
          </div>
        ))}
      </div>
      <div style={{ marginTop: 24, fontSize: 20, color: C.muted, opacity: enter(f, 52) }}>
        2,184 posts read. You only saw the 73 that mattered.
      </div>
    </div>
  );
}
