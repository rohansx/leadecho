import type { CSSProperties, ReactNode } from "react";
import { interpolate, interpolateColors, useCurrentFrame } from "remotion";
import { C, MONO, card, chip, enter, rise } from "./theme";

const clamp = { extrapolateLeft: "clamp", extrapolateRight: "clamp" } as const;

/** Gentle camera push-in on the focused element. */
export function push(f: number, from: number, to: number, origin: string): CSSProperties {
  return { transform: `scale(${interpolate(f, [from, to], [1, 1.04], clamp)})`, transformOrigin: origin, width: "100%", height: "100%" };
}

function Mark({ p, children }: { p: number; children: ReactNode }) {
  return (
    <span
      style={{
        background: `linear-gradient(${C.marker}, ${C.marker}) no-repeat 0 0 / ${p * 100}% 100%`,
        color: interpolateColors(p, [0.2, 0.8], [C.fg, C.markerFg]),
        padding: "0 3px",
        borderRadius: 3,
      }}
    >
      {children}
    </span>
  );
}

function Meta({ user, age, meta }: { user: string; age: string; meta: string }) {
  return (
    <div style={{ display: "flex", gap: 12, fontSize: 17, color: C.muted }}>
      <span style={{ fontWeight: 500, color: C.soft }}>{user}</span>
      <span>{age}</span>
      <span style={{ marginLeft: "auto" }}>{meta}</span>
    </div>
  );
}

/** The buyer's reply. `m1`/`m2` are highlight progress, `score` is the score row's entrance. */
export function BuyerReply({ m1, m2, score, size = 21 }: { m1: number; m2: number; score: number; size?: number }) {
  return (
    <div style={{ border: `1px solid ${C.muted}`, borderRadius: 12, padding: "16px 18px" }}>
      <Meta user="u/build_in_public" age="2h" meta="12 points" />
      <p style={{ margin: "6px 0 0", fontSize: size, lineHeight: 1.55 }}>
        We tried that and kept missing threads. We need something that <Mark p={m1}>watches Reddit for people describing our problem</Mark>, not just
        our brand name, and it can't keep hitting the API limits. <Mark p={m2}>Budget is $50–100/mo</Mark>.
      </p>
      <div style={{ marginTop: 14, paddingTop: 14, borderTop: `1px solid ${C.border}`, display: "flex", alignItems: "center", gap: 14, fontSize: 18, ...rise(score, 8) }}>
        <span style={{ ...chip, fontSize: 21, transform: `scale(${0.7 + 0.3 * score})` }}>9.2</span>
        <span style={{ color: C.soft }}>Ready to buy. Matches “Reddit monitoring hits rate limits”.</span>
      </div>
    </div>
  );
}

const INBOX = [
  { where: "r/SaaS", title: "What are you all using to find customers on Reddit?", score: "9.2" },
  { where: "LinkedIn", title: "My team spends five hours a week scrolling Reddit for leads", score: "9.0" },
  { where: "X", title: "anyone have a tool that flags buying intent?", score: "8.8" },
  { where: "Indie Hackers", title: "Paying for social listening and it's 90% noise", score: "8.4" },
  { where: "Hacker News", title: "How did you get your first 100 users for a dev tool?", score: "8.1" },
];

function Noise({ dim, user, age, meta, text }: { dim: number; user: string; age: string; meta: string; text: string }) {
  return (
    <div style={{ opacity: 1 - 0.62 * dim, padding: "0 18px" }}>
      <Meta user={user} age={age} meta={meta} />
      <p style={{ margin: "6px 0 0", fontSize: 21, lineHeight: 1.55 }}>{text}</p>
    </div>
  );
}

