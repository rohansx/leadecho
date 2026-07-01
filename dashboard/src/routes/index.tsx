import { createFileRoute } from "@tanstack/react-router";
import { MarketingNav } from "@/components/marketing/nav";
import { Hero } from "@/components/marketing/hero";
import { PlatformsStrip } from "@/components/marketing/platforms-strip";
import { SubheroLine, EditorialStats, TheShift } from "@/components/marketing/stats-and-shift";
import { Person360Section } from "@/components/marketing/person360";
import { ConversationThreadSection } from "@/components/marketing/conversation-thread";
import { AttributionSection, PipelineFlowSection } from "@/components/marketing/attribution-and-pipeline";
import { FeaturesGrid } from "@/components/marketing/features-grid";
import { TerminalInstall } from "@/components/marketing/terminal-install";
import { Testimonials } from "@/components/marketing/testimonials";
import { PricingSection, ComparisonSection } from "@/components/marketing/pricing-and-comparison";
import { FaqSection } from "@/components/marketing/faq";
import { CommunitySection, FinalCta, MarketingFooter } from "@/components/marketing/closing";

export const Route = createFileRoute("/")({
  component: LandingPage,
});

function LandingPage() {
  return (
    <div className="min-h-screen bg-background text-foreground">
      <MarketingNav />
      <Hero />
      <PlatformsStrip />
      <SubheroLine />
      <EditorialStats />
      <TheShift />
      <Person360Section />
      <ConversationThreadSection />
      <AttributionSection />
      <PipelineFlowSection />
      <FeaturesGrid />
      <TerminalInstall />
      <Testimonials />
      <PricingSection />
      <ComparisonSection />
      <FaqSection />
      <CommunitySection />
      <FinalCta />
      <MarketingFooter />
    </div>
  );
}
