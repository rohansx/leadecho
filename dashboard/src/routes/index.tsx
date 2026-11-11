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

export const Route = createFileRoute("/")({
  component: LandingPage,
});

function LandingPage() {
  return (
    <div className="mkt min-h-screen overflow-x-clip bg-background text-foreground">
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
