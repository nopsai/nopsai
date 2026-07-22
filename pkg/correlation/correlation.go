package correlation

import (
	"context"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"google.golang.org/grpc/metadata"
)

const (
	RequestIDHeader   = "X-Request-ID"
	TraceparentHeader = "Traceparent"

	RequestIDMetadata   = "x-request-id"
	TraceparentMetadata = "traceparent"
)

type contextKey string

const (
	ctxKeyRequestID   contextKey = "nopsai-request-id"
	ctxKeyTraceparent contextKey = "nopsai-traceparent"
)

func NewRequestID() string {
	return uuid.NewString()
}

func WithRequestID(ctx context.Context, requestID string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	requestID = strings.TrimSpace(requestID)
	if requestID == "" {
		return ctx
	}
	return context.WithValue(ctx, ctxKeyRequestID, requestID)
}

func RequestIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	requestID, _ := ctx.Value(ctxKeyRequestID).(string)
	return strings.TrimSpace(requestID)
}

func EnsureRequestID(ctx context.Context) (context.Context, string) {
	if ctx == nil {
		ctx = context.Background()
	}
	if requestID := RequestIDFromContext(ctx); requestID != "" {
		return ctx, requestID
	}
	requestID := NewRequestID()
	return WithRequestID(ctx, requestID), requestID
}

func WithTraceparent(ctx context.Context, traceparent string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	traceparent = strings.TrimSpace(traceparent)
	if traceparent == "" {
		return ctx
	}
	return context.WithValue(ctx, ctxKeyTraceparent, traceparent)
}

func TraceparentFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	traceparent, _ := ctx.Value(ctxKeyTraceparent).(string)
	return strings.TrimSpace(traceparent)
}

func FromHTTPHeaders(ctx context.Context, headers http.Header) (context.Context, string, string) {
	if ctx == nil {
		ctx = context.Background()
	}
	requestID := strings.TrimSpace(headers.Get(RequestIDHeader))
	if requestID == "" {
		requestID = RequestIDFromContext(ctx)
	}
	if requestID == "" {
		requestID = NewRequestID()
	}
	ctx = WithRequestID(ctx, requestID)

	traceparent := strings.TrimSpace(headers.Get(TraceparentHeader))
	if traceparent == "" {
		traceparent = TraceparentFromContext(ctx)
	}
	if traceparent != "" {
		ctx = WithTraceparent(ctx, traceparent)
	}
	return ctx, requestID, traceparent
}

func SetHTTPHeaders(ctx context.Context, headers http.Header) {
	if headers == nil {
		return
	}
	requestID := RequestIDFromContext(ctx)
	if requestID == "" {
		_, requestID = EnsureRequestID(ctx)
	}
	headers.Set(RequestIDHeader, requestID)
	if traceparent := TraceparentFromContext(ctx); traceparent != "" {
		headers.Set(TraceparentHeader, traceparent)
	}
}

func FromIncomingGRPC(ctx context.Context) (context.Context, string, string) {
	if ctx == nil {
		ctx = context.Background()
	}
	md, _ := metadata.FromIncomingContext(ctx)
	requestID := firstMetadataValue(md, RequestIDMetadata)
	if requestID == "" {
		requestID = RequestIDFromContext(ctx)
	}
	if requestID == "" {
		requestID = NewRequestID()
	}
	ctx = WithRequestID(ctx, requestID)

	traceparent := firstMetadataValue(md, TraceparentMetadata)
	if traceparent == "" {
		traceparent = TraceparentFromContext(ctx)
	}
	if traceparent != "" {
		ctx = WithTraceparent(ctx, traceparent)
	}
	return ctx, requestID, traceparent
}

func OutgoingGRPCContext(ctx context.Context) context.Context {
	ctx, requestID := EnsureRequestID(ctx)
	pairs := []string{RequestIDMetadata, requestID}
	if traceparent := TraceparentFromContext(ctx); traceparent != "" {
		pairs = append(pairs, TraceparentMetadata, traceparent)
	}
	return metadata.AppendToOutgoingContext(ctx, pairs...)
}

func firstMetadataValue(md metadata.MD, key string) string {
	for _, value := range md.Get(key) {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}
