package redisotel

import (
	"fmt"

	"go.opentelemetry.io/otel/metric"
	semconv "go.opentelemetry.io/otel/semconv/v1.27.0"
)

// db.client.connection.create_time and db.client.connection.use_time are recorded using hooks.
// todo custom redis.PoolStats Hits, Misses counters
type poolStatsInstruments struct {
	connCount           metric.Int64ObservableUpDownCounter
	connIdleMax         metric.Int64ObservableUpDownCounter
	connIdleMin         metric.Int64ObservableUpDownCounter
	connMax             metric.Int64ObservableUpDownCounter
	connPendingRequests metric.Int64ObservableUpDownCounter
	connTimeouts        metric.Int64ObservableCounter
	connWaitTime        metric.Float64Histogram
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
	connCount, err := meter.Int64ObservableUpDownCounter(
		semconv.DBClientConnectionCountName,
		metric.WithDescription(semconv.DBClientConnectionCountDescription),
		metric.WithUnit(semconv.DBClientConnectionCountUnit),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create %s instrument: %w", semconv.DBClientConnectionCountName, err)
	}

	connIdleMax, err := meter.Int64ObservableUpDownCounter(
		semconv.DBClientConnectionIdleMaxName,
		metric.WithDescription(semconv.DBClientConnectionIdleMaxDescription),
		metric.WithUnit(semconv.DBClientConnectionIdleMaxUnit),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create %s instrument: %w", semconv.DBClientConnectionIdleMaxName, err)
	}

	connIdleMin, err := meter.Int64ObservableUpDownCounter(
		semconv.DBClientConnectionIdleMinName,
		metric.WithDescription(semconv.DBClientConnectionIdleMinDescription),
		metric.WithUnit(semconv.DBClientConnectionIdleMinUnit),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create %s instrument: %w", semconv.DBClientConnectionIdleMinName, err)
	}

	connMax, err := meter.Int64ObservableUpDownCounter(
		semconv.DBClientConnectionMaxName,
		metric.WithDescription(semconv.DBClientConnectionMaxDescription),
		metric.WithUnit(semconv.DBClientConnectionMaxUnit),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create %s instrument: %w", semconv.DBClientConnectionMaxName, err)
	}

	connPendingRequests, err := meter.Int64ObservableUpDownCounter(
		semconv.DBClientConnectionPendingRequestsName,
		metric.WithDescription(semconv.DBClientConnectionPendingRequestsDescription),
		metric.WithUnit(semconv.DBClientConnectionPendingRequestsUnit),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create %s instrument: %w", semconv.DBClientConnectionPendingRequestsName, err)
	}

	connTimeouts, err := meter.Int64ObservableCounter(
		semconv.DBClientConnectionTimeoutsName,
		metric.WithDescription(semconv.DBClientConnectionTimeoutsDescription),
		metric.WithUnit(semconv.DBClientConnectionTimeoutsUnit),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create %s instrument: %w", semconv.DBClientConnectionTimeoutsName, err)
	}

	connWaitTime, err := meter.Float64Histogram(
		semconv.DBClientConnectionWaitTimeName,
		metric.WithDescription(semconv.DBClientConnectionWaitTimeDescription),
		metric.WithUnit(semconv.DBClientConnectionWaitTimeUnit),
		metric.WithExplicitBucketBoundaries(buckets...),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create %s instrument: %w", semconv.DBClientConnectionWaitTimeName, err)
	}

	return &poolStatsInstruments{
		connCount:           connCount,
		connIdleMax:         connIdleMax,
		connIdleMin:         connIdleMin,
		connMax:             connMax,
		connPendingRequests: connPendingRequests,
		connTimeouts:        connTimeouts,
		connWaitTime:        connWaitTime,
	}, nil
}

func newHookInstruments(meter metric.Meter) (*hookInstruments, error) {
	oprDuration, err := meter.Float64Histogram(
		semconv.DBClientOperationDurationName,
		metric.WithDescription(semconv.DBClientOperationDurationDescription),
		metric.WithUnit(semconv.DBClientOperationDurationUnit),
		metric.WithExplicitBucketBoundaries(buckets...),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create %s instrument: %w", semconv.DBClientOperationDurationName, err)
	}

	createTime, err := meter.Float64Histogram(
		semconv.DBClientConnectionCreateTimeName,
		metric.WithDescription(semconv.DBClientConnectionCreateTimeDescription),
		metric.WithUnit(semconv.DBClientConnectionCreateTimeUnit),
		metric.WithExplicitBucketBoundaries(buckets...),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create %s instrument: %w", semconv.DBClientConnectionCreateTimeName, err)
	}

	useTime, err := meter.Float64Histogram(
		semconv.DBClientConnectionUseTimeName,
		metric.WithDescription(semconv.DBClientConnectionUseTimeDescription),
		metric.WithUnit(semconv.DBClientConnectionUseTimeUnit),
		metric.WithExplicitBucketBoundaries(buckets...),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create %s instrument: %w", semconv.DBClientConnectionUseTimeName, err)
	}

	return &hookInstruments{
		oprDuration: oprDuration,

		createTime: createTime,
		useTime:    useTime,
	}, nil
}