/** Scene 3: the inbox. The thread is read, the buyer's sentence is marked, the score lands. */
export function Thread() {
  const f = useCurrentFrame();
  const dim = interpolate(f, [28, 48], [0, 1], clamp);
  const m1 = interpolate(f, [46, 72], [0, 1], clamp);
  const m2 = interpolate(f, [74, 90], [0, 1], clamp);
  const score = enter(f, 96);
  return (
    <div>
      <div style={{ display: "flex", height: 774 }}>
        <div style={{ width: 350, borderRight: `1px solid ${C.border}`, padding: "24px 0" }}>
          <div style={{ padding: "0 22px 14px", fontSize: 22, fontWeight: 600 }}>Inbox</div>
          {INBOX.map((t, i) => (
            <div key={t.title} style={{ padding: "13px 22px", background: i === 0 ? C.secondary : "transparent", borderTop: `1px solid ${C.border}` }}>
              <div style={{ display: "flex", justifyContent: "space-between", fontSize: 16, color: C.muted }}>
                {t.where}
                <span style={{ fontFamily: MONO, fontVariantNumeric: "tabular-nums", color: C.soft }}>{t.score}</span>
              </div>
              <div style={{ marginTop: 3, fontSize: 18, color: i === 0 ? C.fg : C.soft, whiteSpace: "nowrap", overflow: "hidden", textOverflow: "ellipsis" }}>
                {t.title}
              </div>
            </div>
          ))}
        </div>
        <div style={{ flex: 1, padding: "24px 36px 24px 30px", minWidth: 0 }}>
          <div style={{ ...card, overflow: "hidden", ...push(f, 30, 150, "50% 50%"), height: "auto" }}>
            <div style={{ display: "flex", gap: 12, padding: "15px 22px", borderBottom: `1px solid ${C.border}`, fontSize: 18, color: C.muted }}>
              <span style={{ fontWeight: 500, color: C.fg }}>r/SaaS</span>
              <span>What are you all using to find customers on Reddit?</span>
            </div>
            <div style={{ padding: "22px 14px", display: "flex", flexDirection: "column", gap: 22 }}>
              <Noise dim={dim} user="u/growthmarco" age="3h" meta="41 points" text="Honestly just search the sub every morning with coffee. Works fine at our size." />
              <BuyerReply m1={m1} m2={m2} score={score} />
              <Noise dim={dim} user="u/no_ads_please" age="2h" meta="33 points" text="Please, no more replies that are just a referral link." />
            </div>
            <div style={{ padding: "14px 22px", borderTop: `1px solid ${C.border}`, fontSize: 17, color: C.muted }}>
              LeadEcho read all 214 replies. One person is looking to buy.
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}

const FACTS = [
  ["Role", "Founder and engineer"],
  ["Company", "Cursive Labs"],
  ["Stage", "Comparing vendors"],
  ["Budget", "Has a budget, $50–100/mo"],
  ["Seen in", "r/SaaS, 2 hours ago"],
  ["Matches", "Reddit monitoring hits rate limits"],
];

/** Scene 4: who is asking. */
export function Person() {
  const f = useCurrentFrame();
  const p = enter(f, 8);
  return (
    <div>
      <div style={{ display: "flex", gap: 36, padding: "36px 36px", alignItems: "flex-start" }}>
        <div style={{ width: 500, flexShrink: 0 }}>
          <BuyerReply m1={1} m2={1} score={1} size={19} />
        </div>
        <div style={{ flex: 1, ...push(f, 25, 110, "50% 40%"), height: "auto" }}>
        <div style={{ ...card, padding: 30, ...rise(p, 20) }}>
          <div style={{ display: "flex", alignItems: "center", gap: 18 }}>
            <span
              style={{
                width: 68,
                height: 68,
                borderRadius: 68,
                background: C.secondary,
                border: `1px solid ${C.border}`,
                display: "flex",
                alignItems: "center",
                justifyContent: "center",
                fontSize: 24,
                fontWeight: 600,
                color: C.soft,
              }}
            >
              DR
            </span>
            <div style={{ minWidth: 0 }}>
              <div style={{ fontSize: 32, fontWeight: 600, letterSpacing: "-0.02em", lineHeight: 1.2 }}>Daniel Reyes</div>
              <div style={{ fontSize: 19, color: C.muted }}>Founder and engineer at Cursive Labs</div>
            </div>
            <div style={{ marginLeft: "auto", textAlign: "right" }}>
              <div style={{ fontFamily: MONO, fontWeight: 600, fontSize: 46, lineHeight: 1, color: C.gold, fontVariantNumeric: "tabular-nums" }}>88%</div>
              <div style={{ fontSize: 17, color: C.muted, marginTop: 4 }}>fit</div>
            </div>
          </div>
          <div style={{ marginTop: 24, borderTop: `1px solid ${C.border}` }}>
            {FACTS.map(([k, v], i) => (
              <div
                key={k}
                style={{ display: "flex", padding: "17px 0", borderBottom: `1px solid ${C.border}`, fontSize: 22, ...rise(enter(f, 24 + i * 9), 8) }}
              >
                <span style={{ width: 160, color: C.muted }}>{k}</span>
                <span>{v}</span>
              </div>
            ))}
          </div>
          <div style={{ marginTop: 18, fontSize: 17, color: C.muted, opacity: enter(f, 80) }}>Researched from the public profile behind u/build_in_public.</div>
        </div>
        </div>
      </div>
    </div>
  );
}
