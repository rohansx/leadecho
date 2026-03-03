import { createFileRoute, Link } from "@tanstack/react-router";
import { Text } from "@/components/ui/text";

export const Route = createFileRoute("/privacy")({
  component: PrivacyPage,
});

function Section({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <div className="space-y-3">
      <Text as="h3" className="text-lg font-semibold border-b border-border pb-2">{title}</Text>
      <div className="text-sm text-muted-foreground leading-relaxed space-y-2">{children}</div>
    </div>
  );
}

function PrivacyPage() {
  return (
    <div className="min-h-screen bg-background text-foreground">
      {/* Nav */}
      <nav className="border-b-2 border-border bg-card">
        <div className="max-w-3xl mx-auto px-6 h-14 flex items-center justify-between">
          <Link to="/" className="flex items-center gap-2">
            <div className="w-7 h-7 bg-primary border-2 border-border rounded flex items-center justify-center font-[family-name:var(--font-head)] text-primary-foreground text-xs font-bold">
              LE
            </div>
            <span className="font-[family-name:var(--font-head)] text-base">LeadEcho</span>
          </Link>
          <Link to="/" className="text-sm text-muted-foreground hover:text-foreground">← Back</Link>
        </div>
      </nav>

      {/* Content */}
      <main className="max-w-3xl mx-auto px-6 py-12 space-y-10">
        <div>
          <Text as="h1" className="text-3xl font-bold mb-2">Privacy Policy</Text>
          <Text as="p" className="text-sm text-muted-foreground">
            Effective date: January 1, 2025 · Last updated: January 1, 2025
          </Text>
        </div>

        <Section title="What the extension collects">
          <p>
            The LeadEcho Chrome Extension collects the following data from pages you actively visit
            on supported platforms (Reddit, Twitter/X, LinkedIn, Hacker News, Dev.to, Lobsters, Indie Hackers):
          </p>
          <ul className="list-disc pl-5 space-y-1">
            <li><strong>Post text content</strong> — the body of posts and comments visible on the page</li>
            <li><strong>Post metadata</strong> — title, URL, platform name, author username (all publicly visible)</li>
            <li><strong>Engagement signals</strong> — upvote/vote counts, comment counts (publicly visible)</li>
          </ul>
          <p>The extension does <strong>not</strong> collect:</p>
          <ul className="list-disc pl-5 space-y-1">
            <li>Private messages or direct messages</li>
            <li>Passwords or authentication credentials</li>
            <li>Browsing history outside supported platforms</li>
            <li>Any data from pages on unsupported sites</li>
            <li>Personal information beyond what is publicly visible on the page</li>
          </ul>
        </Section>

        <Section title="Where your data goes">
          <p>
            All data collected by the extension is sent <strong>exclusively to the LeadEcho backend URL
            you configure</strong> in the extension Settings tab. This is a server you control — either
            a self-hosted instance on your own infrastructure or a LeadEcho cloud instance at an address
            you provide.
          </p>
          <p>
            <strong>No data is sent to Anthropic, Google, any third-party analytics service, or any
            server other than the URL you explicitly configure.</strong> If no backend URL is configured,
            no data is transmitted.
          </p>
        </Section>

        <Section title="AI processing">
          <p>
            Post content may be sent from your LeadEcho backend to an AI provider (e.g. OpenAI, Anthropic)
            for intent classification and reply drafting. This is handled entirely by your backend using
            the API key you supply (Bring Your Own Key). The extension itself never communicates with any
            AI provider directly.
          </p>
        </Section>

        <Section title="Local storage">
          <p>The extension stores the following data in Chrome's local storage:</p>
          <ul className="list-disc pl-5 space-y-1">
            <li><code className="bg-muted px-1 rounded text-xs">apiUrl</code> — your backend URL (for connecting to your LeadEcho instance)</li>
            <li><code className="bg-muted px-1 rounded text-xs">apiKey</code> — your extension token (for authenticating API requests)</li>
          </ul>
          <p>
            This data never leaves your browser except in requests to the backend URL you configured.
            Session storage (cleared on browser restart) is used temporarily to hold pending reply
            content between the background worker and content scripts.
          </p>
        </Section>

        <Section title="Revoking access">
          <p>You can stop all data transmission at any time by:</p>
          <ol className="list-decimal pl-5 space-y-1">
            <li>
              <strong>Revoking the Extension Key</strong> — In the LeadEcho dashboard under
              Settings → Extension Token, click "Revoke". The API key is immediately invalidated
              and all requests from the extension will fail.
            </li>
            <li>
              <strong>Removing the extension</strong> — Uninstalling the extension clears all locally stored data.
            </li>
          </ol>
        </Section>

        <Section title="Data retention">
          <p>
            The extension itself retains no data beyond your local Chrome storage. Data sent to your
            LeadEcho backend is subject to the retention policies you configure on your own infrastructure.
          </p>
        </Section>

        <Section title="Changes to this policy">
          <p>
            If this policy changes materially, the extension version will be updated and the revised
            policy will be published to the Chrome Web Store listing. Continued use of the extension
            after an update constitutes acceptance of the revised policy.
          </p>
        </Section>

        <Section title="Contact">
          <p>
            For questions about this privacy policy, please open an issue at:{" "}
            <a
              href="https://github.com/your-org/leadecho/issues"
              target="_blank"
              rel="noreferrer"
              className="text-primary underline"
            >
              github.com/your-org/leadecho/issues
            </a>
          </p>
        </Section>
      </main>

      <footer className="border-t-2 border-border bg-card py-6 text-center">
        <Text as="span" className="text-xs text-muted-foreground">© 2025 LeadEcho · MIT License</Text>
      </footer>
    </div>
  );
}
