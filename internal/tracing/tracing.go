package tracing

import (
	"context"

	"go.opentelemetry.io/contrib/propagators/ot"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

type TracingConfig struct {
	Enabled        bool
	Endpoint       string
	ErrorHandler   func(error)
	Insecure       bool
	ServiceName    string
	ServiceVersion string
}

type logger interface {
	Println(v ...interface{})
}

func Instrument(config TracingConfig, l logger) (func(), error) {
	if config.ErrorHandler != nil {
		otel.SetErrorHandler(otel.ErrorHandlerFunc(config.ErrorHandler))
	}

	if config.Enabled {
		ctx := context.Background()

		otlpOptions := []otlptracegrpc.Option{otlptracegrpc.WithEndpoint(config.Endpoint)}
		if config.Insecure {
			otlpOptions = append(otlpOptions, otlptracegrpc.WithInsecure())
		}

		driver := otlptracegrpc.NewClient(otlpOptions...)

		exporter, err := otlptrace.New(ctx, driver)
		if err != nil {
			return nil, err
		}

		attributes := []attribute.KeyValue{attribute.String("service.name", config.ServiceName)}

		if config.ServiceVersion != "" {
			attributes = append(attributes, attribute.String("service.version", config.ServiceVersion))
		}

		resource, err := resource.New(ctx, resource.WithAttributes(attributes...))
		if err != nil {
			return nil, err
		}

		batchSpanProcessor := sdktrace.NewBatchSpanProcessor(exporter)

		tracerProvider := sdktrace.NewTracerProvider(
			sdktrace.WithResource(resource),
			sdktrace.WithSpanProcessor(batchSpanProcessor),
		)

		//TODO use structured logging
		// l.Println("Tracing enabled, configuring tracing provider", kvp.String("endpoint", appConfig.TracingEndpoint))
		otel.SetTracerProvider(tracerProvider)

		ot := ot.OT{}

		otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
			propagation.TraceContext{}, // W3C for compatibility with other tracing systems
			propagation.Baggage{},      // W3C baggage support
			ot,                         // OpenTracing support
		))

		return func() {
			err := tracerProvider.Shutdown(ctx)
			if err != nil {
				l.Println("failed to stop tracer", err)
			}
		}, nil
	}

	l.Println("Tracing disabled, configuring noop tracing provider")
	otel.SetTracerProvider(trace.NewNoopTracerProvider())
	return func() {}, nil
}
