Architecture:
```
┌───────────────────────────────────┐
|                Host               |
|  ┌─────────────────────────────┐  |
|  |                             |  |
|  |  ┌───────────────────────┐  |  |
|  |  |Container with OTEL SDK|  |  |                                               
|  |  └────────────┬──────────┘  |  |                                                    ┌─────────────────┐             ┌─────────────────┐
|  |               |             |  |                                                    |                 |             |                 |
|  |               v             |  |           ┌────────────────────────┐               │      Jaeger     ├----Store--->│  ElasticSearch  |
|  |  ┌───────────────────────┐  |  |           |                        |          ┌─────────┐            |<---Reload---┤                 |
|  |  | OTEL Collector Agent  ├--├--├---------> │ OTEL Collector Gateway ├--------->│  Trace  │────────────┘             └─────────────────┘
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

- Source: https://www.youtube.com/watch?v=d2OtY9OX8cA&list=PLNxnp_rzlqf6z1cC0IkIwp6yjsBboX945&index=9&ab_channel=Aspecto
- The server now will start a websocket. Calling a certain API will call to that websocket, send a message and cancel the socket connection.
