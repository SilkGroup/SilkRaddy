package http

import (
	"context"
	"fmt"
	"net/http"

	"github.com/golang-jwt/jwt/v5"
	"github.com/oklog/ulid/v2"
)

type ctxKey string

const ctxKeyRequestID ctxKey = "request_id"

// requestID accepts an incoming X-Request-Id header or generates a fresh ULID,
// echoes it back in the response, and stashes it on the request context so
// downstream code can read it for log correlation.
func requestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-Id")
		if id == "" {
			id = ulid.Make().String()
		}
		w.Header().Set("X-Request-Id", id)
		ctx := context.WithValue(r.Context(), ctxKeyRequestID, id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RequestIDFromContext returns the request ID set by the requestID middleware,
// or "" if none is present.
func RequestIDFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(ctxKeyRequestID).(string); ok {
		return v
	}
	return ""
}

func (s *Server) jwtAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("jwt")
		if err != nil {
			jsonUnauthorized(w, "Unauthorized, access denied.")
			return
		}

		token, err := jwt.Parse(cookie.Value, func(token *jwt.Token) (any, error) {
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])

			}
			return []byte(s.config.JWTSign), nil
		})

		if err != nil || !token.Valid {
			jsonUnauthorized(w, "Invalid token.")
			return
		}

		next.ServeHTTP(w, r)
	})
}
