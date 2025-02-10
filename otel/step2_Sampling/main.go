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

	"go.opentelemetry.io/contrib/bridges/otelslog"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/log/global"
	"go.opentelemetry.io/otel/metric"
	sdkLog "go.opentelemetry.io/otel/sdk/log"
	sdkMetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdkTrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
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

	// Ensure default SDK resources and the required service name are set.
	// This resource will override the OTEL_RESOURCE_ATTRIBUTES I have in the run.sh btw.
	// Say, if I don't pass the resource to the logger, the service_name will be "dice", instead of "exampleService".
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
	traceProvider, err := newTraceProvider(ctx, r)
	if err != nil {
		panic(err)
	}
	defer func() {
		traceProvider.Shutdown(ctx)
	}()

	otel.SetTracerProvider(traceProvider)

	tracer = traceProvider.Tracer(packageName)

	meterProvider, err := newMeterProvider(ctx, r)
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

	// // Start another Server to export prometheus metrics.
	// // Otherwise, Jaeger will be spammed by Prometheus constantly scraping data by calling localhost:8090/metrics.
	// go func() {
	// 	log.Printf("serving metrics at localhost:8091/metrics")
	// 	http.Handle("/metrics", promhttp.Handler())
	// 	err := http.ListenAndServe(":8091", nil)
	// 	if err != nil {
	// 		fmt.Printf("error serving http: %v", err)
	// 		return
	// 	}
	// }()

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
	handleFunc("/rolldice/{playerID}/{tier}", rolldice)

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
	ctx, span := tracer.Start(
		r.Context(),
		"first roll",
		// Workaround for the Head sampling.
		// It can be tagged outside and the Tail sampling will still work, not the Head though.
		trace.WithAttributes(attribute.Key("tier").String(r.PathValue("tier"))),
	)
	defer span.End()

	startTime := time.Now()

	opt := metric.WithAttributes(
		attribute.Key("handler").String("/rolldice"),
		attribute.Key("another_label").String("another_value"),
	)

	span.AddEvent("first roll")
	apiCnt.Add(ctx, 1, opt)

	// If the URL is like: /rolldice?fail=w.e
	// The "fail" value can be anything for me to fail this endpoint.
	manualFail := r.URL.Query().Get("fail")

	if len(manualFail) != 0 {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("500 - Something bad happened!"))
		span.RecordError(fmt.Errorf("some nasty internal error"))

		return
	}

	fmt.Println("Rolling dice...")
	fmt.Printf("PlayerID: [%s]\n", r.PathValue("playerID"))
	fmt.Printf("Tier: [%s]\n", r.PathValue("tier"))
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

	rollAgain(ctx, r, w)

	span.AddEvent("some expensive operation starting...")
	time.Sleep(time.Duration(rand.Intn(100)) * time.Millisecond)

	duration := time.Since(startTime)

	// 4.
	rollHistogram.Record(ctx, duration.Seconds(), opt)
}

