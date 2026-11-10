import { createFileRoute } from "@tanstack/react-router";
import { MarketingNav } from "@/components/marketing/nav";
import { Hero } from "@/components/marketing/hero";
import { HowItWorks } from "@/components/marketing/how-it-works";
import { LeadAndReply } from "@/components/marketing/lead-and-reply";
import { Attribution } from "@/components/marketing/attribution";
import { FeaturesGrid } from "@/components/marketing/features-grid";
import { PricingSection, ComparisonSection } from "@/components/marketing/pricing-and-comparison";
import { TerminalInstall } from "@/components/marketing/terminal-install";
import { FaqSection } from "@/components/marketing/faq";
import { FinalCta, MarketingFooter } from "@/components/marketing/closing";

export const Route = createFileRoute("/")({
  component: LandingPage,
});

function LandingPage() {
  return (
    <div className="mkt min-h-screen bg-background text-foreground">
      <MarketingNav />
      <main>
        <Hero />
        <HowItWorks />
        <LeadAndReply />
        <Attribution />
        <FeaturesGrid />
        <PricingSection />
        <ComparisonSection />
        <TerminalInstall />
        <FaqSection />
        <FinalCta />
      </main>
      <MarketingFooter />
    </div>
  );
}
