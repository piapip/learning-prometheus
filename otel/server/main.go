// Endpoint:
//
//	localhost:8090/rolldice/
//	localhost:8090/rolldice/{playerID}
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"time"

	"golang.org/x/exp/rand"

	"go.opentelemetry.io/contrib/bridges/otelslog"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/stdout/stdoutlog"
	"go.opentelemetry.io/otel/exporters/stdout/stdoutmetric"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/log/global"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	sdkLog "go.opentelemetry.io/otel/sdk/log"
	sdkMetric "go.opentelemetry.io/otel/sdk/metric"
	sdkTrace "go.opentelemetry.io/otel/sdk/trace"
)

const packageName = "learn-prometheus"

var (
	tracer  = otel.Tracer(packageName)
	meter   = otel.Meter(packageName)
	logger  = otelslog.NewLogger(packageName)
	rollCnt metric.Int64Counter
)

func init() {
	otelResAttr := os.Getenv("OTEL_RESOURCE_ATTRIBUTES")
	if otelResAttr == "" {
		panic("Missing OTEL_RESOURCE_ATTRIBUTES!!!")
	}

	var err error
	rollCnt, err = meter.Int64Counter("dice.rolls",
		metric.WithDescription("The number of rolls by roll value"),
		metric.WithUnit("{roll}"),
	)
	if err != nil {
		panic(err)
	}
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	otelShutdown, err := setupOtelSDK(ctx)
	if err != nil {
		return
	}
	defer func() {
		err = errors.Join(err, otelShutdown(ctx))
	}()

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
	case err = <-srvErr:
		// Error when the server starts.
		return
	case <-ctx.Done():
		// Wait for the first Ctrl+C.
		// Stop receiving signal notifications as soon as possible.
		stop()
	}

	// When Shutdown is called, ListenAndServe immediately returns ErrServerClosed.
	err = srv.Shutdown(context.Background())
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
	ctx, span := tracer.Start(r.Context(), "roll")
	defer span.End()

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

	logger.InfoContext(ctx, msg, "result", roll)
	rollValueAttr := attribute.Int("roll.value", roll)
	span.SetAttributes(rollValueAttr)
	rollCnt.Add(ctx, 1, metric.WithAttributes(rollValueAttr))

	resp := strconv.Itoa(roll) + "\n"
	if _, err := io.WriteString(w, resp); err != nil {
		log.Printf("Failed roll dice: %v\n", err)
	}
}

// setupOTelSDK bootstraps the OpenTelemetry pipeline.
// If it does not return an error, make sure to call shutdown for proper cleanup.
func setupOtelSDK(ctx context.Context) (shutdown func(context.Context) error, err error) {
	var shutDownFuncs []func(context.Context) error

	// shutdown calls cleanup functions registered via shutdownFuncs.
	// The errors from the calls are joined.
	// Each registered cleanup will be invoked once.
	shutdown = func(ctx context.Context) error {
		var err error

		for _, fn := range shutDownFuncs {
			err = errors.Join(err, fn(ctx))
		}

		shutDownFuncs = nil

		return err
	}

	// handleErr calls shutdown for cleanup and makes sure that all errors are returned.
	handleErr := func(inErr error) {
		err = errors.Join(inErr, shutdown(ctx))
	}

	// Set up propagator.
	prop := newPropagator()
	otel.SetTextMapPropagator(prop)

	// Set up trace provider.
	traceProvider, err := newTraceProvider()
	if err != nil {
		handleErr(err)
		return
	}

	shutDownFuncs = append(shutDownFuncs, traceProvider.Shutdown)
	otel.SetTracerProvider(traceProvider)

	// Set up meter provider.
	meterProvider, err := newMeterProvider()
	if err != nil {
		handleErr(err)
		return
	}

	shutDownFuncs = append(shutDownFuncs, meterProvider.Shutdown)
	otel.SetMeterProvider(meterProvider)

	// Set up logger provider.
	loggerProvider, err := newLoggerProvider()
	if err != nil {
		handleErr(err)
		return
	}

	shutDownFuncs = append(shutDownFuncs, loggerProvider.Shutdown)
	global.SetLoggerProvider(loggerProvider)

	return
}

func newPropagator() propagation.TextMapPropagator {
	return propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	)
}

func newTraceProvider() (*sdkTrace.TracerProvider, error) {
	traceExporter, err := stdouttrace.New(
		stdouttrace.WithPrettyPrint(),
	)
	if err != nil {
		return nil, err
	}

	traceProvider := sdkTrace.NewTracerProvider(
		sdkTrace.WithBatcher(
			traceExporter,
			// Default is 5s. Set to 1s for demonstrative purposes.
			sdkTrace.WithBatchTimeout(time.Second)),
	)

	return traceProvider, nil
}

func newMeterProvider() (*sdkMetric.MeterProvider, error) {
	metricExporter, err := stdoutmetric.New()
	if err != nil {
		return nil, err
	}

	metricProvider := sdkMetric.NewMeterProvider(
		sdkMetric.WithReader(sdkMetric.NewPeriodicReader(
			metricExporter,
			// Default is 1m. Set to 3s for demonstrative purposes.
			sdkMetric.WithInterval(3*time.Second),
		)),
	)

	return metricProvider, nil
}

func newLoggerProvider() (*sdkLog.LoggerProvider, error) {
	logExporter, err := stdoutlog.New()
	if err != nil {
		return nil, err
	}

	logProvider := sdkLog.NewLoggerProvider(
		sdkLog.WithProcessor(sdkLog.NewBatchProcessor(logExporter)),
	)

	return logProvider, nil
}
