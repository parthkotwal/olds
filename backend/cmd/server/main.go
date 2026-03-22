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

	"github.com/gin-gonic/gin"
	"github.com/olds/backend/internal/article"
	"github.com/olds/backend/internal/behavior"
	"github.com/olds/backend/internal/db"
	"github.com/olds/backend/internal/graph"
	"github.com/olds/backend/internal/guardian"
	"github.com/olds/backend/internal/handler"
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

	jwtSecret := os.Getenv("SUPABASE_JWT_SECRET")
	if jwtSecret == "" {
		log.Fatal("SUPABASE_JWT_SECRET environment variable is required")
	}

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

	// ── 7. Hydrate in-memory stores from Postgres ─────────────────────────────
	// HydrateFromDB runs synchronously before the HTTP server starts so that
	// the very first request sees the full persisted state. The sequence is:
	//   LoadAll → store.Add → graph.Add → LoadSignals → bs.BulkLoad
	//
	// Non-fatal: if hydration fails (e.g., fresh DB with no rows), the server
	// starts with empty stores. The startup ingestion goroutine (step 9) will
	// repopulate from the news APIs.
	log.Println("hydrating in-memory stores from database...")
	if err := repository.HydrateFromDB(
		context.Background(),
		articleRepo, behaviorRepo,
		store, g, bs,
	); err != nil {
		log.Printf("hydration failed (starting with empty stores): %v", err)
	}

	// ── 8. Construct the handler and register routes ──────────────────────────
	articleHandler := handler.NewArticleHandler(
		store, client, guardianClient, mlClient, g, bs,
		articleRepo, behaviorRepo,
	)

	// ── 9. Register routes ────────────────────────────────────────────────────
	r := gin.Default()

	// CORS middleware — required so the frontend (localhost:5173) can call the
	// backend (localhost:8080) without the browser blocking cross-origin requests.
	//
	// In Go, middleware is a function that wraps a handler. gin.Default() gives
	// us Logger and Recovery; we add CORS by calling r.Use() with our own
	// middleware function. r.Use() registers middleware that runs for every request.
	//
	// The middleware pattern in Go:
	//   - c.Header() sets a response header
	//   - c.AbortWithStatus() stops the middleware chain and returns immediately
	//   - c.Next() passes control to the next handler in the chain
	//
	// OPTIONS is the HTTP "preflight" request that browsers send before any
	// cross-origin POST or GET with custom headers. We need to handle it here
	// or the browser will block the actual request before it reaches our handlers.
	r.Use(func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		// Authorization is added here so the browser's CORS preflight allows
		// the frontend to send the JWT in the Authorization: Bearer header.
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
	authorized.Use(middleware.Auth(jwtSecret))
	{
		// POST /behavior requires auth so user IDs are attached to every signal.
		// This is the foundation for per-user feed personalization.
		authorized.POST("/behavior", articleHandler.RecordBehavior)
	}

	// ── 10. Start background ingestion goroutine ──────────────────────────────
	// `go func() { ... }()` launches an anonymous function as a goroutine —
	// a lightweight concurrent execution unit managed by the Go runtime.
	//
	// Why launch this after hydration? Hydration loads the persisted state;
	// ingestion fetches fresh articles. By running ingestion after hydration,
	// we avoid a race where fresh articles overwrite the hydrated state.
	//
	// Why still use a goroutine (not call directly)? r.Run() blocks forever —
	// it IS the server. Calling ingestion directly before r.Run() would mean
	// the server isn't listening while fetching/enriching all articles (~30s).
	// A goroutine means the server starts immediately and serves the hydrated
	// feed while new articles are fetched in parallel.
	go func() {
		log.Println("starting initial article ingestion...")
		if err := articleHandler.RunStartupIngest(); err != nil {
			log.Printf("initial ingestion failed: %v", err)
			// Log and return — the server stays up with the hydrated store.
			// Hit POST /ingest to retry once the issue is resolved.
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
