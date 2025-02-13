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

Mainly to add the OTEL Gateway in this attempt, so everything in the code only needs to go through 1 port. Everything else will be done via the config file. Sounds like the infra as code solution. Also in step0, the application is still exposing metrics data via some port, now OTEL does that automatically. "Automatically" means more magic huffing.
```
/\___/\
(๑>ヮ<๑) 
/ > 🍪 Ohhh! A cookie!
```

Also, there are so many ports being mentioned in the docker-compose file that I don't know their purposes.

Source: https://www.youtube.com/watch?v=L_gjG4BjvSE&list=PLNxnp_rzlqf6z1cC0IkIwp6yjsBboX945&ab_channel=Aspecto

The applications produces the metric/trace/log data to only the localhost:4318 port. The instrumentation is so samey now. Which is so nice coding wise I suppose.
The OTEL gateway then collects from that 4318 port via the otel-collector-gateway in the docker-compose file. Then it forwards the data via the otel-config-gateway.yml file so other Jaeger, Prometheus, Loki will now only look at that 4318 port and gets their respective data.

Application --> gateway (4318)
- Gateway--metric--> localhost:8889/metrics <<< Prometheus is constantly checking this endpoint
- Gateway--traces--> Export directly to Jaeger via localhost:4317.
- Gateway--Loki--> Export directly to Loki via http://loki:3100/otlp.

The exported metric data is altered compared to step0 data btw, go to localhost:8889/metrics to check the altered content.

Also, the configuration for metrics is weird... To check if the prometheus has the correct scrape_config, go to the scrape_config_link/metrics to see if you get the seemingly correct data.
