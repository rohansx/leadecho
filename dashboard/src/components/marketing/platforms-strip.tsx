import { useEffect, useRef } from "react";
import {
  useAnimate,
  useReducedMotion,
  type AnimationPlaybackControls,
} from "motion/react";
import {
  RedditIcon,
  XIcon,
  LinkedInIcon,
  HackerNewsIcon,
  DevtoIcon,
  LobstersIcon,
  IndieHackersIcon,
  QuoraIcon,
} from "./platform-logos";

const platforms = [
  { name: "Reddit", Icon: RedditIcon },
  { name: "X", Icon: XIcon },
  { name: "LinkedIn", Icon: LinkedInIcon },
  { name: "Hacker News", Icon: HackerNewsIcon },
  { name: "Dev.to", Icon: DevtoIcon },
  { name: "Lobsters", Icon: LobstersIcon },
  { name: "Indie Hackers", Icon: IndieHackersIcon },
  { name: "Quora", Icon: QuoraIcon },
];

// Render the list twice back-to-back; equal per-item horizontal padding (no flex
// gap) keeps spacing uniform across the seam so translateX -50% wraps gaplessly.
const track = [...platforms, ...platforms];

const fade =
  "linear-gradient(to right, transparent, black 6%, black 94%, transparent)";

export function PlatformsStrip() {
  const reduce = useReducedMotion();
  const [scope, animate] = useAnimate();
  const controls = useRef<AnimationPlaybackControls | null>(null);

  useEffect(() => {
    if (reduce) return;
    const anim = animate(
      scope.current,
      { x: ["0%", "-50%"] },
      { duration: 30, ease: "linear", repeat: Infinity },
    );
    controls.current = anim;
    return () => anim.stop();
  }, [reduce, animate, scope]);

  return (
    <div
      aria-label="Monitored platforms"
      onMouseEnter={() => controls.current?.pause()}
      onMouseLeave={() => controls.current?.play()}
      className="border-y border-border bg-card overflow-hidden py-4"
      style={{ maskImage: fade, WebkitMaskImage: fade }}
    >
      <div ref={scope} className="flex w-max">
        {(reduce ? platforms : track).map(({ name, Icon }, i) => (
          <div
            key={`${name}-${i}`}
            className="flex shrink-0 items-center gap-2 px-6 text-sm text-muted-foreground font-[family-name:var(--font-mono)]"
          >
            <Icon className="size-5 shrink-0" />
            <span>{name}</span>
          </div>
        ))}
      </div>
    </div>
  );
}
