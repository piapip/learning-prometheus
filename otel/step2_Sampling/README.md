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

- Source: https://www.youtube.com/watch?v=tb6VHrihPZI&list=PLNxnp_rzlqf6z1cC0IkIwp6yjsBboX945&index=5&ab_channel=Aspecto
- Here's the endpoint to make some random status 500 error: http://localhost:8090/rolldice/Alice?q=sadf&fail=we 
- Here's the endpoint for premium users: http://localhost:8090/rolldice/Alice/premium
- The docker image for the Gateway must be "otel/opentelemetry-collector-contrib" to support tail sampling.
- The status code in the otel-config-gateway.yaml is the span's status code, not the HTTP status code. So remember to record the error? Or what's up?

Why sampling traces? Because trace data is very expensive, so exporting 100% traces is i/o expensive, and storing 100% traces is db expensive. He says a lot of cases, only 20-50% are exported, some even go to the extreme of 5%.

2 types of sampling:
- Tail sampling: It's decided if we need to collect the traces after the span ends.
- Head sampling: It's decided if we need to collect the traces before the span starts.

# Tail

If we need the process's result to decide if we need the trace, tail sampling is the way to go here. Here's some example:
- We'd like to collect all the traces that produce errors.
- We'd like to collect all the traces whose latency is >= 1m.
- We'd like to collect all the traces from premium user. Some other services will inform if the request comes from a premium user or not.
- The rest, idc, discard all of them or sample probablistic % of them if defined.
- In the example otel-config-gateway.yaml, I've configured so it will only export trace for [premium users], and [normal users if there's an error].

In real life, the Orchestrator service can be the target for this strategy, it commands other services to follow a pipeline when the end user makes an order.
- When we have an error in making an order, we'll need the trace to tell the whole story, say the orchestrator going to A getting the user data successfully, but B says that this end user wallet doesn't exist due to the some stinky fetch command, so can't bill this end user.
- Or we see our service keep repeating handling 1 process for this end user X. The trace then tell us that the end user X is trying to find an item for "Lorem ipsum...", showing that the input is too long, and our item service is having hard time handling long, nonsense input, causing the service to be OOM, restarted and repeat the process. In this case, the trace tell us to either add more memory/add a workaround for the long, nonsense input case.

Personally, I think if we ever do Tail sampling, it's always better to export 100% trace, because it's fine if the bug is related to some error. But a lot of the time, orchestrator-like bug also occurs in non-error pipeline. For   example, it's not actually a bug, it's a misconfugration from the user side and they are not aware of such details, telling us that the design needing some improvement. ₍^. .^₎⟆

Also, now we are talking about scaling, because our system will not just handle 1 request at a time, it should be like 2000 requests in 1 second. That's like 2000 tracing stored in the memory at a time. So:
- Let's just add 50 MB worth of memory (or add some extra % based on the main application memory).

Also, with 2000 tracing a second, 1 otel collector agent ain't gonna work anymore, we'll need load balancer for the collector. But hey, in microservice, the trace is not just in 1 service, it's from multiple services, so 1 trace can have like 50 pieces. So if I need 2 load balancer.
- 1 Load balancer only doing the act of sorting based on the traceID and forwarding the trace data to the next load balancer.
- Another load balancer, then doing the task of checking status code, latency, decision making if it's going to sample the trace data or not.

I'm not going to add this to my beautiful architecture above so here's a scuff version:

QUESTION: Where is this Load Balancer? Is it a part of the OTEL Collector Gateway or Agent? Or a completely separated component?
```
┌─────────────────────────┐
| After the container     |          ┌───────────────┐                     ┌──────────────────────────────┐                   ┌────────────────────────┐
| with OTEL SDK forward   ├--------->| Load balancer ├----Sort traceID---->| OTEL Collector Load balancer ├---┬---Sampling--->| OTEL Collector Gateway |
| data to OTEL Agent      |          └───────┬───────┘                     └──────────────────────────────┘   |               └────────────────────────┘
└─────────────────────────┘                  |                             ┌──────────────────────────────┐   |               ┌────────────────────────┐
                                             └-----------------------------| OTEL Collector Load balancer ├---┼---Sampling--->| OTEL Collector Gateway |
                                                                           └──────────────────────────────┘   |               └────────────────────────┘
                                                                                                              |               ┌────────────────────────┐
                                                                                                              └---Sampling--->| OTEL Collector Gateway |
                                                                                                                              └────────────────────────┘
```

