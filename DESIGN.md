# Email & Communication Platform — Comprehensive Design System & Architecture

> **Status:** Production-Ready Design Specification v1.0
> **Target:** Enterprise Email Platform with Admin Portal, Developer Infrastructure & Corporate Mail
> **Frontend:** React + Next.js + TypeScript + Tailwind CSS + shadcn/ui
> **Design Direction:** Modern Enterprise SaaS (Linear × Vercel × Resend aesthetics)
> **Last Updated:** 2026-08-20

---

> ## ⚠ Implemented design system (supersedes theming sections below)
>
> The shipped web UI (`apps/web`) implements a **single Silver / Graphite theme** — there is
> no light/dark switching and no public landing page (`/` redirects into the product).
> Source of truth: `apps/web/src/app/globals.css` (tokens) and `apps/web/src/components/ui.tsx` (kit).
>
> - **Surfaces:** app canvas `#eff1f5`, sidebar `#e6e9ee`, work surfaces `#fbfcfe`, hover `#dde1e8`
> - **Ink:** primary `#161d27`, secondary `#5c6470`, tertiary `#838b96`; borders `#d7dbe1` / `#bfc5cd`
> - **Selection / primary action:** graphite `#242c38`; accent (links, focus, unread): steel blue `#2961ae`
> - **Radius:** controls 7px · panels 10px · dialogs 12px; hairline 1px borders, minimal shadows
> - **Type:** Inter 13–13.5px body, 20px page titles; JetBrains Mono for IDs, IPs and timestamps
> - **Shell:** Sidebar → Top bar → Workspace; mail is a split view (list + reading pane),
>   compose is a floating window, admin includes a Logs & Audit center; `Ctrl/Cmd+K` command palette
>
> Where a section below (e.g. “Dark Theme”) conflicts with this, the implemented system wins.

---

## Table of Contents

