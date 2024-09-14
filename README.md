# goredisotel

A fork of go-redis/extra/redisotel/v9 that follows newer version of Semantic Conventions.

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
if err := goredisotel.InstrumentTracing(rdb); err != nil {
    panic(err)
}

// Enable metrics instrumentation.
if err := goredisotel.InstrumentMetrics(rdb); err != nil {
    panic(err)
}
```

See [example](./example) and
for details.
