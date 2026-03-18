# OpenTelemetry Integration

PetalTrace accepts traces via the standard OpenTelemetry Protocol (OTLP), making it compatible with any OTel-instrumented application. This guide covers how to send traces from various languages and frameworks.

## OTLP Endpoints

PetalTrace exposes two OTLP receivers:

| Protocol | Port | Endpoint |
|----------|------|----------|
| gRPC | 4317 | `localhost:4317` |
| HTTP | 4318 | `http://localhost:4318/v1/traces` |

## LLM-Aware Features

PetalTrace provides special handling for LLM spans that follow the [OTel GenAI semantic conventions](https://opentelemetry.io/docs/specs/semconv/gen-ai/). When these attributes are present, PetalTrace:

- Classifies spans as LLM spans
- Computes cost estimates
- Extracts prompts and completions for inspection
- Enables prompt-level search and diff

### GenAI Semantic Conventions

| Attribute | Description | Example |
|-----------|-------------|---------|
| `gen_ai.system` | Provider name | `anthropic`, `openai` |
| `gen_ai.request.model` | Model identifier | `claude-sonnet-4-20250514` |
| `gen_ai.request.temperature` | Sampling temperature | `0.7` |
| `gen_ai.request.max_tokens` | Max output tokens | `4096` |
| `gen_ai.usage.input_tokens` | Input token count | `1000` |
| `gen_ai.usage.output_tokens` | Output token count | `500` |
| `gen_ai.response.finish_reason` | Stop reason | `end_turn` |

For full prompt capture, also include:

| Attribute | Description |
|-----------|-------------|
| `gen_ai.system_prompt` | System prompt text |
| `gen_ai.prompt` | Full prompt or message array JSON |
| `gen_ai.completion` | Full completion text or JSON |

## Python

### Using OpenTelemetry SDK

```python
from opentelemetry import trace
from opentelemetry.sdk.trace import TracerProvider
from opentelemetry.sdk.trace.export import BatchSpanProcessor
from opentelemetry.exporter.otlp.proto.grpc.trace_exporter import OTLPSpanExporter

# Configure exporter
exporter = OTLPSpanExporter(
    endpoint="localhost:4317",
    insecure=True  # Use True for local development
)

# Set up tracer provider
provider = TracerProvider()
provider.add_span_processor(BatchSpanProcessor(exporter))
trace.set_tracer_provider(provider)

tracer = trace.get_tracer(__name__)
```

### Instrumenting LLM Calls

```python
import json
from opentelemetry import trace

tracer = trace.get_tracer(__name__)

def call_llm(model: str, messages: list, temperature: float = 0.7):
    with tracer.start_as_current_span("llm_call") as span:
        # Set GenAI attributes
        span.set_attribute("gen_ai.system", "anthropic")
        span.set_attribute("gen_ai.request.model", model)
        span.set_attribute("gen_ai.request.temperature", temperature)
        span.set_attribute("gen_ai.prompt", json.dumps(messages))

        # Make the actual API call
        response = anthropic_client.messages.create(
            model=model,
            messages=messages,
            temperature=temperature
        )

        # Set response attributes
        span.set_attribute("gen_ai.usage.input_tokens", response.usage.input_tokens)
        span.set_attribute("gen_ai.usage.output_tokens", response.usage.output_tokens)
        span.set_attribute("gen_ai.completion", response.content[0].text)
        span.set_attribute("gen_ai.response.finish_reason", response.stop_reason)

        return response
```

### Using OpenAI Instrumentation

The `opentelemetry-instrumentation-openai` package provides automatic instrumentation:

```bash
pip install opentelemetry-instrumentation-openai
```

```python
from opentelemetry.instrumentation.openai import OpenAIInstrumentor

OpenAIInstrumentor().instrument()

# Now all OpenAI calls are automatically traced
response = openai.chat.completions.create(
    model="gpt-4o",
    messages=[{"role": "user", "content": "Hello"}]
)
```

## Go

### Using OpenTelemetry SDK

```go
package main

import (
    "context"

    "go.opentelemetry.io/otel"
    "go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
    "go.opentelemetry.io/otel/sdk/trace"
)

func initTracer() (*trace.TracerProvider, error) {
    exporter, err := otlptracegrpc.New(
        context.Background(),
        otlptracegrpc.WithEndpoint("localhost:4317"),
        otlptracegrpc.WithInsecure(),
    )
    if err != nil {
        return nil, err
    }

    tp := trace.NewTracerProvider(
        trace.WithBatcher(exporter),
    )
    otel.SetTracerProvider(tp)

    return tp, nil
}
```

### Instrumenting LLM Calls

```go
package main

import (
    "context"
    "encoding/json"

    "go.opentelemetry.io/otel"
    "go.opentelemetry.io/otel/attribute"
)

var tracer = otel.Tracer("myapp")

func callLLM(ctx context.Context, model string, messages []Message) (*Response, error) {
    ctx, span := tracer.Start(ctx, "llm_call")
    defer span.End()

    // Set GenAI attributes
    span.SetAttributes(
        attribute.String("gen_ai.system", "anthropic"),
        attribute.String("gen_ai.request.model", model),
        attribute.Float64("gen_ai.request.temperature", 0.7),
    )

    messagesJSON, _ := json.Marshal(messages)
    span.SetAttributes(attribute.String("gen_ai.prompt", string(messagesJSON)))

    // Make the API call
    response, err := client.CreateMessage(ctx, model, messages)
    if err != nil {
        span.RecordError(err)
        return nil, err
    }

    // Set response attributes
    span.SetAttributes(
        attribute.Int64("gen_ai.usage.input_tokens", int64(response.Usage.InputTokens)),
        attribute.Int64("gen_ai.usage.output_tokens", int64(response.Usage.OutputTokens)),
        attribute.String("gen_ai.completion", response.Content),
        attribute.String("gen_ai.response.finish_reason", response.StopReason),
    )

    return response, nil
}
```

## JavaScript/TypeScript

### Using OpenTelemetry SDK

```typescript
import { NodeSDK } from '@opentelemetry/sdk-node';
import { OTLPTraceExporter } from '@opentelemetry/exporter-trace-otlp-grpc';

const sdk = new NodeSDK({
  traceExporter: new OTLPTraceExporter({
    url: 'http://localhost:4317',
  }),
});

sdk.start();
```

### Instrumenting LLM Calls

```typescript
import { trace, SpanStatusCode } from '@opentelemetry/api';

const tracer = trace.getTracer('myapp');

async function callLLM(model: string, messages: Message[]): Promise<Response> {
  return tracer.startActiveSpan('llm_call', async (span) => {
    try {
      // Set GenAI attributes
      span.setAttribute('gen_ai.system', 'anthropic');
      span.setAttribute('gen_ai.request.model', model);
      span.setAttribute('gen_ai.request.temperature', 0.7);
      span.setAttribute('gen_ai.prompt', JSON.stringify(messages));

      // Make the API call
      const response = await anthropic.messages.create({
        model,
        messages,
        temperature: 0.7,
      });

      // Set response attributes
      span.setAttribute('gen_ai.usage.input_tokens', response.usage.input_tokens);
      span.setAttribute('gen_ai.usage.output_tokens', response.usage.output_tokens);
      span.setAttribute('gen_ai.completion', response.content[0].text);
      span.setAttribute('gen_ai.response.finish_reason', response.stop_reason);

      return response;
    } catch (error) {
      span.setStatus({ code: SpanStatusCode.ERROR, message: error.message });
      throw error;
    } finally {
      span.end();
    }
  });
}
```

## Ruby

### Using OpenTelemetry SDK

```ruby
require 'opentelemetry/sdk'
require 'opentelemetry/exporter/otlp'

OpenTelemetry::SDK.configure do |c|
  c.add_span_processor(
    OpenTelemetry::SDK::Trace::Export::BatchSpanProcessor.new(
      OpenTelemetry::Exporter::OTLP::Exporter.new(
        endpoint: 'http://localhost:4318/v1/traces'
      )
    )
  )
end
```

### Instrumenting LLM Calls

```ruby
tracer = OpenTelemetry.tracer_provider.tracer('myapp')

def call_llm(model:, messages:, temperature: 0.7)
  tracer.in_span('llm_call') do |span|
    span.set_attribute('gen_ai.system', 'anthropic')
    span.set_attribute('gen_ai.request.model', model)
    span.set_attribute('gen_ai.request.temperature', temperature)
    span.set_attribute('gen_ai.prompt', messages.to_json)

    response = anthropic.messages(
      model: model,
      messages: messages,
      temperature: temperature
    )

    span.set_attribute('gen_ai.usage.input_tokens', response.usage.input_tokens)
    span.set_attribute('gen_ai.usage.output_tokens', response.usage.output_tokens)
    span.set_attribute('gen_ai.completion', response.content.first.text)
    span.set_attribute('gen_ai.response.finish_reason', response.stop_reason)

    response
  end
end
```

## Run Correlation

To group spans into runs, set a run identifier on your root span:

```python
with tracer.start_as_current_span("workflow_execution") as span:
    # Set run ID for correlation
    span.set_attribute("petalflow.run.id", f"run-{uuid.uuid4()}")
    span.set_attribute("petalflow.workflow.name", "my-workflow")
    span.set_attribute("petalflow.run.root", True)

    # Execute workflow steps...
```

Without an explicit run ID, PetalTrace uses the trace ID to group spans.

## Tool Call Tracking

Track tool/function calls with these attributes:

```python
with tracer.start_as_current_span("tool_call") as span:
    span.set_attribute("tool.name", "web_search")
    span.set_attribute("tool.input", json.dumps({"query": "AI observability"}))

    result = web_search(query="AI observability")

    span.set_attribute("tool.output", json.dumps(result))
```

## Environment Variables

Configure OTLP exporters via environment variables:

```bash
# gRPC
export OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4317
export OTEL_EXPORTER_OTLP_PROTOCOL=grpc

# HTTP
export OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4318
export OTEL_EXPORTER_OTLP_PROTOCOL=http/protobuf

# Service name
export OTEL_SERVICE_NAME=my-llm-app
```

## Verification

After sending traces, verify they appear in PetalTrace:

```bash
# List recent runs
petaltrace runs list --since 1h

# Check a specific run
petaltrace runs show <run-id> --spans
```

## Troubleshooting

### Traces Not Appearing

1. **Check PetalTrace is running:**
   ```bash
   curl http://localhost:8090/api/health
   ```

2. **Verify OTLP port is accessible:**
   ```bash
   # gRPC
   nc -zv localhost 4317

   # HTTP
   curl -v http://localhost:4318/v1/traces
   ```

3. **Check exporter configuration** — ensure endpoint and protocol match.

### LLM Spans Not Classified

Ensure you set `gen_ai.system` or `gen_ai.request.model` attributes. Without these, spans are classified as "custom".

### Missing Cost Estimates

Cost calculation requires:
- `gen_ai.system` (provider name)
- `gen_ai.request.model` (model name)
- `gen_ai.usage.input_tokens`
- `gen_ai.usage.output_tokens`

The model must be in PetalTrace's pricing table. Check available models:

```bash
curl http://localhost:8090/api/pricing
```

### High Latency

For high-volume applications:

1. Use batch processing in your exporter
2. Increase PetalTrace batch settings:
   ```yaml
   collector:
     batch_size: 500
     flush_interval: "5s"
   ```
