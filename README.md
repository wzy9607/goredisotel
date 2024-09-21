# goredisotel

A fork of go-redis/extra/redisotel/v9 that follows
[Semantic Conventions v1.26](https://github.com/open-telemetry/semantic-conventions/blob/v1.26.0/docs/database/README.md).

## Installation

```bash
go get github.com/wzy9607/goredisotel
```

## Usage

Tracing is enabled by adding a hook:

```go
import (
"github.com/redis/go-redis/v9"
"github.com/wzy9607/goredisotel"
)

rdb := rdb.NewClient(&rdb.Options{...})

// Enable tracing instrumentation.
if err := goredisotel.InstrumentClientWithHooks(rdb); err != nil {
panic(err)
}

// Enable metrics instrumentation.
if err := goredisotel.InstrumentMetrics(rdb); err != nil {
panic(err)
}

// Enable pool metrics instrumentation.
if err := goredisotel.InstrumentPoolStatsMetrics(rdb); err != nil {
panic(err)
}
```

See [example](./example)
for details.
