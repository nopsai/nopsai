package servicelog

import (
	"context"
	"strings"
	"time"

	"nopsai/pkg/correlation"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"google.golang.org/grpc"
	"google.golang.org/grpc/status"
)

func GRPCUnaryServerInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		ctx, requestID, traceparent := correlation.FromIncomingGRPC(ctx)
		start := time.Now()
		resp, err := handler(ctx, req)
		fullMethod := ""
		if info != nil {
			fullMethod = info.FullMethod
		}
		grpcLogEvent(err).
			Str("grpc_method", grpcMethod(fullMethod)).
			Str("grpc_full_method", strings.TrimSpace(fullMethod)).
			Str("grpc_code", status.Code(err).String()).
			Str("request_id", requestID).
			Str("traceparent", traceparent).
			Int64("duration_ms", time.Since(start).Milliseconds()).
			Msg("grpc_request")
		return resp, err
	}
}

func GRPCStreamServerInterceptor() grpc.StreamServerInterceptor {
	return func(srv any, stream grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		ctx, requestID, traceparent := correlation.FromIncomingGRPC(stream.Context())
		wrapped := &correlatedServerStream{ServerStream: stream, ctx: ctx}
		start := time.Now()
		err := handler(srv, wrapped)
		fullMethod := ""
		clientStream := false
		serverStream := false
		if info != nil {
			fullMethod = info.FullMethod
			clientStream = info.IsClientStream
			serverStream = info.IsServerStream
		}
		grpcLogEvent(err).
			Str("grpc_method", grpcMethod(fullMethod)).
			Str("grpc_full_method", strings.TrimSpace(fullMethod)).
			Str("grpc_code", status.Code(err).String()).
			Bool("grpc_client_stream", clientStream).
			Bool("grpc_server_stream", serverStream).
			Str("request_id", requestID).
			Str("traceparent", traceparent).
			Int64("duration_ms", time.Since(start).Milliseconds()).
			Msg("grpc_stream")
		return err
	}
}

func GRPCUnaryClientInterceptor() grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req any, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		ctx = correlation.OutgoingGRPCContext(ctx)
		requestID := correlation.RequestIDFromContext(ctx)
		traceparent := correlation.TraceparentFromContext(ctx)
		start := time.Now()
		err := invoker(ctx, method, req, reply, cc, opts...)
		grpcLogEvent(err).
			Str("grpc_method", grpcMethod(method)).
			Str("grpc_full_method", strings.TrimSpace(method)).
			Str("grpc_code", status.Code(err).String()).
			Str("request_id", requestID).
			Str("traceparent", traceparent).
			Int64("duration_ms", time.Since(start).Milliseconds()).
			Msg("grpc_client_request")
		return err
	}
}

func GRPCStreamClientInterceptor() grpc.StreamClientInterceptor {
	return func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, streamer grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {
		ctx = correlation.OutgoingGRPCContext(ctx)
		requestID := correlation.RequestIDFromContext(ctx)
		traceparent := correlation.TraceparentFromContext(ctx)
		start := time.Now()
		stream, err := streamer(ctx, desc, cc, method, opts...)
		grpcLogEvent(err).
			Str("grpc_method", grpcMethod(method)).
			Str("grpc_full_method", strings.TrimSpace(method)).
			Str("grpc_code", status.Code(err).String()).
			Bool("grpc_client_stream", desc.ClientStreams).
			Bool("grpc_server_stream", desc.ServerStreams).
			Str("request_id", requestID).
			Str("traceparent", traceparent).
			Int64("duration_ms", time.Since(start).Milliseconds()).
			Msg("grpc_client_stream")
		return stream, err
	}
}

type correlatedServerStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (s *correlatedServerStream) Context() context.Context {
	return s.ctx
}

func grpcLogEvent(err error) *zerolog.Event {
	if err == nil {
		return log.Info()
	}
	return log.Warn().Err(err)
}

func grpcMethod(fullMethod string) string {
	fullMethod = strings.TrimSpace(fullMethod)
	if fullMethod == "" {
		return ""
	}
	idx := strings.LastIndex(fullMethod, "/")
	if idx == -1 || idx == len(fullMethod)-1 {
		return fullMethod
	}
	return fullMethod[idx+1:]
}
