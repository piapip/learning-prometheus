#!/bin/bash

# Docker to spin up:
# - ElasticSearch to store and load persisted tracing data.
# - Jaeger to ingest tracing data.
# - Loki to ingest logging data forwarded from the OTEL gateway.
# - OTEL Gateway to forward the OTEL Agent's traces/metrics/logging to somewhere else, like Loki.
# - OTEL Agent to forward application traces/metrics/logging data to OTEL Gateway.
# 
# QUESTION: Why don't just forward everything to OTEL Gateway?

# Prometheus in the prometheus-server
#   ./prometheus --config.file=/home/piapip/Desktop/Tutorial/prometheus/otel/step1.5_OTELAgent_ElasticSearch/docker/prometheus.yml

OTEL_RESOURCE_ATTRIBUTES="service.name=dice,service.version=0.1.0" go run ./otel/step1.5_OTELAgent_ElasticSearch/main.go
