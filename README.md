For the jaeger submodule:
All I use is:
```bash
go run ./examples/hotrod/main.go all
```

and https://github.com/jaegertracing/jaeger/tree/1b158e5d54513b5443491d45f2c18a8f0578e0d8/examples/hotrod
and https://medium.com/jaegertracing/take-jaeger-for-a-hotrod-ride-233cf43e46c2

Check README in the ./otel folder as well.

The example service in the otel folder is an attempt to build the Option #1 in this video and a bit of Option #2.
https://www.youtube.com/watch?v=L_gjG4BjvSE&list=PLNxnp_rzlqf6z1cC0IkIwp6yjsBboX945&ab_channel=Aspecto

# How to read OTEL stuff

Depending on the image being used in the docker-compose, I'd go to the respective github page. If it's `otel/opentelemetry-collector-contrib`, then go to `https://github.com/open-telemetry/opentelemetry-collector-contrib`. If it's `otel/opentelemetry-collector`, then go to `https://github.com/open-telemetry/opentelemetry-collector`.

- That github page is like the schema of the otel.yaml file. For example: [export schema](https://github.com/open-telemetry/opentelemetry-collector/tree/main/exporter).
- Explanation in code in case English can't explain well enough. For example: [how processors.tail_sampling.policies.latency works](https://github.com/open-telemetry/opentelemetry-collector-contrib/blob/1f8c1eedf219e445f02a1504e72bb557a3f22cff/processor/tailsamplingprocessor/internal/sampling/latency.go#L55)

It seems like there's no way to demystify OTEL. I can follow https://github.com/open-telemetry/opentelemetry-go/tree/main/sdk to understand the purpose of the OTEL function. But if you ask me how I know that I need to use that specific function and not the others, I don't know, either found it from some blog or the tutorial. At least that's how it goes so far.
