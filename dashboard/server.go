package dashboard

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/mrzack99s/cocodb"
)

// Server represents the standalone CoCoDB Observability & Metrics Dashboard server.
type Server struct {
	mu          sync.Mutex
	db          *cocodb.DB
	addr        string
	httpSrv     *http.Server
	handler     http.Handler
	running     bool
	lastHits    uint64
	lastMisses  uint64
	lastTime    time.Time
	throughputMu sync.Mutex
}

// Option configures the Dashboard server.
type Option func(*Server)

// NewServer creates a new standalone Observability Dashboard server for a database instance.
func NewServer(db *cocodb.DB, addr string, opts ...Option) *Server {
	if addr == "" {
		addr = ":9090"
	}
	s := &Server{
		db:       db,
		addr:     addr,
		lastTime: time.Now(),
	}
	for _, o := range opts {
		o(s)
	}

	mux := http.NewServeMux()
	s.registerRoutes(mux)
	s.handler = s.corsMiddleware(mux)

	return s
}

// Handler returns the root HTTP handler with CORS support.
func (s *Server) Handler() http.Handler {
	return s.handler
}

// ServeHTTP implements http.Handler.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.handler.ServeHTTP(w, r)
}

// Addr returns the listening address.
func (s *Server) Addr() string {
	return s.addr
}

// URL returns the full HTTP URL to access the dashboard.
func (s *Server) URL() string {
	if s.addr[0] == ':' {
		return fmt.Sprintf("http://localhost%s", s.addr)
	}
	return fmt.Sprintf("http://%s", s.addr)
}

// Start starts the HTTP server in a background goroutine.
func (s *Server) Start() error {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return nil
	}

	s.httpSrv = &http.Server{
		Addr:         s.addr,
		Handler:      s.handler,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 0, // Disable write timeout for SSE streams
	}
	s.running = true
	s.mu.Unlock()

	go func() {
		_ = s.httpSrv.ListenAndServe()
	}()

	return nil
}

// Stop gracefully shuts down the server.
func (s *Server) Stop(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running || s.httpSrv == nil {
		return nil
	}
	s.running = false
	return s.httpSrv.Shutdown(ctx)
}

func (s *Server) corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}
