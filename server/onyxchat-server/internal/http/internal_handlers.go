package http

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/gorilla/mux"
)

// InternalServiceAuth is middleware that gates /internal/* routes behind a
// shared secret. In production the /internal prefix should also be blocked at
// the load balancer so it is never reachable from the public internet.
func InternalServiceAuth(secret string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("X-Service-Secret") != secret {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// GET /internal/users/{id}/push-token
// Called by the notification service to fetch a recipient's push token before
// delivering an offline notification.
func InternalGetPushTokenHandler(userStore userStorer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		idStr := mux.Vars(r)["id"]
		userID, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil || userID <= 0 {
			http.Error(w, "invalid user id", http.StatusBadRequest)
			return
		}

		token, err := userStore.GetPushToken(userID)
		if err != nil {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"pushToken": token})
	}
}

// PUT /api/v1/users/me/push-token
// Allows a client to register (or rotate) its FCM/APNs/web-push token.
func RegisterPushTokenHandler(userStore userStorer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		currentUser := CurrentUser(r)
		if currentUser == nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		var body struct {
			Token string `json:"token"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Token == "" {
			http.Error(w, "token required", http.StatusBadRequest)
			return
		}
		if len(body.Token) > 512 {
			http.Error(w, "token too long", http.StatusBadRequest)
			return
		}

		if err := userStore.SetPushToken(currentUser.ID, body.Token); err != nil {
			http.Error(w, "failed to save token", http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}
