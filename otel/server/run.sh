#!/bin/bash

# docker run --rm --name jaeger -p 16686:16686 -p 4317:4317 -p 4318:4318 jaegertracing/all-in-one:latest
OTEL_RESOURCE_ATTRIBUTES="service.name=dice,service.version=0.1.0" go run ./otel/server/main.go
