---
name: Precision Enterprise Interface
colors:
  surface: '#f9f9ff'
  surface-dim: '#cfdaf2'
  surface-bright: '#f9f9ff'
  surface-container-lowest: '#ffffff'
  surface-container-low: '#f0f3ff'
  surface-container: '#e7eeff'
  surface-container-high: '#dee8ff'
  surface-container-highest: '#d8e3fb'
  on-surface: '#111c2d'
  on-surface-variant: '#434655'
  inverse-surface: '#263143'
  inverse-on-surface: '#ecf1ff'
  outline: '#737686'
  outline-variant: '#c3c6d7'
  surface-tint: '#0053db'
  primary: '#004ac6'
  on-primary: '#ffffff'
  primary-container: '#2563eb'
  on-primary-container: '#eeefff'
  inverse-primary: '#b4c5ff'
  secondary: '#5c5f61'
  on-secondary: '#ffffff'
  secondary-container: '#e0e3e5'
  on-secondary-container: '#626567'
  tertiary: '#943700'
  on-tertiary: '#ffffff'
  tertiary-container: '#bc4800'
  on-tertiary-container: '#ffede6'
  error: '#ba1a1a'
  on-error: '#ffffff'
  error-container: '#ffdad6'
  on-error-container: '#93000a'
  primary-fixed: '#dbe1ff'
  primary-fixed-dim: '#b4c5ff'
  on-primary-fixed: '#00174b'
  on-primary-fixed-variant: '#003ea8'
  secondary-fixed: '#e0e3e5'
  secondary-fixed-dim: '#c4c7c9'
  on-secondary-fixed: '#191c1e'
  on-secondary-fixed-variant: '#444749'
  tertiary-fixed: '#ffdbcd'
  tertiary-fixed-dim: '#ffb596'
  on-tertiary-fixed: '#360f00'
  on-tertiary-fixed-variant: '#7d2d00'
  background: '#f9f9ff'
  on-background: '#111c2d'
  surface-variant: '#d8e3fb'
typography:
  headline-lg:
    fontFamily: Inter
    fontSize: 32px
    fontWeight: '600'
    lineHeight: 40px
    letterSpacing: -0.02em
  headline-lg-mobile:
    fontFamily: Inter
    fontSize: 24px
    fontWeight: '600'
    lineHeight: 32px
    letterSpacing: -0.02em
  headline-md:
    fontFamily: Inter
    fontSize: 20px
    fontWeight: '600'
    lineHeight: 28px
    letterSpacing: -0.01em
  headline-sm:
    fontFamily: Inter
    fontSize: 16px
    fontWeight: '600'
    lineHeight: 24px
  body-lg:
    fontFamily: Inter
    fontSize: 16px
    fontWeight: '400'
    lineHeight: 24px
  body-md:
    fontFamily: Inter
    fontSize: 14px
    fontWeight: '400'
    lineHeight: 20px
  body-sm:
    fontFamily: Inter
    fontSize: 13px
    fontWeight: '400'
    lineHeight: 18px
  label-md:
    fontFamily: Inter
    fontSize: 12px
    fontWeight: '500'
    lineHeight: 16px
    letterSpacing: 0.02em
  code-sm:
    fontFamily: jetbrainsMono
    fontSize: 12px
    fontWeight: '400'
    lineHeight: 16px
rounded:
  sm: 0.25rem
  DEFAULT: 0.5rem
  md: 0.75rem
  lg: 1rem
  xl: 1.5rem
  full: 9999px
spacing:
  unit: 4px
  container-padding-desktop: 24px
  container-padding-mobile: 16px
  gutter: 16px
  stack-sm: 8px
  stack-md: 16px
  stack-lg: 32px
---

## Brand & Style
The design system is rooted in the "Linear-aesthetic"—a high-end, technical minimalism designed for high-velocity enterprise workflows. It prioritizes clarity over decoration, utilizing generous whitespace and a rigorous grid to reduce cognitive load.

The interface evokes a sense of "lightness" and "precision." It avoids heavy visual metaphors, relying instead on razor-sharp borders, subtle surface shifts, and refined typography. The target audience consists of professionals who value efficiency and a tool-like reliability in their communication platform.

