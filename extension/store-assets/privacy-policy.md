# LeadEcho Extension — Privacy Policy

**Effective date:** 2025-01-01
**Last updated:** 2025-01-01

---

## What we collect

The LeadEcho Chrome Extension collects the following data from pages you actively visit on supported platforms (Reddit, Twitter/X, LinkedIn, Hacker News, Dev.to, Lobsters, Indie Hackers):

- **Post text content** — the body of posts and comments visible on the page
- **Post metadata** — title, URL, platform name, author username (publicly visible)
- **Engagement signals** — upvote counts, comment counts (publicly visible)

The extension does **not** collect:

- Private messages or direct messages
- Passwords or authentication credentials
- Browsing history outside supported platforms
- Any data from pages on unsupported sites
- Personal information beyond what is publicly visible on the page

---

## Where your data goes

All data collected by the extension is sent **exclusively to the LeadEcho backend URL you configure in the extension Settings tab**. This is a server you control — either:

- A self-hosted instance running on your own infrastructure (Docker or otherwise)
- A LeadEcho cloud instance at an address you provide

**No data is sent to Anthropic, Google, any third-party analytics service, or any server other than the URL you explicitly configure.**

If the extension has no backend URL configured, no data is transmitted.

---

## AI processing

Post content may be sent from your LeadEcho backend to an AI provider (e.g. OpenAI, Anthropic) for intent classification and reply drafting. This is handled entirely by your backend using the API key you supply (Bring Your Own Key). The extension itself never communicates with any AI provider directly.

---

## Authentication cookies

When you configure browser sessions in the LeadEcho dashboard (Reddit, Twitter/X, or LinkedIn session cookies), those cookies are stored encrypted in the LeadEcho database on your backend. The extension does not read, transmit, or store your session cookies.

---

## Local storage

The extension stores the following data in Chrome's local storage (`chrome.storage.local`):

| Key | Contents | Purpose |
|-----|----------|---------|
| `apiUrl` | Your backend URL | Connecting to your LeadEcho instance |
| `apiKey` | Your extension token | Authenticating API requests |

This data never leaves your browser except in requests to the URL you configured.

The extension uses `chrome.storage.session` (tab-scoped, cleared on browser restart) to temporarily hold pending reply content between the background worker and content scripts.

---

## Revoking access

You can stop all data transmission at any time by:

1. **Revoking the Extension Key** — In the LeadEcho dashboard under Settings → Extension Token, click "Revoke". The extension's API key is immediately invalidated and all requests will fail.
2. **Removing the extension** — Uninstalling the extension clears all locally stored data.
3. **Clearing extension storage** — In Chrome's extension settings, you can clear the extension's storage without uninstalling.

---

## Data retention

The extension itself retains no data beyond your local Chrome storage. Data sent to your LeadEcho backend is subject to the retention policies you configure on your own infrastructure.

---

## Changes to this policy

If this policy changes materially, the extension version will be updated and the new policy will be published to the Chrome Web Store listing. Continued use of the extension after an update constitutes acceptance of the revised policy.

---

## Contact

For questions about this privacy policy, please open an issue at: https://github.com/rohansx/leadecho/issues