Unlike the Tail sampling, the Head sampling doesn't need this Load balancing BS because it can tell if it needs the trace or not based on the request.

About the Tail can only decide "after the span ends" is not 100% true. Our Collector doesn't know when it has received enough data... Wait.. it does... It's when the span is ended. Then wtf is the decision_wait is for?

num_traces is to limit how many traces will be kept in the memory. Because the trace data is complicated, so it'll take a lot of space. It's like PubSub's MAX_OUTSTANDING_MESSAGES.

It seems like the Tail strategy has a bunch of sampling condition:
- always_sample: Self-explanatory.
- Latency: max - min. Keep those that take too long, or those are exceptionally fast.
- <string/numeric/...>_attribute: Say, if I find out that the requested user is a special user I'd like to keep an eye on, like those who pay me, then I'll give that user's spans a bunch of special tags to remind myself to keep that user's traces. I can tag attributes anywhere. In the example code, I'll tag attributes at the start as a workaround for the Head sampling.
- Probabilistic: like normal sampling, % based.
- Status Code: (ok, error, and unset): Keep all the failed processes.
- string_attribute: Like always keep the trace of the paid users? Like sudo HEAD, won't need to reach to the end to know if this is a premium user, but it's not at the beginning of the process.
- rate_limiting: QUESTION ??? So num_traces?
```
   /\_/\
 =(• . •)=
   /   \     
```

# Head

TLDR: Head sampling is useless. It's faster to uncomment the code to run the Head sampling part to see how useless it is that reading my rant below.

If we can tell if we need the trace or not before the first ever spanStart, this is the stuff. For example:
- Nothing. This one is mega useless. Maybe it's for performance optimization. Then who tf stress test on production? We have a dedicated env for that, and in that env, disable the trace to save the money. How's that?
- Unlike Tail, Head can be configured in the code by building a custom sampler using the SDK.
- If Head sampling says no, then Tail sampling can't do anything about it. Have to throw it away. Also, even though it will not record the trace, it will still collect the trace data, and forward to other service a no-op trace, saying that hey, I'm not going to keep it, so you as well. The following service will respect the parent service's decision and follow that as well. But that no-op trace still takes some CPU! So in the end, just save some money for the memory, not the CPU.
- To experience and learn how Head sampling works, I'll make some stupid usecase like checking user tier based on the request's params and decide from HEAD if I'm going to need it or not. I'll comment out this embarrasement after this.

To understand how useless Head sampling is, have to understand how trace works:
- The trace doesn't start in the first start span, it starts before that. So:
  - Original trace (/) --> First start span (first roll) --> Second start span (second roll).

Original trace only holds useless data like this:
```
1. Key: {http.method}, Value: {GET}
2. Key: {http.scheme}, Value: {http}
3. Key: {net.host.name}, Value: {localhost}
4. Key: {net.host.port}, Value: {}
5. Key: {net.sock.peer.addr}, Value: {::1}
6. Key: {net.sock.peer.port}, Value: {}
7. Key: {user_agent.original}, Value: {Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/127.0.0.0 Safari/537.36}
8. Key: {http.target}, Value: {/rolldice/Alice/premium}
```

Maybe http.target is usable, but it will put a burden on the API design, which it should not do. And I struggle to find any useful data there. So what's up with this one?

The first start span (first roll) then has the full request and body so maybe something can be done, but it must be done when trace.StartSpan is called and must be WithAttributes. Bleh, so my code will look so disgusting because I'd like to fetch some extra data, doing extra work to just start a span. Then after doing all that to have the first span working, the second span doesn't have any clue about the first span's attribute. Attributes doesn't inherit in trace, which is understandable. So if I want to export the second span, I'll need to do all the heavy work again, just to get data to inherit the attributes, or makes my code structure sucks. If I have to do that much work to have it working, that's Tail sampling's territory.

```
  /)/)
 (. .) "nom nom"
 /づ🍪
```
