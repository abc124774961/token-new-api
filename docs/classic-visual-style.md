# Classic Visual Style Guide

This document records the target visual language for the `web/classic` theme.
Use the channel status monitor page as the reference page for future dashboard,
console, and base component improvements.

## Reference

- Reference screen: `web/classic/src/components/dashboard/channel-status/ChannelStatusMonitor.jsx`
- Route: `/console/channel-status`
- Theme intent: precise AI operations console, calm monitoring surface, dense but breathable data cards.

## Overall Direction

The classic theme should feel like a modern AI control room:

- Light, clean, high-signal surfaces.
- Soft teal/cyan identity accents with restrained blue and green status colors.
- Dense operational information without table-heavy visual fatigue.
- Rounded but crisp cards, subtle glass effects, thin borders, and very soft shadows.
- Numeric data should be prominent, monospaced, and easy to scan.
- Icons should be functional and quiet, not decorative.

Avoid marketing-style hero sections, oversized typography, heavy gradients,
large empty illustrations, and one-color pages.

## Layout

- Console pages use a fixed header, left sidebar, and a scrollable content canvas.
- Content starts with a compact page header: status eyebrow, title, short metadata, and right-aligned controls.
- Primary controls sit in the page header as segmented controls, pills, icon buttons, or compact action buttons.
- Dashboard content uses a 12-column responsive grid:
  - Summary metrics: 4 across on wide desktop, 2 across on medium screens, 1 on mobile.
  - Operational cards: 4 across on wide desktop, 2 across on medium screens, 1 on mobile.
- Keep page content aligned to the same left/right rhythm as the header and sidebar.
- Prefer vertical rhythm of 16-20px between page bands and 12-16px inside cards.

## Color System

Use these as the visual basis rather than hard requirements for every element:

- Page background: near-white with a faint cool tint, such as `#f8fafc` / `#f9fbfc`.
- Main text: slate/ink, around `#0f172a`.
- Secondary text: muted slate, around `#64748b`.
- Primary accent: teal, around `#0f766e`, `#14b8a6`.
- Soft accent surfaces: cyan/teal washes, around `rgba(204, 251, 241, 0.55)` and `rgba(224, 242, 254, 0.55)`.
- Success: green, around `#16a34a`.
- Warning: amber/orange, around `#d97706`.
- Danger: red, around `#dc2626`.
- Info: blue, around `#3b82f6`.

The page should not read as a single-hue teal page. Teal is the identity accent;
status and data colors should remain semantically distinct.

## Surfaces

Cards and panels should use:

- White or translucent white backgrounds.
- 1px borders with low-contrast slate or teal tint.
- Radius around 18-24px for major data cards.
- Radius around 12-16px for nested metric tiles and buttons.
- Soft shadows only: no heavy elevation.
- Optional pale gradient wash from teal/cyan to white for operational panels.

Do not nest decorative cards inside decorative cards. Nested tiles are allowed
only when they contain a specific metric or control.

## Typography

- Page title: 30-34px, 800-900 weight, tight but readable.
- Section/card title: 18-22px, 800 weight.
- Metric numbers: 24-32px, monospaced, 800-900 weight.
- Small labels: 11-13px, 700 weight, muted slate.
- Body/helper text: 12-14px, medium weight.
- Avoid negative letter spacing and viewport-scaled font sizes.
- Use monospaced numerals for latency, percentages, counts, timestamps, and status readouts.

## Controls

- Segmented controls use rounded pill containers with a soft white surface.
- Active segment uses a teal/cyan fill with a thin teal border.
- Icon buttons are square, compact, rounded 12-14px, and lightly elevated.
- Status pills use uppercase short labels such as `OPERATIONAL`, `DEGRADED`, `SYNCING`.
- Badges/tags should be compact, high-contrast enough to scan, and semantically colored.
- Hover states should lift by 1px or subtly increase the border/accent contrast.

## Cards

Metric summary cards should include:

- A small muted label.
- A large numeric value.
- One supporting line.
- One semantic icon chip on the right.

Operational group cards should include:

- Provider/avatar block.
- Group name and compact provider/model tags.
- Status badge in the top-right.
- Two compact latency tiles when applicable.
- A large availability panel with progress bar.
- A compact row of supporting metrics.
- A recent activity strip with tiny success/error bars.

## Data Visualization

- Use micro visualizations when they improve scan speed:
  - activity bars,
  - small progress bars,
  - sparkline-like strips,
  - compact status markers.
- Keep charts light and embedded; avoid large chart blocks unless the page is primarily analytical.
- Encode success/warning/error consistently across all pages.

## Sidebar And Header

- Header is glassy, fixed, and quiet, with centered pill navigation where applicable.
- Sidebar is functional and compact, with clear grouped navigation.
- Active sidebar item uses a soft blue/teal background and strong accent text/icon.
- Keep brand/logo visible but not oversized.

## Base Component Improvement Direction

When improving base components for `web/classic`, align them to this style:

- `CardPro`: support soft glass card variants, compact header density, and metric/tile sections.
- Tables: keep dense data, but use lighter borders, rounded outer panels, and calmer toolbar controls.
- Filters/toolbars: use segmented controls, icon buttons, and compact pills.
- Status components: standardize `operational`, `degraded`, `syncing`, `offline`, `healthy`, `warning`, `critical`.
- Empty/loading states: prefer compact skeletons or subtle empty states; avoid oversized illustrations in operational pages.
- Modals/drawers: use the same soft surfaces, compact spacing, and clear status badges.

## Do Not Do

- Do not introduce a new unrelated visual language for each console page.
- Do not make pages dominated by purple, beige, dark slate, or heavy gradients.
- Do not use large marketing hero layouts inside the console.
- Do not make cards so padded that operational data falls below the fold unnecessarily.
- Do not hide core metrics behind hover-only interactions.
- Do not use decorative effects that compete with status information.
