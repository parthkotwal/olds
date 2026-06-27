// Package main is the entry point for the Olds backend server.
// This file is the "composition root" — the one place in the application
// where concrete types are constructed and wired together. Everything else
// receives its dependencies through constructors, never by reaching for
// global state. This makes each package independently testable.
package main

import (
	"context"
	"log"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/olds/backend/internal/article"
	"github.com/olds/backend/internal/behavior"
	"github.com/olds/backend/internal/db"
	"github.com/olds/backend/internal/embedclient"
	"github.com/olds/backend/internal/graph"
	"github.com/olds/backend/internal/guardian"
	"github.com/olds/backend/internal/handler"
	"github.com/olds/backend/internal/llm"
	"github.com/olds/backend/internal/middleware"
	"github.com/olds/backend/internal/mlclient"
	"github.com/olds/backend/internal/newsapi"
	"github.com/olds/backend/internal/repository"
)

func main() {
	// ── 1. Read and validate required configuration ───────────────────────────
	// Fail fast if required config is missing. It is better to crash at startup
	// with a clear message than to start serving requests and fail silently.
	apiKey := os.Getenv("NEWSAPI_KEY")
	if apiKey == "" {
		log.Fatal("NEWSAPI_KEY environment variable is required")
	}

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		log.Fatal("DATABASE_URL environment variable is required")
	}

	// SUPABASE_JWT_SECRET is the legacy HS256 shared secret. It is optional —
	// newer Supabase projects use ES256 (asymmetric) and no longer need this.
	// We keep it so that any HS256 tokens still in circulation (e.g. from before
	// a key rotation) continue to be accepted until they expire.
	jwtSecret := os.Getenv("SUPABASE_JWT_SECRET")

	// SUPABASE_URL is required to fetch the JWKS (public keys for ES256 tokens).
	supabaseURL := os.Getenv("SUPABASE_URL")
	if supabaseURL == "" {
		log.Fatal("SUPABASE_URL environment variable is required")
	}

	// Fetch ECDSA public keys from Supabase's JWKS endpoint at startup.
	// These keys are used to verify ES256 tokens issued by Supabase Auth.
	// We fail fast here — if we can't reach the JWKS endpoint the server
	// would accept no tokens, so there's no point starting.
	log.Println("fetching JWKS from Supabase...")
	ecKeys, err := middleware.FetchJWKS(supabaseURL)
	if err != nil {
		log.Fatalf("failed to fetch JWKS: %v", err)
	}
	log.Printf("loaded %d ECDSA key(s) from JWKS", len(ecKeys))

	// ── 2. Run database migrations ────────────────────────────────────────────
	// Apply any pending SQL migrations before opening the connection pool.
	// runMigrations is idempotent — safe to call on every startup. It creates
	// the schema_migrations table on first run and is a no-op thereafter.
	//
	// Migrations run before the pool is opened because the pool's type
	// registration (pgvector) requires the vector extension to already exist
	// in the database. Migrations create the extension; pool.Open reads it.
	log.Println("running database migrations...")
	if err := runMigrations(dbURL); err != nil {
		log.Fatalf("database migration failed: %v", err)
	}

	// ── 3. Open the connection pool ───────────────────────────────────────────
	// One pool per process, shared across all goroutines. db.Open also registers
	// the pgvector type so that vector(384) columns can be scanned correctly.
	//
	// context.Background() is the root context for startup work that is not
	// associated with any HTTP request. It is never cancelled.
	pool, err := db.Open(context.Background(), dbURL)
	if err != nil {
		log.Fatalf("database connection failed: %v", err)
	}
	defer pool.Close()
	log.Println("database connected")

	// ── 4. Construct in-memory stores ────────────────────────────────────────
	// These are created empty and then hydrated from Postgres before the server
	// starts serving requests. They remain the runtime read path throughout the
	// application lifetime.
	store := article.NewStore()
	g := graph.NewGraph()
	bs := behavior.NewStore()

	// ── 5. Construct API clients ──────────────────────────────────────────────
	client := newsapi.NewClient(apiKey)

	// Guardian client is optional — if GUARDIAN_KEY is unset, only NewsAPI
	// articles are ingested. When set, Guardian provides full article body text
	// (unlike NewsAPI's free tier which truncates at ~200 characters).
	var guardianClient *guardian.Client
	if guardianKey := os.Getenv("GUARDIAN_KEY"); guardianKey != "" {
		guardianClient = guardian.NewClient(guardianKey)
		log.Println("Guardian API configured — full article text enabled")
	} else {
		log.Println("GUARDIAN_KEY not set — Guardian ingestion disabled")
	}

	// ML client is optional — if ML_SERVICE_URL is unset the backend runs
	// without enrichment. When the full stack is running via docker-compose,
	// ML_SERVICE_URL is always set.
	var mlClient *mlclient.Client
	if mlURL := os.Getenv("ML_SERVICE_URL"); mlURL != "" {
		mlClient = mlclient.NewClient(mlURL)
		log.Printf("ML service configured at %s", mlURL)
	} else {
		log.Println("ML_SERVICE_URL not set — article enrichment disabled")
	}

	// ── 6. Construct repositories ─────────────────────────────────────────────
	// Repositories own the SQL queries for persisting and loading data.
	// They receive the pool and expose domain-typed methods (no raw SQL outside
	// the repository package).
	articleRepo := repository.NewArticleRepository(pool)
	behaviorRepo := repository.NewBehaviorRepository(pool)
	snapshotRepo := repository.NewSnapshotRepository(pool)

	// LLM_API_KEY is used for both connection explanations (OpenAI chat) and
	// embeddings (OpenAI text-embedding-3-small). Both clients share the key.
	var llmClient *llm.Client
	var embedClient *embedclient.Client
	if llmKey := os.Getenv("LLM_API_KEY"); llmKey != "" {
		llmClient = llm.NewClient(llmKey)
		embedClient = embedclient.NewClient(llmKey)
		log.Println("LLM client configured — connection explanations + embeddings enabled")
	} else {
		log.Println("LLM_API_KEY not set — connection explanations and embeddings disabled")
	}

	// ── 7. Construct the handler and register routes ─────────────────────────
	// Handler is constructed before hydration so the HTTP server can start
	// immediately. Article read routes wait briefly for hydration, while
	// /health stays available for Railway startup checks.
	articleHandler := handler.NewArticleHandler(
		store, client, guardianClient, mlClient, embedClient, g, bs,
		articleRepo, behaviorRepo, snapshotRepo, llmClient,
	)

	// ── 9. Register routes ────────────────────────────────────────────────────
	// Set Gin to release mode unless GIN_MODE=debug is explicitly set.
	// Release mode suppresses the startup route table printout and the
	// "running in debug mode" warnings — both are noise in Railway logs.
	if os.Getenv("GIN_MODE") != "debug" {
		gin.SetMode(gin.ReleaseMode)
	}

	r := gin.Default()

	// CORS middleware — required so the browser allows the frontend to call
	// the backend across origins.
	//
	// In production, FRONTEND_URL must be set to the deployed frontend origin
	// (e.g. https://olds.up.railway.app). Locally, it defaults to "*" so
	// docker-compose and direct browser testing both work without extra config.
	//
	// OPTIONS is the HTTP "preflight" request browsers send before any
	// cross-origin POST or GET with custom headers. We short-circuit it here
	// with 204 so the real request isn't blocked.
	corsOrigin := os.Getenv("FRONTEND_URL")
	if corsOrigin == "" {
		corsOrigin = "*" // local dev fallback — lock this down in production
	}
	r.Use(func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", corsOrigin)
		c.Header("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		// Authorization header carries the Supabase JWT from the frontend.
		c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	})

	// ── Public routes — no auth required ──────────────────────────────────────
	// The article graph and connections are shared across all users. Reading
	// the feed does not require authentication — only writing behavior signals
	// (which are keyed to a user identity) requires a verified JWT.
	r.GET("/health", handler.Health)
	r.GET("/articles", articleHandler.List)
	r.GET("/articles/:id", articleHandler.GetByID)
	r.GET("/articles/:id/connections", articleHandler.Connections)
	r.GET("/ws/connections/:id", articleHandler.WSConnections)
	r.POST("/ingest", articleHandler.Ingest)

	// ── Protected routes — valid Supabase JWT required ────────────────────────
	// Using a route group lets us apply the auth middleware to a subset of routes
	// without wrapping every handler individually. Any route registered under
	// `authorized` will run the Auth middleware before reaching the handler.
	//
	// In Go/Gin, middleware is just a handler that calls c.Next() to pass control
	// to the next handler in the chain, or c.Abort() to stop the chain early.
	authorized := r.Group("/")
	authorized.Use(middleware.Auth(jwtSecret, ecKeys))
	{
		// POST /behavior requires auth so user IDs are attached to every signal.
		// This is the foundation for per-user feed personalization.
		authorized.POST("/behavior", articleHandler.RecordBehavior)

		// GET /stats and /stats/history expose system metrics for Phase 17.
		// JWT-protected: read-only, but contain operational detail that should
		// not be publicly accessible.
		authorized.GET("/stats", articleHandler.Stats)
		authorized.GET("/stats/history", articleHandler.StatsHistory)
	}

	// ── 10. Start scheduled ingestion goroutine ───────────────────────────────
	// Runs immediately on startup, then repeats every INGEST_INTERVAL (default
	// 24 hours). The interval is configurable so stress-testing can use a
	// shorter cycle (e.g. INGEST_INTERVAL=5m) without changing code.
	//
	// Why a goroutine? r.Run() below blocks forever — it IS the server loop.
	// We must launch ingestion as a goroutine so the server starts accepting
	// requests (serving the hydrated feed) while the first fetch runs in parallel.
	//
	// time.Ticker is Go's standard periodic timer. ticker.C is a channel that
	// receives a value every interval — `for range ticker.C` loops on each tick,
	// equivalent to `setInterval` in JS. defer ticker.Stop() cancels the ticker
	// when the goroutine exits (never in normal operation, but good practice).
	// ── Hydrate + ingest in background ───────────────────────────────────────
	// Hydration and initial ingestion run in a single background goroutine so
	// the HTTP server starts immediately. The feed is empty until hydration
	// completes, but health checks pass and Railway doesn't kill the container.
	go func() {
		log.Println("hydrating in-memory stores from database...")
		if err := repository.HydrateFromDB(
			context.Background(),
			articleRepo, behaviorRepo,
			store, g, bs,
		); err != nil {
			log.Printf("hydration failed (starting with empty stores): %v", err)
		}
		articleHandler.MarkHydrated()

		log.Println("starting initial article ingestion...")
		if err := articleHandler.RunScheduledIngest(); err != nil {
			log.Printf("initial ingestion failed: %v", err)
		}

		interval := 24 * time.Hour
		if v := os.Getenv("INGEST_INTERVAL"); v != "" {
			if d, err := time.ParseDuration(v); err == nil && d > 0 {
				interval = d
				log.Printf("ingest interval overridden to %v via INGEST_INTERVAL", interval)
			} else {
				log.Printf("invalid INGEST_INTERVAL %q — using default 24h", v)
			}
		}

		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		log.Printf("scheduled ingestion running every %v", interval)

		// range over a channel blocks until the next value arrives, then runs
		// the loop body. This is the idiomatic Go pattern for periodic tasks —
		// no sleep loops, no manual timing, no goroutine leaks.
		for range ticker.C {
			log.Printf("scheduled ingestion triggered (interval: %v)", interval)
			if err := articleHandler.RunScheduledIngest(); err != nil {
				log.Printf("scheduled ingestion failed: %v", err)
				// Continue — next tick will retry. Don't exit the goroutine.
			}
		}
	}()

	// ── 11. Start the HTTP server ─────────────────────────────────────────────
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	if err := r.Run(":" + port); err != nil {
		log.Fatalf("server failed to start: %v", err)
	}
}
