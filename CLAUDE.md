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
├── backend/          # Go (Gin) — feed, behavior tracking, graph, WebSockets, auth middleware
├── ml-service/       # Python (FastAPI) — entity extraction, embeddings
├── frontend/         # React + Tailwind — feed UI, real-time connection sidebar, Supabase auth (email + Google OAuth)
│   └── DESIGN_BRIEF.md  # Editorial newspaper design language spec
├── docker-compose.yml    # Backend, ML service, frontend, Postgres + pgvector (local dev)
├── .env.example          # Supabase keys, NewsAPI/Guardian keys, DB connection string
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
| Graph           | In-memory Go structure (hydrated from Postgres) | Core of the project; Neo4j is a stretch goal      |
| Database        | PostgreSQL + pgvector              | Local: Docker container. Production: Supabase hosted Postgres |
| Auth            | Supabase Auth (Email + Google OAuth) + JWT | Frontend handles email/password and Google OAuth flows, Go backend verifies JWTs |
| Real-time       | WebSockets (Go)                    | Connection graph updates live as user reads                   |
| News Ingestion  | NewsAPI / The Guardian API         | Solved problem — use APIs, do not scrape                     |
| LLM             | Anthropic or OpenAI API            | Connection explanations only; do not use for summarization or other generic features |
| Deployment      | Railway (services) + Supabase (DB + auth) | Docker containers on Railway; Supabase free tier for data |

## Phase 1 Build Order (Complete)

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

## Current State (Phases 1–14 Complete)

All Phase 1 items plus auth, frontend polish, and stress-testing are built and working end-to-end in Docker:

- **Go backend** is running with Gin, serving the articles feed, handling ingestion, and managing WebSocket connections.
- **Python ML service** extracts entities (spaCy) and generates embeddings (sentence-transformers, `all-MiniLM-L6-v2`) via FastAPI.
- **Ingestion pipeline** fetches articles from NewsAPI/Guardian API, sends them through the ML service, and stores them with entities + vectors.
- **In-memory article graph** in Go computes edge weights from cosine similarity + entity overlap. Graph traversal returns cross-topic connections above a similarity threshold. The graph hydrates from Postgres on startup.
- **PostgreSQL + pgvector** persists articles, entities, embeddings, and behavior signals. Data survives restarts.
- **React frontend** displays the feed with the editorial newspaper design language (serif headlines, warm off-white, columnar layout, thin rules). Connection sidebar shows related stories when reading an article.
- **WebSocket layer** pushes cross-topic connections to the frontend in real time when a user opens an article.
- **Implicit behavior tracking** captures dwell time, scroll depth, and re-opens from the frontend. These signals influence feed ranking.
- **Feed decay** reduces prominence of stories over time when no new developments occur.
- All services are containerized and run via `docker-compose up`.
- **Supabase Auth** handles email/password and Google OAuth login. Go backend verifies JWTs via middleware. Feed ranking and behavior signals are keyed per user.
- **Frontend polish** includes loading states, empty states, graceful feed population, and Playwright tests covering key user flows.
- **Stress-testing** has been run with scheduled ingestion over multiple days. Findings from this phase inform the remaining work.

## Phase 2 Build Order

Continue from where Phase 1 left off. Same principle: each phase should be working and testable before moving to the next.

12. **Authentication via Supabase** ✅ — Complete. Supabase Auth for email/password and Google OAuth. Go backend verifies JWTs. Feed ranking and behavior signals keyed per user.

13. **Frontend polish for portfolio presentation** ✅ — Complete. Loading states, empty states, graceful feed population, micro-interactions reinforcing the editorial design language. Playwright tests covering key user flows.

14. **Stress-test with real usage** ✅ — Complete. Scheduled ingestion running over multiple days. Findings documented and informed the remaining phases.

15. **LLM-powered connection explanations** — When the sidebar shows a cross-topic connection, use an LLM (via API — Anthropic or OpenAI) to generate a 1–2 sentence natural language explanation of *why* two articles are connected. Feed it the entity overlap list and similarity score that already exist in the graph, plus both article titles and summaries. Example output: "Both stories involve Xi Jinping's trade strategy in the South China Sea, but from different angles — one covers military posturing while the other tracks economic sanctions." This replaces the raw "Connected via: entity, entity" approach with something that makes the product demo-ready and adds a genuine LLM integration to the resume. The Go backend calls the LLM API when serving connections (cache results per article pair to avoid redundant calls). Add `LLM_API_KEY` to `.env.example` and environment variable config.

16. **Deploy to Railway + Supabase** — See "Deployment" section below. Deploy all three services to Railway as Docker containers. Point the database connection at Supabase's hosted Postgres. Configure environment variables in Railway's dashboard (including `LLM_API_KEY`). Verify everything works end-to-end at the production URL.

### Post-Deploy Stretch Goals (Not Blocking Launch)

These are nice-to-haves. Only pursue after Phase 16 is live and working at a production URL.

- **Article search** — Add a search bar to the feed page. Full-text search against Postgres (using `tsvector`/`tsquery` or `ILIKE` as a starting point). Low complexity, but not a differentiator — do it if you want the UX convenience.
- **Reading trail visualization** — A page or overlay showing the user's reading path as a visual graph: nodes are articles they've read, edges show connections they followed across topics. This is visually impressive for demos. Keep the visual style consistent with the newspaper aesthetic. Consider a clean timeline-with-branches or a styled small-multiples layout.
- **Article clustering on the feed page** — Instead of a flat ranked list, group connected stories visually: a lead story with 2–3 related stories tucked underneath, like how a newspaper groups related coverage.
- **Graph quality improvements** — Temporal edge weighting, category diversity scoring, and any targeted fixes identified during the stress-test phase.

