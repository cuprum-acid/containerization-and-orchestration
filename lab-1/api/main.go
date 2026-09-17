// api is the service under test for lab 1: a deliberately badly-behaved HTTP
// server that eats memory and burns CPU on demand, so that namespaces, cgroup
// limits and dropped privileges have something to actually act on.
//
//	GET /health      -> ok
//	GET /eat?mb=N    -> allocate N MB and hold them
//	GET /burn?n=N    -> pin N cores (default 1) in a busy loop
//	GET /stop        -> stop burning
//	GET /free        -> release the held memory
//	GET /info        -> what the process sees about itself (pid, uid, ns, caps, cgroup)
//	GET /openapi.yaml -> the API specification, embedded in the binary
//	GET /docs        -> Swagger UI over that specification, also embedded
//
package main

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"math"
	"net/http"
	"os"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Memory is taken in chunks so the log shows how far we got before the kernel
// steps in. A single huge allocation would die with nothing to look at.
const chunkMB = 32

var (
	memMu  sync.Mutex
	held   [][]byte
	pageSz = os.Getpagesize()
)

func heldMB() int {
	memMu.Lock()
	defer memMu.Unlock()
	n := 0
	for _, c := range held {
		n += len(c)
	}
	return n >> 20
}

func eat(w http.ResponseWriter, r *http.Request) {
	mb, err := strconv.Atoi(r.URL.Query().Get("mb"))
	if err != nil || mb <= 0 {
		http.Error(w, "usage: /eat?mb=N  (N > 0)\n", http.StatusBadRequest)
		return
	}
	log.Printf("eat: asked for %d MB (already holding %d MB)", mb, heldMB())
	for done := 0; done < mb; {
		step := chunkMB
		if rem := mb - done; rem < step {
			step = rem
		}
		buf := make([]byte, step<<20)
		// Touch one byte per page. make() hands back zeroed memory that the
		// kernel has not backed with real page frames yet; the write is what
		// faults them in, and the fault is what the cgroup memory controller
		// charges us for. Without this loop the limit would never be hit.
		for i := 0; i < len(buf); i += pageSz {
			buf[i] = 1
		}
		memMu.Lock()
		held = append(held, buf) // keep a reference so the GC cannot reclaim it
		memMu.Unlock()
		done += step
		log.Printf("eat: +%d MB, now holding %d MB", step, heldMB())
	}
	fmt.Fprintf(w, "held %d MB\n", heldMB())
}

func free(w http.ResponseWriter, r *http.Request) {
	memMu.Lock()
	held = nil
	memMu.Unlock()
	runtime.GC()
	debug.FreeOSMemory() // hand the pages back to the kernel, not just to the Go heap
	log.Printf("free: released everything")
	fmt.Fprintf(w, "released, holding %d MB\n", heldMB())
}

var (
	burnMu   sync.Mutex
	burnStop chan struct{}
	burnN    int
	sink     atomic.Uint64 // keeps the busy loop from being optimised away
)

func burn(w http.ResponseWriter, r *http.Request) {
	n := 1
	if v := r.URL.Query().Get("n"); v != "" {
		parsed, err := strconv.Atoi(v)
		if err != nil || parsed <= 0 {
			http.Error(w, "usage: /burn?n=N  (N > 0)\n", http.StatusBadRequest)
			return
		}
		n = parsed
	}
	burnMu.Lock()
	if burnStop == nil {
		burnStop = make(chan struct{})
	}
	stop := burnStop
	burnN += n
	total := burnN
	burnMu.Unlock()

	for i := 0; i < n; i++ {
		go spin(stop)
	}
	log.Printf("burn: started %d burner(s), %d running", n, total)
	fmt.Fprintf(w, "burning on %d core(s), GET /stop to stop\n", total)
}

func spin(stop <-chan struct{}) {
	// One OS thread per burner: a goroutine can be migrated between threads,
	// and we want a steady, measurable load of one core each.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	x := 1.0001
	for i := uint64(0); ; i++ {
		x = math.Sqrt(x*1.0000001 + 1)
		if i&0xFFFFF == 0 {
			sink.Store(math.Float64bits(x))
			select {
			case <-stop:
				return
			default:
			}
		}
	}
}

func stopBurn(w http.ResponseWriter, r *http.Request) {
	burnMu.Lock()
	was := burnN
	if burnStop != nil {
		close(burnStop)
		burnStop = nil
		burnN = 0
	}
	burnMu.Unlock()
	log.Printf("stop: stopped %d burner(s)", was)
	fmt.Fprintf(w, "stopped %d burner(s)\n", was)
}

