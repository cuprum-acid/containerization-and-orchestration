package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

var (
	requests = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "http_requests_total",
		Help: "HTTP requests by route, method and status code.",
	}, []string{"route", "method", "code"})

	failures = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "http_request_errors_total",
		Help: "HTTP requests that ended with a 5xx status.",
	}, []string{"route", "method"})

	duration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "http_request_duration_seconds",
		Help:    "HTTP request duration.",
		Buckets: []float64{.005, .01, .025, .05, .1, .25, .5, 1, 1.5, 2, 2.5, 3, 5},
	}, []string{"route", "method"})
)

var tracer = otel.Tracer("api")

type loggerKey struct{}

func logger(ctx context.Context) *slog.Logger {
	if l, ok := ctx.Value(loggerKey{}).(*slog.Logger); ok {
		return l
	}
	return slog.Default()
}

type statusRecorder struct {
	http.ResponseWriter
	code int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.code = code
	r.ResponseWriter.WriteHeader(code)
}

func instrument(route string, h http.HandlerFunc) http.Handler {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sc := trace.SpanContextFromContext(r.Context())
		l := slog.Default().With("trace_id", sc.TraceID().String(), "span_id", sc.SpanID().String())
		ctx := context.WithValue(r.Context(), loggerKey{}, l)

		rec := &statusRecorder{ResponseWriter: w, code: http.StatusOK}
		h(rec, r.WithContext(ctx))

		took := time.Since(start)
		code := strconv.Itoa(rec.code)
		requests.WithLabelValues(route, r.Method, code).Inc()
		duration.WithLabelValues(route, r.Method).Observe(took.Seconds())

		level := slog.LevelInfo
		if rec.code >= 500 {
			failures.WithLabelValues(route, r.Method).Inc()
			level = slog.LevelError
		}
		l.Log(ctx, level, "request",
			"method", r.Method, "route", route, "status", rec.code,
			"duration_ms", took.Milliseconds(), "remote", r.RemoteAddr)
	})

	return otelhttp.NewHandler(inner, "GET "+route)
}

func health(w http.ResponseWriter, _ *http.Request) {
	fmt.Fprintln(w, "ok")
}

func fail(w http.ResponseWriter, req *http.Request) {
	err := errors.New("simulated failure")
	span := trace.SpanFromContext(req.Context())
	span.RecordError(err)
	span.SetStatus(codes.Error, err.Error())
	logger(req.Context()).Error("handler failed", "error", err.Error())
	http.Error(w, "internal error: "+err.Error(), http.StatusInternalServerError)
}

func slow(w http.ResponseWriter, req *http.Request) {
	d := time.Second + time.Duration(rand.Int64N(int64(2*time.Second)))

	ctx, span := tracer.Start(req.Context(), "slow-op",
		trace.WithAttributes(attribute.Int64("slow.planned_ms", d.Milliseconds())))
	select {
	case <-time.After(d):
	case <-ctx.Done():
	}
	span.End()

	fmt.Fprintf(w, "slept %d ms\n", d.Milliseconds())
}

var loadTargets = map[string]bool{"/health": true, "/fail": true, "/slow": true}

func load(self string) http.HandlerFunc {
	client := &http.Client{Transport: otelhttp.NewTransport(http.DefaultTransport), Timeout: 10 * time.Second}

	return func(w http.ResponseWriter, req *http.Request) {
		n, err := strconv.Atoi(req.URL.Query().Get("n"))
		if err != nil || n <= 0 {
			n = 50
		}
		n = min(n, 1000)
		path := req.URL.Query().Get("path")
		if path == "" {
			path = "/health"
		}
		if !loadTargets[path] {
			http.Error(w, "path must be one of /health, /fail, /slow\n", http.StatusBadRequest)
			return
		}

		var (
			mu     sync.Mutex
			counts = map[int]int{}
			wg     sync.WaitGroup
			sem    = make(chan struct{}, 10)
		)
		for range n {
			wg.Add(1)
			sem <- struct{}{}
			go func() {
				defer wg.Done()
				defer func() { <-sem }()
				code := 0
				r, err := http.NewRequestWithContext(req.Context(), http.MethodGet, self+path, nil)
				if err == nil {
					if resp, err := client.Do(r); err == nil {
						code = resp.StatusCode
						resp.Body.Close()
					}
				}
				mu.Lock()
				counts[code]++
				mu.Unlock()
			}()
		}
		wg.Wait()

		logger(req.Context()).Info("load done", "n", n, "path", path, "codes", fmt.Sprint(counts))
		fmt.Fprintf(w, "sent %d requests to %s: %v\n", n, path, counts)
	}
}

func setupTracing(ctx context.Context) (func(context.Context) error, error) {
	res, err := resource.New(ctx,
		resource.WithAttributes(attribute.String("service.name", "api")),
		resource.WithFromEnv(),
		resource.WithTelemetrySDK(),
	)
	if err != nil {
		return nil, err
	}
	opts := []sdktrace.TracerProviderOption{sdktrace.WithResource(res)}
	if os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT") != "" {
		exp, err := otlptracehttp.New(ctx)
		if err != nil {
			return nil, err
		}
		opts = append(opts, sdktrace.WithBatcher(exp))
	}
	tp := sdktrace.NewTracerProvider(opts...)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.TraceContext{})
	return tp.Shutdown, nil
}

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))

	addr := os.Getenv("ADDR")
	if addr == "" {
		addr = ":8080"
	}
	port := addr[strings.LastIndex(addr, ":")+1:]

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	shutdownTracing, err := setupTracing(ctx)
	if err != nil {
		slog.Error("tracing setup failed", "error", err.Error())
		os.Exit(1)
	}

	mux := http.NewServeMux()
	mux.Handle("GET /health", instrument("/health", health))
	mux.Handle("GET /fail", instrument("/fail", fail))
	mux.Handle("GET /slow", instrument("/slow", slow))
	mux.Handle("GET /load", instrument("/load", load("http://127.0.0.1:"+port)))
	mux.Handle("GET /metrics", promhttp.Handler())
	mux.HandleFunc("GET /healthz", health)

	srv := &http.Server{Addr: addr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		slog.Info("api starting", "addr", addr,
			"otlp_endpoint", os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"))
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("server failed", "error", err.Error())
			stop()
		}
	}()

	<-ctx.Done()
	slog.Info("api stopping")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)
	_ = shutdownTracing(shutdownCtx)
}