1. [Design Vision & Principles](#1-design-vision--principles)
2. [Color System & Typography](#2-color-system--typography)
3. [Component Library](#3-component-library)
4. [Application Shell](#4-application-shell)
5. [Core Modules & Screens](#5-core-modules--screens)
6. [User Flows & Interactions](#6-user-flows--interactions)
7. [Responsive Design](#7-responsive-design)
8. [Accessibility & Dark Mode](#8-accessibility--dark-mode)

---

# 1. Design Vision & Principles

## 1.1 Brand Direction

The platform should be perceived as a modern, professional solution for email infrastructure, security, and corporate communications.

**Core Associations:**
- Reliability & stability
- Speed & performance
- Security & trust
- Technical maturity
- Minimalism & clarity
- High information density without visual noise

**Visual References:** Linear, Vercel, Resend, modern developer-focused SaaS platforms

## 1.2 Core Design Principles

### Clarity First
Every screen must immediately answer:
1. Where am I?
2. What's happening?
3. What needs my attention?
4. What's my primary action?

Decorative elements never compete with data.

### Professional Density
The platform handles massive amounts of data:
- Email events & logs
- Domain configurations
- DNS records
- API keys & credentials
- Security incidents
- Delivery analytics
- User & organization hierarchies

**Balance:** Data-rich but not overwhelming. Compact but not cluttered.

### Progressive Disclosure
- Primary information always visible
- Technical details revealed through:
  - Expandable rows & sections
  - Side panels & drawers
  - Tabs & accordion
  - Inline tooltips
  - "Show details" links
  - Advanced toggles

### Visual Consistency
- Same entities look identical across the platform
- `Delivered` status uses one consistent style
- Domain health scores use one scale
- Destructive actions follow one pattern
- All notifications follow one design

### Role-Based Clarity
- Non-technical users see Mail + Messages + Reminders
- Technical users see Logs + DNS + API + SMTP
- Admins have additional admin sections
- Navigation adapts to user role/permissions

---

# 2. Color System & Typography

## 2.1 Light Theme (Primary)

### Semantic Colors

| Purpose | Color | Hex | Usage |
|---------|-------|-----|-------|
| **Primary Background** | White | `#FFFFFF` | Main content area |
| **Secondary Bg** | Off-white | `#F8FAFC` | Cards, sections, hover states |
| **Tertiary Bg** | Light gray | `#F1F5F9` | Disabled, placeholder states |
| **Border** | Light gray | `#E5E7EB` | Dividers, borders, separators |
| **Border Strong** | Medium gray | `#CBD5E1` | Important borders |
| **Primary Text** | Dark graphite | `#0F172A` | Headlines, body text |
| **Secondary Text** | Neutral gray | `#64748B` | Descriptions, metadata |
| **Tertiary Text** | Light gray | `#94A3B8` | Placeholder, disabled |
| **Accent (Primary)** | Modern Blue | `#3B82F6` | Links, primary actions, active states |
| **Accent (Secondary)** | Bright Blue | `#60A5FA` | Hover states for accent |
| **Success** | Green | `#10B981` | Delivered, success, verified |
| **Warning** | Amber | `#F59E0B` | Deferred, pending, caution |
| **Error** | Red | `#EF4444` | Bounced, failed, blocked |
| **Info** | Cyan | `#06B6D4` | Information, neutral alerts |
| **Muted** | Gray | `#6B7280` | Secondary actions, inactive |

### Specific Component Colors

| Element | Color | Hex | Context |
|---------|-------|-----|---------|
| **Delivered** Badge | Green + Light Green Bg | `#10B981` + `#DBEAFE` | Success status |
| **Bounced** Badge | Red + Light Red Bg | `#EF4444` + `#FEE2E2` | Error status |
| **Deferred** Badge | Amber + Light Amber Bg | `#F59E0B` + `#FEF3C7` | Warning status |
| **Queued** Badge | Blue + Light Blue Bg | `#3B82F6` + `#EFF6FF` | Neutral status |
| **Quarantined** Badge | Orange-Red + Light Bg | `#F97316` + `#FFF7ED` | Security issue |
| **Risk Score High** | Red | `#DC2626` | Risk > 75 |
| **Risk Score Medium** | Amber | `#D97706` | Risk 40-75 |
| **Risk Score Low** | Green | `#059669` | Risk < 40 |

## 2.2 Dark Theme (Full Support)

| Purpose | Color | Hex |
|---------|-------|-----|
| **Primary Background** | Charcoal | `#0F172A` |
| **Secondary Bg** | Dark slate | `#1E293B` |
| **Tertiary Bg** | Slate | `#334155` |
| **Border** | Slate | `#475569` |
| **Text Primary** | Off-white | `#F8FAFC` |
| **Text Secondary** | Light gray | `#CBD5E1` |
| **Accent** | Bright Blue | `#60A5FA` |

Dark mode adjusts all semantic colors proportionally while maintaining contrast ratios.

## 2.3 Typography

### Font Family
- **Primary:** Inter (system font fallback: -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif)
- **Monospace:** JetBrains Mono or "Monaco" (for code snippets, API responses, DNS records)

### Type Scale

| Role | Size | Weight | Line Height | Letter Spacing | Usage |
|------|------|--------|-------------|---------------|----- |
| **Page Title** | 32px | 700 | 1.2 | -0.01em | Main page heading |
| **Section Title** | 24px | 700 | 1.3 | -0.01em | Major sections |
| **Subsection** | 20px | 600 | 1.3 | 0 | Section headers |
| **UI Heading** | 16px | 600 | 1.5 | 0 | Card titles, dialog titles |
| **Body Large** | 16px | 400 | 1.5 | 0 | Default body text |
| **Body** | 14px | 400 | 1.5 | 0 | Secondary text, descriptions |
| **Small** | 12px | 400 | 1.5 | 0.01em | Labels, captions, metadata |
| **Tiny** | 11px | 500 | 1.4 | 0.01em | Small labels, timestamps |
| **Code** | 13px | 400 | 1.6 | 0 | API keys, email addresses, DNS |

### Responsive Typography
- Desktop: Full type scale as above
- Tablet (768px): Reduce title by 2-4px
- Mobile: Optimize for readability, increase line height

---

# 3. Component Library

## 3.1 Core Components

### Buttons

**Variants:**
- **Primary:** Blue background, white text. Main actions. Hover: darker blue.
- **Secondary:** Light blue background, blue text. Alternative actions. Hover: lighter blue.
- **Ghost:** Transparent, blue text. Links/low priority. Hover: light blue background.
- **Danger:** Red background, white text. Destructive actions. Hover: darker red.
- **Muted:** Gray background, dark text. Neutral actions. Hover: darker gray.

**Sizes:**
- **Large:** 48px height, 16px text, 16px padding (desktop primary)
- **Medium:** 40px height, 14px text, 12px padding (default)
- **Small:** 32px height, 12px text, 8px padding (compact UI)
- **Compact:** 28px height, 12px text, 6px padding (tables, dense layouts)

**States:**
- **Default:** Styled as above
- **Hover:** Slight background color shift, shadow lift
- **Active:** Darker shade, shadow
- **Disabled:** 50% opacity, cursor not-allowed
- **Loading:** Spinner icon, disabled state

**Example HTML:**
```jsx
<button className="bg-blue-600 hover:bg-blue-700 text-white px-4 py-2 rounded-lg font-medium transition-colors">
  Primary Action
</button>
```

### Input Fields

**Design:**
- Border: 1px solid `#E5E7EB`
- Border-radius: 8px
- Padding: 10px 12px (medium)
- Font: 14px, Inter
- Background: `#FFFFFF` (light), `#1E293B` (dark)
- Focus: Blue border 2px, shadow
- Error: Red border, red error text below
- Placeholder: `#94A3B8`

**Variants:**
- Text input
- Email input
- Password input (toggle visibility)
- Number input
- URL input
- Search input (with clear button)

### Selects & Dropdowns

**Design:**
- Chevron icon on right
- Smooth dropdown animation
- Hover state on options
- Keyboard navigation support
- Multi-select variant available
- Search-in-dropdown for long lists

**Example States:**
- Default closed
- Open (dropdown visible)
- Selected (highlight)
- Disabled

### Search Field

**Features:**
- Magnifying glass icon on left
- Clear (×) button when text entered
- Debounced search (300ms)
- Placeholder: "Search..."
- Optional filters dropdown
- Keyboard shortcuts: Cmd/Ctrl+K for focus
- Recent searches (optional)

### Badges & Status Indicators

**Styles:**
- **Filled:** Colored background + text
- **Outline:** Colored border + text
- **Dot:** Colored circle + text
- **Custom sizes:** Large (desktop), medium (default), small (compact)

**Status Examples:**
- `Delivered` → Green filled
- `Bounced` → Red filled
- `Deferred` → Amber filled
- `Queued` → Blue filled
- `Quarantined` → Orange filled
- `Pending` → Gray outline

### Tables

**Structure:**
```
┌─────────────────────────────────────────────┐
│ ☐  Column 1  │  Column 2  │  Column 3     │
├─────────────────────────────────────────────┤
│ ☐  Data      │  Data      │  Data • ⋮    │
│ ☐  Data      │  Data      │  Data • ⋮    │
│ ☐  Data      │  Data      │  Data • ⋮    │
└─────────────────────────────────────────────┘
```

**Features:**
- Sorting on headers (click to sort)
- Multi-select with checkboxes
- Hover row highlight
- Pagination controls at bottom
- Rows per page selector
- Striped alternating rows (optional)
- Expandable rows for details
- Bulk action toolbar appears when rows selected
- "No results" empty state

**Row Height:** 44px (optimal for scanning)

### Pagination

**Design:**
- Previous / Next buttons
- Page numbers (show 5-7 pages)
- Current page highlighted
- "X-Y of Z results" text
- "Rows per page" dropdown (10, 25, 50, 100)
- Responsive: stacked on mobile

### Tabs

**Design:**
- Tabs displayed horizontally
- Active tab: Blue bottom border (3px)
- Inactive tab: Gray text
- Hover: Slight background highlight
- Content pane switches smoothly
- Keyboard: Arrow keys to navigate, Enter to activate

**Example:**
```
[Overview]  [DNS]  [Settings]  [Events]
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
Content for selected tab
```

### Modals & Dialogs

**Design:**
- Dark overlay (30% opacity)
- Centered dialog box, max-width 600px
- Close button (×) in top-right
- Title (bold, 20px)
- Content area with padding
- Action buttons at bottom (right-aligned)
- Destructive action on left, primary on right
- Escape key closes (if not destructive)
- Focus trapped inside modal

**Example:**
```
┌────────────────────────────┐
│ Title                   × │
├────────────────────────────┤
│ Content area               │
│                            │
├────────────────────────────┤
│ [Cancel]  [Destructive]  [Primary] │
└────────────────────────────┘
```

### Drawers & Side Panels

**Design:**
- Slide in from right (96% of time)
- Width: 400px (desktop), 100% (mobile)
- Dark overlay
- Close button in header
- Scrollable content area
- Footer with actions (sticky)
- Smooth slide animation

### Notifications & Toasts

**Placement:** Top-right corner, stack upward

**Variants:**
- **Success:** Green icon + green border, "Action completed"
- **Error:** Red icon + red border, "Error message"
- **Warning:** Amber icon + amber border, "Warning message"
- **Info:** Cyan icon + cyan border, "Information"

**Features:**
- Auto-dismiss in 4-6 seconds
- Close button (×)
- Icon + message text
- Optional action link
- Max 3 toasts visible

**CSS Example:**
```css
@keyframes slideIn {
  from { transform: translateX(400px); opacity: 0; }
  to { transform: translateX(0); opacity: 1; }
}
.toast { animation: slideIn 0.3s ease-out; }
```

### Empty States

**Design:**
- Large icon (64px) in gray
- Heading: "No data found" or similar
- Description: Explain why empty + what to do
- Primary CTA: Create/add/import action
- Optional illustration

**Example:**
```
        📧
   No messages yet
   Send your first email to get started.
   [Compose New Email]
```

### Loading States

**Variants:**
- **Skeleton:** Gray placeholder boxes matching content shape
- **Spinner:** Centered rotating icon
- **Progress bar:** Linear progress indicator
- **Skeleton table:** 3-5 placeholder rows with shimmer effect

**Behavior:**
- Show immediately (< 100ms = no skeleton)
- Skeleton fills space while loading
- Spinner for full-page loads
- Progress bar for long operations (uploads, exports)

### Confirmation Dialogs

**Pattern:**
- Clear headline: "Are you sure?"
- Description of what will happen
- Warning if destructive (red text)
- Secondary CTA: Cancel
- Primary CTA: Confirm (red if destructive)
- Enter key = Cancel, not Confirm (safety)

**Example:**
```
┌─────────────────────────────────┐
│ Delete API Key?                 │
├─────────────────────────────────┤
│ This action cannot be undone.   │
│ API Key: sk_live_abc123...      │
│ This will break any integrations│
│ using this key.                 │
├─────────────────────────────────┤
│ [Cancel]  [Delete Key]          │
└─────────────────────────────────┘
```

## 3.2 Specialized Components

### Email List Row

**Columns:**
- Checkbox (select)
- Sender avatar (32px)
- Sender name + email (truncate if long)
- Subject (bold if unread, lighter if read)
- Snippet preview (secondary text, truncate)
- Date / time (right-aligned, gray)
- Status badge (Delivered, Bounced, etc.)
- Actions menu (⋮)

**Behavior:**
- Unread: Slightly bolder background
- Hover: Highlight row
- Click: Open email detail
- Starred: Filled star icon
- Attachment: Paperclip icon
- Reply/Forward: Status icon

**Height:** 56px (optimal for scanning email lists)

### Recipient Selector / Picker

**Features:**
- Search input at top
- Department/Team selector dropdown
- List of users/contacts below
- Checkbox to multi-select
- Selected recipients shown as chips/tags
- Ability to remove recipients by clicking ×
- "Select all" option for team/department
- Suggested recipients (recent, frequent)

**Example:**
```
[Search recipients...           ▼]
Department: [Engineering       ▼]

☐ John Smith (john@company.com)
☐ Jane Doe (jane@company.com)
☐ Mike Wilson (mike@company.com)

Selected: [John Smith ×] [Jane Doe ×]
```

### Status Badge

**Variations:**
- `Delivered` → Green checkmark + green background
- `Bounced` → Red X + red background
- `Deferred` → Yellow exclamation + yellow background
- `Queued` → Gray clock + gray background
- `Quarantined` → Orange shield + orange background
- `Phishing` → Red shield + red background
- `Malware` → Red virus + red background
- `Pending Verification` → Gray circle + gray background
- `Verified` → Green checkmark + green background
- `Failed` → Red error circle + red background

**Styling:**
- Icon (16px) + Text (12px)
- Padding: 4px 8px
- Border-radius: 4px
- Font weight: 500

### Domain Health Score

**Visual:**
- Large number (e.g., "87/100")
- Color-coded circle/bar:
  - 80-100: Green
  - 60-79: Amber
  - Below 60: Red
- Below score: Trend indicator (↑ +5 or ↓ -3)
- Factors breakdown (expandable):
  - SPF: ✓
  - DKIM: ✓
  - DMARC: ✓
  - Reputation: ✓

**Size:** 120px diameter on dashboard

### Risk Score Display

**Large Display (Security Detail):**
```
        Risk Score
           91/100
        
        🔴 CRITICAL

- Domain spoofing detected
- DMARC policy failure
- Phishing indicators found
```

**Compact Display (In Table):**
```
Risk: 91  [See Details]
```

Color: Red (>75), Amber (40-75), Green (<40)

### API Key / Secret Reveal

**Pattern:**
- Key shown as: `sk_live_abc123...xyz789`
- Masked by default (dots or obscured text)
- [Show] button to reveal
- [Copy] button (shows checkmark after click)
- [Regenerate] button (requires confirmation)
- Created date + last used date below

**Example:**
```
API Key
sk_live_•••••••••••••••••••••••••

[Show]  [Copy]  [Regenerate]

Created: Aug 15, 2026
Last used: Aug 20, 2026
```

### DNS Record Display

**Design:**
```
Record Type: TXT
Name/Host: _dmarc.example.com
Value: v=DMARC1; p=reject; rua=mailto:...
Status: ✓ Verified

[Copy]  [Copy as Zone File]
```

**Row Format (in table):**
| Type | Host | Value | Status | Actions |
|------|------|-------|--------|---------|
| TXT | _dmarc | v=DMARC1... | ✓ Verified | ⋮ |
| SPF | — | v=spf1 ... | ✓ Verified | ⋮ |

**Value truncation:** Show first 60 chars + "..." on hover show full

### Permission Selector

**Design:**
- List of permissions as checkboxes
- Grouped by category:
  - **Mail:** Read, Send, Delete
  - **Admin:** Manage Users, Manage API Keys
  - **Security:** View Threats, Manage Quarantine
  - **Analytics:** View Reports
- [Select All] / [Clear All] options
- Permission descriptions on hover

**Example:**
```
☐ Read Mail
☐ Send Mail
☐ Delete Mail
━━━━━━━━━━━━━
☐ Manage Users
☐ Manage API Keys
☐ View Audit Log
```

### Analytics Chart Components

**Variants:**
- **Line chart:** Delivery trend over time
- **Bar chart:** Volume by domain/provider
- **Pie chart:** Distribution (bounced, delivered, etc.)
- **Area chart:** Stacked delivery status
- **Mini chart:** Small inline chart in cards

**Design:**
- Minimal gridlines (light gray)
- Smooth curves for line charts
- Tooltip on hover (label + values)
- Legend below or beside chart
- Responsive: stack on mobile
- No 3D effects or unnecessary decorations

**Color scheme for charts:**
- Primary metric: Blue
- Success: Green
- Warning: Amber
- Error: Red
- Secondary: Gray

**Legend:** Small circles + text, clickable to toggle series

---

# 4. Application Shell

## 4.1 Overall Layout

```
┌────────────────────────────────────────────────────┐
│  Organization │ Search │ Notif │ Help │ User Menu │
├──────────────┬────────────────────────────────────┤
│              │ Breadcrumb                         │
│   SIDEBAR    ├────────────────────────────────────┤
│              │                                    │
│   260px      │       MAIN CONTENT AREA           │
│   (collapsible)│      (Flexible width)            │
│              │                                    │
│              │                                    │
└──────────────┴────────────────────────────────────┘
```

**Grid:**
- Sidebar: 260px (desktop), 0px (mobile, drawer overlay)
- Main: Flex 1 (responsive)
- Max width: None (full width minus sidebar)
- Gutter: 24px padding in main content area

## 4.2 Sidebar Navigation

**Structure:**
```
┌─────────────────────────┐
│  Logo / Company Name    │
├─────────────────────────┤
│                         │
│  MAIN                   │
│  📊 Dashboard           │
│                         │
│  MAIL                   │
│  📥 Inbox          (3)  │
│  📤 Sent                │
│  📝 Drafts         (1)  │
│  🚫 Spam                │
│  ⚠️  Quarantine     (2)  │
│                         │
│  COMMUNICATION          │
│  💬 Messages       (5)  │
│  👥 Contacts            │
│  🏢 Departments         │
│                         │
│  DEVELOPER              │
│  📋 Email Logs          │
│  🌐 Domains             │
│  🔑 API Keys            │
│  📧 SMTP                │
│  🪝 Webhooks            │
│  📄 Templates           │
│                         │
│  ANALYTICS              │
│  📈 Deliverability      │
│  📊 Analytics           │
│  ⭐ Reputation          │
│                         │
│  SECURITY               │
│  🔒 Security Center     │
│  🛡️  Threats       (1)  │
│  ⚠️  Abuse              │
│                         │
│  MANAGE                 │
│  👨‍💼 Team                │
│  🔐 Roles & Permissions │
│  💳 Billing             │
│  🚀 Deployments         │
│  📋 Audit Log           │
│  ⚙️  Settings            │
│                         │
└─────────────────────────┘
```

**Sidebar Features:**
- **Logo/Company:** Clickable to navigate to dashboard or open organization switcher
- **Sections:** Bold uppercase labels, gray text `#64748B`
- **Menu items:** 
  - Icon (20px) + Label
  - Padding: 10px 12px
  - Height: 36px
  - Hover: Light blue background `#EFF6FF`
  - Active: Blue background + blue text
  - Border-left: 3px blue for active item
  - Font: 14px, medium weight
- **Badges:** 
  - Background: Red (for alerts), Gray (for counts)
  - Positioned: Right side of label
  - Font: 11px bold, white text
  - Border-radius: 4px
  - Padding: 2px 6px
- **Collapse button:** Chevron icon, collapses to icon-only sidebar (60px width)
- **Scroll:** Internal scroll if section > viewport height

**Responsive Behavior:**
- Desktop (>768px): Always visible
- Tablet (768px): Collapsible, icon view
- Mobile (<480px): Drawer overlay, full width

## 4.3 Top Navigation Bar

**Structure:**
```
┌────────────────────────────────────────────────────────────────┐
│ Company Switcher │ / Page │ Search [       ] │ Icons │ User   │
└────────────────────────────────────────────────────────────────┘
```

**Elements (left to right):**

1. **Organization/Company Switcher**
   - Dropdown showing current org
   - Click to switch between orgs user has access to
   - Icon: Building + Chevron down
   - Width: 200px max

2. **Breadcrumb Navigation**
   - Example: `Dashboard / Inbox / Email ID: abc123`
   - "/" separator
   - Clickable intermediate levels
   - Last item: Bold (current page)
   - Color: Gray text, blue on hover

3. **Global Search** (Centered)
   - Placeholder: "Search emails, domains, logs..."
   - Magnifying glass icon on left
   - Command+K shortcut shown on right (`⌘K`)
   - Width: 320px
   - On focus: Blue border, dropdown with recent searches
   - Results: Organized by category (Emails, Domains, Users, Logs)
   - Each result: Icon + Label + Context
   - Keyboard navigation: Arrow keys, Enter to select

4. **Right-Side Icons** (24px, aligned right)
   - **Notifications:** Bell icon
     - Blue dot if unread
     - Click to open notification dropdown
     - Show: 5 most recent, "View all" link
     - Dismiss option for each
   - **Help/Docs:** Question mark icon
     - Dropdown with: Docs, Support, Keyboard Shortcuts, Bug Report
   - **Dark mode toggle:** Sun/Moon icon
     - Switches between Light/Dark/System
   - **User Menu:** Avatar (32px)
     - Name below in hover
     - Dropdown: Profile, Settings, Logout

**Styling:**
- Height: 60px
- Background: White `#FFFFFF` (light), Dark slate `#1E293B` (dark)
- Border-bottom: 1px solid `#E5E7EB` (light), `#334155` (dark)
- Padding: 0 24px
- Icons: Gray text, blue on hover
- Flex layout with justify-content: space-between

## 4.4 Main Content Area

**Standard Layout:**
```
┌──────────────────────────────────────┐
│  Page Title            [Actions]    │
├──────────────────────────────────────┤
│                                      │
│  Content (Cards, Tables, etc.)      │
│                                      │
└──────────────────────────────────────┘
```

**Header Section:**
- Page title (32px, bold)
- Subtitle/description (14px, gray, optional)
- Primary action buttons on right (e.g., "Create New")
- Sticky if scrolling

**Content Area:**
- Padding: 24px
- Background: White `#FFFFFF`
- Cards: Border 1px `#E5E7EB`, border-radius 8px, padding 20px
- Spacing between elements: 24px
- Gutters: 24px from edges

---

# 5. Core Modules & Screens

## 5.1 Dashboard

**Purpose:** High-level overview of email infrastructure health and activity

**Layout:**
```
┌─────────────────────────────────────────────────────┐
│ Dashboard              [Date Range Picker]          │
├─────────────────────────────────────────────────────┤
│                                                     │
│  ┌──────────────┐  ┌──────────────┐  ┌─────────┐  │
│  │ Emails Sent  │  │ Delivery Rate│  │ Bounce  │  │
│  │   2,384,102  │  │     98.2%    │  │  1.2%   │  │
│  └──────────────┘  └──────────────┘  └─────────┘  │
│                                                     │
│  ┌──────────────┐  ┌──────────────┐  ┌─────────┐  │
│  │ Complaints   │  │ Domain Health│  │Reputation│  │
│  │     0.3%     │  │   87/100     │  │ Good    │  │
│  └──────────────┘  └──────────────┘  └─────────┘  │
│                                                     │
│  ┌─────────────────────────────────────────────┐  │
│  │ Email Delivery Trend (Last 7 Days)          │  │
│  │                                             │  │
│  │  Delivered ▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓            │  │
│  │  Bounced   ▓▓▓▓▓▓▓▓                         │  │
│  │  Deferred  ▓▓▓                              │  │
│  │  Rejected  ▓▓                               │  │
│  │                                             │  │
│  │  Mon  Tue  Wed  Thu  Fri  Sat  Sun         │  │
│  └─────────────────────────────────────────────┘  │
│                                                     │
│  ┌──────────────────────┐  ┌─────────────────┐  │
│  │ Top Domains          │  │ Security Status │  │
│  │ • mail.company.com   │  │ Quarantined: 2 │  │
│  │ • api.company.com    │  │ Threats: 0     │  │
│  │ • newsletter.co      │  │ Warnings: 1    │  │
│  └──────────────────────┘  └─────────────────┘  │
│                                                     │
│  ┌─────────────────────────────────────────────┐  │
│  │ Recent Events                               │  │
│  │ • DMARC policy updated (20 min ago)         │  │
│  │ • New SMTP credential created (1 hour ago)  │  │
│  │ • Domain verified (2 hours ago)             │  │
│  └─────────────────────────────────────────────┘  │
│                                                     │
└─────────────────────────────────────────────────────┘
```

**Key Cards:**
- **KPI Cards:** 4-column grid, show metric + icon + trend
  - Emails Sent (volume)
  - Delivery Rate (%)
  - Bounce Rate (%)
  - Complaint Rate (%)
  - Deferred %
  - Reject %
- **Domain Health:** Single card, circular progress indicator
- **Reputation:** Status badge (Good/Fair/Poor) + color
- **Delivery Trend Chart:** 7-day or 30-day line chart
- **Top Domains:** List card with domain names + status
- **Security Status:** Quick overview (Quarantined count, Threats, Warnings)
- **Recent Events:** Timeline of recent actions + timestamps

**Date Range Picker:**
- Options: Last 24h, Last 7 days, Last 30 days, Custom
- Positioned top-right
- Refreshes all charts

**Responsive:**
- Desktop: 3-column grid for KPIs
- Tablet: 2-column grid
- Mobile: 1-column stack

---

## 5.2 Corporate Mail (Inbox/Email Client)

### 5.2.1 Mail List View

**Three-column layout:**
```
┌────────────┬─────────────────┬───────────────────────────┐
│  SIDEBAR   │  EMAIL LIST     │  EMAIL DETAIL             │
│            │                 │                           │
│ Inbox (3)  │ ☐ ○ From │Subj │ From: John Smith         │
│ Sent       │ ☐ ○ From │Subj │ Subject: Q2 Meeting      │
│ Drafts (1) │ ☐ ○ From │Subj │                          │
│ Spam       │ ☐ ○ From │Subj │ Dear Team,               │
│ Quarantine │ ☐ ○ From │Subj │ Let's discuss the Q2...  │
│ Trash      │ ☐ ○ From │Subj │                          │
│            │ ☐ ○ From │Subj │ [Reply] [Reply All]      │
│ [+] Folder │ ☐ ○ From │Subj │ [Forward] [Archive]      │
│            │                 │                           │
│ Filters:   │ [< Prev] 1 of 5 │                           │
│ ☐ Unread   │ [Next >]        │                           │
│ ☐ Starred  │                 │                           │
│            │                 │                           │
└────────────┴─────────────────┴───────────────────────────┘
```

**Email List Structure:**
- **Checkbox:** Multi-select rows
- **Star:** Clickable to add/remove from starred
- **Sender avatar:** 32px circle with initials
- **Sender name:** Bold if unread, regular if read
- **Subject:** Truncated, bold if unread
- **Email snippet:** Secondary text preview, 1 line, truncated
- **Date/Time:** Right aligned, gray text
  - Same day: "2:45 PM"
  - This week: "Tuesday"
  - Older: "Aug 15"
- **Status indicators:** Icons right side
  - Attachment: 📎
  - Multiple recipients: 👥
  - Flagged: 🚩
  - Starred: ⭐

**Row Styling:**
- Height: 56px
- Padding: 8px 12px
- Border-bottom: 1px solid `#E5E7EB`
- Hover: Light blue background `#EFF6FF`
- Unread: Slightly bold/darker text
- Active/selected: Blue left border + light blue background

**Sidebar Folder Navigation:**
- Folders list:
  - Inbox (with unread badge if > 0)
  - Sent
  - Drafts (with draft count)
  - Spam (with spam count)
  - Quarantine
  - Trash
  - Custom folders (collapsible "All Folders" section)
- Folder item style:
  - Icon (16px) + Name
  - Badge on right if count > 0
  - Hover: Gray background
  - Active: Blue background + text
- [+] New Folder button at bottom
- Collapse/expand custom folders

**Filters Sidebar:**
- Checkbox options:
  - ☐ Unread (shows count)
  - ☐ Starred
  - ☐ Has attachments
  - ☐ From certain person (expandable)
  - ☐ Date range (custom picker)
- Active filters: Show count of matching emails

### 5.2.2 Email Detail View

**Header:**
```
┌────────────────────────────────────────────────────┐
│ From: John Smith <john@company.com>               │
│ To: team@company.com                              │
│ Cc: manager@company.com                           │
│ Bcc: (hidden)                                     │
│ Date: Tuesday, Aug 15, 2026 at 2:45 PM            │
├────────────────────────────────────────────────────┤
│ Subject: Q2 Planning Meeting                       │
├────────────────────────────────────────────────────┤
│ [Archive] [Spam] [Trash] [⋯ More]                │
└────────────────────────────────────────────────────┘
```

**Content:**
- Full email body (HTML rendered safely)
- Code/structured content rendered nicely
- Blockquotes indented with left border
- Links underlined, blue
- Images embedded inline
- Attachments list at bottom

**Attachments Section:**
```
Attachments (3)
┌──────────────────┬─────────────────────────┐
│ 📄 Document.pdf │ 2.3 MB   [Download]    │
│ 📊 Report.xlsx  │ 1.1 MB   [Download]    │
│ 🖼️  Image.png   │ 456 KB   [Download]    │
└──────────────────┴─────────────────────────┘
```

**Actions Bar (Bottom/Sticky):**
```
[⬅️ Back] [⭐ Star] [Archive] [Move to...] [Delete]
[👤 Reply] [👥 Reply All] [↪️ Forward]
```

### 5.2.3 Compose Email Dialog

**Full-screen or large modal:**
```
┌─────────────────────────────────────────────────────────┐
│ New Email                                            × │
├─────────────────────────────────────────────────────────┤
│                                                         │
│ To: [Search/Select Recipients...] [v]                 │
│ Cc: [Click to add] [v]                                │
│ Bcc: [Click to add] [v]                               │
│                                                         │
│ Subject: [Enter subject...]                           │
│                                                         │
│ ┌─────────────────────────────────────────────────┐   │
│ │ [B] [I] [U] [Link] [Code] [Attachment] [...] │   │
│ ├─────────────────────────────────────────────────┤   │
│ │                                                 │   │
│ │  [Compose message here...]                     │   │
│ │                                                 │   │
│ │                                                 │   │
│ └─────────────────────────────────────────────────┘   │
│                                                         │
│ [Preview] [Save Draft] [Schedule Send] [Send]         │
│                                                         │
└─────────────────────────────────────────────────────────┘
```

**Features:**
- **To/Cc/Bcc fields:** Recipient picker dropdown
  - Search recipients
  - Recent recipients shown first
  - Departments/Teams can be selected (expands to all members)
  - Individual or bulk selection
  - Remove recipient with ×
- **Subject field:** Simple input, placeholder
- **Rich text editor:**
  - Toolbar: Bold, Italic, Underline, Link, Code, Lists, Quote
  - Formatting buttons
  - Paste from Word without styles
  - Tab indentation support
  - Dark mode compatible
- **Attachment area:**
  - Drag & drop zone
  - [+ Add file] button
  - Shows file list with size + remove
  - Max file size: Validated + error message
- **Bottom actions:**
  - Preview: Opens preview modal (read-only)
  - Save Draft: Saves to Drafts folder
  - Schedule Send: Opens date/time picker
  - Send: Sends immediately
- **Auto-save:** Saves draft every 30s (indicator in header)
- **Keyboard shortcuts:**
  - Cmd/Ctrl+Enter: Send
  - Escape: Close (confirm if content present)

### 5.2.4 Mail Flows & Secondary Screens

**Reply/Reply All:**
- Quote original email (indented, gray background)
- Cursor at top
- Compose area similar to new email
- Attachments from original can be included
- Subject auto-filled: "Re: Original Subject"

**Forward:**
- Quote original with "---------- Forwarded message ---------" header
- Shows original From, To, Date
- New compose area at top
- Subject: "Fwd: Original Subject"
- Attachments: Option to include or exclude

**Thread View:**
- Show conversation thread collapsed by default
- Expand individual messages
- Newest at bottom
- Visual threading with left border lines
- Collapse older messages to show just header
- "N more messages" expandable section

**Search:**
- Global search from top bar
- Results show email list
- Highlight matching terms
- Filters: Date, From, To, Subject, Body, Has attachment
- Saved searches option

---

## 5.3 Users, Teams & Departments

### 5.3.1 Team Management

**Team List View:**
```
┌─────────────────────────────────────────────────────┐
│ Team                              [+ New Team]      │
├─────────────────────────────────────────────────────┤
│ ☐ Team Name      │ Members │ Manager  │ Created    │
├─────────────────────────────────────────────────────┤
│ ☐ Engineering    │ 12      │ John... │ Jun 2026   │
│ ☐ Marketing      │ 8       │ Jane... │ Jun 2026   │
│ ☐ Sales          │ 15      │ Bob...  │ Jun 2026   │
│ ☐ Finance        │ 5       │ Alice.. │ Jun 2026   │
│ ☐ HR             │ 3       │ Eve...  │ Jun 2026   │
│                                                     │
│ [< Prev] 1 of 1 [Next >]                          │
└─────────────────────────────────────────────────────┘
```

**Actions:**
- Click row to view team details
- Bulk actions: Delete, Change Manager, Export
- Search bar: Find by name

**Team Detail View:**
```
┌──────────────────────────────────────────────────┐
│ Engineering              [Edit] [Delete] [⋯]    │
├──────────────────────────────────────────────────┤
│                                                  │
│ Overview        Members        Permissions       │
│ ─────────────────────────────────────────────   │
│                                                  │
│ Team Name: Engineering                          │
│ Manager: John Smith                             │
│ Created: Jun 15, 2026                           │
│ Members: 12                                      │
│ Email Quota: 10,000/month                       │
│                                                  │
│ ┌──────────────────────────────────────────────┐ │
│ │ Team Members                   [+ Add]      │ │
│ ├──────────────────────────────────────────────┤ │
│ │ ☐ Name │ Email │ Role │ Status │ Actions   │ │
│ ├──────────────────────────────────────────────┤ │
│ │ ☐ John │ j@c.. │ Lead │ Active │ ⋯        │ │
│ │ ☐ Jane │ j@c.. │ Mem. │ Active │ ⋯        │ │
│ └──────────────────────────────────────────────┘ │
│                                                  │
└──────────────────────────────────────────────────┘
```

### 5.3.2 Departments

**Departments List:**
```
┌──────────────────────────────────────────┐
│ Departments            [+ New Dept]      │
├──────────────────────────────────────────┤
│ Management                               │
│ ├ Finance                                │
│ ├ HR                                      │
│ ├ Legal                                   │
│ Engineering                              │
│ ├ Backend                                │
│ ├ Frontend                               │
│ ├ DevOps                                 │
│ Marketing                                │
│ ├ Content                                │
│ ├ Digital                                │
│ Sales                                    │
│ └ Support                                │
│                                          │
└──────────────────────────────────────────┘
```

**Dept Detail:**
- Similar to Team with members, roles, email settings
- Option to create sub-departments
- Manager assignment

### 5.3.3 Users List

**Users Table:**
```
┌────────────────────────────────────────────────────┐
│ Users                            [+ Invite User]  │
├────────────────────────────────────────────────────┤
│ ☐ Name │ Email │ Dept │ Role │ Status │ Actions │
├────────────────────────────────────────────────────┤
│ ☐ John │ j@c..│Eng  │Admin │ Active │  ⋯     │
│ ☐ Jane │ j@c..│Mkt  │User │ Active │  ⋯     │
│ ☐ Bob  │ b@c..│Sal  │User │ Active │  ⋯     │
│ ☐ Alice│ a@c..│Fin  │User │Inactive│  ⋯     │
│                                                    │
│ [< Prev] 1 of 3 [Next >]                         │
└────────────────────────────────────────────────────┘
```

**User Row Actions:**
- Edit (open side panel with form)
- Reset password
- Disable/Enable
- Remove from org
- View permissions

**User Detail Panel:**
```
┌──────────────────────────┐
│ Edit User                │
├──────────────────────────┤
│ Name: [John Smith    ]   │
│ Email: [john@comp..] (R) │
│ Department: [Engr...▼]  │
│ Role: [Admin      ▼]    │
│ Status: ●Active ○Inactive│
│                          │
│ [Cancel] [Save]         │
└──────────────────────────┘
```

---

## 5.4 Internal Messages

**Messages Interface:**
```
┌────────────┬─────────────────┬───────────────────┐
│ CONTACTS   │ CONVERSATION    │ CHAT              │
│            │ LIST            │                   │
│ Online:    │ ☐ John Sm... (3)│ [Compose Message]│
│ • John S.  │ ☐ Jane Doe      │                   │
│ • Jane D.  │ ☐ Team Dev.  (5)│ John Smith       │
│ • Bob W.   │ ☐ Mike Wilson   │ Hey team, did    │
│            │ ☐ Marketing (7) │ you see the new  │
│ Offline:   │ ☐ Sales Group   │ email policy?    │
│ • Mike W.  │                 │                   │
│ • Eve L.   │ [Search...]     │ Jane Doe         │
│            │                 │ Yes! I updated   │
│ Groups:    │                 │ our templates    │
│ [Teams▼]   │                 │                   │
│ [Depts▼]   │                 │ John Smith       │
│            │                 │ Perfect, thanks! │
│            │                 │                   │
│            │                 │ [Type message...]│
│            │                 │ [  ] [Emoji] [Atta│
└────────────┴─────────────────┴───────────────────┘
```

**Features:**
- Online/offline indicators (green/gray circle)
- Direct messages + group conversations
- Unread badge on conversations
- Search through messages
- Typing indicators
- Message reactions
- Edit/delete messages
- Pinned messages
- Attachments support
- @mentions + notifications

---

## 5.5 Reminders & Follow-ups

**Reminders View:**
```
┌────────────────────────────────────────────┐
│ Reminders                  [+ New Reminder]│
├────────────────────────────────────────────┤
│ Today                                      │
│ ○ Review Q2 proposal (John Smith) - 2pm   │
│ ○ Call with client ABC Corp - 3:30pm     │
│                                            │
│ Tomorrow                                   │
│ ○ Send monthly report - 9am                │
│ ○ Team standup - 10am                      │
│                                            │
│ This Week                                  │
│ ○ Finish security audit - Wed              │
│ ○ Review team feedback - Fri               │
│                                            │
│ Later                                      │
│ ✓ Fix email template (Done - Aug 18)       │
│ ✓ Deploy API (Done - Aug 19)               │
│                                            │
└────────────────────────────────────────────┘
```

**Create Reminder:**
```
┌──────────────────────────────────┐
│ New Reminder                     │
├──────────────────────────────────┤
│ What? [Attend board meeting]     │
│                                  │
│ When? [Date: Aug 25] [Time: 10am]│
│                                  │
│ Assign to: [Me] ☐ Team member    │
│ If team member:                  │
│ [Search/Select...         ▼]     │
│                                  │
│ Notify: ☑ Email ☑ In-app        │
│                                  │
│ [Cancel] [Save]                 │
└──────────────────────────────────┘
```

**Email Reminder:**
- On email thread, "Remind me about this" button
- Sets a follow-up for that email
- Shows in Reminders view with email preview

---

## 5.6 Email Logs & Infrastructure

### 5.6.1 Email Logs View

**Log Table:**
```
┌──────────────────────────────────────────────────────────────┐
│ Email Logs                    [Date Range] [Filters] [Export]│
├──────────────────────────────────────────────────────────────┤
│ ☐ Status │ Recipient │ Sender │ Subject │ Domain │ Time     │
├──────────────────────────────────────────────────────────────┤
│ ☐ ✓Del. │ j@abc... │ sys@.. │ Welcome │ main.. │ 14:32   │
│ ☐ ✗Boun  │ x@xyz... │ news.. │ Week... │ news.. │ 14:31   │
│ ☐ ⏱Defer│ m@test.. │ alert..│ Alert  │ sys..  │ 14:30   │
│ ☐ ✓Del. │ y@yyy... │ api@.. │ Notif. │ api..  │ 14:29   │
│ ☐ ⚠Quar  │ z@test..│ promo..│ Promo  │ promo..│ 14:28   │
│                                                              │
│ [< Prev] Showing 1-5 of 2,384,102 [Next >]                │
│ Per page: [10 ▼]                                           │
│                                                              │
└──────────────────────────────────────────────────────────────┘
```

**Columns:**
- Status (with icon + label):
  - ✓ Delivered (green)
  - ✗ Bounced (red)
  - ⏱ Deferred (amber)
  - ✓ Queued (blue)
  - ⚠ Quarantined (orange)
  - ✗ Rejected (red)
- Recipient email (truncate, full email in tooltip)
- Sender email
- Subject (truncate)
- Domain
- Date/Time (clickable to detail)

**Filters (Sidebar or Dropdown):**
- Date range: Last 24h, 7d, 30d, Custom
- Status: (multi-select checkboxes)
- Sender: (text search or select)
- Recipient: (text search)
- Domain: (multi-select dropdown)
- Subject: (text search)
- Message ID: (exact match)
- Export: CSV, JSON

**Row Actions:**
- Click to open detail
- Hover: Show "View details" link
- Bulk: Select multiple, bulk actions toolbar appears
  - Mark as spam
  - Block sender
  - Block domain
  - Delete logs

### 5.6.2 Email Detail / Lifecycle

**Email Lifecycle View:**
```
┌────────────────────────────────────────────────────┐
│ Message Details                                  × │
├────────────────────────────────────────────────────┤
│ Message ID: msg_abc123def456                       │
│ Status: ✓ Delivered                              │
│                                                    │
│ From: alerts@company.com                          │
│ To: john@client.com                               │
│ Subject: Q2 Report Ready                          │
│                                                    │
│ ────────────────────────────────────────────────── │
│ Delivery Timeline:                                 │
│                                                    │
│ ✓ Accepted       → 14:27:32                        │
│   Received by mail server                         │
│   IP: 192.168.1.100                              │
│                                                    │
│ ✓ Queued         → 14:27:35 (+3s)                  │
│   Waiting in delivery queue                       │
│                                                    │
│ ✓ Scanned        → 14:27:45 (+10s)                 │
│   Virus/spam scan completed                       │
│   Score: 0.1 (clean)                             │
│                                                    │
│ ✓ Routed         → 14:27:50 (+5s)                  │
│   Determining delivery route                      │
│   Next hop: gmail-smtp-in.l.google.com           │
│                                                    │
│ ✓ SMTP Attempt   → 14:28:00 (+10s)                 │
│   Attempting delivery to recipient                │
│   Response: 250 OK id=abc123                     │
│                                                    │
│ ✓ Delivered      → 14:28:02 (+2s)                  │
│   Message successfully delivered                  │
│   Time to delivery: 30 seconds                    │
│                                                    │
│ ────────────────────────────────────────────────── │
│ Additional Details:                               │
│ Domain: company.com                              │
│ Stream: transactional                            │
│ SPF: ✓ Pass                                       │
│ DKIM: ✓ Pass                                      │
│ DMARC: ✓ Pass                                     │
│ TLS: ✓ Yes                                        │
│ Reputation: Clean                                │
│                                                    │
│ [Close]                                          │
└────────────────────────────────────────────────────┘
```

**Bounce/Error Details:**
If bounced, show:
- Bounce type: Hard / Soft
- Bounce code: 550 - Permanent failure
- Bounce message: "User unknown"
- Recommended action: Remove from list / Retry
- Can retry delivery if soft bounce

---

## 5.7 Domains & DNS Management

### 5.7.1 Domains List

**Domains Table:**
```
┌──────────────────────────────────────────────────────────┐
│ Domains                              [+ Add Domain]      │
├──────────────────────────────────────────────────────────┤
│ Domain │ Verification │ Health │ Sending │ Inbound │ Rep │
├──────────────────────────────────────────────────────────┤
│ comp.. │ ✓ Verified   │ 87/100 │ ✓ Ok   │ ✓ Ok   │Good│
│ news.. │ ⏱ Pending    │ —      │ ✓ Ok   │ ✗ Error│ —  │
│ api... │ ✓ Verified   │ 92/100 │ ✓ Ok   │ ✓ Ok   │Good│
│ test..│ ✗ Failed     │ 45/100 │ ✗ Error│ ✗ Error│Poor│
│                                                          │
│ [< Prev] 1 of 2 [Next >]                               │
│                                                          │
└──────────────────────────────────────────────────────────┘
```

**Status Indicators:**
- Verification: ✓ Verified / ⏱ Pending / ✗ Failed
- Health Score: 0-100 number, color-coded
- Sending/Inbound: ✓ Ok / ⚠ Warning / ✗ Error (with tooltip)
- Reputation: Good / Fair / Poor (color-coded)

### 5.7.2 Domain Detail View

**Tabs:**
```
┌─────────────────────────────────────────────────────────┐
│ company.com                              [Edit] [...] │
├─────────────────────────────────────────────────────────┤
│ [Overview] [DNS] [Sending] [Inbound] [Reputation] [Events] [Settings]
├─────────────────────────────────────────────────────────┤
│                                                         │
```

#### Overview Tab:
```
Domain: company.com
Status: ✓ Verified
Added: Jun 15, 2026

Health Score: 87/100
├ SPF: ✓ Configured
├ DKIM: ✓ Configured
├ DMARC: ✓ Configured
└ TLS: ✓ Configured

Reputation: Good
├ Gmail: 98% Delivery
├ Outlook: 96% Delivery
├ Yahoo: 94% Delivery
└ Others: 95% Avg

Recent Activity:
• DNS records updated (2 days ago)
• DMARC policy changed (5 days ago)
```

#### DNS Tab:
```
Quick Setup
Copy all records and paste into your domain provider:

[Copy All Records as Zone File]

Records List:

Type │ Host │ Value │ Status │ Actions
──────────────────────────────────────
TXT  │ @ (root) │ v=spf1 include:sendgrid... │ ✓ │ ⋯
CNAME│ _dmarc   │ validate.example.com       │ ✓ │ ⋯
TXT  │ _dmarc   │ v=DMARC1; p=reject;...     │ ✓ │ ⋯
CNAME│ *._dkim  │ ssl.example.com            │ ✓ │ ⋯

[Add Custom Record]
```

**DNS Record Dialog:**
```
┌──────────────────────────────────────────┐
│ DNS Record Details                     × │
├──────────────────────────────────────────┤
│                                          │
│ Record Type: TXT                         │
│ Host: _dmarc.company.com                │
│ Value:                                   │
│ v=DMARC1; p=reject;                     │
│ rua=mailto:reports@company.com          │
│                                          │
│ Status: ✓ Verified                      │
│ Last Checked: 2 hours ago                │
│ ────────────────────────────────────────│
│ [Copy]  [Copy as Zone File]  [Done]     │
│                                          │
└──────────────────────────────────────────┘
```

#### Sending/Inbound/Reputation Tabs:
- Similar detail views with settings and status

---

## 5.8 API & SMTP Management

### 5.8.1 API Keys

**API Keys List:**
```
┌────────────────────────────────────────────────────────┐
│ API Keys                                  [+ New Key]  │
├────────────────────────────────────────────────────────┤
│ Name │ Project │ Permissions │ Created │ Last Used │ St│
├────────────────────────────────────────────────────────┤
│ Web  │ Main    │ Read, Send  │ Jun... │ 2h ago   │✓ │
│ API  │ Main    │ Full        │ May... │ 1h ago   │✓ │
│ Test │ Demo    │ Read, Delete│ Apr... │ 30d ago  │⚠ │
│                                                        │
│ [< Prev] 1 of 3 [Next >]                             │
│                                                        │
└────────────────────────────────────────────────────────┘
```

**Create API Key Dialog:**
```
┌───────────────────────────────────┐
│ Create New API Key                │
├───────────────────────────────────┤
│ Key Name: [Development API    ]  │
│                                   │
│ Project: [Main              ▼]   │
│                                   │
│ Permissions:                      │
│ ☐ Read Mail                       │
│ ☐ Send Mail                       │
│ ☐ Delete Mail                     │
│ ☐ Manage Webhooks                │
│ ☐ Read Analytics                 │
│ ☐ Manage Settings                │
│                                   │
│ IP Whitelist (optional):          │
│ [192.168.1.1           ]          │
│ [192.168.1.2           ]          │
│ [+ Add IP]                        │
│                                   │
│ [Cancel] [Create]                │
│                                   │
└───────────────────────────────────┘
```

**After Creation (show once):**
```
┌───────────────────────────────────┐
│ ✓ API Key Created                │
├───────────────────────────────────┤
│ Save this key securely.           │
│ You won't see it again.           │
│                                   │
│ sk_live_abc123def456ghi789        │
│ [Copy to Clipboard]               │
│                                   │
│ Name: Development API             │
│ Status: Active                    │
│ Permissions: Read, Send, Delete   │
│ Created: Just now                 │
│                                   │
│ [Done]                            │
│                                   │
└───────────────────────────────────┘
```

**Key Detail View:**
```
┌────────────────────────────────────┐
│ Development API                    │
├────────────────────────────────────┤
│ Key: sk_live_abc123...xyz789       │
│ [Show] [Copy] [Regenerate]         │
│                                    │
│ Status: ✓ Active                   │
│ Project: Main                      │
│ Created: Jun 15, 2026 at 2:30 PM  │
│ Last Used: 2 hours ago             │
│                                    │
│ Permissions: Read, Send, Delete    │
│ IP Whitelist: None                │
│                                    │
│ ⚠️  Regenerating will invalidate   │
│ all integrations using this key.   │
│                                    │
│ [Revoke Key]  [Regenerate]        │
│                                    │
└────────────────────────────────────┘
```

### 5.8.2 SMTP Credentials

**SMTP List:**
```
┌──────────────────────────────────────────────┐
│ SMTP Credentials                [+ New Cred] │
├──────────────────────────────────────────────┤
│ Name │ Username │ Domains │ Created │ Status │
├──────────────────────────────────────────────┤
│ App  │ user_123 │ 2       │ Jun... │ ✓      │
│ Webhook│user_456│ 1       │ May... │ ✓      │
│                                              │
│ [< Prev] 1 of 2 [Next >]                    │
│                                              │
└──────────────────────────────────────────────┘
```

**Create SMTP Credential:**
```
┌──────────────────────────────────────┐
│ New SMTP Credential                  │
├──────────────────────────────────────┤
│ Name: [Application SMTP         ]   │
│                                      │
│ Allowed Domains (select):            │
│ ☐ company.com                       │
│ ☐ newsletter.co                     │
│                                      │
│ IP Allowlist (optional):             │
│ [192.168.1.1                  ]      │
│ [+ Add another IP]                  │
│                                      │
│ Send Limit (emails/hour):           │
│ [1000                         ]      │
│                                      │
│ [Cancel] [Create]                   │
│                                      │
└──────────────────────────────────────┘
```

**After Creation:**
```
┌────────────────────────────────────────┐
│ ✓ SMTP Credential Created             │
├────────────────────────────────────────┤
│ Server: smtp.company.com              │
│ Port: 587 (TLS) or 25, 465            │
│                                        │
│ Username: user_abc123                 │
│ Password: pass_xyz789...              │
│ [Show] [Copy]                         │
│                                        │
│ ⚠️  Save this password securely.       │
│ You won't see it again.               │
│                                        │
│ Usage Example:                        │
│ ```                                  │
│ curl --ssl-reqd                      │
│   --url 'smtp://smtp.company.com'   │
│   --user 'user_abc123:pass_xyz789'  │
│   ...                               │
│ ```                                  │
│                                        │
│ [Done]                               │
│                                        │
└────────────────────────────────────────┘
```

---

## 5.9 Webhooks

**Webhooks List:**
```
┌──────────────────────────────────────────────────────┐
│ Webhooks                                 [+ New Hook]│
├──────────────────────────────────────────────────────┤
│ Endpoint │ Events │ Status │ Last Delivery │ Actions│
├──────────────────────────────────────────────────────┤
│ http... │ All    │ ✓ Ok  │ 1h ago        │ ⋯     │
│ http... │ Del... │ ⚠ Fail│ 3h ago        │ ⋯     │
│ http... │ Error  │ ✓ Ok  │ 30m ago       │ ⋯     │
│                                                      │
│ [< Prev] 1 of 3 [Next >]                           │
│                                                      │
└──────────────────────────────────────────────────────┘
```

**Create Webhook:**
```
┌──────────────────────────────────────┐
│ New Webhook                          │
├──────────────────────────────────────┤
│ Endpoint URL:                        │
│ [https://api.example.com/webhook]   │
│                                      │
│ Events (select):                     │
│ ☐ Email Delivered                   │
│ ☐ Email Bounced                     │
│ ☐ Email Complained                  │
│ ☐ Email Deferred                    │
│ ☐ Email Rejected                    │
│ ☐ Message Opened                    │
│ ☐ Link Clicked                      │
│ ☐ Domain Verified                   │
│ ☐ Security Event                    │
│                                      │
│ HTTP Method: [POST       ▼]         │
│                                      │
│ Custom Headers (optional):           │
│ Key: [          ] Value: [       ]  │
│ [+ Add Header]                       │
│                                      │
│ Authentication:                      │
│ ☑ Sign requests with secret         │
│                                      │
│ [Cancel] [Create]                   │
│                                      │
└──────────────────────────────────────┘
```

**Webhook Detail / Delivery Logs:**
```
┌──────────────────────────────────────────────────┐
│ Webhook: https://api.example.com/webhook        │
├──────────────────────────────────────────────────┤
│ Status: ✓ Active                                │
│ Events: Email Delivered, Bounced                │
│ Last Delivery: 1 hour ago (✓ Success)          │
│                                                  │
│ ────────────────────────────────────────────── │
│ Delivery Attempts:                              │
│                                                  │
│ ✓ Aug 20, 14:32 (200 OK) [View]                │
│ ✓ Aug 20, 13:15 (200 OK) [View]                │
│ ✗ Aug 20, 12:45 (503 Error) [View] [Retry]   │
│ ✗ Aug 20, 12:30 (Timeout) [Retry]             │
│                                                  │
│ [< Prev] [Next >]                              │
│                                                  │
│ [Resend All Failures]  [Edit]  [Delete]        │
│                                                  │
└──────────────────────────────────────────────────┘
```

**Delivery Attempt Detail:**
```
┌──────────────────────────────────────────┐
│ Delivery Attempt                       × │
├──────────────────────────────────────────┤
│ Status: ✓ Delivered                    │
│ HTTP Status: 200 OK                    │
│ Timestamp: Aug 20, 2026 at 14:32:15   │
│ Duration: 245ms                        │
│                                         │
│ REQUEST                                 │
│ POST /webhook HTTP/1.1                 │
│ Host: api.example.com                  │
│ Content-Type: application/json         │
│ X-Signature: sha256=abc123...          │
│                                         │
│ Payload:                                │
│ {                                      │
│   "event": "email.delivered",          │
│   "timestamp": 1692547935,             │
│   "data": { ... }                      │
│ }                                      │
│                                         │
│ RESPONSE                                │
│ HTTP/1.1 200 OK                        │
│ Content-Type: application/json         │
│                                         │
│ {                                      │
│   "status": "ok",                      │
│   "id": "evt_123"                      │
│ }                                      │
│                                         │
│ [Close]                                 │
│                                         │
└──────────────────────────────────────────┘
```

---

## 5.10 Analytics & Deliverability

### 5.10.1 Deliverability Dashboard

**Layout:**
```
┌─────────────────────────────────────────────────────────┐
│ Deliverability              [Date Range ▼] [Domain ▼]  │
├─────────────────────────────────────────────────────────┤
│                                                         │
│  Overall Health: 92/100  ↑ +2                          │
│  Delivery Rate: 98.2%    ↑ +0.5%                       │
│  Bounce Rate: 1.2%       ↓ -0.2%                       │
│  Complaint Rate: 0.3%    ↓ -0.1%                       │
│                                                         │
│  ┌───────────────────────────────────────────────────┐ │
│  │ Delivery by Provider (Last 7 Days)               │ │
│  │                                                   │ │
│  │ Gmail      ▓▓▓▓▓▓▓▓▓▓  98.4% (245K)              │ │
│  │ Outlook    ▓▓▓▓▓▓▓▓    96.2% (156K)              │ │
│  │ Yahoo      ▓▓▓▓▓▓▓░    94.1% (98K)               │ │
│  │ Other      ▓▓▓▓▓▓      95.3% (234K)              │ │
│  │                                                   │ │
│  └───────────────────────────────────────────────────┘ │
│                                                         │
│  ┌───────────────────────────────────────────────────┐ │
│  │ Email Volume & Status (Last 30 Days)              │ │
│  │        900K┤                                      │ │
│  │        800K├─ ▁▂▃▄▄▅▅▆▇▇███████▇▇▇▆▅▄▃▂▁─ Delivered
│  │        700K├─ ░░░░░░░░░░░░░░░░░░░░░░░░░░─ Bounced
│  │        600K├                                      │ │
│  │          0├─────────────────────────────────     │ │
│  │            1  5  10 15 20 25 30                  │ │
│  └───────────────────────────────────────────────────┘ │
│                                                         │
│  ┌──────────────────┐  ┌──────────────────┐            │
│  │ Bounce Reasons   │  │ Complaint Rate   │            │
│  │ Invalid: 45%     │  │ 0.3% (Low)      │            │
│  │ Inactive: 35%    │  │ Monitor closely │            │
│  │ Soft: 20%        │  │ Report issues   │            │
│  └──────────────────┘  └──────────────────┘            │
│                                                         │
└─────────────────────────────────────────────────────────┘
```

**Features:**
- Date range selector (Last 24h, 7d, 30d, 90d, Custom)
- Domain filter dropdown
- Provider filter (Gmail, Outlook, Yahoo, Other)
- Send stream filter (if multiple)
- Metric cards with trend indicators (↑/↓)
- Charts: Line (volume), Bar (provider breakdown), Pie (bounce reasons)
- Downloadable data (CSV, JSON)

---

## 5.11 Security Center

### 5.11.1 Security Dashboard

**Layout:**
```
┌────────────────────────────────────────────────────┐
│ Security Center                     [Refresh]      │
├────────────────────────────────────────────────────┤
│                                                    │
│ Overall Risk: 🟢 LOW                             │
│ Threats: 0 Active    Security Incidents: 1        │
│ Quarantined: 2       Last Scan: 2 hours ago      │
│                                                    │
│ ┌────────────────────────────────────────────────┐ │
│ │ Threat Activity (Last 7 Days)                 │ │
│ │  Phishing: ■■■░░ (5 detected)                │ │
│ │  Malware: ■░░░░ (1 detected)                 │ │
│ │  Spoofing: ■■░░░ (3 detected)                │ │
│ │  BEC: ░░░░░ (0 detected)                     │ │
│ │  Spam: ■■■■■■ (18 detected)                 │ │
│ └────────────────────────────────────────────────┘ │
│                                                    │
│ ┌────────────────────────────────────────────────┐ │
│ │ Quarantine (2 items)                      [>] │ │
│ ├────────────────────────────────────────────────┤ │
│ │ 1. Subject: "Urgent: Verify account"       │ │
│ │    Risk: 🔴 Critical (92/100)               │ │
│ │    From: spoof@attacker.com                 │ │
│ │    Reason: Phishing indicators + Spoofing  │ │
│ │                                             │ │
│ │ 2. Subject: "Invoice #3490"                 │ │
│ │    Risk: 🟡 Medium (58/100)                 │ │
│ │    From: vendor@external.com                │ │
│ │    Reason: BEC indicators + New domain     │ │
│ └────────────────────────────────────────────────┘ │
│                                                    │
│ [View All Threats]  [View Quarantine]  [Settings] │
│                                                    │
└────────────────────────────────────────────────────┘
```

### 5.11.2 Quarantine View

**Quarantine Table:**
```
┌──────────────────────────────────────────────────────┐
│ Quarantine                                          │
├──────────────────────────────────────────────────────┤
│ ☐ Risk │ Sender │ Subject │ Reason │ Time │ Actions│
├──────────────────────────────────────────────────────┤
│ ☐ 92   │ spoof@ │ Urgent │ Phishing│14:32│  ⋯   │
│   🔴   │ attac..│ verify │ Spoofing│     │       │
│        │        │ account│        │     │       │
│        │        │        │        │     │       │
│ ☐ 58   │ vendor│ Invoice│ BEC     │ 12:15│  ⋯   │
│   🟡   │ @ext..│ #3490  │ NewDom  │     │       │
│        │        │        │        │     │       │
│ ☐ 35   │ promo@│ LIMITED│ Spam    │ 10:45│  ⋯   │
│   🟢   │ offer.│ TIME..   │ Unauth  │     │       │
│        │        │        │        │     │       │
│                                                      │
│ [< Prev] 1 of 3 [Next >]                           │
│                                                      │
│ Bulk Actions: [Release] [Delete] [Block]           │
│                                                      │
└──────────────────────────────────────────────────────┘
```

**Quarantine Item Detail:**
```
┌────────────────────────────────────────┐
│ Quarantine Details                   × │
├────────────────────────────────────────┤
│                                        │
│ Risk Score: 92/100  🔴 CRITICAL       │
│                                        │
│ From: spoof@attacker.com              │
│ To: john@company.com                  │
│ Subject: Urgent: Verify account       │
│ Received: Aug 20, 14:32               │
│                                        │
│ ────────────────────────────────────── │
│ Detection Signals:                     │
│                                        │
│ 🔴 DOMAIN_IMPERSONATION                │
│    Sender claims to be company.com    │
│    but actual domain is attacker.com  │
│    SPF: ✗ FAIL                        │
│                                        │
│ 🔴 PHISHING_URL                        │
│    Body contains: hxxps://evil.com   │
│    Similar to: www.company.com       │
│    URL reputation: Malicious (known) │
│                                        │
│ 🟡 NEW_DOMAIN                         │
│    Sender domain created 2 days ago   │
│    Typical phishing pattern           │
│                                        │
│ ────────────────────────────────────── │
│ Full Headers:                          │
│ [Show]                                │
│                                        │
│ ────────────────────────────────────── │
│ Actions:                              │
│                                        │
│ [Release to Inbox]                    │
│ [Delete]                              │
│ [Block Sender]                        │
│ [Block Domain]                        │
│ [Report as Phishing]                  │
│                                        │
│ ────────────────────────────────────── │
│ Note: This email appears dangerous.   │
│ Proceed with extreme caution.         │
│                                        │
│ [Close]                               │
│                                        │
└────────────────────────────────────────┘
```

---

## 5.12 Administration & Settings

### 5.12.1 Organization Settings

**Settings Navigation:**
```
┌──────────────────────────────────────────────────────┐
│ Settings                                            │
├──────────────────────────────────────────────────────┤
│ Organization   Team          Billing                │
│ ├ General      ├ Members     ├ Billing Plan         │
│ ├ Branding     ├ Roles       ├ Usage & Limits       │
│ ├ Domains      ├ Permissions ├ Invoices             │
│ └ Email        └ Groups      └ Payment Method       │
│                                                      │
│ Deployments   Audit Log      Integrations           │
│ ├ Self-Hosted ├ Activities   ├ Third-party Apps     │
│ └ Instances   └ Search       └ Webhooks             │
│                                                      │
│ Admin Panels                  Developer Settings    │
│ ├ Mass Mailing             ├ API Settings         │
│ └ Database Management       └ SMTP Settings        │
│                                                      │
└──────────────────────────────────────────────────────┘
```

### 5.12.2 Mass Mailing

**Mass Mailing Dialog:**
```
┌──────────────────────────────────────────────┐
│ Send Bulk Email                            × │
├──────────────────────────────────────────────┤
│                                              │
│ Recipients: [Select Recipients... ▼]        │
│ ☐ Entire Organization                       │
│ ☐ Specific Department/Team                  │
│ ☐ Individual Users                          │
│                                              │
│ If Department/Team:                         │
│ [Select Department ▼]                       │
│ Estimated recipients: 45                    │
│                                              │
│ If Individual Users:                        │
│ [Search/Select Users...              ▼]    │
│ Selected: 12 users                          │
│ [× Clear Selection]                         │
│                                              │
│ ────────────────────────────────────────   │
│                                              │
│ Email Template:                             │
│ [Select Template...               ▼]        │
│ or                                          │
│ [Compose Custom Message]                    │
│                                              │
│ If Custom Message:                          │
│ Subject: [Q2 Strategy Update]               │
│                                              │
│ [Rich Text Editor Area]                     │
│ ────────────────────────────────────────   │
│                                              │
│ Schedule:                                   │
│ ○ Send Immediately                          │
│ ○ Schedule for later:                       │
│   [Date: Aug 25] [Time: 10:00 AM]           │
│                                              │
│ ────────────────────────────────────────   │
│ Preview:                                    │
│                                              │
│ Sending to: 45 recipients                  │
│ Total size: ~2.3 MB                        │
│ Estimated delivery: < 2 minutes            │
│                                              │
│ [Cancel] [Preview Email] [Send]            │
│                                              │
└──────────────────────────────────────────────┘
```

**Preview Email Modal:**
```
┌──────────────────────────────────────┐
│ Email Preview (Read-only)          × │
├──────────────────────────────────────┤
│ From: notifications@company.com      │
│ To: [Recipients will be filled]     │
│ Subject: Q2 Strategy Update         │
│                                      │
│ ─────────────────────────────────── │
│                                      │
│ Dear Team,                           │
│                                      │
│ We are excited to share our updated │
│ Q2 strategy with you...             │
│                                      │
│ [Email body continues...]            │
│                                      │
│ ─────────────────────────────────── │
│                                      │
│ [Close] [Go Back] [Send]            │
│                                      │
└──────────────────────────────────────┘
```

**Confirmation Screen:**
```
┌──────────────────────────────────────────┐
│ ✓ Email Sent Successfully              │
├──────────────────────────────────────────┤
│                                          │
│ Recipients: 45                          │
│ Subject: Q2 Strategy Update             │
│ Sent at: Aug 25, 2026 at 10:00 AM      │
│                                          │
│ Status:                                 │
│ ✓ 45 In Progress (checking delivery)   │
│                                          │
│ Delivery will be tracked automatically. │
│ Check Analytics > Email Logs for        │
│ detailed delivery reports.              │
│                                          │
│ [View Delivery Logs]  [Done]           │
│                                          │
└──────────────────────────────────────────┘
```

### 5.12.3 Audit Log

**Audit Log Table:**
```
┌──────────────────────────────────────────────────────┐
│ Audit Log                            [Filters] [Exp] │
├──────────────────────────────────────────────────────┤
│ User │ Action │ Resource │ Status │ Timestamp │     │
├──────────────────────────────────────────────────────┤
│ John │ UPDATE │ API Key  │ ✓      │ Aug 20.. │ ⋯  │
│      │ Regenerate│dev_api │Success │ 14:32    │    │
│      │                                       │    │
│ Jane │ CREATE │ Domain   │ ✓      │ Aug 19.. │ ⋯  │
│      │ Add Domain│company2 │Success │ 10:15   │    │
│      │                                       │    │
│ Bob  │ DELETE │ User     │ ✓      │ Aug 18.. │ ⋯  │
│      │ Remove │ test@... │ Success│ 09:45    │    │
│      │                                       │    │
│                                                      │
│ [< Prev] Showing 1-3 of 2,847 [Next >]            │
│                                                      │
└──────────────────────────────────────────────────────┘
```

---

# 6. User Flows & Interactions

## 6.1 New User Onboarding

**Flow:**
```
1. Organization Created
   ↓
2. Domain Verification
   ├─ Add domain
   ├─ Copy DNS records
   ├─ Verify
   └─ Repeat for additional domains
   ↓
3. Team Setup (Optional)
   ├─ Create teams/departments
   ├─ Invite members
   └─ Assign roles
   ↓
4. Email Configuration
   ├─ Configure SMTP
   ├─ Set SPF/DKIM/DMARC
   └─ Enable security features
   ↓
5. First Email Test
   ├─ Send test email
   ├─ Verify delivery
   └─ View logs
   ↓
6. Ready to Use
   ├─ Dashboard overview
   ├─ Mailbox ready
   └─ API documentation
```

## 6.2 Sending Email Workflow

**Corporate Mail User:**
```
Dashboard → Inbox → [Compose] → Select Recipients 
→ Write Message → [Send] → Email in Sent folder
```

**API/Developer:**
```
Dashboard → Email Logs → See API deliveries
OR
External integration using API Key/SMTP
```

## 6.3 Domain Verification Flow

```
Dashboard → Domains → [+ Add Domain] → Enter domain
→ Copy DNS records → [Verify] → Check results
→ If failed: Show error, suggest fix
→ If passed: Domain active, ready to use
```

## 6.4 Security Incident Response

```
Security Center → Threats/Quarantine → Click item
→ View Risk Score + Signals → Take action:
├─ [Release] → Accept & move to inbox
├─ [Delete] → Discard
├─ [Block] → Add sender/domain to blocklist
└─ [Report] → Report phishing to authorities
```

---

# 7. Responsive Design

## 7.1 Breakpoints

```css
/* Mobile-first approach */
@media (min-width: 480px) { /* Mobile */ }
@media (min-width: 768px) { /* Tablet */ }
@media (min-width: 1024px) { /* Desktop */ }
@media (min-width: 1440px) { /* Large Desktop */ }
```

## 7.2 Layout Adaptations

### Mobile (<480px)
- Sidebar: Hidden, slide-out drawer (on hamburger)
- Top nav: Simplified, icon-only buttons
- Email list: 1 column (sender + subject only)
- Email detail: Full-screen
- Modals: Full-screen dialogs
- Tables: Card-based layout (stack rows vertically)
- Charts: Scrollable horizontal

### Tablet (768px-1023px)
- Sidebar: Collapsible icon-view (60px)
- Email list: 2 columns (list + preview)
- Charts: Responsive, stack on height
- Buttons: Medium size, less padding
- Fonts: Slightly reduced

### Desktop (1024px+)
- Full 3-column layout (sidebar + list + detail)
- All features visible
- Standard typography & spacing
- Hover states enabled

---

# 8. Accessibility & Dark Mode

## 8.1 Accessibility (WCAG 2.1 AA)

- **Color Contrast:** All text meets WCAG AA standards (4.5:1 for body text)
- **Keyboard Navigation:**
  - Tab through all interactive elements
  - Enter/Space to activate buttons
  - Arrow keys for dropdowns/lists
  - Escape to close modals
  - Cmd/Ctrl+K for global search
- **Screen Readers:** All interactive elements have aria-labels
- **Focus Indicators:** Clear blue outline on all interactive elements
- **Semantic HTML:** Proper heading hierarchy, landmark regions
- **Form Labels:** Every input has associated `<label>`
- **Error Messages:** Clear, associated with fields, color + text

## 8.2 Dark Mode Implementation

**Approach:** CSS Custom Properties (CSS Variables)

```css
:root {
  /* Light Theme */
  --bg-primary: #FFFFFF;
  --bg-secondary: #F8FAFC;
  --text-primary: #0F172A;
  --text-secondary: #64748B;
  --border: #E5E7EB;
  --accent: #3B82F6;
}

@media (prefers-color-scheme: dark) {
  :root {
    /* Dark Theme */
    --bg-primary: #0F172A;
    --bg-secondary: #1E293B;
    --text-primary: #F8FAFC;
    --text-secondary: #CBD5E1;
    --border: #475569;
    --accent: #60A5FA;
  }
}

body {
  background-color: var(--bg-primary);
  color: var(--text-primary);
}
```

**Dark Mode Specifics:**
- All colors adjusted for contrast
- Reduced shadow opacity (dark bg doesn't need as much shadow)
- Icons remain consistent
- No image inversions
- User preference respected via system settings or manual toggle

---

# Implementation Checklist

- [ ] Design system file created (this document)
- [ ] Figma design kit with all components
- [ ] Tailwind CSS config updated with color system
- [ ] React component library built (Button, Input, Table, etc.)
- [ ] Authentication flow UI implemented
- [ ] Dashboard screen implemented
- [ ] Mail inbox UI implemented (3-column)
- [ ] Compose email modal implemented
- [ ] Email detail view implemented
- [ ] Domains management UI implemented
- [ ] Email logs table implemented
- [ ] Security center dashboard implemented
- [ ] Settings/Admin panels implemented
- [ ] Dark mode fully tested
- [ ] Responsive design tested (mobile, tablet, desktop)
- [ ] Accessibility audit (WCAG 2.1 AA)
- [ ] Performance optimizations (images, code splitting, lazy loading)
- [ ] Storybook stories for all components
- [ ] Design documentation in Storybook

---

**End of Design System Specification v1.0**

---

# 4. Color System

Все цвета используются через semantic tokens.

Запрещается использовать случайные hardcoded цвета непосредственно в компонентах.

## Light

```css
--background: 0 0% 100%;
--foreground: 222 24% 10%;

--surface: 220 20% 98%;
--surface-elevated: 0 0% 100%;

--muted: 220 16% 96%;
--muted-foreground: 220 9% 46%;

--border: 220 14% 90%;
--border-strong: 220 13% 82%;

--primary: 221 83% 53%;
--primary-foreground: 0 0% 100%;

--success: 142 71% 40%;
--warning: 38 92% 50%;
--danger: 0 72% 51%;
--info: 199 89% 48%;
```

## Dark

```css
--background: 222 24% 8%;
--foreground: 210 20% 96%;

--surface: 222 21% 10%;
--surface-elevated: 222 19% 13%;

--muted: 220 17% 16%;
--muted-foreground: 218 11% 65%;

--border: 220 15% 18%;
--border-strong: 220 14% 25%;

--primary: 217 91% 60%;
--primary-foreground: 0 0% 100%;

--success: 142 69% 50%;
--warning: 43 96% 56%;
--danger: 0 84% 60%;
--info: 199 89% 55%;
```

---

# 5. Brand Accent

Основной accent — **modern electric blue**.

Использовать для:

* primary buttons;
* active navigation;
* links;
* selected controls;
* focus states;
* key chart series.

Accent не должен занимать большие площади интерфейса.

Основные surfaces остаются neutral.

---

# 6. Typography

Предпочтительный UI font:

```text
Inter
```

Fallback:

```css
font-family:
  Inter,
  ui-sans-serif,
  system-ui,
  -apple-system,
  BlinkMacSystemFont,
  "Segoe UI",
  sans-serif;
```

Для:

* API keys;
* IP;
* domains;
* SMTP codes;
* Message IDs;
* DNS records;
* code;

использовать monospace:

```text
JetBrains Mono
```

или системный monospace fallback.

---

# 7. Typography Scale

## Page Title

```text
28px
font-weight: 650–700
line-height: 34px
```

## Section Title

```text
20px
font-weight: 600
```

## Card Title

```text
14–16px
font-weight: 600
```

## Body

```text
14px
line-height: 20–21px
```

## Secondary

```text
13px
```

## Metadata

```text
12px
```

Не использовать слишком много размеров шрифта.

---

# 8. Spacing

Базовая сетка:

```text
4px
```

Допустимые spacing values:

```text
4
8
12
16
20
24
32
40
48
64
```

Основной gap интерфейса:

```text
16px
```

Page section gap:

```text
24–32px
```

---

# 9. Border Radius

Использовать умеренное скругление.

```text
Small controls: 6px
Buttons:        7–8px
Inputs:         7–8px
Cards:          10px
Dialogs:        12px
```

Не использовать чрезмерно округлый "mobile app" стиль.

Pill radius допустим только для:

* badges;
* status;
* filters;
* avatars.

---

# 10. Shadows

Shadows минимальны.

Основная структура создаётся через:

* background;
* borders;
* spacing.

Shadow используется для:

* dropdown;
* popover;
* modal;
* floating compose;
* command palette.

Карточки dashboard по умолчанию используют border, а не тяжёлую тень.

---

# 11. Application Shell

Desktop layout:

```text
┌───────────────┬──────────────────────────────────────────┐
│               │ Top Bar                                  │
│               ├──────────────────────────────────────────┤
│    Sidebar    │                                          │
│               │              Content                     │
│               │                                          │
│               │                                          │
└───────────────┴──────────────────────────────────────────┘
```

Sidebar:

```text
240–256px expanded
64–72px collapsed
```

Top Bar:

```text
56px
```

---

# 12. Sidebar

Sidebar — основная навигация продукта.

Пример:

```text
Logo

Workspace / Organization switcher

OVERVIEW
Dashboard

MAIL
Inbox
Sent
Drafts
Spam
Quarantine

DEVELOPER
Email Logs
Domains
API Keys
SMTP
Webhooks
Templates

ANALYTICS
Deliverability
Analytics
Reputation

SECURITY
Security Center
Threats
Abuse

MANAGE
Team
Billing
Deployments
Audit

Settings
```

Пункты показываются в зависимости от:

* product edition;
* permissions;
* RBAC;
* enabled modules.

---

# 13. Sidebar Item

Высота:

```text
36–40px
```

Структура:

```text
[icon] Label                     Badge
```

Active state:

* subtle accent background;
* accent foreground/icon;
* medium font weight.

Hover должен быть заметным, но спокойным.

---

# 14. Organization Switcher

В верхней части sidebar:

```text
┌──────────────────────────┐
│ AC   Acme Corporation  ▾ │
│      Production          │
└──────────────────────────┘
```

Switcher позволяет менять:

```text
Tenant
Organization
Project
```

Текущий context всегда должен быть очевиден.

---

# 15. Top Bar

Содержит:

```text
Breadcrumbs
Search
Quick actions
Notifications
Help
Theme
User menu
```

Пример:

```text
Acme / Production / Domains

                     Search     + Send email     🔔     JD
```

---

# 16. Global Search

Shortcut:

```text
⌘ K
Ctrl K
```

Search/Command Palette ищет:

* messages;
* domains;
* organizations;
* projects;
* API keys;
* incidents;
* documentation/actions.

---

# 17. Page Header

Стандарт:

```text
Page Title                       Primary Action
Short page description           Secondary Action
```

Пример:

```text
Domains                          + Add domain

Manage sending and inbound domains.
```

Не помещать Page Header внутрь card.

---

# 18. Buttons

Variants:

```text
Primary
Secondary
Ghost
Outline
Destructive
Link
```

Размеры:

```text
sm      32px
default 36px
lg      40px
```

Primary button на странице обычно только один.

---

# 19. Inputs

Высота:

```text
36–40px
```

Обязательные states:

```text
default
hover
focus
disabled
error
success
```

Focus ring должен быть хорошо видим в Light и Dark themes.

---

# 20. Cards

Card:

```text
background: surface-elevated
border: 1px solid border
radius: 10px
```

Типичный padding:

```text
16–20px
```

Карточки не должны использоваться для каждого блока страницы.

Если достаточно обычной секции — использовать секцию.

---

# 21. Tables

Tables — один из основных компонентов платформы.

Используются для:

* Domains;
* Email Logs;
* API Keys;
* SMTP Credentials;
* Webhooks;
* Security Events;
* Team;
* Audit;
* Deployments.

Header:

```text
height: 40px
font-size: 12px
font-weight: 500–600
```

Rows:

```text
44–52px
```

Row hover обязателен.

---

# 22. Table Toolbar

Стандарт:

```text
[Search................] [Status ▾] [Domain ▾] [Date ▾]     Columns  Export
```

Active filters отображаются отдельными chips.

Должна существовать команда:

```text
Clear filters
```

---

# 23. Status Badges

Использовать компактные badges.

Примеры:

```text
● Delivered
● Queued
● Deferred
● Bounced
● Failed
● Quarantined
● Verified
● Pending
```

Не использовать полностью насыщенный цветной background.

Предпочтительно:

```text
light tinted background
colored text
small status dot
```

---

# 24. Health Score

Domain/IP health:

```text
90–100 Excellent
75–89  Good
50–74  Needs attention
25–49  Poor
0–24   Critical
```

Компонент:

```text
Domain Health

92
Excellent

██████████████████░░
```

Цвет не должен быть единственным способом передачи состояния.

Всегда добавлять текст.

---

# 25. Dashboard

Главный Dashboard должен отвечать:

> Что сейчас происходит с моей почтовой инфраструктурой?

Первый viewport:

```text
Dashboard                         Last 24 hours ▾

[ Emails Sent ] [ Delivery Rate ] [ Bounce Rate ] [ Complaints ]

Delivery Overview
────────────────────────────────────────────────────
                  chart

Domain Health             Security
company.kz     96         4 quarantined
notify.kz      88         1 phishing
```

---

# 26. Dashboard Metrics

Основные показатели:

```text
Emails sent
Delivered
Delivery rate
Deferred
Bounced
Complaints
Blocked
Quarantined
```

Карточка:

```text
Delivery Rate

99.42%

↑ 0.18%
vs previous period
```

---

# 27. Charts

Charts должны быть минималистичными.

Не использовать:

* gradients без необходимости;
* 3D;
* тяжёлые grid lines;
* десятки цветов.

Для временных рядов:

```text
Delivered
Deferred
Bounced
Complaints
```

Tooltip содержит точные значения.

---

# 28. Domains

Domains screen:

```text
Domains                              + Add domain

Search domains...

Domain               Status       Health       Sending      Inbound
company.kz           Verified     96           Active       Active
notify.company.kz    Verified     88           Active       —
promo.company.kz     Warning      64           Limited      —
```

---

# 29. Domain Details

Header:

```text
company.kz

● Verified       Health 96       Sending Active
```

Tabs:

```text
Overview
DNS
Sending
Inbound
Reputation
Events
Settings
```

---

# 30. DNS Wizard

DNS setup является важным onboarding flow.

Каждая запись:

```text
DKIM

Type
TXT

Host
mail._domainkey.company.kz

Value
v=DKIM1; k=rsa; p=...

[ Copy ]

● Verified
```

Records:

* Verification TXT;
* SPF;
* DKIM;
* DMARC;
* Return-Path;
* MTA-STS;
* TLS-RPT.

Должна существовать кнопка:

```text
Verify DNS
```

и timestamp последней проверки.

---

# 31. Email Logs

Один из наиболее важных developer screens.

```text
Email Logs

Search by recipient, message ID...

Status      Recipient             Subject              Time
Delivered   john@example.com      Verification code    14:32
Deferred    user@example.org      Invoice #2041        14:31
Bounced     old@example.com       Password reset       14:29
```

---

# 32. Message Details

Message открывается предпочтительно в side sheet или отдельной странице.

```text
Verification code

● Delivered

From
no-reply@company.kz

To
john@example.com

Message ID
em_01J...

Stream
transactional
```

Lifecycle:

```text
14:32:01 Accepted
     │
14:32:01 Queued
     │
14:32:02 Scanned
     │
14:32:02 Routed
     │
14:32:03 SMTP attempt
     │
14:32:04 Delivered
```

Для каждого события можно раскрыть technical details.

---

# 33. API Keys

```text
API Keys                           + Create API key

Name              Project       Created       Last used
production-shop   Production    Aug 18        2 min ago
backend           Production    Aug 12        1 hour ago
```

После создания secret показывается только один раз:

```text
Your API key

em_live_xxxxxxxxxxxxxxxxx

[ Copy ]

Save this key now.
You won't be able to see it again.
```

---

# 34. SMTP Credentials

Аналогичный security pattern.

Показывать:

* credential name;
* username;
* allowed domains;
* IP allowlist;
* created;
* last used;
* status.

Password никогда не показывать повторно.

---

# 35. Webhooks

Webhook details:

```text
Production webhook

Endpoint
https://example.com/email/events

Status
● Active

Events
✓ delivered
✓ bounced
✓ complained
✓ failed
```

Ниже:

```text
Delivery attempts
```

с:

* status;
* HTTP code;
* latency;
* timestamp;
* retry.

---

# 36. Security Center

Security Center визуально должен отличаться от обычной analytics page, но оставаться частью общей системы.

Header:

```text
Security Center

Threat activity across your organizations
```

Metrics:

```text
Quarantined
Phishing
Malware
Spoofing
BEC
```

---

# 37. Security Incident

```text
Potential phishing attempt

Risk Score
91 / 100

Action
Quarantined

Signals
DMARC_FAIL
DOMAIN_IMPERSONATION
PHISHING_URL
NEW_DOMAIN
```

Risk score должен быть визуально заметен.

---

# 38. Security Signal

Каждый signal показывает:

```text
Icon
Signal
Severity
Explanation
```

Пример:

```text
High

DOMAIN IMPERSONATION

Sender domain is visually similar
to a protected organization domain.
```

---

# 39. Quarantine

```text
Quarantine

Sender             Subject                Risk       Reason
billing@...        Urgent payment         91         Phishing
admin@...          Password expiration    84         Credential theft
```

Actions:

```text
Release
Delete
Block sender
Block domain
Report phishing
```

Dangerous email content не исполняется автоматически.

---

# 40. Deliverability

Основной экран:

```text
Deliverability

Overall Health
92 / 100

Gmail       96
Outlook     89
Yahoo       93
Other       91
```

Далее:

```text
Delivery rate
Bounce rate
Complaint rate
Blocks
Deferrals
```

---

# 41. Reputation

Поддерживать визуализацию:

```text
IP Reputation
Domain Reputation
Sender Reputation
Provider Reputation
```

Не превращать reputation в единственное абстрактное число.

Показывать факторы, влияющие на score.

---

# 42. Empty States

Пустая страница никогда не должна выглядеть как ошибка.

Пример Domains:

```text
No domains yet

Add your first domain to start
sending authenticated email.

[ Add domain ]
```

Empty state содержит:

1. icon;
2. title;
3. explanation;
4. primary action.

---

# 43. Loading

Использовать:

* skeleton rows;
* skeleton cards;
* subtle spinner для локальных операций.

Не блокировать весь интерфейс global spinner, если загружается только один компонент.

---

# 44. Errors

Ошибки должны объяснять:

* что произошло;
* что пользователь может сделать.

Плохо:

```text
Error 500
```

Хорошо:

```text
We couldn't verify the DNS records.

Your existing configuration has not been changed.

[ Try again ]
```

Technical ID можно показать ниже.

---

# 45. Toasts

Toast используется для краткого подтверждения:

```text
Domain added
API key copied
Webhook retry scheduled
Settings saved
```

Для критических ошибок использовать persistent alert, а не исчезающий toast.

---

# 46. Modals

Modal использовать только для коротких focused actions:

* confirmation;
* create API key;
* delete;
* rotate secret;
* small form.

Большие workflows должны использовать отдельную страницу или Sheet.

---

# 47. Destructive Actions

Для:

* Delete domain;
* Revoke API key;
* Delete webhook;
* Suspend tenant;
* Delete quarantine item.

требуется confirmation.

Для особенно опасных действий:

```text
Type company.kz to confirm
```

---

# 48. Business Mail

Будущий Mail интерфейс должен использовать ту же design system.

Layout desktop:

```text
┌───────────┬───────────────────────┬─────────────────────────────┐
│ Mail Nav  │ Message List          │ Reading Pane                │
│           │                       │                             │
│ Inbox 24  │ John Smith            │ Subject                     │
│ Starred   │ Welcome...            │                             │
│ Sent      │ 10:42                 │ John Smith <john@...>       │
│ Drafts 3  │                       │                             │
│ Spam      │ Jane Doe              │ Message body...             │
│ Trash     │ Invoice...            │                             │
└───────────┴───────────────────────┴─────────────────────────────┘
```

---

# 49. Mail Navigation

Основные folders:

```text
Inbox
Starred
Snoozed
Sent
Drafts
Spam
Quarantine
Trash
```

Custom folders/labels поддерживаются в будущем.

Unread count:

```text
Inbox                     24
```

---

# 50. Message List

Message row показывает:

```text
checkbox
star
sender
subject
preview
attachments
timestamp
```

Unread:

```text
font-weight: 600
```

Read:

```text
font-weight: 400
```

Selected message имеет subtle accent background.

---

# 51. Reading Pane

Header:

```text
Subject

John Smith <john@example.com>
to me

10:42
```

Actions:

```text
Reply
Reply all
Forward
Archive
Spam
Delete
More
```

Длинные technical headers скрыты по умолчанию.

---

# 52. Conversation Threads

Несколько писем объединяются:

```text
Subject

John Smith
Message preview...

You
Message preview...

John Smith
Expanded message...
```

Предыдущие сообщения могут быть collapsed.

---

# 53. Compose

Compose может быть:

* floating desktop window;
* full page;
* mobile full-screen.

Desktop:

```text
┌──────────────────────────────────────────────┐
│ New message                             — □ × │
├──────────────────────────────────────────────┤
│ To                                           │
│ Cc Bcc                                       │
├──────────────────────────────────────────────┤
│ Subject                                      │
├──────────────────────────────────────────────┤
│                                              │
│ Message...                                   │
│                                              │
├──────────────────────────────────────────────┤
│ Send     📎   Formatting             ⋯   🗑  │
└──────────────────────────────────────────────┘
```

---

# 54. Compose Autosave

Draft автоматически сохраняется.

State:

```text
Saving...
Saved
Offline
Save failed
```

Не показывать toast при каждом autosave.

---

# 55. Attachments

Attachment component:

```text
┌──────────────────────────────┐
│ 📄 invoice.pdf               │
│ 2.4 MB                   ×   │
└──────────────────────────────┘
```

Upload:

```text
Uploading 62%
████████████░░░░
```

---

# 56. Mail Search

Search должен поддерживать advanced filters.

Пример:

```text
from:john@example.com
to:billing@company.kz
subject:invoice
has:attachment
after:2026-08-01
```

UI не должен требовать знания этих операторов — должен существовать visual filter builder.

---

# 57. Responsive Design

Breakpoints:

```text
mobile   < 768
tablet   768–1023
desktop  >= 1024
wide     >= 1440
```

Desktop является основной рабочей средой Admin Portal.

Но критические функции должны работать на mobile.

---

# 58. Mobile Mail

На mobile:

```text
Message List
      ↓
Open Message
      ↓
Back
```

Не пытаться сохранить desktop three-pane layout.

Compose становится fullscreen.

---

# 59. Mobile Admin

Sidebar превращается в drawer.

Tables могут:

1. горизонтально прокручиваться для технических данных;
2. превращаться в cards там, где это улучшает UX.

Не скрывать критические данные только ради mobile layout.

---

# 60. Accessibility

Минимальные требования:

* keyboard navigation;
* visible focus;
* semantic HTML;
* labels для inputs;
* ARIA только где необходимо;
* screen reader descriptions;
* достаточный contrast;
* status не передаётся только цветом.

Target:

```text
WCAG 2.1 AA
```

---

# 61. Icons

Рекомендуемый набор:

```text
Lucide Icons
```

Default:

```text
16px
```

Large UI:

```text
18–20px
```

Stroke должен оставаться визуально лёгким.

Не смешивать несколько icon libraries без необходимости.

---

# 62. Motion

Animations:

```text
120–200ms
```

Использовать для:

* hover;
* dropdown;
* sidebar;
* modal;
* sheet;
* accordion.

Не использовать длинные decorative animations.

Учитывать:

```css
prefers-reduced-motion
```

---

# 63. Developer-Oriented UI

Технические данные должны быть легко копируемыми.

Для:

```text
API key
Message ID
Domain
IP
DNS record
Webhook ID
SMTP response
```

использовать pattern:

```text
em_01J9K...                 Copy
```

После copy:

```text
Copied
```

---

# 64. Technical Detail Blocks

Для SMTP/API/debug information использовать code-like surfaces:

```text
SMTP Response

550 5.1.1 The email account that you tried
to reach does not exist.
```

Monospace + muted surface.

---

# 65. Date & Time

Backend использует UTC.

UI отображает timezone пользователя.

Tooltip:

```text
Aug 20, 2026 · 14:32

UTC
2026-08-20 09:32:18 UTC
```

Для недавних событий допустимо:

```text
2 min ago
```

с точным timestamp в tooltip.

---

# 66. RBAC UI

Недоступные пользователю действия предпочтительно:

* скрывать, если они вообще не относятся к роли;
* disable + explanation, если пользователю важно знать о существовании возможности.

Пример:

```text
Delete domain
```

disabled tooltip:

```text
Requires Organization Admin permission.
```

---

# 67. Audit UI

Audit event:

```text
14:32

John Doe
rotated API credential

Project
Production

IP
185.xxx.xxx.xxx
```

Filters:

```text
Actor
Action
Resource
Organization
Date
```

---

# 68. Self-Hosted / Fleet

Deployment card:

```text
Production Kazakhstan

● Healthy

Version
3.2.1

Nodes
6

Mail Queue
124

Last contact
32 sec ago
```

States:

```text
Healthy
Warning
Critical
Offline
Updating
```

---

# 69. Security UX Rules

Security action всегда должна показывать причину.

Нельзя ограничиваться:

```text
Blocked
```

Нужно:

```text
Blocked

Reason:
High-confidence phishing detection

Risk Score:
91 / 100

Signals:
DMARC failure
Domain impersonation
Known phishing URL
```

Это особенно важно для Security Analyst.

---

# 70. Confirmation Philosophy

Routine actions:

```text
Save
Send
Retry
Verify
```

не требуют лишнего confirmation.

Irreversible actions требуют confirmation.

Цель — не создавать confirmation fatigue.

---

# 71. Component Architecture

Базовые primitives строить поверх `shadcn/ui`.

Пример:

```text
components/
├── ui/
│   ├── button
│   ├── input
│   ├── dialog
│   ├── dropdown-menu
│   ├── sheet
│   ├── tabs
│   ├── tooltip
│   └── ...
│
├── platform/
│   ├── status-badge
│   ├── health-score
│   ├── metric-card
│   ├── page-header
│   ├── data-table
│   ├── code-value
│   ├── timeline
│   ├── risk-score
│   └── empty-state
│
└── mail/
    ├── mail-sidebar
    ├── message-list
    ├── message-row
    ├── message-view
    ├── compose
    └── attachment
```

---

# 72. Data Tables

Использовать:

```text
TanStack Table
```

для сложных таблиц.

Поддерживать при необходимости:

* sorting;
* filtering;
* pagination;
* row selection;
* column visibility;
* server-side pagination.

URL должен сохранять важные filters.

Пример:

```text
/emails?status=bounced&domain=company.kz
```

---

# 73. Forms

Validation:

```text
Zod
```

Ошибки показываются непосредственно возле поля.

Не использовать toast как единственный способ показать validation error.

---

# 74. Query States

TanStack Query states должны иметь единый UI pattern:

```text
Loading
Success
Empty
Error
Refreshing
```

При background refresh существующие данные не должны исчезать.

---

# 75. Product Terminology

Использовать терминологию ТЗ последовательно:

```text
Tenant
Organization
Sub-Organization
Project
Domain
Sending Stream
API Key
SMTP Credential
IP Pool
Security Incident
Quarantine
Deployment
```

Не заменять эти сущности случайными синонимами между страницами.

---

# 76. Information Hierarchy

Главный пользовательский контекст:

```text
Tenant
  ↓
Organization
  ↓
Project
  ↓
Domain
```

Интерфейс всегда должен позволять понять, к какому уровню относится объект.

---

# 77. Onboarding

Первый onboarding:

```text
Create organization
      ↓
Create project
      ↓
Add domain
      ↓
Configure DNS
      ↓
Verify domain
      ↓
Create API key / SMTP credential
      ↓
Send test email
      ↓
View delivery event
```

Прогресс должен сохраняться.

---

# 78. First Send Experience

После domain verification:

```text
Your domain is ready

Send your first email.

REST API
SMTP

[ Send test email ]
```

Пользователь должен иметь возможность получить первый успешный результат без чтения большой документации.

---

# 79. Documentation Integration

Developer pages могут содержать contextual snippets:

```text
Send with cURL

curl https://api.example.kz/v1/emails \
  -H "Authorization: Bearer $API_KEY" \
  ...
```

Code examples должны иметь:

```text
Copy
```

и language tabs при наличии SDK.

---

# 80. Design Anti-Patterns

Не использовать:

* excessive gradients;
* glassmorphism как основной стиль;
* giant rounded cards;
* excessive shadows;
* emoji вместо UI icons;
* слишком большие headings;
* rainbow dashboards;
* чрезмерно просторные tables;
* modal для каждого действия;
* бесконечные nested cards;
* hidden technical information без возможности раскрытия.

---

# 81. Page Density

Admin Portal должен использовать эффективную ширину.

Recommended content:

```text
max-width: 1600px
```

Dashboard может использовать почти всю доступную ширину.

Settings/forms:

```text
max-width: 720–900px
```

Не растягивать формы на 1600px.

---

# 82. Z-Index Layers

Единая система:

```text
base          0
sticky        10
dropdown      30
sheet         40
modal         50
toast         60
command       70
```

Не использовать случайные:

```text
z-[999999]
```

---

# 83. UX Performance

Интерфейс должен ощущаться быстрым.

Использовать:

* optimistic updates, где безопасно;
* skeleton;
* cached data;
* background refresh;
* prefetch;
* pagination/virtualization для больших logs.

Не рендерить десятки тысяч email events непосредственно в DOM.

---

# 84. Final Visual Character

Итоговый интерфейс должен ощущаться как:

```text
Professional
Technical
Secure
Fast
Calm
Precise
Modern
```

а не:

```text
Playful
Overdecorated
Consumer-only
Template-like
```

---

# 85. Final Design Rule

При выборе между:

```text
красивее
```

и

```text
понятнее
```

выбирать **понятнее**.

При выборе между:

```text
больше информации
```

и

```text
правильная информационная иерархия
```

выбирать **иерархию**.

При выборе между:

```text
уникальным компонентом
```

и

```text
существующим понятным UI pattern
```

по умолчанию использовать **понятный pattern**.

Дизайн платформы должен помогать пользователю управлять сложной email-инфраструктурой так, чтобы сама сложность системы не ощущалась сложностью интерфейса.
