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
	"go.opentelemetry.io/otel/propagation"
	sdkLog "go.opentelemetry.io/otel/sdk/log"
	sdkMetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdkTrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/net/websocket"
)

var (
	port          = 8090
	tracer        trace.Tracer
	meter         metric.Meter
	rollCnt       metric.Int64Counter
	rollHistogram metric.Float64Histogram
	apiCnt        metric.Int64Counter
	cpuSpeedGauge metric.Int64Gauge
	logger        *slog.Logger

	// I make another tracer to visually separate the HTTP and the websocket trace.
	// There's no other intention.
	socketTracer trace.Tracer
	// defaultPropagators will be used for extracting and injecting context.
	// Because the websocket process is separated from the HTTP handler,
	// so to connect the span of the websocket process to the span of the http handler,
	// I'll use this defaultPropagators to extract the trace data and inject it to the websocket process.
	defaultPropagators = []propagation.TextMapPropagator{
		propagation.TraceContext{},
		propagation.Baggage{},
	}
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

	///////////////////////////////////////////////////
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
	///////////////////////////////////////////////////
	socketRes, err := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName("socketExampleService"),
			semconv.ServiceVersion("0.1.0"),
		),
	)
	if err != nil {
		panic(err)
	}

	socketTraceProvider, err := newTraceProvider(ctx, socketRes)
	if err != nil {
		panic(err)
	}
	defer func() {
		traceProvider.Shutdown(ctx)
	}()

	socketTracer = socketTraceProvider.Tracer(packageName)
	///////////////////////////////////////////////////
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
	///////////////////////////////////////////////////
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
		Addr:         fmt.Sprintf(":%d", port),
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

	socketServer := NewSocketServer()

	// handleHTTPFunc is a replacement for mux.HandleFunc
	// which enriches the handler's HTTP instrumentation with the pattern as the http.route.
	// Consider this like a middleware.
	handleHTTPFunc := func(pattern string, handlerFunc func(http.ResponseWriter, *http.Request)) {
		// Configure the "http.route" for the HTTP instrumentation.
		handler := otelhttp.WithRouteTag(pattern, http.HandlerFunc(handlerFunc))
		mux.Handle(pattern, handler)
	}

	// Register handlers.
	handleHTTPFunc("/rolldice/", rolldice)
	handleHTTPFunc("/rolldice/{playerID}", rolldice)
	handleHTTPFunc("/rolldice/{playerID}/{tier}", rolldice)

	handleSocketFunc := func(pattern string, handlerFunc func(*websocket.Conn)) {
		handler := otelhttp.WithRouteTag(pattern, websocket.Handler(handlerFunc))
		mux.Handle(pattern, handler)
	}

	// HTTP will create a connection to the socket later.
	// Like I shake my own hand.
	handleSocketFunc("/ws", socketServer.rolldiceSocket)

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
	time.Sleep(time.Duration(rand.Intn(50)) * time.Millisecond)

	hitSocket(ctx, r, w)

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

	time.Sleep(time.Duration(rand.Intn(50)) * time.Millisecond)

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

