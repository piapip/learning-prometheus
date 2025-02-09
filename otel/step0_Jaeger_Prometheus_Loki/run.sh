#!/bin/bash

# Docker to spin up:
# - Jaeger to ingest tracing data.
# - Loki to ingest logging data forwarded from the OTEL gateway.
# - OTEL gateway to forward the application's logging to somewhere else, like Loki.

# Prometheus in the prometheus-server, may need to adjust the prometheus.yml file to match with the export endpoint.
# ./prometheus --config.file=/home/piapip/Desktop/Tutorial/prometheus/otel/step0_Jaeger_Prometheus_Loki/docker/prometheus.yml

OTEL_RESOURCE_ATTRIBUTES="service.name=dice,service.version=0.1.0" go run ./otel/step0_Jaeger_Prometheus_Loki/main.go
