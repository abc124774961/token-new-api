# Homepage UI Baseline V3

This document locks the accepted visual baseline for the public homepage
redesign. Future homepage design and implementation work should use this
version as the starting point unless the direction is explicitly changed.

## Baseline Artifacts

- Design image: `docs/homepage-ui-baseline-v3.png`
- Image generation prompt: `docs/homepage-ui-baseline-v3.prompt.txt`
- Original generated output: `output/imagegen/homepage-ui-concept-v3-spacious.png`

## Accepted Direction

Use the third generated version as the base direction.

Key requirements:

- Overall layout should feel spacious and premium, not crowded.
- Major sections should roughly map to one desktop screen each.
- Use larger visual anchors, more breathing room, and fewer small widgets.
- Keep the top navigation style from the first version: clean glassy white
  business header, steady horizontal navigation, clear brand, compact right
  status/action.
- The public homepage should feel like a professional API platform for Codex
  and Claude Code users, not an internal admin dashboard.
- The real operations curve must support group switching because each group can
  have different channel access, latency, request volume, and protection-event
  behavior.

## Implementation Implications

- Prefer large full-width sections over dense card grids.
- Keep copy short and let diagrams, charts, and routing visuals carry the
  message.
- Build the homepage rhythm around section-level scroll experiences:
  sticky/glassy header, pinned chart feel, layered route lines, and calm section
  transitions.
- Use the operations curve as a primary trust surface, not a small decorative
  widget.
- Preserve the light enterprise SaaS palette: cool white/gray base, teal/cyan
  identity accents, blue data accents, green healthy status, amber cooldown or
  protection status, red only for failure semantics.
