import { createFileRoute, useNavigate } from "@tanstack/react-router";
import { useState } from "react";
import { useMutation } from "@tanstack/react-query";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { createProfile, updateOnboarding } from "@/lib/api";

export const Route = createFileRoute("/_dashboard/onboarding")({
  component: OnboardingPage,
});

const PLATFORMS = [
  { value: "reddit", label: "Reddit" },
  { value: "hackernews", label: "Hacker News" },
  { value: "twitter", label: "Twitter / X" },
  { value: "linkedin", label: "LinkedIn" },
  { value: "devto", label: "Dev.to" },
  { value: "lobsters", label: "Lobsters" },
  { value: "indiehackers", label: "Indie Hackers" },
];

function OnboardingPage() {
  const navigate = useNavigate();
  const [step, setStep] = useState(1);

  // Collected data
  const [productName, setProductName] = useState("");
  const [productDescription, setProductDescription] = useState("");
  const [painPoints, setPainPoints] = useState("");
  const [competitors, setCompetitors] = useState("");
  const [platforms, setPlatforms] = useState<string[]>(["reddit", "hackernews"]);
  const [subreddits, setSubreddits] = useState("");

  const completeMutation = useMutation({
    mutationFn: async () => {
      await createProfile({
        name: productName,
        description: productDescription,
        pain_points: painPoints.split("\n").map((s) => s.trim()).filter(Boolean),
      });
      await updateOnboarding({ completed: true, step: 4 });
    },
    onSuccess: () => {
      navigate({ to: "/inbox" });
    },
  });

  function togglePlatform(p: string) {
    setPlatforms((prev) =>
      prev.includes(p) ? prev.filter((x) => x !== p) : [...prev, p],
    );
  }

  function next() {
    setStep((s) => s + 1);
  }

  const progress = ((step - 1) / 3) * 100;

  return (
    <div
      style={{
        position: "fixed",
        inset: 0,
        zIndex: 50,
        background: "hsl(var(--background))",
        display: "flex",
        flexDirection: "column",
        alignItems: "center",
        justifyContent: "center",
        padding: "24px",
      }}
    >
      <div style={{ width: "100%", maxWidth: "520px" }}>
        {/* Progress bar */}
        <div
          style={{
            height: "4px",
            background: "hsl(var(--muted))",
            borderRadius: "2px",
            marginBottom: "32px",
            overflow: "hidden",
          }}
        >
          <div
            style={{
              height: "100%",
              width: `${progress}%`,
              background: "#27c17b",
              borderRadius: "2px",
              transition: "width 0.3s ease",
            }}
          />
        </div>

        {step === 1 && (
          <Step title="Tell us about your product" step={1}>
            <label style={labelStyle}>Product name</label>
            <Input
              placeholder="e.g. Acme SaaS"
              value={productName}
              onChange={(e) => setProductName(e.target.value)}
              style={{ marginBottom: "12px" }}
            />
            <label style={labelStyle}>Short description</label>
            <Input
              placeholder="What does it do? Who is it for?"
              value={productDescription}
              onChange={(e) => setProductDescription(e.target.value)}
            />
            <Button
              onClick={next}
              disabled={!productName.trim()}
              style={btnStyle}
            >
              Next →
            </Button>
          </Step>
        )}

        {step === 2 && (
          <Step title="What pain points does your product solve?" step={2}>
            <label style={labelStyle}>Pain points (one per line)</label>
            <textarea
              placeholder={"e.g.\nToo expensive\nHard to set up\nNo good free alternative"}
              value={painPoints}
              onChange={(e) => setPainPoints(e.target.value)}
              rows={5}
              style={textareaStyle}
            />
            <label style={{ ...labelStyle, marginTop: "12px" }}>
              Competitor names (comma-separated, optional)
            </label>
            <Input
              placeholder="e.g. CompetitorA, CompetitorB"
              value={competitors}
              onChange={(e) => setCompetitors(e.target.value)}
            />
            <Button onClick={next} disabled={!painPoints.trim()} style={btnStyle}>
              Next →
            </Button>
          </Step>
        )}

        {step === 3 && (
          <Step title="Where should we monitor?" step={3}>
            <label style={labelStyle}>Platforms</label>
            <div style={{ display: "flex", flexWrap: "wrap", gap: "8px", marginBottom: "16px" }}>
              {PLATFORMS.map((p) => (
                <button
                  key={p.value}
                  onClick={() => togglePlatform(p.value)}
                  style={{
                    padding: "6px 14px",
                    borderRadius: "20px",
                    border: "1px solid",
                    fontSize: "13px",
                    cursor: "pointer",
                    background: platforms.includes(p.value) ? "#27c17b" : "transparent",
                    borderColor: platforms.includes(p.value) ? "#27c17b" : "#444",
                    color: platforms.includes(p.value) ? "#0f1117" : "#ccc",
                    fontWeight: platforms.includes(p.value) ? 600 : 400,
                  }}
                >
                  {p.label}
                </button>
              ))}
            </div>
            {platforms.includes("reddit") && (
              <>
                <label style={labelStyle}>Subreddits to monitor (optional, space-separated)</label>
                <Input
                  placeholder="e.g. SaaS entrepreneur startups"
                  value={subreddits}
                  onChange={(e) => setSubreddits(e.target.value)}
                />
              </>
            )}
            <Button onClick={next} disabled={platforms.length === 0} style={btnStyle}>
              Next →
            </Button>
          </Step>
        )}

        {step === 4 && (
          <Step title="You're all set!" step={4}>
            <p style={{ color: "hsl(var(--muted-foreground))", fontSize: "15px", lineHeight: 1.6, marginBottom: "24px" }}>
              Your monitor is running. New leads will appear in your inbox as
              people post about your space across the web.
            </p>
            <Button
              onClick={() => completeMutation.mutate()}
              disabled={completeMutation.isPending}
              style={btnStyle}
            >
              {completeMutation.isPending ? "Setting up…" : "Open Inbox →"}
            </Button>
          </Step>
        )}
      </div>
    </div>
  );
}

function Step({
  title,
  step,
  children,
}: {
  title: string;
  step: number;
  children: React.ReactNode;
}) {
  return (
    <div>
      <p style={{ fontSize: "12px", color: "#27c17b", fontWeight: 600, marginBottom: "6px", textTransform: "uppercase", letterSpacing: "0.5px" }}>
        Step {step} of 4
      </p>
      <h1 style={{ fontSize: "24px", fontWeight: 700, marginBottom: "20px", lineHeight: 1.2 }}>
        {title}
      </h1>
      {children}
    </div>
  );
}

const labelStyle: React.CSSProperties = {
  display: "block",
  fontSize: "12px",
  color: "#888",
  textTransform: "uppercase",
  letterSpacing: "0.5px",
  marginBottom: "6px",
};

const btnStyle: React.CSSProperties = {
  marginTop: "20px",
  width: "100%",
};

const textareaStyle: React.CSSProperties = {
  width: "100%",
  padding: "8px 10px",
  background: "hsl(var(--input))",
  border: "1px solid hsl(var(--border))",
  borderRadius: "6px",
  color: "hsl(var(--foreground))",
  fontSize: "13px",
  resize: "vertical",
};
