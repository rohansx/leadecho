import { createFileRoute } from "@tanstack/react-router";
import { MarketingNav } from "@/components/marketing/nav";
import { Hero } from "@/components/marketing/hero";
import { ThreadDemo } from "@/components/marketing/thread-demo";
import { FeatureRows } from "@/components/marketing/feature-rows";
import { Extras } from "@/components/marketing/extras";
import { PricingSection } from "@/components/marketing/pricing-and-comparison";
import { TerminalInstall } from "@/components/marketing/terminal-install";
import { FaqSection } from "@/components/marketing/faq";
import { FinalCta, MarketingFooter } from "@/components/marketing/closing";
import { FeedbackWidget } from "@/components/feedback-widget";
import { useAuth } from "@/lib/auth";

export const Route = createFileRoute("/")({
  component: LandingPage,
});

function LandingPage() {
  const { user } = useAuth();
  return (
    <div className="mkt relative isolate min-h-screen overflow-x-clip bg-background text-foreground">
      <div className="mkt-aura" aria-hidden="true" />
      {/* Signed-in visitors only: the widget config needs a session, and Lobby then
          shows its button only to invited reporters (testers, claimed prospects). */}
      {user && <FeedbackWidget />}
      <MarketingNav />
      <main>
        <Hero />
        <ThreadDemo />
        <FeatureRows />
        <Extras />
        <PricingSection />
        <TerminalInstall />
        <FaqSection />
        <FinalCta />
      </main>
      <MarketingFooter />
    </div>
  );
}
