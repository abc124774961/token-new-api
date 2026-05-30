# Classic Visual Style Guide

This document records the target visual language for the `web/classic` theme.
Use it as the shared baseline for dashboard, console, and base component
improvements.

## Reference

- Light reference screen: `web/classic/src/components/dashboard/channel-status/ChannelStatusMonitor.jsx`
- Light reference route: `/console/channel-status`
- Dark reference screen: `web/classic/src/components/dashboard/index.jsx`
- Dark reference route: `/console`
- Theme intent: precise AI operations console, calm monitoring surface, dense but breathable data cards.

The left sidebar and top shell are global console frame components. They should
stay visually compatible with the page, but they are not part of a page-specific
dashboard card style. When documenting or implementing page content, judge the
main content canvas, cards, metrics, controls, charts, and empty states first.

## Overall Direction

The classic theme should feel like a modern AI control room:

- Clean, high-signal surfaces with enough contrast for long operational use.
- A restrained AI operations language: precise, quiet, technical, and scan-first.
- Soft teal/cyan identity accents in light mode; muted graphite, blue-gray, and
  low-saturation semantic accents in dark mode.
- Dense operational information without table-heavy visual fatigue.
- Rounded but crisp cards, compact controls, and low visual noise.
- Numeric data should be prominent, monospaced, and easy to scan.
- Icons should be functional and quiet, with selective color to improve scanning.
- Important text may use semantic color, but large blocks of text should remain neutral.

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

Use these as the visual basis rather than hard requirements for every element.

### Common Semantics

- Success / healthy / remaining: green.
- Usage / active throughput: teal or cyan.
- Resource cost / warning: amber.
- Performance / speed / API: blue or indigo.
- Risk / high usage / critical: red.
- Neutral / disabled / unavailable: slate or blue-gray.
- Numeric values may inherit the semantic color of their metric icon.

Do not color every label. Prefer color on:

- primary metric values,
- active states,
- key status labels,
- progress bars,
- icon chips,
- small count/status badges.

### Light Mode

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

### Dark Mode

The dark console dashboard uses a graphite / dark blue-gray foundation. It
should feel stable and professional, not neon, glowing, or game-like.

- Page background: deep graphite blue, around `#121926`.
- Header/sidebar-compatible shell: dark blue-gray, around `#182131`.
- Major panels/cards: slightly lifted graphite, around `#1f2937`.
- Nested metric tiles and controls: deeper blue-gray, around `#182131`.
- Hover/selected neutral fill: `#273244` to `#3d4a5f`.
- Main text: near-white, around `#f8fafc`.
- Secondary text: cool slate, around `#cbd5e1`.
- Subtle text: muted slate, around `#94a3b8`.
- Teal/cyan emphasis: low-saturation values such as `#7cc7bd` / `#7cc7d8`.
- Blue emphasis: `#93b7e8`.
- Green emphasis: `#7dcaa6`.
- Amber emphasis: `#dfb467`.
- Violet emphasis: `#baa5e8`.
- Orange emphasis: `#dfa17a`.
- Danger emphasis: muted red such as `#e58f8f`.

Dark mode should avoid:

- bright cyan as the dominant page color,
- pure black panels,
- strong white cards,
- high-glow outlines,
- heavy shadows,
- large gradients,
- one-note teal/green pages.

## Surfaces

Cards and panels should use these common rules:

- Radius around 18-24px for major data cards.
- Radius around 12-16px for nested metric tiles and buttons.
- Avoid heavy elevation and floating-card drama.
- Do not nest decorative cards inside decorative cards.
- Nested tiles are allowed only when they contain a specific metric or control.

Light mode surfaces:

- White or translucent white backgrounds.
- 1px borders with low-contrast slate or teal tint.
- Soft shadows only.
- Optional pale gradient wash from teal/cyan to white for operational panels.

Dark mode surfaces:

- Prefer pure color blocks over gradients.
- Use no shadow or nearly invisible shadow.
- Prefer no visible border on primary panels; separate depth by fill color.
- Use `#1f2937` for major panels and `#182131` for nested metric tiles.
- Avoid hover lift; use a subtle fill change instead.

## Typography

- Page title: 30-34px, 800-900 weight, tight but readable.
- Section/card title: 18-22px, 800 weight.
- Metric numbers: 24-32px, monospaced, 800-900 weight.
- Small labels: 11-13px, 700 weight, muted slate.
- Body/helper text: 12-14px, medium weight.
- Avoid negative letter spacing and viewport-scaled font sizes.
- Use monospaced numerals for latency, percentages, counts, timestamps, and status readouts.
- In dense dashboards, color only the values or short emphasis fragments, not full paragraphs.

