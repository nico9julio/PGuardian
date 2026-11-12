package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/nico9julio/PGuardian/internal/auth"
	"github.com/nico9julio/PGuardian/internal/backup"
	"github.com/nico9julio/PGuardian/internal/cloud"
	"github.com/nico9julio/PGuardian/internal/database"
	"github.com/nico9julio/PGuardian/internal/ipc"
	"github.com/nico9julio/PGuardian/internal/notify"
	"github.com/nico9julio/PGuardian/internal/scheduler"
	"github.com/nico9julio/PGuardian/internal/updater"
)

type callerCtxKey struct{}

type Server struct {
	store      *database.Store
	authMgr    *auth.Manager
	engine     *backup.Engine
	cloudSvc   *cloud.Service
	dispatcher *notify.Dispatcher
	sched      *scheduler.Coordinator
	upd        *updater.Coordinator
	httpSrv    *http.Server
	listener   net.Listener
}

func NewServer(
	store *database.Store,
	authMgr *auth.Manager,
	engine *backup.Engine,
	cloudSvc *cloud.Service,
	dispatcher *notify.Dispatcher,
	sched *scheduler.Coordinator,
	upd *updater.Coordinator,
) *Server {
	s := &Server{
		store:      store,
		authMgr:    authMgr,
		engine:     engine,
		cloudSvc:   cloudSvc,
		dispatcher: dispatcher,
		sched:      sched,
		upd:        upd,
	}

	mux := http.NewServeMux()
	s.registerRoutes(mux)

	s.httpSrv = &http.Server{
		Handler:           s.recoverMiddleware(mux),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
		ConnContext: func(ctx context.Context, c net.Conn) context.Context {
			id := ipc.InspectCaller(c)
			return context.WithValue(ctx, callerCtxKey{}, id)
		},
	}
	return s
}

func (s *Server) StartNamedPipe() error {
	ln, err := ipc.ListenNamedPipe()
	if err != nil {
		return err
	}
	s.listener = ln
	go func() {
		if err := s.httpSrv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("named pipe IPC server error", "error", err)
		}
	}()
	return nil
}

func (s *Server) Stop(ctx context.Context) error {
	if s.httpSrv != nil {
		return s.httpSrv.Shutdown(ctx)
	}
	return nil
}

func (s *Server) callerFromReq(r *http.Request) ipc.CallerIdentity {
	if v, ok := r.Context().Value(callerCtxKey{}).(ipc.CallerIdentity); ok {
		return v
	}
	return ipc.CallerIdentity{}
}

func (s *Server) isAuthenticated(r *http.Request) bool {
	hdr := r.Header.Get("Authorization")
	if !strings.HasPrefix(hdr, "Bearer ") {
		return false
	}
	tok := strings.TrimSpace(strings.TrimPrefix(hdr, "Bearer "))
	return s.authMgr.ValidateToken(tok)
}

func (s *Server) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.isAuthenticated(r) {
			writeError(w, http.StatusUnauthorized, auth.ErrUnauthorized.Error())
			return
		}
		next(w, r)
	}
}

func (s *Server) requireSetupOrAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.store.IsSetupCompleted() || s.isAuthenticated(r) {
			next(w, r)
			return
		}
		writeError(w, http.StatusUnauthorized, auth.ErrUnauthorized.Error())
	}
}

func (s *Server) recoverMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				slog.Error("recovered panic in HTTP IPC handler", "path", r.URL.Path, "panic", rec)
				writeError(w, http.StatusInternalServerError, "internal server error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, code int, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

func decodeJSON(r *http.Request, dst any) error {
	defer func() { _ = r.Body.Close() }()
	return json.NewDecoder(http.MaxBytesReader(nil, r.Body, 4*1024*1024)).Decode(dst)
}
