package http

import (
	"context"
	"log"
	"net/http"
	"time"
)

// readyCheckTimeout bounds how long the readiness probe will wait for a DB
// connection. It's kept below the tightest external health-check timeout
// (the Docker HEALTHCHECK's 3s) so the handler itself returns a clean 503
// instead of being cut off mid-request by the caller giving up.
const readyCheckTimeout = 2 * time.Second

func LiveHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"live"}`))
}

func ReadyHandler(userStore userStorer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), readyCheckTimeout)
		defer cancel()

		if err := userStore.Ping(ctx); err != nil {
			log.Printf("readiness ping failed: %v", err)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"status":"not_ready"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ready"}`))
	}
}
