#!/bin/bash

# Prometheus in the prometheus-server
#   ./prometheus --config.file=/home/piapip/Desktop/Tutorial/prometheus/otel/step1/docker/prometheus.yml

OTEL_RESOURCE_ATTRIBUTES="service.name=dice,service.version=0.1.0" go run ./otel/step1/main.go
