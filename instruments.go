package redisotel

import (
	"fmt"

	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/semconv/v1.34.0/dbconv"
)

// db.client.connection.create_time and db.client.connection.use_time are recorded using hooks.
// todo custom redis.PoolStats Hits, Misses counters
type poolStatsInstruments struct {
	connCount           metric.Int64ObservableUpDownCounter
	connIdleMax         metric.Int64ObservableUpDownCounter
	connIdleMin         metric.Int64ObservableUpDownCounter
	connMax             metric.Int64ObservableUpDownCounter
	connPendingRequests metric.Int64UpDownCounter
	connTimeouts        metric.Int64ObservableCounter
	connWaitTime        metric.Float64Histogram

	connHitCount      metric.Int64ObservableCounter
	connMissCount     metric.Int64ObservableCounter
	connWaitCount     metric.Int64ObservableCounter
	connWaitTimeTotal metric.Float64ObservableCounter
}

type hookInstruments struct {
	oprDuration metric.Float64Histogram

	createTime metric.Float64Histogram
	useTime    metric.Float64Histogram
}

var buckets = []float64{.001, .005, .01, .025, .05, .075, .1, .25, .5, .75, 1, 2.5, 5, 7.5, 10}

// SetBuckets sets the buckets used for OpenTelemetry metrics.
// The default buckets of .001, .005, .01, .025, .05, .075, .1, .25, .5, .75, 1, 2.5, 5, 7.5, 10
// are used if SetBuckets is not called.
// The default buckets are finer than the one in the Semantic Conventions.
func SetBuckets(b []float64) {
	buckets = b
}

