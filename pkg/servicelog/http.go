package servicelog

import (
	"net/http"
	"time"

	"nopsai/pkg/correlation"

	"github.com/rs/zerolog/log"
)

type responseRecorder struct {
	http.ResponseWriter
	status int
	length int
}

func (r *responseRecorder) Unwrap() http.ResponseWriter { return r.ResponseWriter }

func (r *responseRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

func (r *responseRecorder) Write(payload []byte) (int, error) {
	n, err := r.ResponseWriter.Write(payload)
	r.length += n
	return n, err
}

func HTTPMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, requestID, traceparent := correlation.FromHTTPHeaders(r.Context(), r.Header)
		w.Header().Set(correlation.RequestIDHeader, requestID)
		if traceparent != "" {
			w.Header().Set(correlation.TraceparentHeader, traceparent)
		}

		start := time.Now()
		rec := &responseRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r.WithContext(ctx))

		event := log.Info().
			Str("method", r.Method).
			Str("path", r.URL.Path).
			Int("status", rec.status).
			Int("bytes", rec.length).
			Str("request_id", requestID).
			Str("remote_ip", r.RemoteAddr).
			Str("user_agent", r.UserAgent()).
			Int64("duration_ms", time.Since(start).Milliseconds())
		if traceparent != "" {
			event = event.Str("traceparent", traceparent)
		}
		event.Msg("http_request")
	})
}
