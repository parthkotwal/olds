# Olds — Cross-Topic News Connection Engine

## What This Project Is

A personalized news reader that surfaces non-obvious connections between stories across topic categories in real time. Not "more articles like this" — but "this crime story in Jakarta connects to the South China Sea piece you read yesterday." The feed learns from how you read, not what you explicitly rate.

The name is "Olds" — a play on "news."

## Critical Context: Developer Experience Level

**This is my first Go project.** I am experienced with Python and JavaScript/React, but I am learning Go through building this. When working on Go code:

- Explain Go idioms and patterns as you introduce them (error handling conventions, struct design, interface usage, goroutine patterns)
- Do not assume familiarity with Go's standard library — call out which packages you're using and why
- When there's a Go-specific way to do something that differs from Python/JS, flag it explicitly
- Prefer idiomatic Go over clever Go — I need to understand and explain this code on my resume
- If I ask you to do something that has a "Go way" vs a "Python way," show me the Go way and explain the difference

## Planning Protocol

**Before writing large sections of code or changes, plan first.** Follow this process:

1. **Restate the task** — Confirm what you understand I'm asking for in one or two sentences
2. **Identify affected files** — List which files will be created or modified
3. **Outline your approach** — Describe the steps you'll take and any design decisions involved
4. **Flag risks or open questions** — If anything is ambiguous, underspecified, or could go two ways, ask before proceeding
5. Wait for confirmation for non-trivial changes; proceed automatically for clearly scoped tasks

For small, unambiguous changes (typo fixes, renaming a variable), you don't need to do this.

## Architecture

Three services in a monorepo:

```
olds/
├── backend/          # Go (Gin) — feed, behavior tracking, graph, WebSockets
├── ml-service/       # Python (FastAPI) — entity extraction, embeddings
├── frontend/         # React + Tailwind — feed UI, real-time connection sidebar
├── docker-compose.yml
├── CLAUDE.md
└── README.md
```

### Backend (Go + Gin)
- REST API serving the personalized article feed
- User behavior tracking (dwell time, scroll depth, re-opens) — all implicit signals, no explicit ratings
- In-memory article graph with entity-aware edge weighting (upgrade path to Neo4j if time allows)
- WebSocket server pushing cross-topic connection updates to the frontend in real time
- Graph traversal triggered on article open: find cross-topic neighbors above a similarity threshold

### ML Microservice (Python + FastAPI)
- Receives raw article text from the Go backend
- Extracts named entities (people, places, organizations, events) using spaCy or similar
- Generates sentence-transformer embeddings (pretrained model — do not train anything)
- Returns entity list + vector to the Go backend for graph storage

### Frontend (React + Tailwind)
- Personalized feed view seeded by interest categories (global affairs, crime, disasters, sports)
- Connection sidebar: when reading an article, shows cross-topic related stories in real time via WebSocket
- Feed decay: stories without new developments naturally lose prominence — no manual curation needed

### Data Flow
1. **Ingestion:** News article arrives via API (NewsAPI / Guardian API) → Python extracts entities and embeds text → Go stores article with vector + entity list → Go computes edge weights to existing articles → article enters the graph
2. **Reading:** User opens article → Go traverses graph in real time → finds cross-topic neighbors above threshold → pushes results to frontend over WebSocket

## Tech Stack

| Layer           | Technology                         | Notes                                                        |
|-----------------|------------------------------------|--------------------------------------------------------------|
| Backend         | Go (Gin)                           | First Go project — prioritize idiomatic, readable code       |
| ML Microservice | Python (FastAPI)                   | Entity extraction + embeddings; ML stays in Python           |
| Frontend        | React + Tailwind                   | Standard stack; use functional components + hooks             |
| Graph           | In-memory Go structure (Neo4j stretch goal) | Core of the project                               |
| Real-time       | WebSockets (Go)                    | Connection graph updates live as user reads                   |
| News Ingestion  | NewsAPI / The Guardian API         | Solved problem — use APIs, do not scrape                     |
| Deployment      | Docker + docker-compose            | All three services containerized                              |

