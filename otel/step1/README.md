Structure:
```
┌───────────────────────────────────┐
|                Host               |
|  ┌─────────────────────────────┐  |
|  |                             |  |
|  |  ┌───────────────────────┐  |  |
|  |  |Container with OTEL SDK|  |  |                                               
|  |  └────────────┬──────────┘  |  |                                                    ┌─────────────────┐
|  |               |             |  |                                                    |                 |
|  |               v             |  |           ┌────────────────────────┐               │      Jaeger     │
|  |  ┌───────────────────────┐  |  |           |                        |          ┌─────────┐            |
|  |  | OTEL Collector Agent  ├--├--├---------> │ OTEL Collector Gateway ├--------->│  Trace  │────────────┘
|  |  └───────────────────────┘  |  |      ┌─────────┐                   |          └─────────┘
|  |                             |  |      |  Trace  │───────┬───────────┘                              ┌─────────────────┐
|  └─────────────────────────────┘  |      ├─────────┤       ^  Prometheus scrape metric every 0.01s    |                 |
|                                   |      │  Metric │       └------from the endpoint exposed by--------│    Prometheus   │
└───────────────────────────────────┘      ├─────────┤                    the gateway.              ┌────────┐            |
                                           │   Log   │                                              │ Metric │────────────┘
                                           └────┬────┘                                              └────────┘
                                                |                                                        ┌─────────────────┐
                                                |                                                        |                 |
                                                └------------------------------------------------------->|       Loki      |
                                                                                                    ┌─────────┐            |
                                                                                                    │   Log   │────────────┘
                                                                                                    └─────────┘
```

NOTE:

Source: https://www.youtube.com/watch?v=L_gjG4BjvSE&list=PLNxnp_rzlqf6z1cC0IkIwp6yjsBboX945&ab_channel=Aspecto

Otel colletor gateway now exposes a butt load amount of port...

The applications produces the metric/trace/log data to the localhost:4318 port.
The collector then collects from that port via the otel-collector-gateway in the docker-compose file. Then it forwards the data via the otel-config-gateway.yml file.

Application --> gateway (4318)
  --metric--> localhost:8889/metrics <<< Prometheus is constantly checking this endpoint
  --traces--> Export directly to Jaeger via localhost:4317.
  --Loki--> Export directly to Loki via http://loki:3100/otlp.

The exported metric data is altered compared to step0 data btw, go to localhost:8889/metrics to check the altered content.