// rollAgain generates a random (dice + 6) look-a-like value.
// To demo:
// - Connecting traces.
//
// Because "second roll" is the child span, it doesn't inherit any god damn attribute from the parent trace,
// and its Head sampling doesn't contain the "http.target" data, so I'll need to reassign the attribute for this one.
func rollAgain(ctx context.Context, r *http.Request, w http.ResponseWriter) {
	_, span := tracer.Start(ctx, "second roll",
		// Workaround for the Head sampling.
		// It can be tagged outside and the Tail sampling will still work, not the Head though.
		trace.WithAttributes(attribute.Key("tier").String(r.PathValue("tier"))),
	)
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

// newTraceExporter initializes and returns a OTEL Gateway exporter for trace.
func newTraceExporter(ctx context.Context) (sdkTrace.SpanExporter, error) {
	otelGatewayExporterEndpoint := "localhost:4318"

	return otlptracehttp.New(ctx,
		// Change from HTTPS -> HTTP.
		otlptracehttp.WithInsecure(),
		otlptracehttp.WithEndpoint(otelGatewayExporterEndpoint),
	)
}

// // headSampler is the struct that implement the custom Head sampler that:
// // - Only sample trace for the premium users, no pleb allowed.
// type headSampler struct{}

// // isPremiumTier checks if the endpoint belongs to the premium users.
// // If this is one of the usecase, it's better to leave tier either at the beginning of the paramters or at the end.
// // This is just for learning, so this implementation will miss like 10_000 edge cases.
// // This is very unrealistic implementation, and this is the task for the Tail sampling with the string_attribute condition.
// func isPremiumTier(endpoint string) bool {
// 	// Hey, idgaf.
// 	return strings.Contains(endpoint, "premium")

// }

// // ShouldSample makes a decision if this trace will be kept upon its initiation.
// // ShouldSample is as the required function for the sdkTrace.SamplingResult interface.
// func (s *headSampler) ShouldSample(parameters sdkTrace.SamplingParameters) sdkTrace.SamplingResult {
// 	fmt.Printf("Name: %s,len: %+v\n", parameters.Name, len(parameters.Attributes))
// 	for i, attr := range parameters.Attributes {
// 		// Print it out to see how I know to get the "http.target" key.
// 		fmt.Printf("%d. Key: {%s}, Value: {%s}\n", i+1, attr.Key, attr.Value.AsString())
// 		if (attr.Key == attribute.Key("tier") && attr.Value.AsString() == "premium") ||
// 			(attr.Key == attribute.Key("http.target") && isPremiumTier(attr.Value.AsString())) {
// 			fmt.Println("got em")
// 			return sdkTrace.SamplingResult{
// 				Decision:   sdkTrace.RecordAndSample,
// 				Attributes: parameters.Attributes,
// 			}
// 		}
// 	}
// 	fmt.Println("missed :'(")

// 	return sdkTrace.SamplingResult{
// 		Decision: sdkTrace.Drop,
// 	}
// }

// // Description is as the required function for the sdkTrace.SamplingResult interface.
// func (s *headSampler) Description() string {
// 	return "where is this being shown?"
// }

// // newHeadSampler returns a Head sampler that will always collect traces of the paid user, aka containing the premium=true in the parameter.
// //
// // THIS IS NOT HOW HEAD SAMPLING WORK! This is a workaround so I can try Head sampling. It's Tail sampling's appendix. Just cut it off and forget about it.
// func newHeadSampler() sdkTrace.Sampler {
// 	return &headSampler{}
// }

func newTraceProvider(ctx context.Context, res *resource.Resource) (*sdkTrace.TracerProvider, error) {
	exp, err := newTraceExporter(ctx)
	// exp, err := newConsoleExporter()
	if err != nil {
		return nil, err
	}

	return sdkTrace.NewTracerProvider(
		sdkTrace.WithBatcher(exp),
		sdkTrace.WithResource(res),
		// Uncomment this to try Head sampling.
		// sdkTrace.WithSampler(newHeadSampler()),
	), nil
}

// newMetricExporter initializes and returns a OTEL Gateway exporter for metric.
func newMetricExporter(ctx context.Context) (sdkMetric.Exporter, error) {
	// otelGatewayExporterEndpoint := "localhost:4318/v1/metrics"
	otelGatewayExporterEndpoint := "localhost:4318"

	return otlpmetrichttp.New(ctx,
		otlpmetrichttp.WithInsecure(),
		otlpmetrichttp.WithEndpoint(otelGatewayExporterEndpoint),
	)
}

func newMeterProvider(ctx context.Context, res *resource.Resource) (*sdkMetric.MeterProvider, error) {
	exp, err := newMetricExporter(ctx)
	if err != nil {
		return nil, err
	}

	meterProvider := sdkMetric.NewMeterProvider(
		sdkMetric.WithResource(res),
		sdkMetric.WithReader(
			sdkMetric.NewPeriodicReader(
				exp,
				// Default is 1m. Set to 3s for demonstrative purposes.
				sdkMetric.WithInterval(3*time.Second),
			),
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
		// If I comment out this WithResource,
		// the exported logs in Loki will belong to the service named "dice", defined in run.sh's hacky OTEL_RESOURCE_ATTRIBUTES export,
		// instead of the one defined on the top named "exampleService".
		sdkLog.WithResource(res),
		sdkLog.WithProcessor(sdkLog.NewBatchProcessor(logExporter)),
	)

	return logProvider, nil
}
