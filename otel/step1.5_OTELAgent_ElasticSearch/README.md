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

Source: https://www.youtube.com/watch?v=L_gjG4BjvSE&list=PLNxnp_rzlqf6z1cC0IkIwp6yjsBboX945&ab_channel=Aspecto

- Getting used to the concept of networkings in docker with this one.
- So far up until now, jaeger's tracing data is still being stored in the memory, it's not persistent data. Will need to export the data to somewhere else, like ElasticSearch.
- The Jaeger image of jaegertracing/jaeger:2.2.0 was fine until I need ElasticSearch. To have ElasticSearch working, I'll need to setup collector and agent for Jaeger, like how to have Loki working, I need both otel agent and gateway. So for simplicity of not having to set up another millions containers in docker-compose just for learning, I'll use the convenient all-in-one.
- In this demo, Jaeger's restart policy is set to on-failure. This is a pretty bad workaround. In production, we'll always have the ElasticSearch deployed before Jaeger is even started. Also Jaeger on production will be splitted into smaller pieces, so if ElasticSearch is collapse for who-know-what, we'll have a better gracious shutdown.
- With ElasticSearch running now, even if I shutdown my docker, the next spin up will contain the previous data.
- To check ElasticSearch data, go to this endpoint: `http://localhost:9200/_cat/indices?v` or `http://localhost:9200/_cat/indices/{index}?v` for example: `http://localhost:9200/_cat/indices/jaeger-service-2025-02-09?v`. It doesn't show concrete data, but close enough. Check this `http://localhost:9200/_cat` to see all the ElasticSearch available endpoints.
- For a clearer ElasticSearch nonsense, you can checkout this endpoint `http://localhost:9200/{index}/_search` for example: `http://localhost:9200/jaeger-service-2025-02-09/_search`.

```
 ∧,,,∧
(_•·•_)
/ づ♡ I love you 100%!
```
