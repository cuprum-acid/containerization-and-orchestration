// worker is the back of the shop for lab 3: it picks new orders from Postgres
// and marks them processed. Several replicas run side by side; FOR UPDATE SKIP
// LOCKED gives each order to exactly one of them.
//
//	GET /health  -> ok; 503 while HEALTH_FAIL=true
//	GET /metrics -> Prometheus metrics
//
// Config: DATABASE_URL (without it the worker idles), ADDR (default :8080),
// BATCH (default 10), INTERVAL (default 2s), HEALTH_FAIL.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var version = "dev"

var (
	processed = promauto.NewCounter(prometheus.CounterOpts{
		Name: "shop_orders_processed_total",
		Help: "Orders this worker marked processed.",
	})
	batchErrors = promauto.NewCounter(prometheus.CounterOpts{
		Name: "shop_worker_batch_errors_total",
		Help: "Batches that failed against the database.",
	})
	lastSuccess = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "shop_worker_last_success_timestamp_seconds",
		Help: "Unix time of the last batch that reached the database successfully.",
	})
	pending = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "shop_orders_pending",
		Help: "Orders still in status 'new', as seen by this worker.",
	})
	buildInfo = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "shop_build_info",
		Help: "Always 1; the version label tells which build is running.",
	}, []string{"version"})
)

// One statement, one transaction: lock a batch of new orders that no other
// worker holds (SKIP LOCKED), mark them, return how many.
const claim = `
UPDATE orders SET status = 'processed', processed_at = now()
WHERE id IN (
    SELECT id FROM orders WHERE status = 'new'
    ORDER BY id LIMIT $1
    FOR UPDATE SKIP LOCKED
)`

func health(w http.ResponseWriter, _ *http.Request) {
	if os.Getenv("HEALTH_FAIL") == "true" {
		http.Error(w, "unhealthy: HEALTH_FAIL=true", http.StatusServiceUnavailable)
		return
	}
	fmt.Fprintln(w, "ok")
}

func envInt(name string, def int) int {
	if v, err := strconv.Atoi(os.Getenv(name)); err == nil && v > 0 {
		return v
	}
	return def
}

func envDuration(name string, def time.Duration) time.Duration {
	if v, err := time.ParseDuration(os.Getenv(name)); err == nil && v > 0 {
		return v
	}
	return def
}

func tick(ctx context.Context, db *pgxpool.Pool, batch int) {
	tag, err := db.Exec(ctx, claim, batch)
	if err != nil {
		// The table may not exist yet: the api creates it on its first start.
		batchErrors.Inc()
		slog.Warn("batch failed", "error", err.Error())
		return
	}
	lastSuccess.SetToCurrentTime()
	if n := tag.RowsAffected(); n > 0 {
		processed.Add(float64(n))
		slog.Info("processed orders", "count", n)
	}
	var left int64
	if err := db.QueryRow(ctx, `SELECT count(*) FROM orders WHERE status = 'new'`).Scan(&left); err == nil {
		pending.Set(float64(left))
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
	batch := envInt("BATCH", 10)
	interval := envDuration("INTERVAL", 2*time.Second)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", health)
	mux.Handle("GET /metrics", promhttp.Handler())
	srv := &http.Server{Addr: addr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("server failed", "error", err.Error())
			stop()
		}
	}()

	if dbURL == "" {
		// Nothing to process yet: stay up and observable until a database is
		// configured (part 2 of the lab runs without one).
		slog.Warn("DATABASE_URL is not set, idling")
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
		return
	}

	// pgxpool connects lazily and reconnects by itself: a lost database shows
	// up as failed batches (and metrics), not as a crashed worker.
	db, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		slog.Error("bad DATABASE_URL", "error", err.Error())
		os.Exit(1)
	}
	defer db.Close()

	slog.Info("worker starting", "addr", addr, "version", version, "batch", batch, "interval", interval.String())
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			slog.Info("worker stopping")
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = srv.Shutdown(shutdownCtx)
			return
		case <-t.C:
			tick(ctx, db, batch)
		}
	}
}