## Scope Guardrails — Read This Before Every Task

- **DO NOT** build a scraper. Use NewsAPI or Guardian API for ingestion.
- **DO NOT** train any ML models. Use a pretrained sentence-transformers model.
- **DO NOT** build roles or an admin panel. Supabase Auth handles email/password and Google OAuth; Go backend only verifies JWTs.
- **DO NOT** replace the in-memory Go graph with a database query layer. Postgres is for persistence; the Go graph is for real-time traversal. Hydrate the graph from Postgres on startup.
- **DO NOT** add Neo4j unless all Phase 2 items are complete and working.
- **DO NOT** prematurely optimize. Stress-testing (Phase 14) is complete — apply targeted fixes only, not speculative performance work.
- **DO NOT** use the LLM for generic features (article summarization, chatbots, search). It is scoped to connection explanations only.
- **DO** keep the three services cleanly separated. They communicate over HTTP and WebSocket only.
- **DO** write tests. At minimum, test the graph traversal logic, ML service entity extraction, and key frontend flows (Playwright).
- **DO** use Docker for all services. If it doesn't run in Docker, it doesn't count.
- **DO** follow the build order. Each phase should be working and testable before starting the next.
- **DO** use environment variables for all secrets and connection strings. Never hardcode Supabase keys, API keys, or DB credentials.

## Local vs Production Environment

The project uses different infrastructure locally vs in production, but the application code is the same. All differences are handled via environment variables.

| Concern    | Local (docker-compose)                     | Production                                |
|------------|--------------------------------------------|-------------------------------------------|
| Database   | Postgres + pgvector in a Docker container  | Supabase hosted Postgres (pgvector enabled) |
| Auth       | Supabase Auth (same project, dev mode)     | Supabase Auth (same project, prod mode)   |
| Backend    | Docker container on localhost              | Railway (Docker deploy)                    |
| ML Service | Docker container on localhost              | Railway (Docker deploy)                    |
| Frontend   | Docker container or `npm run dev`          | Railway (static build or Docker deploy)    |
| Secrets    | `.env` file (gitignored)                   | Railway environment variables              |

**Required environment variables** (list in `.env.example`, never commit actual values):
- `DATABASE_URL` — Postgres connection string (local Docker or Supabase)
- `SUPABASE_URL` — Supabase project URL (e.g., `https://xxxxx.supabase.co`)
- `SUPABASE_ANON_KEY` — Public anon key (safe for frontend)
- `SUPABASE_JWT_SECRET` — For JWT verification on the Go backend (never expose to frontend)
- `NEWSAPI_KEY` — NewsAPI access key
- `GUARDIAN_API_KEY` — The Guardian Open Platform key
- `LLM_API_KEY` — API key for Anthropic or OpenAI (used for connection explanations)

The Go backend reads `DATABASE_URL` to connect to Postgres — it doesn't know or care whether that's a local Docker container or Supabase's hosted instance. Same for `SUPABASE_JWT_SECRET` — the JWT verification middleware works identically in both environments.

## Deployment (Railway + Supabase)

**Supabase** (set up first, handles data + auth):
- Free tier: 500MB Postgres with pgvector, unlimited auth users
- OAuth providers (Google) configured in Supabase dashboard; email/password auth enabled
- Connection string and keys used by both local dev and production

**Railway** (handles compute):
- Each service (backend, ml-service, frontend) deploys as a separate Railway service from its Dockerfile
- Railway auto-detects Dockerfiles and builds on push
- Environment variables set in Railway dashboard per service
- Internal networking: backend and ml-service communicate via Railway's private network
- Custom domain or Railway-provided URL for the frontend

**Deployment steps** (Phase 16):
1. Create a Railway project with three services, each pointed at the relevant subdirectory
2. Set environment variables in Railway (DATABASE_URL pointing to Supabase, plus all Supabase keys, API keys, and LLM_API_KEY)
3. Deploy and verify each service starts and connects
4. Test the full flow end-to-end at the production URL
5. Set up the scheduled ingestion goroutine to run against production

Do not deploy until all prior phases are working locally. Railway is the last phase, not an ongoing concern during development.

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

## Skills

Skill definitions live in `.claude/skills/`. Use the `Skill` tool to invoke them.

- **frontend-design** (`.claude/skills/frontend-design/SKILL.md`) — Use when building any React component, page, or UI element for the frontend. Invoke it whenever creating or restyling components in `frontend/`.
- **webapp-testing** (`.claude/skills/webapp-testing/SKILL.md`) — Use when testing the frontend or any web-facing part of the application with Playwright. Invoke it when asked to test the UI or verify a feature end-to-end in the browser.

## What This Demonstrates on a Resume

This matters because it shapes what we emphasize in the code:

- Polyglot microservice architecture (Go + Python)
- Graph algorithms (real-time traversal, edge weighting by entity overlap + semantic similarity)
- NLP pipeline (entity extraction, sentence-transformer embeddings)
- Vector similarity search (pgvector)
- Authentication (Supabase Auth with email/password + Google OAuth, JWT verification)
- Real-time serving (WebSockets)
- Full-stack (React frontend through to ML inference layer)
- Implicit feedback system (behavior-driven personalization, no explicit ratings)
- LLM integration (natural language connection explanations generated from graph data)
- Docker-based service orchestration with Postgres persistence
- Cloud deployment (Railway + Supabase)