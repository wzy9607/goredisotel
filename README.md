# goredisotel

[![ci](https://github.com/wzy9607/goredisotel/actions/workflows/pull-request.yml/badge.svg)](https://github.com/wzy9607/goredisotel/actions/workflows/pull-request.yml)
[![codecov](https://codecov.io/gh/wzy9607/goredisotel/graph/badge.svg?token=VVMWEWOQFO)](https://codecov.io/gh/wzy9607/goredisotel)

A fork of go-redis/extra/redisotel/v9 that follows
[Semantic Conventions v1.37](https://github.com/open-telemetry/semantic-conventions/blob/v1.37.0/docs/database/README.md).

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

## Metric Instruments

We support some of the standard metrics, and provide some custom metrics as well.
Some standard Connection pool metrics might be slightly different compared to the
[Semantic Conventions](https://opentelemetry.io/docs/specs/semconv/database/database-metrics/#connection-pools)
due to the way go-redis exposes them.
The custom metrics may be changed without semconv upgrade, so please use them with caution.

The details of the metrics are documented below:

### Database Operation Metrics

| Name                           | Instrument Type | Unit (UCUM) |
|--------------------------------|-----------------|-------------|
| `db.client.operation.duration` | Histogram       | `s`         |

### Connection Pool Metrics

Standard metrics:

| Name                               | Instrument Type | Unit (UCUM)    | Difference                              |
|------------------------------------|-----------------|----------------|-----------------------------------------|
| `db.client.connection.count`       | UpDownCounter   | `{connection}` | Observed as a `ObservableUpDownCounter` |
| `db.client.connection.idle.max`    | UpDownCounter   | `{connection}` | Observed as a `ObservableUpDownCounter` |
| `db.client.connection.idle.min`    | UpDownCounter   | `{connection}` | Observed as a `ObservableUpDownCounter` |
| `db.client.connection.max`         | UpDownCounter   | `{connection}` | Observed as a `ObservableUpDownCounter` |
| `db.client.connection.timeouts`    | Counter         | `{timeout}`    | Observed as a `ObservableCounter`       |
| `db.client.connection.create_time` | Histogram       | `s`            |                                         |
| `db.client.connection.use_time`    | Histogram       | `s`            |                                         |

Custom metrics:

| Name                                 | Instrument Type      | Unit (UCUM) | Description                                                            |
|--------------------------------------|----------------------|-------------|------------------------------------------------------------------------|
| `db.client.connection.redis.hits`    | Counter (Observable) | `{hit}`     | The number of times free connections was found in the pool             |
| `db.client.connection.redis.misses`  | Counter (Observable) | `{miss}`    | The number of times free connections was NOT found in the pool         |
| `db.client.connection.waits`         | Counter (Observable) | `{wait}`    | The number of times it waited to obtain open connections from the pool |
| `db.client.connection.wait_duration` | Counter (Observable) | `s`         | The total time it took to obtain open connections from the pool        |

The following metrics are not supported yet:

- `db.client.connection.pending_requests`
- `db.client.connection.wait_time`
