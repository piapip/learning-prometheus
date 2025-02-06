// docker run --rm --name jaeger -p 16686:16686 -p 4317:4317 -p 4318:4318 -p 5778:5778 -p 9411:9411 jaegertracing/jaeger:2.2.0

package main

import (
	"fmt"
	"math/rand"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func main() {
	pingCounter := prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "ping_request_count",
			Help: "Number of request handled by Ping handler",
		},
	)

	histogramVec := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "ping_process",
			Help:    "Histogram of the ping process",
			Buckets: []float64{0.001, 0.005, 0.01, 0.02, 0.03, 0.05, 0.1, 0.2, 0.5, 1, 5},
		},
		[]string{"endpoint", "handler"},
	)

	prometheus.MustRegister(pingCounter)
	prometheus.MustRegister(histogramVec)

	http.HandleFunc("/ping", func(w http.ResponseWriter, r *http.Request) {
		pingCounter.Inc()

		startHandlingTime := time.Now().UTC()

		ping(w, r)

		histogramVec.WithLabelValues("/ping", "normalPing").Observe(time.Since(startHandlingTime).Seconds())
	})

	http.HandleFunc("/pingPing", func(w http.ResponseWriter, r *http.Request) {
		pingCounter.Inc()

		startHandlingTime := time.Now().UTC()

		ping(w, r)
		time.Sleep(100 * time.Millisecond)

		histogramVec.WithLabelValues("/pingPing", "heavyPing").Observe(time.Since(startHandlingTime).Seconds())
	})

	http.Handle("/metrics", promhttp.Handler())

	http.ListenAndServe(":8090", nil)
}

func ping(w http.ResponseWriter, _ *http.Request) {
	rand.New(rand.NewSource(time.Now().UnixNano()))
	n := rand.Intn(100) // n will be between 0 and 1000
	fmt.Printf("Sleeping %d milliseconds...\n", n)
	time.Sleep(time.Duration(n) * time.Millisecond)
	fmt.Println("Done")

	fmt.Fprintf(w, "pong")
}
