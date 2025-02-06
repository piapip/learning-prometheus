package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/exp/rand"

	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/sdk/resource"
	sdkTrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

var tracer trace.Tracer

const packageName = "learn-prometheus"

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	exp, err := newJaegerExporter(ctx)
	// exp, err := newConsoleExporter()
	if err != nil {
		panic(err)
	}

	// Create a new tracer provider with a batch span processor and the given exporter.
	traceProvider := newTraceProvider(exp)
	defer func() {
		traceProvider.Shutdown(ctx)
	}()

	otel.SetTracerProvider(traceProvider)

	tracer = traceProvider.Tracer(packageName)

	// Start HTTP Server
	srv := &http.Server{
		Addr:         ":8090",
		BaseContext:  func(_ net.Listener) context.Context { return ctx },
		ReadTimeout:  time.Second,
		WriteTimeout: 10 * time.Second,
		Handler:      newHTTPHandler(),
	}
	srvErr := make(chan error, 1)
	go func() {
		srvErr <- srv.ListenAndServe()
	}()

	// Wait for interruption.
	select {
	case err := <-srvErr:
		// Error when the server starts.
		panic(err)
		// // Error when the server starts.
		// return
	case <-ctx.Done():
		// Wait for the first Ctrl+C.
		// Stop receiving signal notifications as soon as possible.
		stop()
	}
}

func newHTTPHandler() http.Handler {
	mux := http.NewServeMux()

	// handleFunc is a replacement for mux.HandleFunc
	// which enriches the handler's HTTP instrumentation with the pattern as the http.route.
	// Consider this like a middleware.
	handleFunc := func(pattern string, handlerFunc func(http.ResponseWriter, *http.Request)) {
		// Configure the "http.route" for the HTTP instrumentation.
		handler := otelhttp.WithRouteTag(pattern, http.HandlerFunc(handlerFunc))
		mux.Handle(pattern, handler)
	}

	// Register handlers.
	handleFunc("/rolldice/", rolldice)
	handleFunc("/rolldice/{playerID}", rolldice)

	// Add HTTP instrumentation for the whole server.
	handler := otelhttp.NewHandler(mux, "/")
	return handler
}

func rolldice(w http.ResponseWriter, r *http.Request) {
	ctx, span := tracer.Start(r.Context(), "first roll")
	defer span.End()

	span.AddEvent("first roll")

	fmt.Println("Rolling dice...")
	fmt.Printf("PlayerID: [%s]\n", r.PathValue("playerID"))
	fmt.Printf("Query: [%s]\n", r.URL.Query().Get("q"))
	roll := 1 + rand.Intn(6)

	var msg string
	if playerID := r.PathValue("playerID"); playerID != "" {
		msg = fmt.Sprintf("%s is rolling the dice", playerID)
		span.SetAttributes(attribute.String("var.player.id", playerID))
	} else {
		msg = "Anonymous player is rolling the dice"
	}
	fmt.Println("msg: ", msg)

	span.SetAttributes(attribute.Int("roll.value", roll))

	resp := fmt.Sprintf("Roll 1: %s\n", strconv.Itoa(roll))
	if _, err := io.WriteString(w, resp); err != nil {
		log.Printf("Failed roll dice: %v\n", err)
	}

	rollAgain(ctx, w)
}

func rollAgain(ctx context.Context, w http.ResponseWriter) {
	_, span := tracer.Start(ctx, "second roll")
	defer span.End()

	span.AddEvent("second roll")

	roll := 7 + rand.Intn(6)
	span.SetAttributes(attribute.Int("roll.value", roll))

	resp := fmt.Sprintf("Roll 2: %s\n", strconv.Itoa(roll))
	if _, err := io.WriteString(w, resp); err != nil {
		log.Printf("Failed roll again: %v\n", err)
	}
}

// newConsoleExporter returns the exporter that output all the trace information to the console.
// fmt.Print the trace information basically.
func newConsoleExporter() (sdkTrace.SpanExporter, error) {
	return stdouttrace.New()
}

// newJaegerExporter initializes and returns a Jaeger exporter.
func newJaegerExporter(ctx context.Context) (sdkTrace.SpanExporter, error) {
	// Jaeger:
	//   4317 for gRPC
	//   4318 for HTTP API
	jaegerEndpoint := "localhost:4318"

	return otlptracehttp.New(ctx,
		// Change from HTTPS -> HTTP.
		otlptracehttp.WithInsecure(),
		otlptracehttp.WithEndpoint(jaegerEndpoint),
	)
}

func newTraceProvider(exp sdkTrace.SpanExporter) *sdkTrace.TracerProvider {
	// Ensure default SDK resources and the required service name are set.
	r, err := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName("exampleService"),
		),
	)
	if err != nil {
		panic(err)
	}

	return sdkTrace.NewTracerProvider(
		sdkTrace.WithBatcher(exp),
		sdkTrace.WithResource(r),
	)
}
