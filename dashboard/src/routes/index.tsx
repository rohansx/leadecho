import { createFileRoute } from "@tanstack/react-router";
import { MarketingNav } from "@/components/marketing/nav";
import { Hero } from "@/components/marketing/hero";
import { Toc } from "@/components/marketing/toc";
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
      {/* The page is one framed column (thin rules either side), like a printed sheet. */}
      <div className="mx-auto max-w-[1200px] border-border min-[1240px]:border-x">
        <main>
          <Hero />
          <div className="grid grid-cols-[minmax(0,1fr)] gap-12 border-t border-border px-5 pt-16 sm:px-10 lg:grid-cols-[13rem_minmax(0,1fr)] lg:gap-16">
            <aside className="hidden lg:block">
              <Toc />
            </aside>
            <div className="max-w-[46rem]">
              <HowItWorks />
              <LeadAndReply />
              <Attribution />
              <FeaturesGrid />
              <PricingSection />
              <ComparisonSection />
              <TerminalInstall />
              <FaqSection />
            </div>
          </div>
          <FinalCta />
        </main>
        <MarketingFooter />
      </div>
    </div>
  );
}