## Build Order (Follow This Sequence)

Build vertically, not horizontally. Get a thin slice working end-to-end before expanding.

1. **Go backend scaffold** — Gin server, single `/articles` endpoint, Docker container that builds and runs
2. **News ingestion** — Fetch articles from NewsAPI or Guardian API, store in memory or a simple data structure
3. **Python ML service** — FastAPI service that accepts article text, returns entities + embedding vector. Docker container.
4. **Wire backend → ML service** — When a new article is ingested, Go calls Python, stores the returned vector and entities
5. **In-memory graph** — Build the article graph in Go. Compute edge weights from entity overlap + cosine similarity of embeddings
6. **Graph traversal API** — Endpoint: given an article ID, return top N cross-topic connected articles above a similarity threshold
7. **React frontend** — Feed view consuming the articles API. Basic layout, article cards, category filtering
8. **WebSocket integration** — When user opens an article, frontend opens a WebSocket; Go traverses graph and pushes connections in real time
9. **Implicit behavior tracking** — Track dwell time, scroll depth, re-opens. Use these signals to re-weight the feed
10. **Feed decay** — Stories with no new developments lose prominence over time

## Scope Guardrails — Read This Before Every Task

- **DO NOT** build a scraper. Use NewsAPI or Guardian API for ingestion.
- **DO NOT** train any ML models. Use a pretrained sentence-transformers model.
- **DO NOT** build a user authentication system. Assume a single user for now.
- **DO NOT** use a database unless explicitly told to. Start with in-memory data structures in Go.
- **DO NOT** prematurely optimize. Get it working, then make it fast.
- **DO** keep the three services cleanly separated. They communicate over HTTP and WebSocket only.
- **DO** write tests. At minimum, test the graph traversal logic and the ML service entity extraction.
- **DO** use Docker for all services from day one. If it doesn't run in Docker, it doesn't count.

## Code Conventions

### Go
- Use standard Go project layout (`cmd/`, `internal/`, `pkg/` as appropriate)
- Format with `gofmt` / `goimports`
- Error handling: always handle errors explicitly — no underscore-ignoring unless there's a comment explaining why
- Comments: exported functions get doc comments; internal logic gets inline comments explaining the "why"

### Python
- Type hints on all function signatures
- Use Pydantic models for request/response schemas
- `black` for formatting, `ruff` for linting

### React
- Functional components only, hooks for state
- Tailwind for styling — no CSS files unless absolutely necessary
- Component files named in PascalCase (`ArticleCard.jsx`, `ConnectionSidebar.jsx`)

### General
- Commit messages: imperative mood, concise (`Add article ingestion endpoint`, not `Added some stuff`)
- Every PR-worthy chunk of work should leave the project in a runnable state

## Skills (Install via Plugin Marketplace)

Install the Anthropic skills plugin marketplace, then install these relevant skills:

```
/plugin marketplace add anthropics/skills
/plugin install frontend-design@anthropic-agent-skills
/plugin install webapp-testing@anthropic-agent-skills
```

### When to use each skill:
- **frontend-design** — Use when building any React component, page, or UI element for the frontend. This produces polished, production-grade interfaces. Invoke it whenever creating or restyling components in `frontend/`.
- **webapp-testing** — Use when testing the frontend or any web-facing part of the application with Playwright. Invoke it when I ask to test the UI or verify that a feature works end-to-end in the browser.

## What This Demonstrates on a Resume

This matters because it shapes what we emphasize in the code:

- Polyglot microservice architecture (Go + Python)
- Graph algorithms (real-time traversal, edge weighting by entity overlap + semantic similarity)
- NLP pipeline (entity extraction, sentence-transformer embeddings)
- Real-time serving (WebSockets)
- Full-stack (React frontend through to ML inference layer)
- Implicit feedback system (behavior-driven personalization, no explicit ratings)
- Docker-based service orchestration