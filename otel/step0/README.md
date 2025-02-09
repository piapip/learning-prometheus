Structure:
```
┌───────────────────────────────────┐
|                Host               |
|  ┌─────────────────────────────┐  |
|  |                             |  |           ┌─────────────────┐
|  |  ┌───────────────────────┐  |  |           |                 |
|  |  |Container with OTEL SDK├--├--├---------> │      Jaeger     │
|  |  └────────────┬──────────┘  |  |      ┌─────────┐            |
|  |               |             |  |      │  Trace  │────────────┘                            ┌─────────────────┐
|  |               |             |  |      └─────────┘                                         |                 |
|  |               ├-------------├--├------------------Prometheus scrape metric every 0.01s----│    Prometheus   │
|  |               |             |  |                    from the endpoint exposed by      ┌────────┐            |
|  |               v             |  |                            the container.            │ Metric │────────────┘
|  |  ┌───────────────────────┐  |  |                                                      └────────┘
|  |  | OTEL Collector Agent  |  |  |
|  |  └────────────┬──────────┘  |  |             ┌────────────────────────┐                   ┌─────────────────┐
|  |               |             |  |             |                        |                   |                 |
|  |               └-------------├--├-----------> │ OTEL Collector Gateway ├------------------>|       Loki      |
|  |                             |  |        ┌─────────┐                   |              ┌─────────┐            |
|  |                             |  |        │   Log   │───────────────────┘              │   Log   │────────────┘
|  └─────────────────────────────┘  |        └─────────┘                                  └─────────┘
|                                   |
└───────────────────────────────────┘
```

NOTE:

Source: I didn't follow word by word any documentation, it's a combination of:
1. https://opentelemetry.io/docs/languages/go/instrumentation/ (Make some customized trace/metric/log provider following the 3 otel docs first.)
2. https://opentelemetry.io/docs/languages/go/exporters/
3. https://opentelemetry.io/docs/languages/go/resources/
4. To ingest data with external source, dig the Internet for help.
5. Ingest tracing with Jaeger: https://github.com/antonputra/tutorials/tree/main/lessons/178
6. Ingest metrics with Prometheus: https://github.com/open-telemetry/opentelemetry-go-contrib/tree/main/examples
7. Ingest Logs with Loki: https://grafana.com/docs/loki/latest/send-data/otel/otel-collector-getting-started/

[6] Don't even bother trying to install Loki the proper way, just docker it up.

Run this thing blindly following this documentation to export the logs to Loki in Grafana.
https://grafana.com/docs/loki/latest/send-data/otel/otel-collector-getting-started/
1. Do pretty much w/e it says.
2. About instrumentating Loki into OTEL, based on the video linked in the Getting Started page, there's not much needed to be done, making the default LoggerProvider and expose it globally is enough, the otel-collector and its otel-config will handle the rest of the heavy lifting. The video doesn't say that explicitly, but I kinda guess so.
3. The original docker-compose file in the video contains more than I ask for, I already have grafana locally, so I removed the grafana part from the docker-compose, and install the Loki in my Grafana myself. Loki is already Grafana's built-in stuff so no extra download required.
4. The prick in otel-collector also takes up the Jaeger port, can I have one at a time only? Either Jaeger at 4318 or OTEL Collector for Loki in 4318?
- The logs are exported to Loki via http://localhost:4318/v1/logs
- The traces are exported to Jaeger via http://localhost:4318/v1/traces

So what's up?

It turns out that I can export the traces via another port, I selected 4319 for this one, and update the Jaeger port-forwarding to support that shenanigan.

Command I ran were:
```bash
# Spin the Loki support in 1 terminal
docker-compose -f ./otel/step0/loki/docker-compose.yaml up -d
```

```bash
# Spin the Jaeger in another terminal
# Note that I have to port-forward port 4320 to Jaeger's gRPC 4317
# and HTTP 4319 to Jaeger's HTTP 4318 to avoid the port conflict.
docker run --rm --name jaeger -e COLLECTOR_OTLP_ENABLED=true -p 16686:16686 -p 4320:4317 -p 4319:4318 jaegertracing/jaeger:2.2.0
```

```bash
# Spin the application in another terminal
./otel/step0/run.sh
```

![Jaeger and Loki working well together](./Screenshot%20from%202025-02-08%2001-57-56.png?raw=true)

![Jaeger and Loki working well together](./Screenshot%20from%202025-02-08%2001-58-05.png?raw=true)
