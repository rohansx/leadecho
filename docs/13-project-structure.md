# LeadEcho - Project Structure & Frontend Architecture

This document defines the complete file structure and architecture for the Next.js dashboard and Chrome extension. It is written to be implementation-ready: every directory, file, component, and data flow pattern is specified with enough detail to begin coding immediately.

---

## Table of Contents

1. [Next.js Dashboard Structure](#1-nextjs-dashboard-structure)
2. [Key Page Architectures](#2-key-page-architectures)
3. [Chrome Extension Structure](#3-chrome-extension-structure)
4. [Shared Types & Code](#4-shared-types--code)
5. [State Management Architecture](#5-state-management-architecture)
6. [Component Design System](#6-component-design-system)
7. [Performance Optimizations](#7-performance-optimizations)

---

## 1. Next.js Dashboard Structure

### Complete File Tree

```
web/
├── src/
│   ├── app/                              # Next.js App Router (all routing lives here)
│   │   ├── (auth)/                       # Route group: auth pages (no sidebar, minimal layout)
│   │   │   ├── login/
│   │   │   │   └── page.tsx              # Clerk <SignIn /> wrapper with redirect logic
│   │   │   ├── signup/
│   │   │   │   └── page.tsx              # Clerk <SignUp /> wrapper with onboarding flow
│   │   │   └── layout.tsx                # Centered card layout, no sidebar, brand header only
│   │   │
│   │   ├── (dashboard)/                  # Route group: main app (sidebar + header layout)
│   │   │   ├── inbox/                    # Unified Intent Inbox (core feature)
│   │   │   │   ├── page.tsx              # Inbox view — split panel, SSE stream, filters
│   │   │   │   ├── loading.tsx           # Skeleton: left list shimmer + right panel shimmer
│   │   │   │   └── _components/          # Private folder: inbox-specific components
│   │   │   │       ├── mention-list.tsx          # Virtualized scrollable mention list (left panel)
│   │   │   │       ├── mention-item.tsx          # Single mention row: icon, author, snippet, score badge
│   │   │   │       ├── mention-detail.tsx        # Full mention detail view (right panel)
│   │   │   │       ├── reply-composer.tsx         # AI reply editor: 3 variants, edit area, post button
│   │   │   │       ├── thread-viewer.tsx          # Expandable thread context viewer
│   │   │   │       ├── inbox-filters.tsx          # Filter bar: platform, status, intent, keyword, date
│   │   │   │       ├── inbox-toolbar.tsx          # Bulk actions: archive, star, assign, mark read
│   │   │   │       ├── mention-score-badge.tsx    # Relevance score visual indicator (1-10)
│   │   │   │       └── platform-icon.tsx          # Platform logo resolver (Reddit, HN, X, LinkedIn)
│   │   │   │
│   │   │   ├── pipeline/                 # Lead Pipeline (Kanban board)
│   │   │   │   ├── page.tsx              # Kanban board with drag-and-drop columns
│   │   │   │   ├── loading.tsx           # Skeleton: column placeholders with card shimmer
│   │   │   │   └── _components/
│   │   │   │       ├── pipeline-board.tsx         # DnD context provider + column layout
│   │   │   │       ├── pipeline-column.tsx        # Single kanban column (droppable zone)
│   │   │   │       ├── lead-card.tsx              # Draggable lead card: company, value, source
│   │   │   │       ├── lead-detail-sheet.tsx      # Slide-over panel with full lead details
│   │   │   │       ├── lead-form.tsx              # Create/edit lead form (React Hook Form + Zod)
│   │   │   │       ├── pipeline-filters.tsx       # Filter: source platform, value range, assignee
│   │   │   │       └── pipeline-stats.tsx         # Column summary: count, total value
│   │   │   │
│   │   │   ├── analytics/                # Analytics Dashboard
│   │   │   │   ├── page.tsx              # KPI cards + charts grid + date range picker
│   │   │   │   ├── loading.tsx           # Skeleton: card shimmer row + chart placeholders
│   │   │   │   └── _components/
│   │   │   │       ├── kpi-cards.tsx              # Row of metric cards (mentions, replies, CTR, conversions)
│   │   │   │       ├── mention-trends-chart.tsx   # Area chart: mentions by platform over time
│   │   │   │       ├── conversion-funnel.tsx      # Funnel chart: mention → reply → click → signup → revenue
│   │   │   │       ├── keyword-table.tsx          # Sortable data table: keyword performance metrics
│   │   │   │       ├── platform-comparison.tsx    # Grouped bar chart: platform-by-platform breakdown
│   │   │   │       ├── reply-performance.tsx      # Reply style A/B comparison chart
│   │   │   │       ├── date-range-picker.tsx      # Calendar popover controlling all chart queries
│   │   │   │       └── analytics-export.tsx       # CSV/PDF export button
│   │   │   │
│   │   │   ├── knowledge-base/           # RAG Document Manager
│   │   │   │   ├── page.tsx              # Document list + upload + inline editor
│   │   │   │   ├── loading.tsx           # Skeleton: document list shimmer
│   │   │   │   └── _components/
│   │   │   │       ├── document-list.tsx          # Sortable document table with status badges
│   │   │   │       ├── document-upload.tsx        # Drag-and-drop zone (react-dropzone) + URL input
│   │   │   │       ├── document-detail.tsx        # Content preview, chunk count, metadata
│   │   │   │       ├── document-editor.tsx        # Inline markdown/text editor for documents
│   │   │   │       ├── chunk-viewer.tsx           # Visualize how document was chunked for RAG
│   │   │   │       └── format-badges.tsx          # Supported format indicators: .md, .pdf, .txt, URL
│   │   │   │
│   │   │   ├── workflows/                # Workflow Builder
│   │   │   │   ├── page.tsx              # Workflow list with enable/disable toggles
│   │   │   │   ├── [id]/
│   │   │   │   │   └── page.tsx          # Individual workflow builder/editor
│   │   │   │   ├── loading.tsx           # Skeleton: workflow list shimmer
│   │   │   │   └── _components/
│   │   │   │       ├── workflow-list.tsx          # Table of workflows with status toggles
│   │   │   │       ├── workflow-builder.tsx       # Visual builder: trigger → conditions → actions
│   │   │   │       ├── trigger-config.tsx         # Trigger condition editor (keyword, score, platform)
│   │   │   │       ├── action-chain.tsx           # Action sequence editor (draft, notify, approve, post)
│   │   │   │       ├── workflow-preview.tsx       # Dry-run preview of workflow execution
│   │   │   │       ├── execution-history.tsx      # Table: past runs with status, timing, outcome
│   │   │   │       └── workflow-stats.tsx         # Execution metrics per workflow
│   │   │   │
│   │   │   ├── team/                     # Team Management
│   │   │   │   ├── page.tsx              # Team member list + invite + role management
│   │   │   │   ├── loading.tsx           # Skeleton: member list shimmer
│   │   │   │   └── _components/
│   │   │   │       ├── member-list.tsx            # Team members table with role badges
│   │   │   │       ├── invite-dialog.tsx          # Invite modal: email input, role selector
│   │   │   │       ├── role-selector.tsx          # Role picker: admin, editor, viewer
│   │   │   │       └── member-activity.tsx        # Per-member activity feed (replies, approvals)
│   │   │   │
│   │   │   ├── settings/                 # Workspace Settings
│   │   │   │   ├── page.tsx              # Settings overview / redirect to first tab
│   │   │   │   ├── general/
│   │   │   │   │   └── page.tsx          # Workspace name, logo, default preferences
│   │   │   │   ├── keywords/
│   │   │   │   │   └── page.tsx          # Keyword management: add, edit, delete, test
│   │   │   │   ├── platforms/
│   │   │   │   │   └── page.tsx          # Platform connections: Reddit OAuth, X API keys, etc.
│   │   │   │   ├── notifications/
│   │   │   │   │   └── page.tsx          # Notification preferences: email, Slack, Discord
│   │   │   │   ├── billing/
│   │   │   │   │   └── page.tsx          # Stripe billing portal, plan management
│   │   │   │   ├── api/
│   │   │   │   │   └── page.tsx          # API key management, webhook URLs
│   │   │   │   ├── loading.tsx           # Skeleton: settings form shimmer
│   │   │   │   └── layout.tsx            # Settings sub-layout with vertical tab navigation
│   │   │   │
│   │   │   └── layout.tsx                # Dashboard layout: sidebar + header + main content area
│   │   │
│   │   ├── api/                          # Next.js Route Handlers (BFF layer)
│   │   │   ├── mentions/
│   │   │   │   └── stream/
│   │   │   │       └── route.ts          # SSE endpoint: proxies Go backend SSE to client
│   │   │   ├── webhooks/
│   │   │   │   ├── clerk/
│   │   │   │   │   └── route.ts          # Clerk webhook: user/org sync events
│   │   │   │   └── stripe/
│   │   │   │       └── route.ts          # Stripe webhook: subscription lifecycle events
│   │   │   └── health/
│   │   │       └── route.ts              # Health check endpoint for monitoring
│   │   │
│   │   ├── layout.tsx                    # Root layout: html/body, fonts, providers wrapper
│   │   ├── page.tsx                      # Landing page or redirect to /inbox if authenticated
│   │   ├── not-found.tsx                 # Custom 404 page
│   │   ├── error.tsx                     # Global error boundary
│   │   └── globals.css                   # Tailwind directives + CSS variables + design tokens
│   │
│   ├── components/
│   │   ├── ui/                           # shadcn/ui components (auto-generated, do not edit directly)
│   │   │   ├── button.tsx
│   │   │   ├── card.tsx
│   │   │   ├── badge.tsx
│   │   │   ├── input.tsx
│   │   │   ├── textarea.tsx
│   │   │   ├── select.tsx
│   │   │   ├── dialog.tsx
│   │   │   ├── sheet.tsx
│   │   │   ├── dropdown-menu.tsx
│   │   │   ├── command.tsx               # cmdk-based command palette
│   │   │   ├── data-table.tsx            # TanStack Table wrapper
│   │   │   ├── tabs.tsx
│   │   │   ├── tooltip.tsx
│   │   │   ├── popover.tsx
│   │   │   ├── calendar.tsx
│   │   │   ├── separator.tsx
│   │   │   ├── skeleton.tsx
│   │   │   ├── sonner.tsx                # Toast notification wrapper (sonner)
│   │   │   ├── scroll-area.tsx
│   │   │   ├── switch.tsx
│   │   │   ├── toggle.tsx
│   │   │   ├── avatar.tsx
│   │   │   ├── chart.tsx                 # Recharts wrapper from shadcn/ui charts
│   │   │   └── ... (other shadcn primitives as needed)
│   │   │
│   │   ├── layout/                       # App-wide layout components
│   │   │   ├── sidebar.tsx               # Collapsible sidebar: nav links, org switcher, user menu
│   │   │   ├── sidebar-nav.tsx           # Navigation items with icons and active state
│   │   │   ├── header.tsx                # Top header: breadcrumbs, search, notifications bell
│   │   │   ├── breadcrumbs.tsx           # Auto-generated breadcrumb trail from route
│   │   │   ├── notification-bell.tsx     # Unread count badge + dropdown with recent notifications
│   │   │   ├── command-menu.tsx          # Global cmd+k command palette (search mentions, navigate)
│   │   │   └── mobile-nav.tsx            # Responsive hamburger menu for mobile breakpoints
│   │   │
│   │   ├── charts/                       # Reusable chart components (built on shadcn/ui chart)
│   │   │   ├── area-chart.tsx            # Configurable area chart with tooltip and legend
│   │   │   ├── bar-chart.tsx             # Configurable bar chart (vertical, horizontal, grouped)
│   │   │   ├── funnel-chart.tsx          # Conversion funnel visualization
│   │   │   ├── kpi-card.tsx              # Metric card: value, label, trend arrow, sparkline
│   │   │   └── chart-skeleton.tsx        # Loading skeleton matching chart dimensions
│   │   │
│   │   └── shared/                       # Cross-cutting shared components
│   │       ├── empty-state.tsx           # Illustration + message + CTA for empty lists
│   │       ├── error-boundary.tsx        # Reusable error boundary with retry button
│   │       ├── loading-spinner.tsx       # Inline spinner for button/action loading states
│   │       ├── confirm-dialog.tsx        # Reusable confirmation modal (destructive actions)
│   │       ├── platform-badge.tsx        # Colored badge per platform (Reddit orange, HN orange, etc.)
│   │       ├── relative-time.tsx         # "2 hours ago" with tooltip showing absolute time
│   │       ├── copy-button.tsx           # Click-to-copy with toast confirmation
│   │       ├── keyboard-shortcut.tsx     # Renders keyboard shortcut hint (e.g., "J" for next)
│   │       └── infinite-scroll.tsx       # Intersection observer trigger for pagination
│   │
│   ├── lib/
│   │   ├── api/                          # API client functions (used by TanStack Query)
│   │   │   ├── client.ts                 # Base fetch wrapper: auth headers, error handling, base URL
│   │   │   ├── mentions.ts              # getMentions, getMention, archiveMention, starMention
│   │   │   ├── replies.ts               # draftReply, submitReply, getReplyHistory
│   │   │   ├── leads.ts                 # getLeads, createLead, updateLeadStage, deleteLead
│   │   │   ├── analytics.ts             # getKPIs, getMentionTrends, getConversionFunnel, etc.
│   │   │   ├── knowledge-base.ts        # getDocuments, uploadDocument, deleteDocument, updateDocument
│   │   │   ├── workflows.ts             # getWorkflows, createWorkflow, toggleWorkflow, getExecutions
│   │   │   ├── team.ts                  # getMembers, inviteMember, updateRole, removeMember
│   │   │   ├── settings.ts              # getSettings, updateSettings, getKeywords, updateKeywords
│   │   │   └── auth.ts                  # Token refresh, session validation helpers
│   │   │
│   │   ├── validations/                  # Zod schemas (shared source of truth for types)
│   │   │   ├── mention.ts               # MentionSchema, MentionFilterSchema
│   │   │   ├── lead.ts                  # LeadSchema, LeadCreateSchema, LeadUpdateSchema
│   │   │   ├── reply.ts                 # ReplySchema, ReplyDraftSchema, ReplySubmitSchema
│   │   │   ├── workflow.ts              # WorkflowSchema, TriggerSchema, ActionSchema
│   │   │   ├── keyword.ts              # KeywordSchema, KeywordCreateSchema
│   │   │   ├── settings.ts             # WorkspaceSettingsSchema, NotificationPrefsSchema
│   │   │   ├── auth.ts                 # LoginSchema, SignupSchema
│   │   │   └── common.ts               # PaginationSchema, DateRangeSchema, SortSchema
│   │   │
│   │   ├── utils.ts                     # cn(), formatDate(), formatNumber(), truncate(), etc.
│   │   ├── constants.ts                 # API_BASE_URL, PLATFORMS, LEAD_STAGES, INTENT_TYPES, etc.
│   │   └── query-keys.ts               # TanStack Query key factory (type-safe, hierarchical)
│   │
│   ├── hooks/                           # Custom React hooks
│   │   ├── use-sse.ts                   # SSE connection manager → injects into TanStack Query cache
│   │   ├── use-debounce.ts              # Debounced value hook (for search inputs)
│   │   ├── use-keyboard-shortcuts.ts    # Global keyboard shortcut registration
│   │   ├── use-media-query.ts           # Responsive breakpoint detection
│   │   ├── use-intersection.ts          # Intersection observer for infinite scroll
│   │   └── use-local-storage.ts         # Typed localStorage with SSR safety
│   │
│   ├── stores/                          # Zustand stores (client-side UI state only)
│   │   ├── sidebar-store.ts             # Sidebar collapsed/expanded state
│   │   ├── inbox-store.ts              # Selected mention ID, reply composer open/closed
│   │   ├── pipeline-store.ts           # Dragging state, active detail sheet
│   │   ├── command-menu-store.ts       # Command palette open/closed state
│   │   └── notification-store.ts       # Unread notification count, toast queue
│   │
│   ├── providers/                       # React context providers (wrapped in root layout)
│   │   ├── query-provider.tsx           # TanStack QueryClientProvider with default options
│   │   ├── theme-provider.tsx           # next-themes ThemeProvider (dark/light/system)
│   │   └── clerk-provider.tsx           # Clerk auth provider wrapper
│   │
│   ├── actions/                         # Next.js Server Actions
│   │   ├── mention-actions.ts           # archiveMention, bulkArchive, starMention
│   │   ├── reply-actions.ts             # submitReply, approveReply
│   │   ├── lead-actions.ts              # updateLeadStage, createLead
│   │   ├── knowledge-base-actions.ts    # uploadDocument, deleteDocument
│   │   └── settings-actions.ts          # updateWorkspaceSettings, addKeyword
│   │
│   └── types/                           # TypeScript type definitions
│       ├── mention.ts                   # Mention, MentionFilter, MentionStatus, IntentType
│       ├── lead.ts                      # Lead, LeadStage, LeadSource
│       ├── reply.ts                     # Reply, ReplyVariant, ReplyStatus
│       ├── workflow.ts                  # Workflow, Trigger, Action, Execution
│       ├── analytics.ts                # KPI, TrendDataPoint, FunnelStep, KeywordMetric
│       ├── knowledge-base.ts           # Document, DocumentChunk, DocumentFormat
│       ├── team.ts                     # Member, Role, Invitation
│       ├── platform.ts                 # Platform enum, PlatformConfig
│       ├── api.ts                      # ApiResponse<T>, PaginatedResponse<T>, ApiError
│       └── common.ts                   # DateRange, SortDirection, FilterOperator
│
├── public/
│   ├── icons/                           # Platform SVG icons
│   │   ├── reddit.svg
│   │   ├── hackernews.svg
│   │   ├── twitter.svg
│   │   └── linkedin.svg
│   └── images/                          # Static images (empty states, onboarding)
│
├── next.config.ts                       # Next.js configuration
├── tailwind.config.ts                   # Tailwind CSS v4 configuration (if not CSS-first)
├── tsconfig.json                        # TypeScript configuration with path aliases
├── components.json                      # shadcn/ui configuration
├── package.json
├── vitest.config.ts                     # Vitest test runner configuration
├── playwright.config.ts                 # Playwright E2E configuration
└── .env.local                           # Environment variables (never committed)
```

### Directory Convention Rationale

| Pattern | Why |
|---------|-----|
| `(auth)` / `(dashboard)` route groups | Separate layout shells. Auth pages get a minimal centered layout. Dashboard pages get the sidebar + header chrome. URL paths remain clean (no `/dashboard/inbox`, just `/inbox`). |
| `_components/` private folders | Colocate route-specific components next to their page. The underscore prefix tells Next.js to exclude this directory from routing. Components here are NOT shared across routes. |
| `components/ui/` | shadcn/ui auto-generated files. Treat as library code. Customize via the shadcn CLI, not by hand-editing. |
| `components/layout/` | Structural components used in layouts (sidebar, header). Imported by `layout.tsx` files. |
| `components/shared/` | Reusable building blocks used across multiple pages. Generic enough to appear anywhere. |
| `lib/api/` | Pure functions that call the Go backend. No React, no hooks. Consumed by TanStack Query `queryFn` functions and Server Actions. |
| `lib/validations/` | Zod schemas that serve as the single source of truth for types. `z.infer<typeof Schema>` generates the TypeScript types. |
| `hooks/` | Custom hooks used across multiple pages. Page-specific hooks live in `_components/`. |
| `stores/` | Zustand stores for client-only UI state. Server data lives in TanStack Query cache, never in Zustand. |
| `providers/` | React context providers composed together in the root `layout.tsx`. |
| `actions/` | Server Actions for mutations that benefit from server-side execution (auth headers, revalidation). |

---

## 2. Key Page Architectures

### 2.1 Inbox Page (Core Feature)

The Inbox is the most complex page in the application and the primary feature users interact with daily. It combines real-time data, complex filtering, keyboard navigation, and a multi-panel layout.

#### Layout Structure

```
┌─────────────────────────────────────────────────────────────────────────┐
│  inbox-toolbar.tsx                                                       │
│  [Bulk Actions: Archive | Star | Assign]  [Sort: Newest | Score | ...]  │
├─────────────────────────────────────────────────────────────────────────┤
│  inbox-filters.tsx                                                       │
│  [Platform ▼] [Status ▼] [Intent ▼] [Keyword Search...] [Date Range]   │
├───────────────────────────────┬──────────────────────────────────────────┤
│  mention-list.tsx (left)      │  mention-detail.tsx (right)              │
│  ┌─────────────────────────┐  │  ┌──────────────────────────────────┐   │
│  │ mention-item.tsx        │  │  │ Platform | Author | Timestamp    │   │
│  │ [Reddit] @user123       │  │  │                                  │   │
│  │ "Looking for a tool..." │  │  │ Full mention content rendered    │   │
│  │ Score: 8/10  ·  2h ago  │  │  │ as markdown with links.          │   │
│  │ ★ Intent: Buying Signal │  │  │                                  │   │
│  ├─────────────────────────┤  │  ├──────────────────────────────────┤   │
│  │ mention-item.tsx        │  │  │ thread-viewer.tsx                 │   │
│  │ [HN] @techfounder       │  │  │ ▶ Show Thread Context (3 replies)│   │
│  │ "Anyone tried X for..." │  │  │                                  │   │
│  │ Score: 6/10  ·  5h ago  │  │  ├──────────────────────────────────┤   │
│  │ ★ Intent: Recommendation│  │  │ reply-composer.tsx                │   │
│  ├─────────────────────────┤  │  │                                  │   │
│  │ mention-item.tsx        │  │  │ Variant A: [Value-first reply]   │   │
│  │ [X] @startupceo         │  │  │ Variant B: [Technical reply]     │   │
│  │ "HubSpot alternative?"  │  │  │ Variant C: [Soft-sell reply]     │   │
│  │ Score: 9/10  ·  1h ago  │  │  │                                  │   │
│  │ ★ Intent: Buying Signal │  │  │ [Edit Area: editable textarea]   │   │
│  └─────────────────────────┘  │  │                                  │   │
│  ▼ Infinite scroll trigger    │  │ [Platform: Reddit ▼] [Post Reply]│   │
│                               │  └──────────────────────────────────┘   │
└───────────────────────────────┴──────────────────────────────────────────┘
```

#### Component Specification

**`page.tsx` (Inbox Page)**

Server Component that renders the page shell. Defers data fetching to client components.

```tsx
// src/app/(dashboard)/inbox/page.tsx
import { Suspense } from "react";
import { InboxFilters } from "./_components/inbox-filters";
import { InboxToolbar } from "./_components/inbox-toolbar";
import { MentionList } from "./_components/mention-list";
import { MentionDetail } from "./_components/mention-detail";

export default function InboxPage() {
  return (
    <div className="flex h-full flex-col">
      <InboxToolbar />
      <InboxFilters />
      <div className="flex flex-1 overflow-hidden">
        <Suspense fallback={<MentionListSkeleton />}>
          <MentionList />
        </Suspense>
        <Suspense fallback={<MentionDetailSkeleton />}>
          <MentionDetail />
        </Suspense>
      </div>
    </div>
  );
}
```

**`mention-list.tsx`**

Client Component. Fetches mentions via TanStack Query. Subscribes to SSE for real-time updates. Renders a virtualized list for performance. Reads filter state from URL via nuqs.

```tsx
// Key behaviors:
// 1. useQuery({ queryKey: mentionKeys.list(filters), queryFn: ... })
// 2. useSSE("/api/mentions/stream") → on new mention → queryClient.setQueryData(...)
// 3. useQueryStates (nuqs) for platform, status, intent, keyword, dateRange
// 4. TanStack Virtual for virtualized rendering (handles 1000+ mentions)
// 5. onClick → update selectedMentionId in inbox-store (Zustand)
// 6. Keyboard: j/k to navigate, Enter to select
```

Core data flow:
- Initial load: TanStack Query fetches `GET /api/v1/mentions?platform=...&status=...`
- Real-time: SSE pushes `mention.detected` events → `queryClient.setQueryData` prepends to list
- Filter changes: nuqs updates URL search params → TanStack Query refetches with new params
- Selection: Zustand `inbox-store` tracks `selectedMentionId` → detail panel re-renders

**`mention-item.tsx`**

Pure presentational component. Receives a `Mention` object and renders a compact row.

```
Visual structure:
┌──────────────────────────────────────────┐
│ [Platform Icon]  @authorName     · 2h ago│
│ "First 120 chars of mention content..."  │
│ [Score: 8] [Intent: Buy Signal] [★]      │
└──────────────────────────────────────────┘
```

Props:
- `mention: Mention` — the mention data
- `isSelected: boolean` — active highlight state
- `onClick: () => void` — selection handler

Key details:
- Platform icon resolved from `mention.platform` enum
- Relevance score displayed as a colored badge: 8-10 green, 5-7 yellow, 1-4 gray
- Intent type shown as a subtle pill badge
- Unread mentions have a bold left border indicator
- Starred mentions show a filled star icon

**`mention-detail.tsx`**

Client Component. Displays the full content of the selected mention and hosts the reply composer.

Sections:
1. **Header**: Platform icon, author name with link to profile, original post URL, timestamp
2. **Content**: Full mention text rendered as markdown (sanitized). Mentions of the user's keywords are highlighted.
3. **Thread Context** (`thread-viewer.tsx`): Collapsible section showing parent/sibling comments for context. Fetched on demand when expanded.
4. **Reply Composer** (`reply-composer.tsx`): The AI reply drafting interface.

**`reply-composer.tsx`**

The most interactive component on the page. Handles AI draft generation and reply submission.

```
Workflow:
1. User clicks "Generate Drafts" (or auto-generates on mention selection)
2. API call: POST /api/v1/mentions/:id/draft → returns 3 ReplyVariant objects
3. Display 3 tab-like variant selectors:
   - Variant A: "Value-First" — helpful answer, no product mention
   - Variant B: "Technical" — detailed explanation with subtle product reference
   - Variant C: "Soft-Sell" — value + gentle product mention with link
4. Selected variant populates an editable textarea
5. User edits the text freely
6. Platform selector dropdown (which platform to reply on)
7. "Post Reply" button → POST /api/v1/mentions/:id/reply
8. On success: toast notification, mention status updates to "replied"
```

State management:
- Variant selection: local component state
- Edit buffer: React Hook Form `useForm` with the textarea value
- Submission: TanStack Query `useMutation` with optimistic update on mention status
- Loading states: skeleton variants while AI generates, spinner on post button

**`inbox-filters.tsx`**

Filter bar that drives the mention list query. All filter values are persisted in the URL via nuqs.

| Filter | UI Control | URL Param | Values |
|--------|-----------|-----------|--------|
| Platform | Multi-select dropdown | `?platform=reddit,hn` | reddit, hn, twitter, linkedin |
| Status | Select dropdown | `?status=unread` | unread, read, replied, archived, starred |
| Intent Type | Multi-select dropdown | `?intent=buy_signal,recommendation` | buy_signal, recommendation, complaint, question, discussion |
| Keyword Search | Text input (debounced) | `?q=hubspot` | Free text |
| Date Range | Calendar popover | `?from=2026-01-01&to=2026-02-23` | ISO date strings |
| Sort | Select dropdown | `?sort=newest` | newest, oldest, score_desc, score_asc |

Implementation:
```tsx
// All filters use nuqs useQueryStates for URL persistence:
const [filters, setFilters] = useQueryStates({
  platform: parseAsArrayOf(parseAsStringEnum(PLATFORMS)),
  status: parseAsStringEnum(MENTION_STATUSES),
  intent: parseAsArrayOf(parseAsStringEnum(INTENT_TYPES)),
  q: parseAsString,
  from: parseAsIsoDateTime,
  to: parseAsIsoDateTime,
  sort: parseAsStringEnum(SORT_OPTIONS).withDefault("newest"),
});
// Filter changes update the URL → TanStack Query re-fetches with new params
```

#### Keyboard Shortcuts

Registered via the `use-keyboard-shortcuts` hook at the inbox page level:

| Key | Action | Implementation |
|-----|--------|---------------|
| `j` | Select next mention | `inboxStore.selectNext()` → updates selectedMentionId |
| `k` | Select previous mention | `inboxStore.selectPrev()` → updates selectedMentionId |
| `Enter` | Open mention detail | Focus shifts to detail panel |
| `r` | Focus reply composer | Scrolls to and focuses the reply textarea |
| `a` | Archive selected mention | Triggers `archiveMention` mutation |
| `s` | Star/unstar selected mention | Triggers `starMention` mutation |
| `e` | Mark as read | Triggers `markRead` mutation |
| `/` | Focus search input | Focuses the keyword search in filters |
| `Esc` | Deselect / close composer | Clears selection or closes reply panel |

#### SSE Integration Detail

```
Connection lifecycle:
1. useSSE hook connects to /api/mentions/stream on mount
2. Server sends events: { type: "mention.new", data: Mention }
3. Hook parses event and calls queryClient.setQueryData to prepend
4. If mention matches current filters → appears at top of list with highlight animation
5. If mention does not match filters → counter badge shows "+N new" above list
6. On disconnect: automatic reconnect with exponential backoff (1s, 2s, 4s, max 30s)
7. On reconnect: refetch full list to sync any missed events
```

---

### 2.2 Pipeline Page (Kanban Board)

#### Layout Structure

```
┌─────────────────────────────────────────────────────────────────────────────┐
│  pipeline-filters.tsx                                                        │
│  [Platform ▼] [Value Range] [Assignee ▼]      [+ Add Lead]                 │
├─────────────────────────────────────────────────────────────────────────────┤
│  pipeline-board.tsx (DndContext provider)                                     │
│                                                                              │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────┐     │
│  │ Prospect │  │ Qualified│  │ Engaged  │  │ Converted│  │ Lost     │     │
│  │ (12)     │  │ (8)      │  │ (5)      │  │ (3)      │  │ (2)      │     │
│  │ $24,000  │  │ $18,000  │  │ $12,000  │  │ $8,000   │  │ $4,000   │     │
│  ├──────────┤  ├──────────┤  ├──────────┤  ├──────────┤  ├──────────┤     │
│  │┌────────┐│  │┌────────┐│  │┌────────┐│  │┌────────┐│  │┌────────┐│     │
│  ││ Acme   ││  ││ Contoso││  ││ Fabrikm││  ││ Initech││  ││ Globex ││     │
│  ││ @ceo   ││  ││ @dev   ││  ││ @vp    ││  ││ @cto   ││  ││ @pm    ││     │
│  ││ $5,000 ││  ││ $3,000 ││  ││ $4,000 ││  ││ $3,000 ││  ││ $2,000 ││     │
│  ││ Reddit ││  ││ HN     ││  ││ X      ││  ││ Reddit ││  ││ LI     ││     │
│  ││ 2h ago ││  ││ 1d ago ││  ││ 3h ago ││  ││ 5d ago ││  ││ 1w ago ││     │
│  │└────────┘│  │└────────┘│  │└────────┘│  │└────────┘│  │└────────┘│     │
│  │┌────────┐│  │┌────────┐│  │          │  │          │  │          │     │
│  ││ ...    ││  ││ ...    ││  │          │  │          │  │          │     │
│  │└────────┘│  │└────────┘│  │          │  │          │  │          │     │
│  └──────────┘  └──────────┘  └──────────┘  └──────────┘  └──────────┘     │
└─────────────────────────────────────────────────────────────────────────────┘
```

#### Component Specification

**`pipeline-board.tsx`**

Client Component. Wraps the board in `@dnd-kit` `DndContext` and `SortableContext` providers.

```tsx
// Key architecture:
// 1. DndContext with custom collision detection (closestCorners)
// 2. SortableContext per column for within-column reordering
// 3. DragOverlay renders a ghost card during drag
// 4. onDragEnd handler:
//    - If dropped in same column: reorder (update sort_order)
//    - If dropped in different column: update lead stage via mutation
//    - Optimistic update: immediately move card, revert on API failure
// 5. All columns rendered in a horizontal flex container with overflow-x-auto
```

**`pipeline-column.tsx`**

Droppable container for a single pipeline stage.

Props:
- `stage: LeadStage` — the stage enum value (prospect, qualified, engaged, converted, lost)
- `leads: Lead[]` — leads in this stage
- `onAddLead: () => void` — trigger for the add lead form

Renders:
- Column header: stage name, lead count, total value sum
- Scrollable card list with `useDroppable` from @dnd-kit
- "Add lead" button at the bottom of the column

**`lead-card.tsx`**

Draggable card representing a single lead. Uses `useSortable` from @dnd-kit.

```
Visual structure:
┌──────────────────────────────┐
│ Acme Corp                    │
│ @ceo_acme · Reddit           │
│ $5,000 estimated value       │
│ Source: "Looking for CRM..." │
│ Last activity: 2 hours ago   │
│ [Assigned: JD]               │
└──────────────────────────────┘
```

Interactions:
- Click → opens `lead-detail-sheet.tsx` (Sheet component, slides in from right)
- Drag → enters drag overlay mode, original card shows drop placeholder
- Hover → subtle scale/shadow lift

**`lead-detail-sheet.tsx`**

A shadcn Sheet (slide-over panel) that shows full lead details.

Sections:
1. **Lead Info**: Company name, contact details, estimated value, assigned team member
2. **Source Mention**: Link to the original mention that created this lead, with inline preview
3. **Activity Timeline**: Chronological list of events (stage changes, replies sent, notes added)
4. **Notes**: Free-text notes area for team collaboration
5. **Actions**: Edit lead, change stage, delete lead, link to CRM

---

### 2.3 Analytics Page

#### Layout Structure

```
┌─────────────────────────────────────────────────────────────────────────────┐
│  Header: "Analytics"                        [date-range-picker.tsx]          │
│                                             [Last 7 days ▼] [Export CSV]    │
├─────────────────────────────────────────────────────────────────────────────┤
│  kpi-cards.tsx (responsive grid: 4 columns on desktop, 2 on tablet, 1 mob) │
│  ┌─────────────┐ ┌─────────────┐ ┌─────────────┐ ┌─────────────┐          │
│  │ Total       │ │ Replies     │ │ Click-       │ │ Conversions │          │
│  │ Mentions    │ │ Sent        │ │ Through Rate │ │             │          │
│  │ 1,234       │ │ 156         │ │ 12.3%        │ │ 23          │          │
│  │ ↑ 12% vs   │ │ ↑ 8% vs    │ │ ↓ 2% vs     │ │ ↑ 15% vs   │          │
│  │ prev period │ │ prev period │ │ prev period  │ │ prev period │          │
│  │ [sparkline] │ │ [sparkline] │ │ [sparkline]  │ │ [sparkline] │          │
│  └─────────────┘ └─────────────┘ └─────────────┘ └─────────────┘          │
├─────────────────────────────────┬───────────────────────────────────────────┤
│  mention-trends-chart.tsx       │  conversion-funnel.tsx                    │
│  Area chart: stacked by         │  Funnel visualization:                   │
│  platform (Reddit, HN, X, LI)  │  Mentions → Replies → Clicks →           │
│  over selected date range       │  Signups → Revenue                       │
│  X-axis: dates, Y-axis: count  │  Shows drop-off % between stages         │
├─────────────────────────────────┴───────────────────────────────────────────┤
│  keyword-table.tsx                                                           │
│  Sortable data table:                                                        │
│  | Keyword | Mentions | Replies | Clicks | Conversions | CTR | Conv Rate |  │
│  | "CRM"   | 450      | 89      | 34     | 12          | 38% | 14%       |  │
│  | "alt"   | 320      | 65      | 28     | 8           | 43% | 12%       |  │
├─────────────────────────────────────────────────────────────────────────────┤
│  platform-comparison.tsx                                                     │
│  Grouped bar chart: side-by-side comparison per platform                     │
│  Metrics: mentions, replies, conversions                                     │
│  Each platform in its brand color                                            │
└─────────────────────────────────────────────────────────────────────────────┘
```

#### Data Flow

All charts share a single date range state managed via nuqs URL params (`?from=...&to=...`). Each chart component has its own TanStack Query hook that includes the date range in its query key:

```tsx
// Example: mention-trends-chart.tsx
const { from, to } = useDateRange(); // nuqs hook
const { data } = useQuery({
  queryKey: analyticsKeys.mentionTrends(from, to),
  queryFn: () => getMentionTrends({ from, to }),
});
```

When the user changes the date range, all query keys invalidate simultaneously, and all charts refetch in parallel. Each chart has its own loading skeleton via Suspense boundaries.

#### KPI Cards Detail

Each card shows:
- **Metric value**: Large number, prominently displayed
- **Trend indicator**: Arrow + percentage change vs. previous equivalent period
- **Sparkline**: 7-point mini line chart showing the metric over the selected period
- **Color coding**: Green for positive trends, red for negative, gray for neutral

---

### 2.4 Knowledge Base Page

#### Layout Structure

```
┌─────────────────────────────────────────────────────────────────────────────┐
│  Header: "Knowledge Base"                  [+ Upload Document]              │
├─────────────────────────────────┬───────────────────────────────────────────┤
│  document-list.tsx (left/full)  │  document-detail.tsx (right, when open)   │
│                                 │                                           │
│  ┌─────────────────────────┐    │  ┌─────────────────────────────────────┐  │
│  │ [.md] Product FAQ       │    │  │ Product FAQ                         │  │
│  │ 12 chunks · Updated 2d  │    │  │                                     │  │
│  │                         │    │  │ Rendered markdown content preview    │  │
│  │ [.pdf] Sales Playbook   │    │  │ with syntax highlighting            │  │
│  │ 34 chunks · Updated 1w  │    │  │                                     │  │
│  │                         │    │  │ Chunks: 12 | Last updated: 2d ago   │  │
│  │ [URL] Blog Article      │    │  │                                     │  │
│  │ 8 chunks · Updated 3d   │    │  │ [Edit] [Re-process] [Delete]        │  │
│  │                         │    │  │                                     │  │
│  │ [.txt] Brand Voice Guide│    │  │ ┌─── chunk-viewer.tsx ───────────┐  │  │
│  │ 5 chunks · Updated 5d   │    │  │ │ Chunk 1: "Our product is..."  │  │  │
│  └─────────────────────────┘    │  │ │ Chunk 2: "Key features..."    │  │  │
│                                 │  │ │ Chunk 3: "Pricing details..." │  │  │
│  ┌─────────────────────────┐    │  │ └────────────────────────────────┘  │  │
│  │ document-upload.tsx      │    │  └─────────────────────────────────────┘  │
│  │ ┌───────────────────┐   │    │                                           │
│  │ │ Drop files here   │   │    │                                           │
│  │ │ or click to browse│   │    │                                           │
│  │ │ .md .pdf .txt URL │   │    │                                           │
│  │ └───────────────────┘   │    │                                           │
│  │ [Or paste a URL: ___]   │    │                                           │
│  └─────────────────────────┘    │                                           │
└─────────────────────────────────┴───────────────────────────────────────────┘
```

#### Upload Flow

```
1. User drops file onto react-dropzone zone (or clicks to browse)
2. Client-side validation: file type (.md, .pdf, .txt), max size (10MB)
3. Upload via Server Action: multipart form data → Go backend
4. Backend processes document:
   a. Text extraction (PDF → text, URL → scraped content)
   b. Chunking (intelligent paragraph/section splitting)
   c. Embedding generation (Voyage AI)
   d. Storage in PostgreSQL (document + chunks + vectors)
5. Progress indicator shown during processing
6. On completion: document appears in list with chunk count
7. URL input alternative: paste a URL → backend fetches and processes
```

#### Document Editor

For `.md` and `.txt` documents, clicking "Edit" opens `document-editor.tsx`:
- Full-width textarea with monospace font
- Live markdown preview toggle (split view)
- Save triggers re-chunking and re-embedding
- Unsaved changes warning on navigation

---

### 2.5 Workflows Page

#### Layout Structure

```
┌─────────────────────────────────────────────────────────────────────────────┐
│  Header: "Workflows"                          [+ Create Workflow]           │
├─────────────────────────────────────────────────────────────────────────────┤
│  workflow-list.tsx                                                           │
│  ┌────────────────────────────────────────────────────────────────────────┐  │
│  │ Name              │ Trigger          │ Actions │ Runs │ Status │ Last  │  │
│  │ High-Score Reddit │ score > 7 AND    │ Draft → │ 156  │ [ON]   │ 2h    │  │
│  │                   │ platform=reddit  │ Notify  │      │        │ ago   │  │
│  │ HN Buying Signal  │ intent=buy AND   │ Draft → │ 89   │ [ON]   │ 5h    │  │
│  │                   │ platform=hn      │ Approve │      │        │ ago   │  │
│  │ X Competitor Buzz │ keyword=competitor│ Notify  │ 34   │ [OFF]  │ 1d    │  │
│  │                   │ AND score > 5    │         │      │        │ ago   │  │
│  └────────────────────────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────────────────────┘

Clicking a workflow row → navigates to /workflows/[id]:

┌─────────────────────────────────────────────────────────────────────────────┐
│  workflow-builder.tsx                                                        │
│                                                                              │
│  ┌─── trigger-config.tsx ─────┐     ┌─── action-chain.tsx ──────────────┐   │
│  │                            │     │                                    │   │
│  │ WHEN:                      │     │ THEN:                              │   │
│  │ Platform: [Reddit ▼]      │────▶│ 1. [Generate AI Draft ▼]          │   │
│  │ Keyword match: [CRM]      │     │ 2. [Send Slack Notification ▼]    │   │
│  │ Score threshold: [> 7]    │     │ 3. [Wait for Approval ▼]          │   │
│  │ Intent: [Buy Signal ▼]   │     │ 4. [Queue for Posting ▼]          │   │
│  │                            │     │                                    │   │
│  │ AND/OR conditions          │     │ [+ Add Action]                     │   │
│  └────────────────────────────┘     └────────────────────────────────────┘   │
│                                                                              │
│  ┌─── workflow-preview.tsx ─────────────────────────────────────────────┐    │
│  │ Preview: "When a Reddit mention matches 'CRM' with score > 7 and   │    │
│  │ intent is 'buy signal': generate AI draft, notify #sales on Slack, │    │
│  │ wait for team approval, then queue for posting with human-mimicry   │    │
│  │ delays."                                                             │    │
│  └──────────────────────────────────────────────────────────────────────┘    │
│                                                                              │
│  ┌─── execution-history.tsx ─────────────────────────────────────────────┐   │
│  │ | Run ID | Triggered At | Mention         | Status    | Duration |    │   │
│  │ | #156   | 2h ago       | "CRM alt..." | Completed | 45s      |    │   │
│  │ | #155   | 5h ago       | "Looking..."  | Approved  | 2m       |    │   │
│  │ | #154   | 1d ago       | "Help with.." | Rejected  | 1m       |    │   │
│  └───────────────────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────────────────┘
```

#### Trigger Configuration

Triggers are composed of conditions connected by AND/OR logic:

| Condition Type | Configuration | Example |
|---------------|--------------|---------|
| Platform | Multi-select dropdown | Reddit AND HN |
| Keyword Match | Text input with regex support | "CRM" OR "HubSpot alternative" |
| Score Threshold | Number slider (1-10) | Score >= 7 |
| Intent Type | Select dropdown | buy_signal, recommendation |
| Author Karma | Number input (platform-specific) | Reddit karma >= 100 |
| Subreddit/Forum | Text input | r/SaaS, r/startups |

#### Action Types

| Action | Description | Configuration |
|--------|------------|--------------|
| Generate AI Draft | Runs RAG Brain to draft reply variants | Persona selector, variant count |
| Send Notification | Pushes to Slack/Discord/email | Channel/webhook URL, message template |
| Wait for Approval | Pauses workflow until human approves | Timeout duration, fallback action |
| Queue for Posting | Adds to post queue with human-mimicry | Platform, delay range, posting account |
| Move to Pipeline | Creates/updates lead in pipeline | Target stage, assign to member |
| Add Tag | Applies tags to the mention/lead | Tag selector |

---

## 3. Chrome Extension Structure

### Complete File Tree

```
extension/
├── entrypoints/                          # WXT file-based routing for extension pages
│   ├── background.ts                     # Manifest V3 service worker
│   │                                     # - API polling via chrome.alarms
│   │                                     # - Message routing between content scripts and sidepanel
│   │                                     # - Badge update (unread mention count)
│   │                                     # - Auth token management (chrome.storage.session)
│   │                                     # - SSE connection to backend for real-time updates
│   │
│   ├── sidepanel/                        # Side panel UI (main extension interface)
│   │   ├── App.tsx                       # Root component with tab navigation
│   │   ├── index.html                    # HTML entry point (WXT generates manifest reference)
│   │   ├── main.tsx                      # React root mount + providers
│   │   └── tabs/
│   │       ├── MentionsTab.tsx           # Live mention feed with filters
│   │       │                             # - Mirrors inbox functionality in compact form
│   │       │                             # - Platform filter pills
│   │       │                             # - Tap mention → expand to see reply options
│   │       │                             # - "Open in Dashboard" link for complex workflows
│   │       │
│   │       ├── QueueTab.tsx              # Reply approval queue
│   │       │                             # - Pending replies awaiting approval
│   │       │                             # - Swipe/tap to approve or reject
│   │       │                             # - Edit button opens inline editor
│   │       │                             # - Shows target platform and posting account
│   │       │                             # - Countdown timer for auto-expire
│   │       │
│   │       └── SettingsTab.tsx           # Extension preferences
│   │                                     # - Backend URL configuration
│   │                                     # - Auth state display (logged in as...)
│   │                                     # - Notification preferences
│   │                                     # - Human-mimicry delay range slider
│   │                                     # - Platform toggle switches
│   │                                     # - "Disconnect" / logout
│   │
│   ├── content/                          # Content scripts injected into platform pages
│   │   ├── linkedin.ts                   # LinkedIn feed scanner
│   │   │                                 # - Runs on linkedin.com/feed/*
│   │   │                                 # - MutationObserver watches for new feed items
│   │   │                                 # - Extracts: post text, author, company, engagement metrics
│   │   │                                 # - Matches against user's keywords
│   │   │                                 # - Sends matches to service worker via chrome.runtime.sendMessage
│   │   │                                 # - Does NOT interact with LinkedIn DOM for posting
│   │   │                                 #   (posting handled by human-mimicry engine)
│   │   │
│   │   ├── reddit.ts                     # Reddit reply helper
│   │   │                                 # - Runs on reddit.com/r/*/comments/*
│   │   │                                 # - Detects when user is on a matched mention's page
│   │   │                                 # - Injects a floating "LeadEcho Reply" button near reply box
│   │   │                                 # - Button click opens sidepanel with pre-loaded reply variants
│   │   │                                 # - On "Post" action: uses human-mimicry to type into
│   │   │                                 #   Reddit's native reply box and submit
│   │   │
│   │   └── twitter.ts                    # X reply helper
│   │                                     # - Runs on x.com/*/status/*
│   │                                     # - Same pattern as reddit.ts:
│   │                                     #   detect matched mention → inject helper button
│   │                                     #   → open sidepanel with variants → human-mimicry posting
│   │                                     # - Handles X's contenteditable reply box
│   │                                     # - Manages character count awareness (280 limit)
│   │
│   └── popup/                            # Browser toolbar popup (quick glance)
│       ├── App.tsx                        # Compact status view
│       │                                 # - Unread mention count by platform
│       │                                 # - Quick link to open sidepanel
│       │                                 # - Connection status indicator (green/red)
│       │                                 # - "Open Dashboard" button
│       └── index.html                    # HTML entry point
│
├── components/                           # Shared React components for extension UI
│   ├── MentionCard.tsx                   # Compact mention display for sidepanel
│   ├── ReplyEditor.tsx                   # Inline reply editor with variant tabs
│   ├── PlatformBadge.tsx                 # Small platform icon + name badge
│   ├── StatusIndicator.tsx               # Connection status dot (green/yellow/red)
│   ├── ApprovalActions.tsx               # Approve/Reject/Edit button group
│   └── LoadingSpinner.tsx                # Extension-appropriate loading indicator
│
├── lib/
│   ├── api.ts                            # Backend API client
│   │                                     # - Wraps fetch with auth headers from chrome.storage.session
│   │                                     # - Base URL from extension settings
│   │                                     # - Methods: getMentions, draftReply, submitReply,
│   │                                     #   syncLinkedInSignals, getApprovalQueue
│   │                                     # - Error handling with retry logic
│   │                                     # - Request queuing for rate limiting
│   │
│   ├── messages.ts                       # Type-safe message protocol
│   │                                     # - Defines all message types as a discriminated union:
│   │                                     #   { type: "LINKEDIN_SIGNAL", payload: LinkedInSignal }
│   │                                     #   { type: "MENTION_MATCHED", payload: Mention }
│   │                                     #   { type: "REPLY_APPROVED", payload: Reply }
│   │                                     #   { type: "POST_REPLY", payload: PostReplyRequest }
│   │                                     #   { type: "OPEN_SIDEPANEL", payload: { tab: string } }
│   │                                     #   { type: "UPDATE_BADGE", payload: { count: number } }
│   │                                     # - Type-safe sendMessage and onMessage helpers
│   │                                     # - Ensures content script ↔ service worker ↔ sidepanel
│   │                                     #   communication is fully typed
│   │
│   ├── human-mimicry.ts                  # Typing simulation engine
│   │                                     # - simulateTyping(element, text, options):
│   │                                     #   Types text character-by-character into a DOM element
│   │                                     # - Variable delays per character: 50-150ms base
│   │                                     # - Natural pauses: longer after punctuation (200-400ms)
│   │                                     # - Typo simulation: occasional backspace + retype (2% chance)
│   │                                     # - Think pauses: random 1-3s pauses every 50-100 chars
│   │                                     # - Pre-type delay: 2-8s random wait before starting
│   │                                     # - Triggers proper input/keydown/keyup events
│   │                                     #   (not just setting .value — platforms detect this)
│   │                                     # - Configurable speed profiles: slow, normal, fast
│   │
│   └── storage.ts                        # chrome.storage helpers
│                                         # - Typed get/set wrappers for chrome.storage.local
│                                         # - Schema: { authToken, settings, cachedMentions,
│                                         #   lastSyncTimestamp, mimicryConfig }
│                                         # - Session storage for sensitive data (authToken)
│                                         # - Migration helpers for storage schema changes
│                                         # - Reactive storage: onChange listeners for UI updates
│
├── assets/                               # Extension static assets
│   ├── icon-16.png
│   ├── icon-48.png
│   └── icon-128.png
│
├── wxt.config.ts                         # WXT framework configuration
│                                         # - manifest: { permissions, host_permissions,
│                                         #   side_panel, content_scripts }
│                                         # - Permissions needed:
│                                         #   "sidePanel", "storage", "alarms",
│                                         #   "activeTab", "scripting"
│                                         # - Host permissions:
│                                         #   "*://*.reddit.com/*",
│                                         #   "*://*.linkedin.com/*",
│                                         #   "*://x.com/*",
│                                         #   API backend URL
│
├── tailwind.config.ts                    # Tailwind config (shared design tokens with web/)
├── tsconfig.json
└── package.json
```

### Extension Communication Architecture

```
┌──────────────────────────────────────────────────────────────────────────┐
│                     Chrome Extension Runtime                              │
│                                                                           │
│  ┌──────────────┐   chrome.runtime    ┌─────────────────────────────┐   │
│  │ Content      │   .sendMessage()     │ Service Worker              │   │
│  │ Scripts      │ ──────────────────▶  │ (background.ts)             │   │
│  │              │                      │                             │   │
│  │ linkedin.ts  │   { type: "SIGNAL",  │ Routes messages:            │   │
│  │ reddit.ts    │     payload: {...} } │ - SIGNAL → API sync         │   │
│  │ twitter.ts   │                      │ - POST_REPLY → content      │   │
│  │              │ ◀──────────────────  │ - MENTION → sidepanel       │   │
│  │              │   { type: "POST",    │                             │   │
│  │              │     payload: {...} } │ Also:                       │   │
│  └──────────────┘                      │ - chrome.alarms for polling │   │
│                                        │ - SSE connection to backend │   │
│        ▲                               │ - Badge count updates       │   │
│        │ DOM manipulation              └──────────┬──────────────────┘   │
│        │ (human-mimicry.ts)                       │                      │
│        │                              chrome.runtime                     │
│  ┌─────┴────────┐                     .sendMessage()                     │
│  │ Platform     │                             │                          │
│  │ Web Pages    │                             ▼                          │
│  │ (Reddit,     │                      ┌─────────────────────────────┐   │
│  │  X,          │                      │ Side Panel                   │   │
│  │  LinkedIn)   │                      │ (sidepanel/App.tsx)          │   │
│  └──────────────┘                      │                             │   │
│                                        │ Renders:                    │   │
│                                        │ - MentionsTab               │   │
│                                        │ - QueueTab                  │   │
│                                        │ - SettingsTab               │   │
│                                        └─────────────────────────────┘   │
│                                                                           │
└──────────────────────────────────────────────────────────────────────────┘
                                    │
                          REST API / SSE
                                    │
                                    ▼
                          ┌─────────────────┐
                          │ Go Backend      │
                          │ (Railway)       │
                          └─────────────────┘
```

### Content Script Lifecycle

Each content script follows a consistent pattern:

```
1. WXT injects script when URL matches manifest pattern
2. Script waits for DOM ready
3. Creates a MutationObserver on the platform's feed/comment container
4. On new DOM nodes:
   a. Extract text content and metadata from the platform's DOM structure
   b. Check against cached keyword list (fetched from chrome.storage)
   c. If match found: send SIGNAL message to service worker
5. Service worker:
   a. Deduplicates (checks mention ID against local cache)
   b. Syncs to backend API
   c. Updates badge count
   d. Optionally notifies sidepanel
6. For reply posting (triggered from sidepanel):
   a. Service worker sends POST_REPLY message to content script
   b. Content script locates the platform's reply input element
   c. human-mimicry.ts types the reply with natural delays
   d. Content script triggers the platform's submit action
   e. Reports success/failure back to service worker
```

---

## 4. Shared Types & Code

### Type Sharing Strategy

The web dashboard and Chrome extension share types and validation schemas. There are two approaches depending on the repository structure:

**Option A: Monorepo with shared package** (recommended)

```
packages/
├── shared/
│   ├── src/
│   │   ├── schemas/         # Zod schemas (source of truth)
│   │   ├── types/           # Inferred TypeScript types
│   │   ├── constants/       # Shared constants (platforms, stages, etc.)
│   │   └── index.ts         # Public API barrel file
│   ├── package.json         # "name": "@leadecho/shared"
│   └── tsconfig.json
├── web/                     # imports @leadecho/shared
└── extension/               # imports @leadecho/shared
```

**Option B: Copy pattern** (simpler, used if not monorepo)

A single source file in `web/src/lib/validations/` is the source of truth. A build script copies the relevant schemas to `extension/lib/schemas/`. Types are inferred from schemas via `z.infer` on both sides.

### Core Shared Types

```typescript
// === Platform ===

export const Platform = z.enum(["reddit", "hackernews", "twitter", "linkedin"]);
export type Platform = z.infer<typeof Platform>;

export const PLATFORM_CONFIG: Record<Platform, {
  name: string;
  color: string;          // Tailwind color class
  icon: string;           // Icon component name
  replyViaApi: boolean;   // Can reply server-side (Reddit, X) vs extension-only (LinkedIn)
  maxReplyLength: number; // Platform character limits
}> = {
  reddit:     { name: "Reddit",     color: "text-orange-500",  icon: "Reddit",   replyViaApi: true,  maxReplyLength: 10000 },
  hackernews: { name: "Hacker News", color: "text-orange-600",  icon: "HN",       replyViaApi: false, maxReplyLength: 5000  },
  twitter:    { name: "X",          color: "text-neutral-900",  icon: "Twitter",  replyViaApi: true,  maxReplyLength: 280   },
  linkedin:   { name: "LinkedIn",   color: "text-blue-600",     icon: "LinkedIn", replyViaApi: false, maxReplyLength: 3000  },
};

// === Mention ===

export const IntentType = z.enum([
  "buy_signal",        // "Looking for a tool that..."
  "recommendation",    // "Can anyone recommend..."
  "complaint",         // "Frustrated with X product..."
  "question",          // "How do I solve..."
  "discussion",        // General discussion about the space
]);

export const MentionStatus = z.enum([
  "unread",
  "read",
  "replied",
  "archived",
  "starred",
]);

export const MentionSchema = z.object({
  id:              z.string().uuid(),
  platform:        Platform,
  externalId:      z.string(),                // Platform-specific post/comment ID
  externalUrl:     z.string().url(),          // Direct URL to the mention on platform
  author:          z.string(),
  authorUrl:       z.string().url().optional(),
  content:         z.string(),
  snippet:         z.string().max(200),       // Truncated preview
  threadContext:    z.string().optional(),     // Parent/sibling comments
  relevanceScore:  z.number().min(1).max(10),
  intentType:      IntentType,
  matchedKeywords: z.array(z.string()),
  status:          MentionStatus,
  isStarred:       z.boolean(),
  createdAt:       z.string().datetime(),     // When the mention was posted
  detectedAt:      z.string().datetime(),     // When LeadEcho found it
});

export type Mention = z.infer<typeof MentionSchema>;

// === Lead ===

export const LeadStage = z.enum([
  "prospect",
  "qualified",
  "engaged",
  "converted",
  "lost",
]);

export const LeadSchema = z.object({
  id:            z.string().uuid(),
  companyName:   z.string(),
  contactName:   z.string(),
  contactUrl:    z.string().url().optional(),
  estimatedValue: z.number().min(0),
  stage:         LeadStage,
  sourceMention: MentionSchema.optional(),   // The mention that created this lead
  sourcePlatform: Platform,
  assignedTo:    z.string().uuid().optional(),
  notes:         z.string().optional(),
  lastActivity:  z.string().datetime(),
  createdAt:     z.string().datetime(),
  updatedAt:     z.string().datetime(),
});

export type Lead = z.infer<typeof LeadSchema>;

// === Reply ===

export const ReplyVariantType = z.enum(["value", "technical", "soft_sell"]);

export const ReplyVariantSchema = z.object({
  type:    ReplyVariantType,
  content: z.string(),
  label:   z.string(),         // Human-readable label: "Value-First", "Technical", "Soft-Sell"
});

export const ReplySchema = z.object({
  id:           z.string().uuid(),
  mentionId:    z.string().uuid(),
  variants:     z.array(ReplyVariantSchema).length(3),
  selectedVariant: ReplyVariantType.optional(),
  editedContent: z.string().optional(),        // User's edited version
  status:       z.enum(["drafted", "approved", "posted", "failed"]),
  platform:     Platform,
  postedAt:     z.string().datetime().optional(),
  utmUrl:       z.string().url().optional(),   // UTM-tagged link included in reply
  createdAt:    z.string().datetime(),
});

export type Reply = z.infer<typeof ReplySchema>;

// === Keyword ===

export const KeywordSchema = z.object({
  id:        z.string().uuid(),
  phrase:    z.string().min(2).max(100),
  platforms: z.array(Platform).min(1),          // Which platforms to monitor
  isActive:  z.boolean(),
  matchType: z.enum(["exact", "broad", "regex"]),
  createdAt: z.string().datetime(),
});

export type Keyword = z.infer<typeof KeywordSchema>;

// === Workflow ===

export const TriggerConditionSchema = z.object({
  field:    z.enum(["platform", "keyword", "score", "intent", "author_karma", "subreddit"]),
  operator: z.enum(["equals", "contains", "greater_than", "less_than", "in"]),
  value:    z.union([z.string(), z.number(), z.array(z.string())]),
});

export const WorkflowActionSchema = z.object({
  type:   z.enum(["draft_reply", "notify_slack", "notify_discord", "notify_email",
                   "wait_approval", "queue_post", "move_to_pipeline", "add_tag"]),
  config: z.record(z.unknown()),              // Action-specific configuration
  order:  z.number(),
});

export const WorkflowSchema = z.object({
  id:          z.string().uuid(),
  name:        z.string().min(1).max(100),
  description: z.string().optional(),
  triggers:    z.array(TriggerConditionSchema).min(1),
  triggerLogic: z.enum(["and", "or"]),
  actions:     z.array(WorkflowActionSchema).min(1),
  isEnabled:   z.boolean(),
  totalRuns:   z.number(),
  lastRunAt:   z.string().datetime().optional(),
  createdAt:   z.string().datetime(),
  updatedAt:   z.string().datetime(),
});

export type Workflow = z.infer<typeof WorkflowSchema>;

// === API Response Wrappers ===

export const PaginatedResponseSchema = <T extends z.ZodTypeAny>(itemSchema: T) =>
  z.object({
    data:       z.array(itemSchema),
    total:      z.number(),
    page:       z.number(),
    pageSize:   z.number(),
    totalPages: z.number(),
  });

export const ApiErrorSchema = z.object({
  code:    z.string(),
  message: z.string(),
  details: z.record(z.unknown()).optional(),
});

export type ApiError = z.infer<typeof ApiErrorSchema>;
```

---

## 5. State Management Architecture

### Data Flow Diagram

```
┌─────────────────────────────────────────────────────────────────────────┐
│                        DATA SOURCES                                      │
│                                                                          │
│  Go Backend (REST API)          Go Backend (SSE)         Browser         │
│  GET /mentions                  /mentions/stream         URL bar         │
│  POST /replies                  Events: mention.new      ?platform=...   │
│  PATCH /leads                   Events: reply.posted     ?sort=newest    │
│  GET /analytics                 Events: lead.updated                     │
└───────┬──────────────────────────────┬─────────────────────────┬────────┘
        │                              │                         │
        ▼                              ▼                         ▼
┌───────────────┐            ┌─────────────────┐      ┌──────────────────┐
│ TanStack      │            │ useSSE Hook     │      │ nuqs             │
│ Query         │            │                 │      │                  │
│               │            │ Connects to SSE │      │ Parses URL       │
│ queryFn calls │            │ endpoint.       │      │ search params    │
│ lib/api/*     │            │ On each event:  │      │ into typed       │
│ functions.    │ ◀──inject──│ calls           │      │ state objects.   │
│               │            │ queryClient     │      │                  │
│ Caches server │            │ .setQueryData() │      │ Changes trigger  │
│ data. Handles │            │ to prepend/     │      │ TanStack Query   │
│ pagination,   │            │ update cached   │      │ refetch via      │
│ invalidation, │            │ data.           │      │ query key change │
│ optimistic    │            └─────────────────┘      └──────────────────┘
│ updates.      │                                              │
│               │◀─────────────────────────────────────────────┘
└───────┬───────┘            query key includes filter params
        │
        │ useQuery / useMutation return values
        │ { data, isLoading, error, refetch }
        ▼
┌───────────────────────────────────────────────────────────────────────────┐
│                        REACT COMPONENTS                                    │
│                                                                            │
│  Server Components              Client Components                          │
│  (sidebar, settings shell,      (inbox, pipeline, charts,                 │
│   static content)                reply composer, DnD)                      │
│                                                                            │
│  Read data via:                 Read data via:                              │
│  - Server Actions               - useQuery hooks                           │
│  - Direct fetch (RSC)           - useQueryStates (nuqs)                   │
│  - No client JS needed          - useStore (Zustand)                       │
│                                 - useForm (React Hook Form)                │
└───────────────────────────────────────────────────────────────────────────┘
        │
        │ UI interactions (click, type, drag)
        ▼
┌───────────────────────────────────────────────────────────────────────────┐
│                        CLIENT STATE STORES                                 │
│                                                                            │
│  Zustand Stores (UI only)       React Hook Form           nuqs URL State   │
│                                                                            │
│  sidebar-store:                 Mention reply form:        Filter params:   │
│  - isCollapsed                  - variant selection         - platform      │
│  - activeNavItem                - edited content            - status        │
│                                 - platform selection        - intent        │
│  inbox-store:                                               - keyword (q)  │
│  - selectedMentionId           Lead create/edit form:       - dateFrom     │
│  - isComposerOpen              - company, contact, value    - dateTo       │
│  - composerMode                - stage, assignee            - sort         │
│                                                             - page         │
│  pipeline-store:               Workflow builder form:                       │
│  - isDragging                  - triggers array                            │
│  - activeDetailId              - actions array                             │
│  - dragOverColumn              - name, description                         │
│                                                                            │
│  command-menu-store:           Settings forms:                              │
│  - isOpen                      - workspace, notifications                  │
│                                - keywords, platforms                        │
│  notification-store:                                                       │
│  - unreadCount                                                             │
│  - toastQueue                                                              │
└───────────────────────────────────────────────────────────────────────────┘
```

### State Category Rules

These rules prevent state management confusion. Every piece of state falls into exactly one category:

| State Category | Tool | When to Use | Examples |
|---------------|------|------------|---------|
| **Server data** | TanStack Query | Any data that comes from the API. Never duplicated elsewhere. | Mentions, leads, analytics, documents, workflows |
| **Real-time data** | SSE → TanStack Query | Events pushed from the server. Injected into TanStack Query cache, not stored separately. | New mentions, status updates, reply confirmations |
| **URL state** | nuqs | Any state that should survive page refresh, be shareable via link, or affect data fetching. | Filters, pagination, sort order, date ranges, active tab |
| **UI state** | Zustand | Ephemeral client-only state that does not affect data fetching and does not need to survive refresh. | Sidebar open/closed, selected item, modal visibility, drag state |
| **Form state** | React Hook Form | All form inputs and validation state. Reset on navigation. | Reply editor, lead form, workflow builder, settings forms |

### TanStack Query Key Factory

```typescript
// src/lib/query-keys.ts
// Hierarchical key factory for type-safe cache invalidation

export const mentionKeys = {
  all:      ["mentions"] as const,
  lists:    () => [...mentionKeys.all, "list"] as const,
  list:     (filters: MentionFilters) => [...mentionKeys.lists(), filters] as const,
  details:  () => [...mentionKeys.all, "detail"] as const,
  detail:   (id: string) => [...mentionKeys.details(), id] as const,
};

export const leadKeys = {
  all:      ["leads"] as const,
  lists:    () => [...leadKeys.all, "list"] as const,
  list:     (filters: LeadFilters) => [...leadKeys.lists(), filters] as const,
  details:  () => [...leadKeys.all, "detail"] as const,
  detail:   (id: string) => [...leadKeys.details(), id] as const,
  pipeline: (filters?: PipelineFilters) => [...leadKeys.all, "pipeline", filters] as const,
};

export const analyticsKeys = {
  all:              ["analytics"] as const,
  kpis:             (range: DateRange) => [...analyticsKeys.all, "kpis", range] as const,
  mentionTrends:    (range: DateRange) => [...analyticsKeys.all, "mention-trends", range] as const,
  conversionFunnel: (range: DateRange) => [...analyticsKeys.all, "funnel", range] as const,
  keywordTable:     (range: DateRange) => [...analyticsKeys.all, "keywords", range] as const,
  platformComparison: (range: DateRange) => [...analyticsKeys.all, "platforms", range] as const,
};

export const workflowKeys = {
  all:        ["workflows"] as const,
  lists:      () => [...workflowKeys.all, "list"] as const,
  list:       () => [...workflowKeys.lists()] as const,
  detail:     (id: string) => [...workflowKeys.all, "detail", id] as const,
  executions: (id: string) => [...workflowKeys.all, "executions", id] as const,
};

export const knowledgeBaseKeys = {
  all:      ["knowledge-base"] as const,
  lists:    () => [...knowledgeBaseKeys.all, "list"] as const,
  list:     () => [...knowledgeBaseKeys.lists()] as const,
  detail:   (id: string) => [...knowledgeBaseKeys.all, "detail", id] as const,
  chunks:   (id: string) => [...knowledgeBaseKeys.all, "chunks", id] as const,
};
```

### SSE Integration Pattern

```typescript
// src/hooks/use-sse.ts
// Connects to SSE endpoint, injects events into TanStack Query cache

export function useSSE(url: string) {
  const queryClient = useQueryClient();

  useEffect(() => {
    const eventSource = new EventSource(url);
    let reconnectTimeout: ReturnType<typeof setTimeout>;
    let reconnectDelay = 1000; // Start at 1s, max 30s

    eventSource.onmessage = (event) => {
      const data = JSON.parse(event.data);
      reconnectDelay = 1000; // Reset on successful message

      switch (data.type) {
        case "mention.new":
          // Prepend new mention to all matching list queries
          queryClient.setQueriesData(
            { queryKey: mentionKeys.lists() },
            (old: PaginatedResponse<Mention> | undefined) => {
              if (!old) return old;
              return {
                ...old,
                data: [data.payload, ...old.data],
                total: old.total + 1,
              };
            }
          );
          break;

        case "mention.updated":
          // Update specific mention in cache
          queryClient.setQueryData(
            mentionKeys.detail(data.payload.id),
            data.payload
          );
          break;

        case "reply.posted":
          // Invalidate mention to refresh status
          queryClient.invalidateQueries({
            queryKey: mentionKeys.detail(data.payload.mentionId),
          });
          break;

        case "lead.updated":
          // Invalidate pipeline to refresh kanban
          queryClient.invalidateQueries({
            queryKey: leadKeys.all,
          });
          break;
      }
    };

    eventSource.onerror = () => {
      eventSource.close();
      reconnectTimeout = setTimeout(() => {
        // Reconnect with exponential backoff
        reconnectDelay = Math.min(reconnectDelay * 2, 30000);
        // Re-run effect by updating a ref or state
      }, reconnectDelay);
    };

    return () => {
      eventSource.close();
      clearTimeout(reconnectTimeout);
    };
  }, [url, queryClient]);
}
```

---

## 6. Component Design System

### Foundation: shadcn/ui

All base components come from shadcn/ui, installed via the CLI and customized through the `components.json` config. These are owned code (not a dependency), meaning they live in `src/components/ui/` and can be modified directly.

### Design Tokens (CSS Variables)

All visual tokens are defined as CSS variables in `globals.css`. This enables dark mode switching by redefining variables under `.dark`:

```css
/* src/app/globals.css */

@layer base {
  :root {
    /* === Layout === */
    --sidebar-width: 280px;
    --sidebar-collapsed-width: 68px;
    --header-height: 56px;

    /* === Colors (OKLCH for perceptual uniformity) === */
    --background: 0 0% 100%;
    --foreground: 0 0% 3.9%;
    --card: 0 0% 100%;
    --card-foreground: 0 0% 3.9%;
    --popover: 0 0% 100%;
    --popover-foreground: 0 0% 3.9%;
    --primary: 0 0% 9%;
    --primary-foreground: 0 0% 98%;
    --secondary: 0 0% 96.1%;
    --secondary-foreground: 0 0% 9%;
    --muted: 0 0% 96.1%;
    --muted-foreground: 0 0% 45.1%;
    --accent: 0 0% 96.1%;
    --accent-foreground: 0 0% 9%;
    --destructive: 0 84.2% 60.2%;
    --destructive-foreground: 0 0% 98%;
    --border: 0 0% 89.8%;
    --input: 0 0% 89.8%;
    --ring: 0 0% 3.9%;
    --radius: 0.5rem;

    /* === Platform Brand Colors === */
    --platform-reddit: 12 100% 50%;         /* #FF4500 - Reddit Orange */
    --platform-hackernews: 26 100% 50%;     /* #FF6600 - HN Orange */
    --platform-twitter: 0 0% 0%;             /* #000000 - X Black */
    --platform-linkedin: 210 100% 40%;       /* #0A66C2 - LinkedIn Blue */

    /* === Relevance Score Colors === */
    --score-high: 142 76% 36%;               /* Green: 8-10 */
    --score-medium: 45 93% 47%;              /* Yellow: 5-7 */
    --score-low: 0 0% 64%;                   /* Gray: 1-4 */

    /* === Pipeline Stage Colors === */
    --stage-prospect: 210 100% 50%;          /* Blue */
    --stage-qualified: 262 83% 58%;          /* Purple */
    --stage-engaged: 45 93% 47%;             /* Yellow */
    --stage-converted: 142 76% 36%;          /* Green */
    --stage-lost: 0 84% 60%;                 /* Red */

    /* === Spacing Scale === */
    --space-1: 0.25rem;   /* 4px */
    --space-2: 0.5rem;    /* 8px */
    --space-3: 0.75rem;   /* 12px */
    --space-4: 1rem;      /* 16px */
    --space-6: 1.5rem;    /* 24px */
    --space-8: 2rem;      /* 32px */

    /* === Typography === */
    --font-sans: "Inter", system-ui, -apple-system, sans-serif;
    --font-mono: "JetBrains Mono", "Fira Code", monospace;
  }

  .dark {
    --background: 0 0% 3.9%;
    --foreground: 0 0% 98%;
    --card: 0 0% 3.9%;
    --card-foreground: 0 0% 98%;
    --popover: 0 0% 3.9%;
    --popover-foreground: 0 0% 98%;
    --primary: 0 0% 98%;
    --primary-foreground: 0 0% 9%;
    --secondary: 0 0% 14.9%;
    --secondary-foreground: 0 0% 98%;
    --muted: 0 0% 14.9%;
    --muted-foreground: 0 0% 63.9%;
    --accent: 0 0% 14.9%;
    --accent-foreground: 0 0% 98%;
    --destructive: 0 62.8% 30.6%;
    --destructive-foreground: 0 0% 98%;
    --border: 0 0% 14.9%;
    --input: 0 0% 14.9%;
    --ring: 0 0% 83.1%;

    /* Platform colors stay the same in dark mode */
    /* Score/stage colors stay the same in dark mode */
  }
}
```

### Dark Mode Implementation

Dark mode uses the `class` strategy via `next-themes`:

```tsx
// src/providers/theme-provider.tsx
import { ThemeProvider as NextThemesProvider } from "next-themes";

export function ThemeProvider({ children }: { children: React.ReactNode }) {
  return (
    <NextThemesProvider
      attribute="class"      // Adds .dark to <html>
      defaultTheme="system"  // Respects OS preference
      enableSystem            // Watches prefers-color-scheme
      disableTransitionOnChange // Prevents FOUC during theme switch
    >
      {children}
    </NextThemesProvider>
  );
}
```

Toggle component in the header:
```tsx
// Uses shadcn/ui Toggle or DropdownMenu with three options: Light, Dark, System
// Stored in localStorage by next-themes automatically
```

### Custom Component Patterns

All custom components follow these conventions:

**1. Variant definitions via CVA (class-variance-authority):**

```tsx
// Example: platform-badge.tsx
import { cva, type VariantProps } from "class-variance-authority";

const platformBadgeVariants = cva(
  "inline-flex items-center gap-1 rounded-full px-2 py-0.5 text-xs font-medium",
  {
    variants: {
      platform: {
        reddit:     "bg-[hsl(var(--platform-reddit)/0.1)] text-[hsl(var(--platform-reddit))]",
        hackernews: "bg-[hsl(var(--platform-hackernews)/0.1)] text-[hsl(var(--platform-hackernews))]",
        twitter:    "bg-[hsl(var(--platform-twitter)/0.1)] text-[hsl(var(--platform-twitter))]",
        linkedin:   "bg-[hsl(var(--platform-linkedin)/0.1)] text-[hsl(var(--platform-linkedin))]",
      },
    },
  }
);

interface PlatformBadgeProps extends VariantProps<typeof platformBadgeVariants> {
  platform: Platform;
}

export function PlatformBadge({ platform }: PlatformBadgeProps) {
  return (
    <span className={platformBadgeVariants({ platform })}>
      <PlatformIcon platform={platform} className="h-3 w-3" />
      {PLATFORM_CONFIG[platform].name}
    </span>
  );
}
```

**2. Composition over configuration:**

Components are composed from shadcn primitives rather than building monolithic components with many props.

```tsx
// Good: Composed from primitives
<Card>
  <CardHeader>
    <CardTitle>Total Mentions</CardTitle>
  </CardHeader>
  <CardContent>
    <KPIValue value={1234} trend={12} />
    <Sparkline data={sparklineData} />
  </CardContent>
</Card>

// Bad: Monolithic component with many props
<KPICard
  title="Total Mentions"
  value={1234}
  trend={12}
  sparklineData={sparklineData}
  showTrend
  showSparkline
/>
```

**3. Props interface convention:**

```tsx
// Always extend React.HTMLAttributes for wrapper elements
interface MentionItemProps extends React.HTMLAttributes<HTMLDivElement> {
  mention: Mention;
  isSelected: boolean;
}

// Use forwardRef for components that need ref forwarding
const MentionItem = React.forwardRef<HTMLDivElement, MentionItemProps>(
  ({ mention, isSelected, className, ...props }, ref) => {
    return (
      <div
        ref={ref}
        className={cn("...", isSelected && "bg-accent", className)}
        {...props}
      />
    );
  }
);
```

### Component Inventory

| Category | Component | Built On | Purpose |
|----------|----------|----------|---------|
| **Layout** | Sidebar | shadcn Sheet (mobile) + custom (desktop) | Main navigation with collapse/expand |
| **Layout** | Header | custom | Breadcrumbs, search, notifications, user menu |
| **Layout** | CommandMenu | shadcn Command (cmdk) | Global search and quick navigation (Cmd+K) |
| **Data Display** | MentionItem | shadcn Card | Compact mention row in list |
| **Data Display** | LeadCard | shadcn Card + @dnd-kit useSortable | Draggable pipeline card |
| **Data Display** | KPICard | shadcn Card + Recharts | Metric with trend and sparkline |
| **Data Display** | PlatformBadge | shadcn Badge + CVA | Colored platform indicator |
| **Data Display** | ScoreBadge | shadcn Badge + CVA | Relevance score with color coding |
| **Data Display** | RelativeTime | custom | "2h ago" with absolute tooltip |
| **Data Entry** | ReplyComposer | shadcn Tabs + Textarea + Button | AI variant selection and editing |
| **Data Entry** | InboxFilters | shadcn Select + Popover + Input | Multi-filter bar with URL sync |
| **Data Entry** | DateRangePicker | shadcn Calendar + Popover | Date range selection for analytics |
| **Data Entry** | DocumentUpload | react-dropzone + shadcn Dialog | File upload with drag-and-drop |
| **Data Entry** | WorkflowBuilder | shadcn Card + Select + custom | Trigger/action chain editor |
| **Feedback** | EmptyState | custom | Illustration + message for empty lists |
| **Feedback** | ErrorBoundary | custom | Catch and display component errors |
| **Feedback** | Toast | sonner (via shadcn) | Success/error notification popups |
| **Feedback** | ConfirmDialog | shadcn AlertDialog | Destructive action confirmation |
| **Charts** | AreaChart | Recharts via shadcn chart | Stacked area for trends |
| **Charts** | BarChart | Recharts via shadcn chart | Grouped bars for comparison |
| **Charts** | FunnelChart | Recharts via shadcn chart | Conversion funnel |

---

## 7. Performance Optimizations

### Server Components vs. Client Components

The default in the App Router is Server Components. Only add `"use client"` when the component requires interactivity.

| Component | Type | Reason |
|-----------|------|--------|
| Sidebar navigation | Server | Static links, no interactivity beyond CSS hover |
| Sidebar collapse toggle | Client | Requires onClick + Zustand store |
| Header (breadcrumbs) | Server | Generated from route segments, no client JS |
| Header (notification bell) | Client | Real-time count, dropdown interaction |
| Inbox filters | Client | User interaction, URL state management |
| Mention list | Client | SSE subscription, selection state, virtual scrolling |
| Mention detail | Client | Dynamic content based on selection |
| Reply composer | Client | Form state, API mutations, interactive editing |
| Pipeline board | Client | Drag-and-drop requires DOM interaction |
| Analytics KPI cards | Server | Static render from fetched data (no interaction) |
| Analytics charts | Client | Recharts requires DOM, tooltips require interaction |
| Settings pages (forms) | Client | Form inputs and submission |
| Settings pages (shell) | Server | Static layout, tab navigation |
| Knowledge base list | Client | Selection, upload interaction |
| Workflow list | Client | Toggle switches, navigation |

### Suspense Boundaries & Loading States

Every `(dashboard)` route has a `loading.tsx` that renders a skeleton matching the page layout. This provides instant visual feedback while data loads.

```tsx
// src/app/(dashboard)/inbox/loading.tsx
export default function InboxLoading() {
  return (
    <div className="flex h-full flex-col">
      {/* Toolbar skeleton */}
      <div className="flex items-center gap-2 border-b p-4">
        <Skeleton className="h-8 w-24" />
        <Skeleton className="h-8 w-24" />
        <Skeleton className="ml-auto h-8 w-32" />
      </div>
      {/* Filter bar skeleton */}
      <div className="flex items-center gap-2 border-b p-3">
        <Skeleton className="h-8 w-28" />
        <Skeleton className="h-8 w-28" />
        <Skeleton className="h-8 w-28" />
        <Skeleton className="h-8 w-48" />
      </div>
      {/* Split panel skeleton */}
      <div className="flex flex-1">
        {/* Mention list skeleton */}
        <div className="w-[400px] border-r p-3 space-y-3">
          {Array.from({ length: 8 }).map((_, i) => (
            <div key={i} className="space-y-2 p-3">
              <Skeleton className="h-4 w-3/4" />
              <Skeleton className="h-3 w-full" />
              <Skeleton className="h-3 w-1/2" />
            </div>
          ))}
        </div>
        {/* Detail panel skeleton */}
        <div className="flex-1 p-6 space-y-4">
          <Skeleton className="h-6 w-1/3" />
          <Skeleton className="h-4 w-full" />
          <Skeleton className="h-4 w-full" />
          <Skeleton className="h-4 w-2/3" />
          <Skeleton className="mt-8 h-32 w-full" />
        </div>
      </div>
    </div>
  );
}
```

### Virtual Scrolling

The mention list in the inbox can contain hundreds or thousands of items. Rendering all of them is not viable. Virtual scrolling renders only the visible items plus a small buffer.

Implementation using TanStack Virtual:

```tsx
// Inside mention-list.tsx
import { useVirtualizer } from "@tanstack/react-virtual";

function MentionList() {
  const parentRef = useRef<HTMLDivElement>(null);
  const { data } = useMentions(filters); // TanStack Query

  const virtualizer = useVirtualizer({
    count: data?.data.length ?? 0,
    getScrollElement: () => parentRef.current,
    estimateSize: () => 88, // Estimated height of a mention item in px
    overscan: 5,            // Render 5 extra items above/below viewport
  });

  return (
    <div ref={parentRef} className="h-full overflow-auto">
      <div
        style={{ height: `${virtualizer.getTotalSize()}px`, position: "relative" }}
      >
        {virtualizer.getVirtualItems().map((virtualItem) => (
          <div
            key={virtualItem.key}
            style={{
              position: "absolute",
              top: 0,
              left: 0,
              width: "100%",
              height: `${virtualItem.size}px`,
              transform: `translateY(${virtualItem.start}px)`,
            }}
          >
            <MentionItem
              mention={data!.data[virtualItem.index]}
              isSelected={selectedId === data!.data[virtualItem.index].id}
            />
          </div>
        ))}
      </div>
    </div>
  );
}
```

### Infinite Scroll Pagination

Rather than loading all mentions at once, the list uses cursor-based pagination with an intersection observer trigger at the bottom:

```tsx
// Infinite query with TanStack Query
const { data, fetchNextPage, hasNextPage, isFetchingNextPage } = useInfiniteQuery({
  queryKey: mentionKeys.list(filters),
  queryFn: ({ pageParam }) => getMentions({ ...filters, cursor: pageParam }),
  getNextPageParam: (lastPage) => lastPage.nextCursor,
  initialPageParam: undefined,
});

// Intersection observer triggers fetchNextPage when scroll reaches bottom
const { ref: loadMoreRef } = useIntersection({
  onIntersect: () => {
    if (hasNextPage && !isFetchingNextPage) {
      fetchNextPage();
    }
  },
});
```

### Image Optimization

All images (platform icons, avatars, empty state illustrations) use `next/image`:

```tsx
import Image from "next/image";

// Platform icons served as optimized SVGs
<Image
  src={`/icons/${platform}.svg`}
  alt={`${PLATFORM_CONFIG[platform].name} icon`}
  width={16}
  height={16}
  className="inline-block"
/>

// User avatars (external URLs) with domain allowlist in next.config.ts
<Image
  src={authorAvatarUrl}
  alt={`${authorName} avatar`}
  width={32}
  height={32}
  className="rounded-full"
/>
```

`next.config.ts` image configuration:

```typescript
const nextConfig: NextConfig = {
  images: {
    remotePatterns: [
      { protocol: "https", hostname: "**.reddit.com" },
      { protocol: "https", hostname: "**.redd.it" },
      { protocol: "https", hostname: "pbs.twimg.com" },
      { protocol: "https", hostname: "**.licdn.com" },
      { protocol: "https", hostname: "img.clerk.com" },
    ],
  },
};
```

### Bundle Optimization

**Code splitting** happens automatically per route in the App Router. Each page is its own chunk. Additional optimizations:

```typescript
// Dynamic imports for heavy components not needed on initial load
const WorkflowBuilder = dynamic(
  () => import("./_components/workflow-builder"),
  { loading: () => <WorkflowBuilderSkeleton /> }
);

const ChartContainer = dynamic(
  () => import("@/components/charts/area-chart"),
  { ssr: false } // Charts don't need SSR
);
```

**Bundle analysis** via `@next/bundle-analyzer`:

```bash
# In package.json scripts:
"analyze": "ANALYZE=true next build"
```

```typescript
// next.config.ts
import bundleAnalyzer from "@next/bundle-analyzer";

const withBundleAnalyzer = bundleAnalyzer({
  enabled: process.env.ANALYZE === "true",
});

export default withBundleAnalyzer(nextConfig);
```

### Prefetching Strategy

- Next.js automatically prefetches `<Link>` destinations when they enter the viewport
- The sidebar navigation links are always visible, so their pages are prefetched on initial load
- For the pipeline detail sheet: prefetch lead detail data on card hover using TanStack Query `prefetchQuery`
- For inbox: prefetch mention detail on hover with a 200ms delay (cancel on mouse leave)

```tsx
// Mention item hover prefetching
const queryClient = useQueryClient();

function handleMouseEnter(mentionId: string) {
  queryClient.prefetchQuery({
    queryKey: mentionKeys.detail(mentionId),
    queryFn: () => getMention(mentionId),
    staleTime: 30_000, // Consider fresh for 30s
  });
}
```

### Performance Targets

| Metric | Target | Strategy |
|--------|--------|----------|
| First Contentful Paint | < 1.0s | Server Components for shell, streaming SSR |
| Largest Contentful Paint | < 2.0s | Skeleton loading states, font optimization |
| Time to Interactive | < 2.5s | Code splitting, deferred hydration for non-critical UI |
| Cumulative Layout Shift | < 0.05 | Fixed dimensions on skeletons, `next/image` aspect ratios |
| Interaction to Next Paint | < 100ms | Virtual scrolling, optimistic updates, debounced inputs |
| SSE Event Display Latency | < 300ms | Direct cache mutation (no refetch), no debounce on SSE |
| Mention List Scroll | 60fps | TanStack Virtual, no re-renders during scroll |
| Pipeline Drag | 60fps | DnD Kit with CSS transforms, no layout recalculation |

---

*This document defines the implementation-ready frontend architecture for LeadEcho. All directory structures, component specifications, state management patterns, and performance strategies are specified to enable parallel development across the dashboard and extension workstreams.*
