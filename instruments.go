package redisotel

import (
	"fmt"

	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/semconv/v1.41.0/dbconv"
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
	connCount, err := dbconv.NewClientConnectionCountObservable(meter)
	if err != nil {
		return nil, fmt.Errorf("failed to create %s instrument: %w",
			dbconv.ClientConnectionCountObservable{Int64ObservableUpDownCounter: nil}.Name(), err)
	}

	connIdleMax, err := dbconv.NewClientConnectionIdleMaxObservable(meter)
	if err != nil {
		return nil, fmt.Errorf("failed to create %s instrument: %w",
			dbconv.ClientConnectionIdleMaxObservable{Int64ObservableUpDownCounter: nil}.Name(), err)
	}

	connIdleMin, err := dbconv.NewClientConnectionIdleMinObservable(meter)
	if err != nil {
		return nil, fmt.Errorf("failed to create %s instrument: %w",
			dbconv.ClientConnectionIdleMinObservable{Int64ObservableUpDownCounter: nil}.Name(), err)
	}

	connMax, err := dbconv.NewClientConnectionMaxObservable(meter)
	if err != nil {
		return nil, fmt.Errorf("failed to create %s instrument: %w",
			dbconv.ClientConnectionMaxObservable{Int64ObservableUpDownCounter: nil}.Name(), err)
	}

	connPendingRequests, err := dbconv.NewClientConnectionPendingRequests(meter)
	if err != nil {
		return nil, fmt.Errorf("failed to create %s instrument: %w",
			dbconv.ClientConnectionPendingRequests{Int64UpDownCounter: nil}.Name(), err)
	}

	connTimeouts, err := dbconv.NewClientConnectionTimeoutsObservable(meter)
	if err != nil {
		return nil, fmt.Errorf("failed to create %s instrument: %w",
			dbconv.ClientConnectionTimeoutsObservable{Int64ObservableCounter: nil}.Name(), err)
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
		connCount:           connCount.Inst(),
		connIdleMax:         connIdleMax.Inst(),
		connIdleMin:         connIdleMin.Inst(),
		connMax:             connMax.Inst(),
		connPendingRequests: connPendingRequests.Inst(),
		connTimeouts:        connTimeouts.Inst(),
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
