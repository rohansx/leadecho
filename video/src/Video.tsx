import type { ReactNode } from "react";
import { AbsoluteFill, Sequence, interpolate, useCurrentFrame } from "remotion";
import { C, FADE, FONT, MAIN, MONO, T } from "./theme";
import { Stream, Funnel } from "./scenes-a";
import { Thread, Person } from "./scenes-b";
import { Draft, Outcome } from "./scenes-c";

const NAV = ["Inbox", "Leads", "Prospects", "Replies", "Analytics", "Settings"];

/** Browser frame + app sidebar. Static, so the loop point is seamless. */
function Shell() {
  return (
    <div
      style={{
        position: "absolute",
        left: 60,
        top: 36,
        width: 1480,
        height: 820,
        background: C.bg,
        border: `1px solid ${C.border}`,
        borderRadius: 14,
        overflow: "hidden",
      }}
    >
      <div style={{ height: 44, borderBottom: `1px solid ${C.border}`, display: "flex", alignItems: "center", padding: "0 18px", gap: 8 }}>
        {[0, 1, 2].map((i) => (
          <span key={i} style={{ width: 11, height: 11, borderRadius: 99, background: C.border }} />
        ))}
        <div
          style={{
            margin: "0 auto",
            width: 420,
            height: 26,
            borderRadius: 8,
            background: C.secondary,
            color: C.muted,
            fontSize: 14,
            display: "flex",
            alignItems: "center",
            justifyContent: "center",
          }}
        >
          LeadEcho
        </div>
        <span style={{ fontSize: 14, color: C.muted, width: 90, textAlign: "right" }}>sample data</span>
      </div>
      <div style={{ position: "absolute", top: 44, bottom: 0, left: 0, width: 200, borderRight: `1px solid ${C.border}`, padding: 14 }}>
        <div style={{ display: "flex", alignItems: "center", gap: 10, padding: "8px 10px 18px", fontSize: 19, fontWeight: 600 }}>
          <span style={{ width: 20, height: 20, borderRadius: 6, border: `1.5px solid ${C.fg}` }} />
          LeadEcho
        </div>
        {NAV.map((n, i) => (
          <div
            key={n}
            style={{
              display: "flex",
              justifyContent: "space-between",
              padding: "9px 10px",
              borderRadius: 10,
              fontSize: 17,
              fontWeight: i === 0 ? 500 : 400,
              color: i === 0 ? C.fg : C.muted,
              background: i === 0 ? C.secondary : "transparent",
            }}
          >
            {n}
            {i === 0 && <span style={{ fontFamily: MONO, fontSize: 15, color: C.soft }}>73</span>}
          </div>
        ))}
      </div>
    </div>
  );
}

interface SceneProps {
  from: number;
  to: number;
  caption: string;
  children: ReactNode;
}

function SceneBody({ dur, caption, children }: { dur: number; caption: string; children: ReactNode }) {
  const f = useCurrentFrame();
  const opacity = interpolate(f, [0, FADE, dur, dur + FADE], [0, 1, 1, 0], { extrapolateLeft: "clamp", extrapolateRight: "clamp" });
  return (
    <AbsoluteFill style={{ opacity }}>
      <div style={{ position: "absolute", ...MAIN, overflow: "hidden", borderBottomRightRadius: 13, background: C.bg }}>{children}</div>
      <div style={{ position: "absolute", left: 60, top: 884, fontSize: 31, fontWeight: 500, letterSpacing: "-0.01em", color: C.fg }}>{caption}</div>
    </AbsoluteFill>
  );
}

function Scene({ from, to, caption, children }: SceneProps) {
  return (
    <Sequence from={from} durationInFrames={to - from + FADE}>
      <SceneBody dur={to - from} caption={caption}>
        {children}
      </SceneBody>
    </Sequence>
  );
}

export function Video() {
  return (
    <AbsoluteFill style={{ background: C.page, color: C.fg, fontFamily: FONT, lineHeight: 1.45 }}>
      <Shell />
      <Scene from={T.s1} to={T.s2} caption="Thousands of posts a day.">
        <Stream />
      </Scene>
      <Scene from={T.s2} to={T.s3} caption="Only the buyers get through.">
        <Funnel />
      </Scene>
      <Scene from={T.s3} to={T.s4} caption="It reads the whole thread.">
        <Thread />
      </Scene>
      <Scene from={T.s4} to={T.s5} caption="It knows who is asking.">
        <Person />
      </Scene>
      <Scene from={T.s5} to={T.s6} caption="A reply, drafted from your docs. You approve.">
        <Draft />
      </Scene>
      {/* Last scene ends FADE early so the final frame is the bare shell, same as frame 0. */}
      <Scene from={T.s6} to={T.end - FADE} caption="61 replies. 14 signups. $4,872.">
        <Outcome />
      </Scene>
    </AbsoluteFill>
  );
}
