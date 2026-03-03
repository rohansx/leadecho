# LeadEcho - Chrome Extension Architecture

## Overview

The Chrome Extension handles what server-side can't: LinkedIn monitoring (no public API) and safe engagement posting using the user's authenticated browser sessions. Built with WXT framework + React + Manifest V3.

---

## Architecture

```
┌─────────────────────────────────────────────┐
│ Service Worker (background.ts)              │
│  ├── Cloud API communication (fetch + SSE)  │
│  ├── Alarm-based polling (every 60s)        │
│  ├── Message router                         │
│  └── State persistence (chrome.storage)     │
├─────────────────────────────────────────────┤
│ Side Panel (React)                          │
│  ├── Mentions tab (real-time feed)          │
│  ├── Queue tab (pending replies)            │
│  └── Settings tab (API key, preferences)    │
├─────────────────────────────────────────────┤
│ Content Scripts                             │
│  ├── linkedin.ts (feed monitoring)          │
│  ├── reddit.ts (reply injection)            │
│  ├── twitter.ts (reply injection)           │
│  └── human-mimicry.ts (typing simulation)   │
└─────────────────────────────────────────────┘
```

---

## WXT Project Structure

```
extension/
├── entrypoints/
│   ├── background.ts              # Service worker
│   ├── sidepanel/
│   │   ├── index.html
│   │   ├── App.tsx
│   │   └── tabs/
│   │       ├── MentionsTab.tsx
│   │       ├── QueueTab.tsx
│   │       └── SettingsTab.tsx
│   ├── content/
│   │   ├── linkedin.ts            # LinkedIn feed scanner
│   │   ├── reddit.ts              # Reddit reply helper
│   │   └── twitter.ts             # X reply helper
│   └── popup/
│       ├── index.html
│       └── App.tsx                # Quick status
├── components/
│   ├── MentionCard.tsx
│   ├── ReplyEditor.tsx
│   └── StatusBadge.tsx
├── lib/
│   ├── api.ts                     # Backend API client
│   ├── messages.ts                # Type-safe message protocol
│   ├── human-mimicry.ts           # Typing simulation
│   └── storage.ts                 # chrome.storage helpers
├── wxt.config.ts
├── package.json
└── tsconfig.json
```

### wxt.config.ts

```typescript
import { defineConfig } from 'wxt';

export default defineConfig({
  modules: ['@wxt-dev/module-react'],
  manifest: {
    name: 'LeadEcho',
    permissions: ['activeTab', 'sidePanel', 'storage', 'alarms', 'tabs'],
    host_permissions: [
      'https://www.linkedin.com/*',
      'https://www.reddit.com/*',
      'https://x.com/*',
      'https://twitter.com/*',
    ],
    side_panel: {
      default_path: 'sidepanel/index.html',
    },
  },
});
```

---

## LinkedIn Monitoring (Content Script)

```typescript
// entrypoints/content/linkedin.ts
export default defineContentScript({
  matches: ['https://www.linkedin.com/feed*'],
  main() {
    const SCAN_INTERVAL = 30_000; // 30 seconds
    const seenPosts = new Set<string>();

    // Watch for new posts in the feed
    const observer = new MutationObserver((mutations) => {
      for (const mutation of mutations) {
        for (const node of mutation.addedNodes) {
          if (node instanceof HTMLElement) {
            const posts = node.querySelectorAll('[data-urn]');
            posts.forEach(processPost);
          }
        }
      }
    });

    const feedContainer = document.querySelector('.scaffold-finite-scroll__content');
    if (feedContainer) {
      observer.observe(feedContainer, { childList: true, subtree: true });
    }

    function processPost(element: Element) {
      const urn = element.getAttribute('data-urn');
      if (!urn || seenPosts.has(urn)) return;
      seenPosts.add(urn);

      const content = element.querySelector('.feed-shared-text')?.textContent?.trim();
      const author = element.querySelector('.feed-shared-actor__name')?.textContent?.trim();
      const headline = element.querySelector('.feed-shared-actor__description')?.textContent?.trim();

      if (!content) return;

      // Send to service worker for keyword matching + cloud sync
      chrome.runtime.sendMessage({
        type: 'LINKEDIN_SIGNAL',
        payload: {
          platform_id: urn,
          url: `https://www.linkedin.com/feed/update/${urn}`,
          content,
          author: { name: author, headline },
          detected_at: new Date().toISOString(),
        },
      });
    }
  },
});
```

**Security:** Never stores LinkedIn cookies/tokens. Only reads publicly visible feed data. All data sent to user's own backend, never to third parties.

---

## Human-Mimicry Engine

```typescript
// lib/human-mimicry.ts

interface TypingConfig {
  baseDelayMs: number;      // 80ms (≈75 WPM)
  variationMs: number;      // ±40ms
  pauseAfterPeriod: number; // 300ms
  pauseAfterComma: number;  // 150ms
  pauseAfterNewline: number;// 500ms
  preEngageDelay: [number, number]; // [2000, 8000] ms
}

const DEFAULT_CONFIG: TypingConfig = {
  baseDelayMs: 80,
  variationMs: 40,
  pauseAfterPeriod: 300,
  pauseAfterComma: 150,
  pauseAfterNewline: 500,
  preEngageDelay: [2000, 8000],
};

