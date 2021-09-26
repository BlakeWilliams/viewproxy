package tracinghooks

import (
	"context"

	"github.com/blakewilliams/viewproxy"
	"github.com/blakewilliams/viewproxy/pkg/multiplexer"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

func AddServeHTTPHook(server *viewproxy.Server) {
	server.Notifier.Around(viewproxy.EventServeHTTP, func(ctx context.Context, f func(ctx context.Context)) {
		tracer := otel.Tracer("server")
		var span trace.Span
		ctx, span = tracer.Start(ctx, "ServeHTTP")
		defer span.End()

		f(ctx)
	})
}

func AddMultiplexerFetchAllHook(server *viewproxy.Server) {
	server.Notifier.Around(multiplexer.EventFetchAll, func(ctx context.Context, f func(ctx context.Context)) {
		tracer := otel.Tracer("multiplexer")
		var span trace.Span
		ctx, span = tracer.Start(ctx, "fetch_urls")
		defer span.End()

		f(ctx)
	})
}

func AddMultiplexerFetchSingleHook(server *viewproxy.Server) {
	server.Notifier.Around(multiplexer.EventFetchSingle, func(ctx context.Context, f func(ctx context.Context)) {
		tracer := otel.Tracer("multiplexer")
		var span trace.Span
		ctx, span = tracer.Start(ctx, "fetch_url")

		requestable := multiplexer.RequestableFromContext(ctx)

		for key, value := range requestable.Metadata() {
			span.SetAttributes(attribute.KeyValue{
				Key:   attribute.Key(key),
				Value: attribute.StringValue(value),
			})
		}
		defer span.End()

		f(ctx)
	})
}
