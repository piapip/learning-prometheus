#!/bin/bash

OTEL_RESOURCE_ATTRIBUTES="service.name=dice,service.version=0.1.0" go run ./otel/step-1_Console_everything/main.go
