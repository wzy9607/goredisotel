# goredisotel

[![ci](https://github.com/wzy9607/goredisotel/actions/workflows/pull-request.yml/badge.svg)](https://github.com/wzy9607/goredisotel/actions/workflows/pull-request.yml)
[![codecov](https://codecov.io/gh/wzy9607/goredisotel/graph/badge.svg?token=VVMWEWOQFO)](https://codecov.io/gh/wzy9607/goredisotel)

A fork of go-redis/extra/redisotel/v9 that follows
[Semantic Conventions v1.32](https://github.com/open-telemetry/semantic-conventions/blob/v1.32.0/docs/database/README.md).

## Installation

```bash
go get github.com/wzy9607/goredisotel
```

## Usage

Tracing is enabled by adding a hook:

```go
package main

import (
	redis "github.com/redis/go-redis/v9"
	"github.com/wzy9607/goredisotel"
)

func main() {
	rdb := redis.NewClient(&redis.Options{...})

	// Enable tracing and metrics instrumentation.
	if err := goredisotel.InstrumentClientWithHooks(rdb); err != nil {
		panic(err)
	}

	// Enable tracing instrumentation only.
	if err := goredisotel.InstrumentClientWithHooks(rdb, goredisotel.DisableMetrics()); err != nil {
		panic(err)
	}

	// Enable pool statistics metrics instrumentation.
	if err := goredisotel.InstrumentPoolStatsMetrics(rdb); err != nil {
		panic(err)
	}
}

```

See [example](./example)
for details.
