# Homepage Redesign Brief

This brief defines the target direction for the public homepage of the classic
theme. It exists to stop iterative visual drift and give implementation a clear
standard before writing more UI code.

## Goal

Rebuild the homepage as a credible business landing page for a Codex and Claude
Code API gateway.

The page should convince a technical buyer in 10 seconds that the service is:

- stable for long-running AI coding sessions;
- able to switch channels without user-side config changes;
- faster and cheaper than relying on one upstream account;
- operated like a real API platform, not a thin proxy script.

The target audience is developers, small AI teams, automation-heavy users, and
API resellers who care about uptime, cost, token routing, and billing clarity.

## Current Problems

The current homepage fails because it feels like a narrow dashboard excerpt
placed on a marketing page.

- The first viewport is visually weak: small content width, weak hierarchy, too
  much empty page below the hero, and no clear enterprise-level confidence.
- The hero panel looks like an internal admin widget, not a public proof surface.
- The reliability chart feels decorative and underpowered; it does not clearly
  explain recent success rate, latency, request volume, or failover events.
- Section cards are repetitive and generic. They do not show real Codex /
  Claude Code usage scenarios.
- The page does not create the feeling that the product is actively used and
  professionally operated.

## Positioning

Primary headline direction:

> Codex / Claude Code 稳定高速中转站

Supporting message:

> 多渠道无感切换，低价接入主流编程模型

Proof message:

> 公开展示脱敏后的近期成功率、延迟、请求量和保护事件，让稳定性可验证。

Do not overclaim unsupported numbers such as “thousands of customers” unless the
number comes from real aggregate data. Use real telemetry when available.

## Visual Direction

Use a light, premium, enterprise SaaS style. It should feel mature, efficient,
and operationally trustworthy.

Recommended tone:

- calm white and cool-gray base;
- teal/cyan identity accents;
- blue for data and active routes;
- green for healthy/success state;
- amber/red only for warning and failure semantics.

Avoid:

- dark hero blocks that occupy only the top and fade awkwardly into white;
- big purple/blue gradients;
- decorative blobs, orbs, bokeh, and random abstract art;
- generic card grids with equal-looking sections;
- tiny dashboard UI copied directly into the hero;
- oversized whitespace after the CTA.

The homepage may be more promotional than `docs/classic-visual-style.md`, but it
must still inherit the classic theme’s operational clarity, quiet data surfaces,
thin borders, and readable metrics.

## Layout Principles

Desktop width:

- Use a wider content canvas: `min(1360px, calc(100vw - 48px))`.
- Hero should use the available width; avoid a centered narrow column.
- First viewport should contain headline, CTA, proof metrics, and a substantial
  product visual.

Section rhythm:

- Use fewer, stronger sections.
- Each section must have a different job: positioning, proof, scenarios,
  routing, cost, integration, ecosystem, conversion.
- Avoid repeating the same four-card pattern more than once.

Cards:

- Use cards for specific repeated items or product panels only.
- Do not nest large decorative cards inside other decorative cards.
- Keep card radius around 8-12px for homepage implementation.
- Use strong internal alignment and stable dimensions.

Mobile:

- Hero becomes single-column.
- CTA remains visible in the first screen.
- Product visual becomes a compact status/proof panel, not a giant image.
- Charts must not overflow horizontally.

## Page Structure

### 1. Header

Purpose: establish brand and provide fast navigation.

Content:

- logo and system name;
- navigation: 首页, 控制台, 模型广场, 文档, 关于;
- right side: status or login/control action.

Design:

- glassy white header with subtle border;
- active nav pill with teal fill;
- compact height, no heavy shadow.

### 2. Hero

Purpose: instantly explain the product and create trust.

Required content:

- brand pill: `Codex 与 Claude Code 高稳定中转站`;
- headline: `Codex / Claude Code 稳定高速中转站`;
- subheadline: `多渠道无感切换，低价接入主流编程模型`;
- paragraph explaining unified API entry, health routing, cost control, and
  compatibility with Codex, Claude Code, OpenAI SDK;
- primary CTA: 获取密钥;
- secondary CTA: 查看文档;
- three proof metrics: success rate, latency, available channel pool or recent
  requests.

Design:

- large hero typography; Chinese headline should not break awkwardly;
- left side is clear marketing copy;
- right side is a polished product proof panel, not an admin detail panel;
- hero background should include subtle operational context: route lines,
  terminal strip, metric bands, or request-flow traces.

Hero product panel should show:

- request enters from Codex / Claude Code;
- gateway routes by health/cost/group;
- upstream channels have healthy/degraded/standby states;
- recent aggregate success/latency/protection data;
- Base URL quick setup.

Do not show too much raw configuration in the hero. Base URL can appear, but it
should not dominate the visual.

### 3. Reliability Proof

Purpose: make “stable” measurable.

Required data:

- recent success rate;
- average or P95 latency;
- request volume;
- protected events: rate limit, timeout, 5xx, stream error;
- last updated time;
- 7/30 day view if practical.

Data policy:

- Use public anonymized aggregate data only.
- Do not expose channel names, group names, model names, request IDs, users,
  token names, or error reasons.
- Empty data state should say `等待真实请求数据`, not fake numbers.

Design:

- This should be the strongest section after hero.
- Use a large horizontal proof panel, not tiny toy charts.
- Combine metric tiles with a clear chart.
- Show success rate as a line or area curve.
- Show request volume as bars or heat cells.
- Show protected events as small semantic markers.

