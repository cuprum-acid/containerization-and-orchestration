// api is the front of the shop for lab 3: it takes orders and lists them,
// storing everything in Postgres. The worker marks the orders processed.
//
//	GET  /health  -> ok; 503 while HEALTH_FAIL=true (a deliberately broken release)
//	POST /order   -> {"item":"book","qty":2} -> 201 with the stored order
//	GET  /orders  -> the latest orders, newest first (?limit=N, default 50)
//	GET  /metrics -> Prometheus metrics
//
// Config: DATABASE_URL (a postgres:// URI, CloudNativePG puts one into the
// <cluster>-app Secret under the key "uri"; without it the order routes
// answer 503), ADDR (default :8080), HEALTH_FAIL. The build version comes in
// through -ldflags.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Set at build time: go build -ldflags "-X main.version=1.0.0".
var version = "dev"

var (
	requests = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "http_requests_total",
		Help: "HTTP requests by route, method and status code.",
	}, []string{"route", "method", "code"})

	duration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "http_request_duration_seconds",
		Help:    "HTTP request duration.",
		Buckets: []float64{.001, .005, .01, .025, .05, .1, .25, .5, 1, 2.5},
	}, []string{"route", "method"})

	ordersCreated = promauto.NewCounter(prometheus.CounterOpts{
		Name: "shop_orders_created_total",
		Help: "Orders stored by POST /order.",
	})

	buildInfo = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "shop_build_info",
		Help: "Always 1; the version label tells which build is running.",
	}, []string{"version"})
)

const schema = `
CREATE TABLE IF NOT EXISTS orders (
    id           bigserial   PRIMARY KEY,
    item         text        NOT NULL,
    qty          integer     NOT NULL CHECK (qty > 0),
    status       text        NOT NULL DEFAULT 'new',
    created_at   timestamptz NOT NULL DEFAULT now(),
    processed_at timestamptz
);
CREATE INDEX IF NOT EXISTS orders_new_idx ON orders (id) WHERE status = 'new';`

type order struct {
	ID          int64      `json:"id"`
	Item        string     `json:"item"`
	Qty         int        `json:"qty"`
	Status      string     `json:"status"`
	CreatedAt   time.Time  `json:"created_at"`
	ProcessedAt *time.Time `json:"processed_at,omitempty"`
}

type server struct {
	// nil until the database is reachable; the order routes answer 503 meanwhile.
	db atomic.Pointer[pgxpool.Pool]
	// false when DATABASE_URL is not set at all.
	configured bool
}

func (s *server) pool(w http.ResponseWriter) *pgxpool.Pool {
	db := s.db.Load()
	switch {
	case db != nil:
	case !s.configured:
		http.Error(w, "database not configured", http.StatusServiceUnavailable)
	default:
		http.Error(w, "database not ready", http.StatusServiceUnavailable)
	}
	return db
}

// --- middleware --------------------------------------------------------------

type statusRecorder struct {
	http.ResponseWriter
	code int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.code = code
	r.ResponseWriter.WriteHeader(code)
}

func instrument(route string, h http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, code: http.StatusOK}
		w.Header().Set("X-App-Version", version)
		h(rec, r)
		took := time.Since(start)
		requests.WithLabelValues(route, r.Method, strconv.Itoa(rec.code)).Inc()
		duration.WithLabelValues(route, r.Method).Observe(took.Seconds())
		level := slog.LevelInfo
		if rec.code >= 500 {
			level = slog.LevelError
		}
		slog.Log(r.Context(), level, "request", "method", r.Method, "route", route,
			"status", rec.code, "duration_ms", took.Milliseconds())
	})
}

// --- handlers ----------------------------------------------------------------

func health(w http.ResponseWriter, _ *http.Request) {
	// Deliberately does not touch the database: liveness must not fail
	// because Postgres is restarting, or every api pod would be killed with it.
	if os.Getenv("HEALTH_FAIL") == "true" {
		http.Error(w, "unhealthy: HEALTH_FAIL=true", http.StatusServiceUnavailable)
		return
	}
	fmt.Fprintln(w, "ok")
}