// hitSocket will create a client and send a signal to roll dice to the websocket server.
// After the client receives the feedback from the websocket server, it'll terminate itself.
func hitSocket(ctx context.Context, r *http.Request, w http.ResponseWriter) {
	ctx, span := tracer.Start(ctx, "client signal to roll",
		// Workaround for the Head sampling.
		// It can be tagged outside and the Tail sampling will still work, not the Head though.
		trace.WithAttributes(attribute.Key("tier").String(r.PathValue("tier"))),
	)
	defer span.End()

	socket, err := websocket.Dial(fmt.Sprintf("ws://localhost:%d/ws", port), "", "http://localhost:3002")
	if err != nil {
		panic(err)
	}

	// HTTP --call--> socket
	// HTTP <--response-- socket

	serverClosed := make(chan struct{})

	go subscribeSocket(socket, serverClosed, w)

	// Send to the server a signal to roll the dice, the content doesn't matter.
	logger.InfoContext(ctx, "signal the websocket server to roll the dice...")

	// Inject the trace context to the carrier.
	// HeaderCarrier is fine too, but I like Mapcarrier because it seems simplers.
	// headerData := make(http.Header)
	// carrier := propagation.HeaderCarrier(headerData)
	traceData := make(map[string]string)
	carrier := propagation.MapCarrier(traceData)

	for _, propagator := range defaultPropagators {
		propagator.Inject(ctx, carrier)
	}

	fmt.Printf("traceData after: %+v\n", traceData)
	err = websocket.JSON.Send(socket, &traceData)
	if err != nil {
		panic(err)
	}

	// This is a very dangerous implementation pattern that may cause memory leak,
	// due to the serverClosed channel is not closed via the defer.
	// It's fine here, but imagine there's an error that can terminate the process above,
	// then this implementation is not good anymore.
	for range serverClosed {
		fmt.Println("done hitting socket...")
		close(serverClosed)
	}
}

// subscribeSocket waits for the response from the client.
// If the server is closed, this will push some data to the serverChan to signal the program to terminate.
func subscribeSocket(ws *websocket.Conn, serverChan chan struct{}, w http.ResponseWriter) {
	for {
		var message string

		err := websocket.Message.Receive(ws, &message)
		if err == io.EOF {
			serverChan <- struct{}{}
			return
		} else if err != nil {
			panic(err)
		}

		resp := fmt.Sprintf("Roll socket: %s\n", message)
		if _, err := io.WriteString(w, resp); err != nil {
			log.Printf("Failed roll again: %v\n", err)
		}
		// Add a signal to terminate this channel manually.
		// This client is meant to send 1 signal and consume 1 signal,
		// there's no point keeping this one alive. So send a signal to serverChan to close it.
		serverChan <- struct{}{}
	}
}

type SocketServer struct{}

func NewSocketServer() *SocketServer {
	return &SocketServer{}
}

// rolldiceSocket roll the dice when receive the signal from the client,
// then return the roll value to the client.
func (s *SocketServer) rolldiceSocket(ws *websocket.Conn) {
	ctx := context.Background()

	logger.InfoContext(ctx, "waiting for client's signal to roll dice...")

	// After the connection is established, the server will roll the dice,
	// the return the dice result to the client.
	for {
		var traceData map[string]string
		err := websocket.JSON.Receive(ws, &traceData)
		// // Using head or map for trace context propagation is fine,
		// // I use map for simplicity.
		// var headerData http.Header
		// err := websocket.JSON.Receive(ws, &headerData)
		// This will occur when the connection is close from the other side.
		if err == io.EOF {
			fmt.Printf("Connection %s close...\n", ws.RemoteAddr().String())
			break
		}

		fmt.Printf("receive traceData: %+v\n", traceData)

		// carrier := propagation.HeaderCarrier(headerData)
		carrier := propagation.MapCarrier(traceData)

		// Extract trace context from HTTP request headers
		for _, propagator := range defaultPropagators {
			ctx = propagator.Extract(ctx, carrier)
		}

		_, span := socketTracer.Start(
			ctx,
			"server receive roll request via socket",
			// Set as premium so it can be shown in trace.
			// Normally, this should be inferred by checking the data of the end user,
			// or the content the end user sent to us, but for now, I don't care about that.
			trace.WithAttributes(attribute.Key("tier").String("premium")),
		)

		time.Sleep(time.Duration(rand.Intn(20)) * time.Millisecond)

		roll := 1 + rand.Intn(6)
		err = websocket.Message.Send(ws, strconv.Itoa(roll))
		if err != nil {
			fmt.Println("Failed to send via socket: ", err)

			return
		}

		// Some concurrent process after sending the response.
		// Like publishing PubSub events in the background.
		time.Sleep(time.Duration(rand.Intn(50)) * time.Millisecond)

		// Similar to PubSub, this process will never end until being told to.
		// So I'll have to manually end this trace.
		// Normally, this handling should be in another function so I can defer there.
		span.End()
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
