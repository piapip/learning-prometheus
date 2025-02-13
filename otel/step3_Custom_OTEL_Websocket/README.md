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

- I don't really follow the source, there's no custom Instrumentation in this attempt, and I don't see the future where I'd like to ever do that. I like the idea of testing with WebSocket so I try it here.
- Source: https://www.youtube.com/watch?v=d2OtY9OX8cA&list=PLNxnp_rzlqf6z1cC0IkIwp6yjsBboX945&index=9&ab_channel=Aspecto
- The server now will start a websocket, on standing by waiting for the client connection.
- When the rolldice endpoint is triggered:
  - [first roll] The HTTP server rolls the dice and return to the client.
    - [second roll] The HTTP server rolls the dice again and return to the client.
      - The HTTP server creates a socket connection, becomes the client of its Socket server.
      - [client signal to roll] The HTTP server then sends a request to roll dice to the socket server with the details of the ongoing trace.
        - [server receive roll request via socket] The Socket server then propagate the trace to continue the trace, roll the dice, and return the result to the HTTP server to print it out.
- There's no documentation of how to inject/extract/connecting. This is done by a lot of copying, trials, and errors.

![Traces are propagated and connected](./Screenshot%20from%202025-02-13.png?raw=true)