**Key Stylistic Pillars:**
- **Minimalism:** Use whitespace as a functional separator rather than lines whenever possible.
- **Precision:** Every element is aligned to a strict 4px baseline and grid.
- **Technical Sophistication:** Subtle use of monospaced accents for metadata or technical identifiers.

## Colors
The palette is intentionally restrained to keep the focus on user content. 
- **Surface Strategy:** The main background is pure white (#FFFFFF). Secondary surfaces (#F8FAFC) are used for sidebars, utility panels, or nested containers to create logical grouping without adding visual weight.
- **Typography:** Headings use a deep Graphite/Slate (#1E293B) for maximum legibility, while body text uses a softer Slate (#475569) to reduce eye strain during long reading sessions.
- **Accents:** The Vibrant Blue (#2563EB) is used sparingly for primary actions, active states, and focus indicators.
- **Semantic Colors:** Success, Warning, and Error states utilize industry-standard hues but are desaturated slightly to maintain the sophisticated tone of the design system.

## Typography
Inter is the workhorse of this design system. It is implemented with a tight tracking (letter-spacing) on headings to achieve a modern, premium look. 

- **Scale:** The hierarchy is compact. For an enterprise SaaS, information density is critical; therefore, `body-md` (14px) is the default for most interface text and email lists.
- **Weight:** Use `600` (Semi-Bold) for headings to provide clear structural anchoring. Use `500` (Medium) for labels and interactive UI elements like buttons or navigation links.
- **Technical Accents:** For IDs, metadata, or timestamps, use JetBrains Mono at 12px to reinforce the technical/precise nature of the tool.

## Layout & Spacing
The design system employs a **12-column fluid grid** for main content areas, but transitions to a **fixed-width sidebar model** for navigation.

- **Sidebar:** Fixed at 240px or 280px depending on nesting depth.
- **Margins:** Desktop views use 24px outer margins. Internal component spacing (e.g., within an email thread) follows a strict 8px/16px/24px rhythm.
- **Density:** High-density lists (Inbox) should use 8px vertical padding between items, while focus modes (Message View) increase this to 16px or 24px to promote readability.
- **Breakpoints:** 
  - Mobile: < 768px (Sidebar hidden, Drawer menu used).
  - Tablet: 768px - 1024px (Collapsed sidebar icons).
  - Desktop: > 1024px (Full expanded navigation).

## Elevation & Depth
Depth is created primarily through **Tonal Layers** and **Subtle Outlines** rather than shadows. 

- **Level 0 (Base):** #FFFFFF. The primary canvas for content.
- **Level 1 (Sub-surface):** #F8FAFC. Used for sidebars and header backgrounds to differentiate global navigation from the workspace.
- **Level 2 (Popovers/Modals):** Pure white background with a very fine 1px border (#E5E7EB) and a soft, diffused shadow (`0px 4px 12px rgba(0, 0, 0, 0.05)`).
- **Interactive States:** On hover, items should transition to a #F1F5F9 (Slate-100) background rather than increasing shadow depth.
- **Borders:** Use 1px solid #E5E7EB for all structural divisions. Avoid 2px borders except for focus states.

## Shapes
The shape language is consistent and "soft-geometric." 
- **Standard Radius:** 8px (`rounded-md`) is the default for buttons, input fields, and small cards.
- **Large Containers:** 12px (`rounded-lg`) is used for modals or secondary panels.
- **Utility:** Icons and small status badges (pills) may use a fully rounded/circular shape to distinguish them from structural UI elements.

## Components
- **Buttons:** 
  - *Primary:* Solid #2563EB with white text. 8px radius. 
  - *Secondary:* White background with 1px #E5E7EB border. Slate text.
  - *Ghost:* No border or background; #F8FAFC background on hover.
- **Input Fields:** 1px border (#E5E7EB). On focus, the border changes to #2563EB with a 2px "ring" at 20% opacity.
- **Cards:** Avoid large, shadowed cards. Use 1px borders to define areas. If a card must be elevated, use the softest possible shadow.
- **Chips/Badges:** Small, 12px text, 4px-6px horizontal padding. Subtle backgrounds (e.g., Success is light green background with dark green text).
- **Icons:** Use Lucide-style outline icons with a 1.5px or 2px stroke width. Ensure icons are consistently sized to 16px or 20px within their containers.
- **Lists:** Inbox items should have a 1px bottom border. Active/Selected items use a subtle blue left-border accent (2px width) to indicate focus.