### 4. Scenario Section

Purpose: connect technical features to real customer pain.

Required scenarios:

- 长时间编码会话: Codex / Claude Code keeps generating, testing, and editing;
- 上游限流自动转移: 429, 5xx, timeout, stream errors trigger protection;
- 团队共享与成本控制: token, quota, group ratio, logs, subscription;
- 脚本与 IDE 自动化: stable Base URL for automation and IDE clients.

Design:

- Use richer scenario blocks instead of generic feature cards.
- Each block should include a short title, one sentence, a small operational
  visual, and one measurable benefit.
- Do not use stock illustrations.

### 5. Routing And Cost

Purpose: explain why the gateway is better than a single upstream key.

Required content:

- one Base URL and one key for the user side;
- routing by model, group ratio, priority, health, and cooldown;
- fallback when upstream has rate limits or failure;
- cost optimization through low-price groups and billing logs.

Design:

- Use a process strip or flow diagram.
- Keep each step short.
- Use icons only when they improve scanning.

### 6. Compatibility

Purpose: reduce adoption friction.

Required tools:

- Codex CLI;
- Claude Code;
- OpenAI SDK;
- Cursor;
- Cherry Studio;
- Lobe Chat;
- OpenCode;
- ChatBox.

Design:

- Compact integration grid.
- Show “replace Base URL and key” as the core action.
- Do not over-explain basic setup in visible copy.

### 7. Provider Ecosystem

Purpose: show breadth without becoming logo soup.

Required providers:

- OpenAI, Claude, Gemini, DeepSeek, Qwen, Grok, Azure AI, and other available
  icons.

Design:

- Group logos into a calm grid.
- Use consistent icon size.
- Include `40+` as a channel/provider ecosystem signal if accurate for the
  product.

### 8. Operational Console

Purpose: prove this is a real managed gateway.

Required capabilities:

- token and quota;
- group ratio;
- channel monitor;
- usage logs;
- subscription billing;
- model pricing.

Design:

- Show as capability chips plus a compact product panel.
- Do not include a huge console screenshot unless it is clean and legible.

### 9. Final CTA

Purpose: convert.

Required copy:

- `把 Codex 和 Claude Code 切到更稳的 API 入口`;
- primary CTA: `进入控制台`;
- secondary CTA: `查看接入文档`.

Design:

- Full-width band with clear contrast.
- Avoid oversized empty bottom space.

## Data Interface Requirements

Homepage telemetry should use a dedicated public API, not the admin status
monitor response.

Endpoint:

```text
GET /api/public/home/status?days=30
```

Allowed windows:

- `7`;
- `30`;
- default: `30`.

Response shape:

```json
{
  "summary": {
    "days": 30,
    "success_rate": 99.98,
    "avg_latency_ms": 184,
    "requests": 123456,
    "enabled_channels": 42,
    "healthy_channels": 39,
    "protected_events": 128
  },
  "daily": [
    {
      "date": "2026-05-17",
      "requests": 4200,
      "success_rate": 99.97,
      "avg_latency_ms": 190,
      "protected_events": 8
    }
  ],
  "updated_at": 1778947200,
  "partial": false
}
```

Security:

- No channel names.
- No group names.
- No model names.
- No request IDs.
- No user IDs.
- No token names.
- No IP addresses.
- No raw error reasons.

Performance:

- Cache response for 60-300 seconds.
- Use refresh locking to avoid duplicate expensive queries.
- Return last known cached response or empty partial state on query failure.

## Copy Guidelines

Prefer concrete, operational copy:

- `按请求最终结果聚合`
- `限流、超时与异常保护`
- `自动避开冷却与异常线路`
- `替换 Base URL 与 Key 即可迁移`

Avoid vague copy:

- `极致体验`
- `无限可能`
- `重新定义 AI`
- `业界领先`
- `超强能力`

Avoid unsupported claims:

- `最多用户选择`
- `全网最低价`
- `永不宕机`
- `100% 可用`

## Acceptance Criteria

Desktop visual:

- First viewport looks like a business-grade product homepage.
- Headline is large and readable.
- CTA is visible without scrolling.
- Hero product visual supports the message instead of looking like admin clutter.
- There is no awkward hard split between dark and light regions.

Data proof:

- Reliability section clearly shows success rate, latency, request volume, and
  protection events.
- Empty state is credible and does not fake telemetry.
- Public data is anonymized.

Responsiveness:

- No horizontal overflow at 390px, 768px, 1280px, 1440px.
- Text never overlaps charts or cards.
- CTA buttons fit their containers.
- Provider grid remains compact.

Engineering:

- New homepage text is internationalized.
- `bunx eslint src/pages/Home/index.jsx --cache` passes.
- `git diff --check` passes.
- Backend public status tests cover empty data, final request outcome, protected
  events, and no sensitive fields in DTO.

## Implementation Notes

The previous visual attempts should not be treated as the design baseline.
Use this brief as the baseline instead.

When implementing, replace the current homepage sections rather than layering
more override CSS on top of the existing block. The current CSS already has too
much accumulated override weight.

Recommended implementation approach:

1. Keep the public status API and DTO.
2. Rebuild `web/classic/src/pages/Home/index.jsx` around the section structure
   above.
3. Replace the `ct-home` CSS block cleanly instead of appending another override
   block.
4. Verify screenshots before handing off.