type info struct {
	PID        int               `json:"pid"`
	PPID       int               `json:"ppid"`
	UID        int               `json:"uid"`
	EUID       int               `json:"euid"`
	GID        int               `json:"gid"`
	Hostname   string            `json:"hostname"`
	NumCPU     int               `json:"num_cpu"`
	HeldMB     int               `json:"held_mb"`
	Burners    int               `json:"burners"`
	Cgroup     string            `json:"cgroup,omitempty"`
	ProcStatus map[string]string `json:"proc_status,omitempty"`
}

// Fields of /proc/self/status that say something about isolation: NSpid shows
// the pid in every nested pid namespace, Cap* the capability sets, Seccomp
// whether a filter is loaded.
var statusKeys = []string{
	"Name", "Pid", "PPid", "NSpid", "Uid", "Gid", "Threads",
	"CapInh", "CapPrm", "CapEff", "CapBnd", "CapAmb",
	"NoNewPrivs", "Seccomp", "Seccomp_filters",
}

func procStatus() map[string]string {
	raw, err := os.ReadFile("/proc/self/status")
	if err != nil {
		return nil // not Linux, or /proc is not mounted in this mount namespace
	}
	want := make(map[string]bool, len(statusKeys))
	for _, k := range statusKeys {
		want[k] = true
	}
	out := map[string]string{}
	for _, line := range strings.Split(string(raw), "\n") {
		k, v, ok := strings.Cut(line, ":")
		if ok && want[k] {
			out[k] = strings.TrimSpace(v)
		}
	}
	return out
}

func whoami(w http.ResponseWriter, r *http.Request) {
	host, _ := os.Hostname()
	cg, _ := os.ReadFile("/proc/self/cgroup")
	burnMu.Lock()
	n := burnN
	burnMu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.Encode(info{
		PID:        os.Getpid(),
		PPID:       os.Getppid(),
		UID:        os.Getuid(),
		EUID:       os.Geteuid(),
		GID:        os.Getgid(),
		Hostname:   host,
		NumCPU:     runtime.NumCPU(),
		HeldMB:     heldMB(),
		Burners:    n,
		Cgroup:     strings.TrimSpace(string(cg)),
		ProcStatus: procStatus(),
	})
}

func health(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintln(w, "ok")
}

// Everything the documentation needs is embedded rather than read from disk or
// fetched from a CDN: the binary is meant to run from a bare rootfs, from a
// scratch image, and inside an empty net namespace, where there is neither a
// file to open nor a network to reach.
//
//go:embed openapi.yaml
var openapiSpec []byte

//go:embed docs.html
var docsPage []byte

//go:embed swaggerui/swagger-ui.css swaggerui/swagger-ui-bundle.js
var swaggerUI embed.FS

func spec(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/yaml")
	w.Write(openapiSpec)
}

func docs(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(docsPage)
}

// swaggerAssets serves the vendored Swagger UI files under /docs/.
func swaggerAssets() http.Handler {
	sub, err := fs.Sub(swaggerUI, "swaggerui")
	if err != nil {
		panic(err) // the tree is embedded at build time; failing here means a broken binary
	}
	return http.StripPrefix("/docs/", http.FileServer(http.FS(sub)))
}

func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s from %s (%s)", r.Method, r.URL.RequestURI(), r.RemoteAddr, time.Since(start).Round(time.Millisecond))
	})
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)

	addr := os.Getenv("ADDR")
	if addr == "" {
		addr = ":8080"
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/health", health)
	mux.HandleFunc("/eat", eat)
	mux.HandleFunc("/free", free)
	mux.HandleFunc("/burn", burn)
	mux.HandleFunc("/stop", stopBurn)
	mux.HandleFunc("/info", whoami)
	mux.HandleFunc("/openapi.yaml", spec)
	mux.HandleFunc("/docs", docs)
	mux.Handle("/docs/", swaggerAssets())

	host, _ := os.Hostname()
	log.Printf("api starting: pid=%d ppid=%d uid=%d gid=%d host=%s cpus=%d addr=%s",
		os.Getpid(), os.Getppid(), os.Getuid(), os.Getgid(), host, runtime.NumCPU(), addr)

	srv := &http.Server{
		Addr:              addr,
		Handler:           logRequests(mux),
		ReadHeaderTimeout: 5 * time.Second,
	}
	log.Fatal(srv.ListenAndServe())
}
