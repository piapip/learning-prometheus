Architecture:
```
┌───────────────────────────────────┐
|                Host               |
|  ┌─────────────────────────────┐  |
|  |                             |  |
|  |  ┌───────────────────────┐  |  |
|  |  |Container with OTEL SDK|  |  |                                               
|  |  └────────────┬──────────┘  |  |                                                    ┌─────────────────┐
|  |               |             |  |                                                    |                 |
|  |               |             |  |           ┌────────────────────────┐               │      Jaeger     │
|  |               |             |  |           |                        |          ┌─────────┐            |
|  |               └-------------├--├---------> │ OTEL Collector Gateway ├--------->│  Trace  │────────────┘
|  |                             |  |      ┌─────────┐                   |          └─────────┘
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

The applications produces the metric/trace/log data to onlly the localhost:4318 port.
The OTEL gateway then collects from that port via the otel-collector-gateway in the docker-compose file. Then it forwards the data via the otel-config-gateway.yml file so other Jaeger, Prometheus, Loki will now only look at that 4318 port and gets 

Application --> gateway (4318)
- Gateway--metric--> localhost:8889/metrics <<< Prometheus is constantly checking this endpoint
- Gateway--traces--> Export directly to Jaeger via localhost:4317.
- Gateway--Loki--> Export directly to Loki via http://loki:3100/otlp.

The exported metric data is altered compared to step0 data btw, go to localhost:8889/metrics to check the altered content.

Also, the configuration for metrics is weird... To check if the prometheus has the correct scrape_config, go to the scrape_config_link/metrics to see if you get the seemingly correct data.
