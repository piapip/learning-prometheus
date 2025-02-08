#!/bin/bash

# docker run --rm --name jaeger -e COLLECTOR_OTLP_ENABLED=true -p 16686:16686 -p 4317:4317 -p 4318:4318 jaegertracing/jaeger:2.2.0
OTEL_RESOURCE_ATTRIBUTES="service.name=dice,service.version=0.1.0" go run ./otel/server/main.go
