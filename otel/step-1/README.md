Architecture:

```
┌───────────────────────────────────┐
|                Host               |
|  ┌─────────────────────────────┐  |
|  |                             |  |           ┌─────────────────┐
|  |  ┌───────────────────────┐  |  |           |                 |
|  |  |Container with OTEL SDK├--├--├---------> │     Console     │
|  |  └────────────┬──────────┘  |  |      ┌─────────┐            |
|  |               |             |  |      │  Trace  │────────────┘                            ┌─────────────────┐
|  |               |             |  |      └─────────┘                                         |                 |
|  |               └-------------├--├------------------Prometheus scrape metric every 0.01s----│     Console     │
|  └─────────────────────────────┘  |                    from the endpoint exposed by      ┌────────┐            |
|                                   |                            the container.            │ Metric │────────────┘
└───────────────────────────────────┘                                                      └────────┘
```

NOTE:

Source: https://opentelemetry.io/docs/languages/go/getting-started/
