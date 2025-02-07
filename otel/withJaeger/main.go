package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"log/slog"
	"math/rand"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/contrib/bridges/otelslog"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/log/global"
	"go.opentelemetry.io/otel/metric"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"

	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	sdkLog "go.opentelemetry.io/otel/sdk/log"
	sdkMetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdkTrace "go.opentelemetry.io/otel/sdk/trace"
)

var (
	tracer        trace.Tracer
	meter         metric.Meter
	rollCnt       metric.Int64Counter
	rollHistogram metric.Float64Histogram
	apiCnt        metric.Int64Counter
	cpuSpeedGauge metric.Int64Gauge
	logger        *slog.Logger
)

const packageName = "learn-prometheus"

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	exp, err := newJaegerExporter(ctx)
	// exp, err := newConsoleExporter()
	if err != nil {
		panic(err)
	}

	// Ensure default SDK resources and the required service name are set.
	r, err := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName("exampleService"),
			semconv.ServiceVersion("0.1.0"),
		),
	)
	if err != nil {
		panic(err)
	}

	// Create a new tracer provider with a batch span processor and the given exporter.
	traceProvider := newTraceProvider(r, exp)
	defer func() {
		traceProvider.Shutdown(ctx)
	}()

	otel.SetTracerProvider(traceProvider)

	tracer = traceProvider.Tracer(packageName)

	meterProvider, err := newMeterProvider(r)
	if err != nil {
		panic(err)
	}
	defer func() {
		meterProvider.Shutdown(ctx)
	}()

	// These 2 lines:
	//   otel.SetMeterProvider(meterProvider)
	//   meter = otel.Meter(packageName)
	// are equivalent to
	//   meter = provider.Meter(packageName)
	// I'm using the former to match with the Getting Started link in the /otel/server.
	otel.SetMeterProvider(meterProvider)
	meter = otel.Meter(packageName)

	rollCnt, err = meter.Int64Counter("dice.rolls",
		metric.WithDescription("The number of rolls by roll value"),
		metric.WithUnit("{roll}"),
	)
	if err != nil {
		panic(err)
	}

	rollHistogram, err = meter.Float64Histogram(
		"roll.duration",
		metric.WithDescription("The duration to roll the dice"),
		metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(0.001, 0.005, 0.01, 0.05, 0.1, 0.2, 0.3, 0.4, 0.5, 0.6, 0.7, 0.8, 0.9, 1, 5),
	)
	if err != nil {
		panic(err)
	}

	apiCnt, err = meter.Int64Counter("api.counter",
		metric.WithDescription("Number of API calls"),
		metric.WithUnit("{call}"),
	)
	if err != nil {
		panic(err)
	}

	cpuSpeedGauge, err = meter.Int64Gauge(
		"cpu.fan.speed",
		metric.WithDescription("Speed of CPU fan"),
		metric.WithUnit("RPM"),
	)
	if err != nil {
		panic(err)
	}

	logProvider, err := newLoggerProvider(ctx, r)
	if err != nil {
		panic(err)
	}
	defer func() {
		logProvider.Shutdown(ctx)
	}()

	// Register as global logger provider so that it can be accessed global.LoggerProvider.
	// Most log bridges use the global logger provider as default.
	// If the global logger provider is not set then a no-op implementation
	// is used, which fails to generate data.
	global.SetLoggerProvider(logProvider)

	logger = otelslog.NewLogger(packageName, otelslog.WithLoggerProvider(logProvider))

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

	// Start another Server to export prometheus metrics.
	// Otherwise, Jaeger will be spammed by Prometheus constantly scraping data by calling localhost:8090/metrics.
	go func() {
		log.Printf("serving metrics at localhost:8091/metrics")
		http.Handle("/metrics", promhttp.Handler())
		err := http.ListenAndServe(":8091", nil)
		if err != nil {
			fmt.Printf("error serving http: %v", err)
			return
		}
	}()

	go func() {
		trackCPUFanSpeed(ctx)
	}()

	// Wait for interruption.
	select {
	case err := <-srvErr:
		// Error when the server starts.
		panic(err)
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

// rolldice generates a random dice look-a-like value.
// To demo:
// 1. Tracing.
// 2. Span parameter.
// 3. Meter Counter instrumentation.
// 4. Meter Histogram instrumentation.
// 5. OTEL Logging.
func rolldice(w http.ResponseWriter, r *http.Request) {
	// 1.
	ctx, span := tracer.Start(r.Context(), "first roll")
	defer span.End()

	startTime := time.Now()

	opt := metric.WithAttributes(
		attribute.Key("handler").String("/rolldice"),
		attribute.Key("another_label").String("another_value"),
	)

	span.AddEvent("first roll")
	apiCnt.Add(ctx, 1, opt)

	fmt.Println("Rolling dice...")
	fmt.Printf("PlayerID: [%s]\n", r.PathValue("playerID"))
	fmt.Printf("Query: [%s]\n", r.URL.Query().Get("q"))
	roll := 1 + rand.Intn(6)

	var msg string
	if playerID := r.PathValue("playerID"); playerID != "" {
		msg = fmt.Sprintf("%s is rolling the dice", playerID)
		// 2.
		span.SetAttributes(attribute.String("var.player.id", playerID))
	} else {
		msg = "Anonymous player is rolling the dice"
	}
	fmt.Println("msg: ", msg)

	rollValueAttr := attribute.Int("roll.value", roll)
	// 2.
	span.SetAttributes(rollValueAttr)
	// 3.
	rollCnt.Add(ctx, 1, metric.WithAttributes(rollValueAttr))
	// 5.
	logger.InfoContext(ctx, msg, "result", roll)

	resp := fmt.Sprintf("Roll 1: %s\n", strconv.Itoa(roll))
	if _, err := io.WriteString(w, resp); err != nil {
		log.Printf("Failed roll dice: %v\n", err)
	}

	rollAgain(ctx, w)

	span.AddEvent("some expensive operation starting...")
	time.Sleep(time.Duration(rand.Intn(100)) * time.Millisecond)

	duration := time.Since(startTime)

	// 4.
	rollHistogram.Record(ctx, duration.Seconds(), opt)
}

// rollAgain generates a random (dice + 6) look-a-like value.
// To demo:
// - Connecting traces.
func rollAgain(ctx context.Context, w http.ResponseWriter) {
	_, span := tracer.Start(ctx, "second roll")
	defer span.End()

	span.AddEvent("second roll")

	roll := 7 + rand.Intn(6)

	rollValueAttr := attribute.Int("roll.value", roll)
	span.SetAttributes(rollValueAttr)
	rollCnt.Add(ctx, 1, metric.WithAttributes(rollValueAttr))

	resp := fmt.Sprintf("Roll 2: %s\n", strconv.Itoa(roll))
	if _, err := io.WriteString(w, resp); err != nil {
		log.Printf("Failed roll again: %v\n", err)
	}
}

// trackCPUFanSpeed mocks the behavior of tracking CPU fan speed.
// To demo:
// - Meter Gauge instrumentation.
func trackCPUFanSpeed(ctx context.Context) {
	fanSpeedSubscription := make(chan int64, 1)
	go func() {
		defer close(fanSpeedSubscription)

		for {
			// Synchronous gauges are used when the measurement cycle is
			// synchronous to the external change.
			time.Sleep(time.Duration(3+rand.Intn(3)) * time.Second)
			fanSpeedSubscription <- getCPUFanSpeed()
		}
	}()

	opt := metric.WithAttributes(
		attribute.Key("A").String("B"),
		attribute.Key("C").String("D"),
	)

	for fanSpeed := range fanSpeedSubscription {
		cpuSpeedGauge.Record(ctx, fanSpeed, opt)
	}
}

// getCPUFanSpeed generates a random fan speed for demonstration purpose.
// In real world applications, replace this to get the actual fan speed.
func getCPUFanSpeed() int64 {
	return int64(1500 + rand.Intn(1000))
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

func newTraceProvider(res *resource.Resource, exp sdkTrace.SpanExporter) *sdkTrace.TracerProvider {
	// // Ensure default SDK resources and the required service name are set.
	// r, err := resource.Merge(
	// 	resource.Default(),
	// 	resource.NewWithAttributes(
	// 		semconv.SchemaURL,
	// 		semconv.ServiceName("exampleService"),
	// 	),
	// )
	// if err != nil {
	// 	panic(err)
	// }

	return sdkTrace.NewTracerProvider(
		sdkTrace.WithBatcher(exp),
		sdkTrace.WithResource(res),
	)
}

func newPrometheusExporter() (sdkMetric.Reader, error) {
	return prometheus.New()
}

func newMeterProvider(res *resource.Resource) (*sdkMetric.MeterProvider, error) {
	// metricExporter, err := stdoutmetric.New()
	metricExporter, err := newPrometheusExporter()
	if err != nil {
		return nil, err
	}

	meterProvider := sdkMetric.NewMeterProvider(
		// sdkMetric.WithResource(res),
		sdkMetric.WithReader(
			// sdkMetric.NewPeriodicReader(
			// 	metricExporter,
			// 	// Default is 1m. Set to 3s for demonstrative purposes.
			// 	sdkMetric.WithInterval(3*time.Second),
			// ),
			metricExporter,
		),
	)

	return meterProvider, nil
}

// This Logger Provider doesn't seem to work because it's trying to send to https://localhost:4318/v1/logs.
// Wtf is that??? It doesn't seem to be related to Jaeger.
//
// It seems like it will require another Log ingestor, like Loki, which I'll look at later.
func newLoggerProvider(ctx context.Context, res *resource.Resource) (*sdkLog.LoggerProvider, error) {
	logExporter, err := otlploghttp.New(
		ctx,
		// Change from HTTPS -> HTTP.
		otlploghttp.WithInsecure(),
	)
	if err != nil {
		return nil, err
	}

	logProvider := sdkLog.NewLoggerProvider(
		sdkLog.WithResource(res),
		sdkLog.WithProcessor(sdkLog.NewBatchProcessor(logExporter)),
	)

	return logProvider, nil
}