func newPoolStatsInstruments(meter metric.Meter) (*poolStatsInstruments, error) {
	// We cannot use dbconv.NewClientConnectionCount etc. for poolStatsInstruments,
	// since they aren't Observable Counter, which we need.
	connCount, err := meter.Int64ObservableUpDownCounter(
		dbconv.ClientConnectionCount{Int64UpDownCounter: nil}.Name(),
		metric.WithDescription(dbconv.ClientConnectionCount{Int64UpDownCounter: nil}.Description()),
		metric.WithUnit(dbconv.ClientConnectionCount{Int64UpDownCounter: nil}.Unit()))
	if err != nil {
		return nil, fmt.Errorf("failed to create %s instrument: %w",
			dbconv.ClientConnectionCount{Int64UpDownCounter: nil}.Name(), err)
	}

	connIdleMax, err := meter.Int64ObservableUpDownCounter(
		dbconv.ClientConnectionIdleMax{Int64UpDownCounter: nil}.Name(),
		metric.WithDescription(dbconv.ClientConnectionIdleMax{Int64UpDownCounter: nil}.Description()),
		metric.WithUnit(dbconv.ClientConnectionIdleMax{Int64UpDownCounter: nil}.Unit()))
	if err != nil {
		return nil, fmt.Errorf("failed to create %s instrument: %w",
			dbconv.ClientConnectionIdleMax{Int64UpDownCounter: nil}.Name(), err)
	}

	connIdleMin, err := meter.Int64ObservableUpDownCounter(
		dbconv.ClientConnectionIdleMin{Int64UpDownCounter: nil}.Name(),
		metric.WithDescription(dbconv.ClientConnectionIdleMin{Int64UpDownCounter: nil}.Description()),
		metric.WithUnit(dbconv.ClientConnectionIdleMin{Int64UpDownCounter: nil}.Unit()))
	if err != nil {
		return nil, fmt.Errorf("failed to create %s instrument: %w",
			dbconv.ClientConnectionIdleMin{Int64UpDownCounter: nil}.Name(), err)
	}

	connMax, err := meter.Int64ObservableUpDownCounter(
		dbconv.ClientConnectionMax{Int64UpDownCounter: nil}.Name(),
		metric.WithDescription(dbconv.ClientConnectionMax{Int64UpDownCounter: nil}.Description()),
		metric.WithUnit(dbconv.ClientConnectionMax{Int64UpDownCounter: nil}.Unit()))
	if err != nil {
		return nil, fmt.Errorf("failed to create %s instrument: %w",
			dbconv.ClientConnectionMax{Int64UpDownCounter: nil}.Name(), err)
	}

	connPendingRequests, err := dbconv.NewClientConnectionPendingRequests(meter)
	if err != nil {
		return nil, fmt.Errorf("failed to create %s instrument: %w",
			dbconv.ClientConnectionPendingRequests{Int64UpDownCounter: nil}.Name(), err)
	}

	connTimeouts, err := meter.Int64ObservableCounter(
		dbconv.ClientConnectionTimeouts{Int64Counter: nil}.Name(),
		metric.WithDescription(dbconv.ClientConnectionTimeouts{Int64Counter: nil}.Description()),
		metric.WithUnit(dbconv.ClientConnectionTimeouts{Int64Counter: nil}.Unit()))
	if err != nil {
		return nil, fmt.Errorf("failed to create %s instrument: %w",
			dbconv.ClientConnectionTimeouts{Int64Counter: nil}.Name(), err)
	}

	connWaitTime, err := dbconv.NewClientConnectionWaitTime(meter, metric.WithExplicitBucketBoundaries(buckets...))
	if err != nil {
		return nil, fmt.Errorf("failed to create %s instrument: %w",
			dbconv.ClientConnectionWaitTime{Float64Histogram: nil}.Name(), err)
	}

	// non-standard metrics start here
	connHitCount, err := meter.Int64ObservableCounter(
		"db.client.connection.redis.hits",
		metric.WithDescription("The number of times free connections was found in the pool."),
		metric.WithUnit("{hit}"))
	if err != nil {
		return nil, fmt.Errorf("failed to create db.client.connection.redis.hits instrument: %w", err)
	}
	connMissCount, err := meter.Int64ObservableCounter(
		"db.client.connection.redis.misses",
		metric.WithDescription("The number of times free connections was NOT found in the pool."),
		metric.WithUnit("{miss}"))
	if err != nil {
		return nil, fmt.Errorf("failed to create db.client.connection.redis.misses instrument: %w", err)
	}
	connWaitCount, err := meter.Int64ObservableCounter(
		"db.client.connection.waits",
		metric.WithDescription("The number of times it waited to obtain open connections from the pool."),
		metric.WithUnit("{wait}"))
	if err != nil {
		return nil, fmt.Errorf("failed to create db.client.connection.waits instrument: %w", err)
	}
	connWaitTimeTotal, err := meter.Float64ObservableCounter(
		"db.client.connection.wait_duration",
		metric.WithDescription("The total time it took to obtain open connections from the pool."),
		metric.WithUnit("s"))
	if err != nil {
		return nil, fmt.Errorf("failed to create db.client.connection.wait_duration instrument: %w", err)
	}

	return &poolStatsInstruments{
		connCount:           connCount,
		connIdleMax:         connIdleMax,
		connIdleMin:         connIdleMin,
		connMax:             connMax,
		connPendingRequests: connPendingRequests.Inst(),
		connTimeouts:        connTimeouts,
		connWaitTime:        connWaitTime.Inst(),
		connHitCount:        connHitCount,
		connMissCount:       connMissCount,
		connWaitCount:       connWaitCount,
		connWaitTimeTotal:   connWaitTimeTotal,
	}, nil
}

func newHookInstruments(conf *config) (*hookInstruments, error) {
	oprDuration, err := dbconv.NewClientOperationDuration(conf.meter, metric.WithExplicitBucketBoundaries(buckets...))
	if err != nil {
		return nil, fmt.Errorf("failed to create %s instrument: %w",
			dbconv.ClientOperationDuration{Float64Histogram: nil}.Name(), err)
	}

	createTime, err := dbconv.NewClientConnectionCreateTime(conf.meter, metric.WithExplicitBucketBoundaries(buckets...))
	if err != nil {
		return nil, fmt.Errorf("failed to create %s instrument: %w",
			dbconv.ClientConnectionCreateTime{Float64Histogram: nil}.Name(), err)
	}

	useTime, err := dbconv.NewClientConnectionUseTime(conf.meter, metric.WithExplicitBucketBoundaries(buckets...))
	if err != nil {
		return nil, fmt.Errorf("failed to create %s instrument: %w",
			dbconv.ClientConnectionUseTime{Float64Histogram: nil}.Name(), err)
	}

	instruments := &hookInstruments{
		oprDuration: oprDuration.Inst(),
		createTime:  createTime.Inst(),
		useTime:     useTime.Inst(),
	}
	return instruments, nil
}