export async function simulateTyping(
  element: HTMLElement,
  text: string,
  config = DEFAULT_CONFIG
): Promise<void> {
  // Pre-engagement delay (2-8 seconds)
  await sleep(randomBetween(...config.preEngageDelay));

  // Focus the element
  element.focus();
  element.click();

  for (let i = 0; i < text.length; i++) {
    const char = text[i];

    // Calculate delay with Gaussian distribution
    let delay = gaussianRandom(config.baseDelayMs, config.variationMs);

    // Add pauses after punctuation
    if (char === '.') delay += config.pauseAfterPeriod;
    if (char === ',') delay += config.pauseAfterComma;
    if (char === '\n') delay += config.pauseAfterNewline;

    // Simulate key events
    const keyEvent = new KeyboardEvent('keydown', { key: char, bubbles: true });
    element.dispatchEvent(keyEvent);

    // For contentEditable elements (LinkedIn, Reddit)
    const inputEvent = new InputEvent('input', {
      data: char,
      inputType: 'insertText',
      bubbles: true,
    });
    element.dispatchEvent(inputEvent);

    // Update the content
    if (element.isContentEditable) {
      document.execCommand('insertText', false, char);
    } else {
      (element as HTMLTextAreaElement).value += char;
    }

    await sleep(delay);
  }
}

function gaussianRandom(mean: number, stdDev: number): number {
  const u = 1 - Math.random();
  const v = Math.random();
  const z = Math.sqrt(-2.0 * Math.log(u)) * Math.cos(2.0 * Math.PI * v);
  return Math.max(20, mean + z * stdDev); // Minimum 20ms
}

function randomBetween(min: number, max: number): number {
  return Math.floor(Math.random() * (max - min + 1)) + min;
}

function sleep(ms: number): Promise<void> {
  return new Promise(resolve => setTimeout(resolve, ms));
}
```

---

## Service Worker

```typescript
// entrypoints/background.ts
export default defineBackground(() => {
  // Poll cloud API every 60 seconds
  chrome.alarms.create('poll-mentions', { periodInMinutes: 1 });

  chrome.alarms.onAlarm.addListener(async (alarm) => {
    if (alarm.name === 'poll-mentions') {
      await pollMentions();
    }
  });

  // Message routing
  chrome.runtime.onMessage.addListener((message, sender, sendResponse) => {
    switch (message.type) {
      case 'LINKEDIN_SIGNAL':
        handleLinkedInSignal(message.payload);
        break;
      case 'POST_REPLY':
        handlePostReply(message.payload);
        break;
      case 'GET_QUEUE':
        getReplyQueue().then(sendResponse);
        return true; // Async response
    }
  });

  async function handleLinkedInSignal(signal: LinkedInSignal) {
    const apiKey = await getApiKey();
    await fetch(`${API_BASE}/api/v1/extension/signals`, {
      method: 'POST',
      headers: {
        'X-API-Key': apiKey,
        'Content-Type': 'application/json',
      },
      body: JSON.stringify({ signals: [signal] }),
    });
  }
});
```

---

## Type-Safe Message Protocol

```typescript
// lib/messages.ts
type Message =
  | { type: 'LINKEDIN_SIGNAL'; payload: LinkedInSignal }
  | { type: 'POST_REPLY'; payload: PostReplyRequest }
  | { type: 'GET_QUEUE'; payload: undefined }
  | { type: 'REPLY_POSTED'; payload: ReplyPostedConfirmation }
  | { type: 'STATUS_UPDATE'; payload: StatusUpdate };

interface LinkedInSignal {
  platform_id: string;
  url: string;
  content: string;
  author: { name: string; headline: string };
  detected_at: string;
}

interface PostReplyRequest {
  queue_id: string;
  reply_id: string;
  platform: 'reddit' | 'twitter' | 'linkedin';
  target_url: string;
  content: string;
}
```

---

## Side Panel UI

```tsx
// entrypoints/sidepanel/App.tsx
import { Tabs, TabsList, TabsTrigger, TabsContent } from '@/components/ui/tabs';
import { MentionsTab } from './tabs/MentionsTab';
import { QueueTab } from './tabs/QueueTab';
import { SettingsTab } from './tabs/SettingsTab';

export function App() {
  return (
    <div className="w-full h-screen bg-background">
      <Tabs defaultValue="mentions" className="w-full">
        <TabsList className="w-full">
          <TabsTrigger value="mentions">Mentions</TabsTrigger>
          <TabsTrigger value="queue">Queue</TabsTrigger>
          <TabsTrigger value="settings">Settings</TabsTrigger>
        </TabsList>
        <TabsContent value="mentions"><MentionsTab /></TabsContent>
        <TabsContent value="queue"><QueueTab /></TabsContent>
        <TabsContent value="settings"><SettingsTab /></TabsContent>
      </Tabs>
    </div>
  );
}
```

---

## Storage Strategy

| Store | Use Case | Persistence |
|-------|----------|-------------|
| `chrome.storage.local` | API key, preferences, cached mentions | Permanent |
| `chrome.storage.session` | Active reply queue, temp state | Until browser closes |
| In-memory (service worker) | Current polling state, seen posts | Until worker terminates |

---

## Chrome Web Store Publishing

1. **Privacy policy** required: Clearly state what data is collected (only public post data from user's feed)
2. **Permission justifications**: Each host permission must be justified in store listing
3. **Review timeline**: Initial review ~3-7 business days
4. **Update strategy**: Staged rollout (10% → 50% → 100%)
5. **Version scheme**: semver matching backend API compatibility