## Controls

- Segmented controls use rounded pill containers with compact spacing.
- Icon buttons are square, compact, rounded 12-14px, and lightly elevated.
- Status pills use uppercase short labels such as `OPERATIONAL`, `DEGRADED`, `SYNCING`.
- Badges/tags should be compact, high-contrast enough to scan, and semantically colored.
- Hover states should subtly increase fill or accent contrast.

Light mode controls:

- Active segment uses a teal/cyan fill with a thin teal border.
- Icon buttons can use soft white surfaces and very soft elevation.
- Hover states may lift by 1px.

Dark mode controls:

- Active segment uses a muted blue-gray fill such as `#3d4a5f`.
- Primary buttons use a dark-theme main fill, not bright cyan.
- Hover uses `#4b5870` or similar muted blue-gray.
- Avoid glow, heavy borders, and bright gradients.

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

For dark dashboard cards:

- Use colored icon chips to create scan anchors.
- Use semantic color on primary values.
- Keep labels and secondary copy neutral.
- Keep card backgrounds flat and quiet.
- Use compact card titles with small duotone or filled icons.

## Data Visualization

- Use micro visualizations when they improve scan speed:
  - activity bars,
  - small progress bars,
  - sparkline-like strips,
  - compact status markers.
- Keep charts light and embedded; avoid large chart blocks unless the page is primarily analytical.
- Encode success/warning/error consistently across all pages.
- In dark mode, chart series should use muted blue-gray and low-saturation semantic colors.
- Avoid neon chart palettes and strong grid lines.

Progress bars should encode state by ratio when the ratio has operational meaning:

- Low/healthy: green.
- Medium/active: cyan or blue.
- High/warning: amber.
- Critical/high-risk: red.
- Empty/unavailable: neutral gray.

Example for subscription or usage progress:

- `<40%`: healthy green.
- `40%-69%`: cyan/blue notice.
- `70%-89%`: amber warning.
- `>=90%`: red danger.
- inactive/unavailable: neutral slate.

## Sidebar And Header

- Header is fixed and quiet, with centered pill navigation where applicable.
- Sidebar is functional and compact, with clear grouped navigation.
- The global sidebar is not the reference for a page's internal card language.
- Page content should align with the global shell without copying sidebar styling into every card.
- Keep brand/logo visible but not oversized.

Light shell:

- Header may be glassy with subtle blur.
- Active sidebar item may use a soft blue/teal background and strong accent text/icon.

Dark shell:

- Header and sidebar should use dark blue-gray surfaces.
- Active navigation uses muted blue-gray, not bright cyan.
- Avoid strong borders and heavy shadows in the shell.

## Base Component Improvement Direction

When improving base components for `web/classic`, align them to this style:

- `CardPro`: support soft glass card variants, compact header density, and metric/tile sections.
- Tables: keep dense data, but use lighter borders, rounded outer panels, and calmer toolbar controls.
- Filters/toolbars: use segmented controls, icon buttons, and compact pills.
- Status components: standardize `operational`, `degraded`, `syncing`, `offline`, `healthy`, `warning`, `critical`.
- Empty/loading states: prefer compact skeletons or subtle empty states; avoid oversized illustrations in operational pages.
- Modals/drawers: use the same soft surfaces, compact spacing, and clear status badges.

## Dark Dashboard Pattern

Use this pattern for dense dark console dashboards such as `/console`:

- Main canvas: deep graphite page background.
- Hero/summary panels: flat `#1f2937` surfaces, 24-26px radius.
- Nested metric tiles: `#182131`, no border, no shadow.
- Buttons: muted blue-gray fill, high-contrast white text.
- Icons: duotone or filled style with low-saturation semantic colors.
- Values: monospaced and semantically colored.
- Labels: neutral cool slate.
- Progress: state color based on ratio, with a neutral track.
- Charts: muted palette, low grid contrast, no neon tooltip borders.
- Empty states: compact icon + title + helper text; no large illustrations.

The goal is a calm control room, not a decorative dark landing page.

## Do Not Do

- Do not introduce a new unrelated visual language for each console page.
- Do not make pages dominated by purple, beige, bright cyan, or heavy gradients.
- Do not use dark mode as a pure black/high-glow theme.
- Do not use large marketing hero layouts inside the console.
- Do not make cards so padded that operational data falls below the fold unnecessarily.
- Do not hide core metrics behind hover-only interactions.
- Do not use decorative effects that compete with status information.