func (s *server) createOrder(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Item string `json:"item"`
		Qty  int    `json:"qty"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&in); err != nil || in.Item == "" || in.Qty <= 0 {
		http.Error(w, `body must be {"item":"<name>","qty":<n > 0>}`, http.StatusBadRequest)
		return
	}
	db := s.pool(w)
	if db == nil {
		return
	}
	var o order
	err := db.QueryRow(r.Context(),
		`INSERT INTO orders (item, qty) VALUES ($1, $2)
		 RETURNING id, item, qty, status, created_at, processed_at`,
		in.Item, in.Qty).Scan(&o.ID, &o.Item, &o.Qty, &o.Status, &o.CreatedAt, &o.ProcessedAt)
	if err != nil {
		slog.Error("insert order", "error", err.Error())
		http.Error(w, "database error", http.StatusServiceUnavailable)
		return
	}
	ordersCreated.Inc()
	writeJSON(w, http.StatusCreated, o)
}

func (s *server) listOrders(w http.ResponseWriter, r *http.Request) {
	limit, err := strconv.Atoi(r.URL.Query().Get("limit"))
	if err != nil || limit <= 0 || limit > 1000 {
		limit = 50
	}
	db := s.pool(w)
	if db == nil {
		return
	}
	rows, err := db.Query(r.Context(),
		`SELECT id, item, qty, status, created_at, processed_at
		 FROM orders ORDER BY id DESC LIMIT $1`, limit)
	if err != nil {
		slog.Error("list orders", "error", err.Error())
		http.Error(w, "database error", http.StatusServiceUnavailable)
		return
	}
	orders, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (order, error) {
		var o order
		err := row.Scan(&o.ID, &o.Item, &o.Qty, &o.Status, &o.CreatedAt, &o.ProcessedAt)
		return o, err
	})
	if err != nil {
		slog.Error("scan orders", "error", err.Error())
		http.Error(w, "database error", http.StatusServiceUnavailable)
		return
	}
	writeJSON(w, http.StatusOK, orders)
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

// --- setup -------------------------------------------------------------------

// connect retries: the api may start before Postgres is ready (the operator
// brings the database up in parallel), and crash-looping in the meantime
// would only add noise.
func connect(ctx context.Context, url string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, err
	}
	cfg.MaxConns = 10
	for attempt := 1; ; attempt++ {
		pool, err := pgxpool.NewWithConfig(ctx, cfg)
		if err == nil {
			if err = pool.Ping(ctx); err == nil {
				if _, err = pool.Exec(ctx, schema); err == nil {
					return pool, nil
				}
			}
			pool.Close()
		}
		slog.Warn("database not ready, retrying", "attempt", attempt, "error", err.Error())
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(min(time.Duration(attempt)*time.Second, 10*time.Second)):
		}
	}
}

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	buildInfo.WithLabelValues(version).Set(1)

	addr := os.Getenv("ADDR")
	if addr == "" {
		addr = ":8080"
	}
	dbURL := os.Getenv("DATABASE_URL")

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	s := &server{configured: dbURL != ""}
	mux := http.NewServeMux()
	// Everything is served from the start: the pod is alive and observable
	// while Postgres is still coming up, and the order routes say 503 until
	// the database is reachable.
	mux.Handle("GET /health", instrument("/health", health))
	mux.Handle("POST /order", instrument("/order", s.createOrder))
	mux.Handle("GET /orders", instrument("/orders", s.listOrders))
	mux.Handle("GET /metrics", promhttp.Handler())

	srv := &http.Server{Addr: addr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		slog.Info("api starting", "addr", addr, "version", version,
			"health_fail", os.Getenv("HEALTH_FAIL") == "true")
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("server failed", "error", err.Error())
			stop()
		}
	}()

	if dbURL == "" {
		// Not an error: the chart can run without a database (part 2 of the
		// lab, before the operator brings one).
		slog.Warn("DATABASE_URL is not set, order routes answer 503")
	} else {
		db, err := connect(ctx, dbURL)
		if err != nil {
			slog.Error("database connect failed", "error", err.Error())
			os.Exit(1)
		}
		defer db.Close()
		s.db.Store(db)
		slog.Info("database ready, orders enabled")
	}

	<-ctx.Done()
	// SIGTERM: stop accepting new connections, let in-flight requests finish.
	// Removing the pod from the Service happens in parallel and takes a moment,
	// so a preStop sleep in the pod spec is what makes rollouts lossless.
	slog.Info("api stopping")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)
}
