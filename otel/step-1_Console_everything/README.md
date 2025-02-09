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

- All the Traces/Metrics/Logging are now logged to console.
- OTEL is like magic, and the SDK is the magic wand, and I'm the villager yelling "burn the witch" with a pitchfork. Kinda impossible to do this by myself if not following the getting started documentation words by words. ₍^. .^₎⟆
```
                            ╱|、
                          (˚ˎ 。7  
                           |、˜〵          
                          じしˍ,)ノ
```
