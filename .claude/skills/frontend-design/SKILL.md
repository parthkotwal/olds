---
name: frontend-design
description: Create distinctive, production-grade frontend interfaces with high design quality. Use this skill when the user asks to build web components, pages, artifacts, posters, or applications (examples include websites, landing pages, dashboards, React components, HTML/CSS layouts, or when styling/beautifying any web UI). Generates creative, polished code and UI design that avoids generic AI aesthetics.
---

This skill guides creation of distinctive, production-grade frontend interfaces that avoid generic "AI slop" aesthetics. Implement real working code with exceptional attention to aesthetic details and creative choices.

The user provides frontend requirements: a component, page, application, or interface to build. They may include context about the purpose, audience, or technical constraints.

## Design Thinking

For this project (Olds), the aesthetic direction is already committed: **editorial broadsheet newspaper meets modern web**. The feeling of a physical newspaper — dense, typographic, structured, inviting lateral reading — expressed through clean, contemporary frontend craft. Nostalgic warmth without skeuomorphic cosplay. The design should feel *still*, like a printed page that occasionally, quietly updates.

Before coding, confirm:
- **Purpose**: What problem does this interface solve? Who uses it?
- **Tone**: Editorial/broadsheet. Quiet confidence of print design — big type, clear hierarchy, no decoration for decoration's sake.
- **Constraints**: Technical requirements (framework, performance, accessibility).
- **Differentiation**: The serif/sans typographic contrast is the core visual identity. The connection sidebar as marginalia is the design signature.

**CRITICAL**: Execute this direction with precision. The design should feel like a newspaper that loads in a browser, not a web app that happens to show news.

Then implement working code (HTML/CSS/JS, React, Vue, etc.) that is:
- Production-grade and functional
- Visually striking and memorable
- Cohesive with a clear aesthetic point-of-view
- Meticulously refined in every detail

## Frontend Aesthetics Guidelines

Focus on:
- **Typography**: Two-family system — the serif/sans contrast is the core visual identity, do not deviate. **Headlines**: Editorial serif — Playfair Display, Lora, or Libre Baskerville. Generous sizing, confident weight. **Body text**: Clean sans-serif — Inter, Source Sans 3, or similar. Highly readable at smaller sizes, tight line-height. **Section labels**: Small-caps sans-serif, letterspaced (e.g., `WORLD AFFAIRS · CRIME · SCIENCE`). **Datelines/metadata**: Smaller weight, slightly muted color — classic newspaper dateline feel.
- **Color & Theme**: Minimal. Almost monochrome. Use CSS variables for consistency. **Background**: Warm off-white (`#FAF8F5` or `#F5F1EB` range) — aged newsprint, not stark white. **Text**: Near-black (`#1A1A1A` or `#2D2D2D`). **Secondary text/dividers**: Muted gray (`#8A8A8A` range). **Single accent**: Muted ink-red (`#C0392B` range) OR deep navy (`#2C3E6B` range) — used only for hover/focus states, category tags, and breaking/live indicators. Pick one accent, use it sparingly. No bright multi-color category systems — categories are differentiated by label text, not color coding.
- **Motion**: Restrained. The page should feel still. **Hover**: Subtle opacity change or underline on headlines — no movement, no scale transforms. **Connection sidebar updates**: Gentle fade-in (200–300ms opacity transition) when new connections arrive via WebSocket — no slide, no bounce. **Page transitions**: Simple fade or instant. No parallax, no bouncing cards, no animated gradients.
- **Spatial Composition**: Columnar, not single-column infinite scroll. Asymmetric grid for the feed — lead story gets prominent placement, secondary stories tile in 2–3 columns below. Varying story sizes create visual hierarchy and encourage lateral reading, mirroring the newspaper page. Generous whitespace between sections, tight spacing within story blocks.
- **Backgrounds & Visual Details**: Warm off-white base. Optional: very subtle paper texture via CSS `background-image` with a low-opacity noise or grain pattern — barely perceptible warmth, not a gimmick. **No card shadows** — stories separated by whitespace and thin `1px solid` horizontal rules in light gray. **No rounded corners** on content containers (square, or 2px max if necessary).

NEVER use: dark mode as default, card-based layouts with heavy box-shadows, bright multi-color category systems, rounded pill buttons, gradient CTAs, hero sections, heavy imagery leading over text, or Inter + purple gradient + white background (the generic AI aesthetic). Newspapers are light — don't make this feel like a web app.

**IMPORTANT**: Match implementation complexity to the aesthetic vision. This is a refined, restrained design — elegance comes from precise typography, correct spacing, and quiet details. Don't add motion or decoration beyond what is specified in this brief.

Remember: The goal is a newspaper that occasionally, quietly updates. Every choice should serve that feeling.

## Layout

- **Feed page**: Asymmetric grid. Lead story gets prominent placement (large headline, optional image). Secondary stories tile below in 2–3 columns. Varying story sizes create visual hierarchy and invite the eye to wander.
- **Article view**: Single readable column for article text, max-width ~680px. Connection sidebar as a narrow right column.
- **Connection sidebar**: The design signature — feels like marginalia. Thin vertical rule separating it from the article. Each connection shows: headline (serif), source, and a small "Connected by: [entity/theme]" label. Gentle fade-in when new connections arrive via WebSocket. Not cards. Not flashy.

## Responsive Behavior

- **Desktop**: Full multi-column layout with sidebar.
- **Tablet**: 2-column feed, sidebar collapses below article or into a drawer.
- **Mobile**: Single-column feed, connections as a bottom sheet or expandable section below the article.
