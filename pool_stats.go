package redisotel

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	semconv "go.opentelemetry.io/otel/semconv/v1.32.0"

	"github.com/redis/go-redis/v9"
)

// InstrumentPoolStatsMetrics starts reporting OpenTelemetry Metrics for the connection pool.
func InstrumentPoolStatsMetrics(rdb redis.UniversalClient, opts ...Option) error {
	conf := newConfig(opts...)

	switch rdb := rdb.(type) {
	case *redis.Client:
		if err := reportPoolStats(rdb, conf); err != nil {
			return err
		}
		return nil
	case *redis.ClusterClient:
		rdb.OnNewNode(func(rdb *redis.Client) {
			if err := reportPoolStats(rdb, conf); err != nil {
				otel.Handle(err)
			}
		})
		return nil
	case *redis.Ring:
		rdb.OnNewNode(func(rdb *redis.Client) {
			if err := reportPoolStats(rdb, conf); err != nil {
				otel.Handle(err)
			}
		})
		return nil
	default:
		return fmt.Errorf("goredisotel: %T not supported", rdb)
	}
}

// reportPoolStats reports connection pool stats.
// todo connPendingRequests and connWaitTime aren't reported.
func reportPoolStats(rdb *redis.Client, conf *config) error {
	poolAttrs := commonPoolAttrs(conf, rdb.Options())
	idleAttrs := attribute.NewSet(append(poolAttrs.ToSlice(), semconv.DBClientConnectionStateIdle)...)
	usedAttrs := attribute.NewSet(append(poolAttrs.ToSlice(), semconv.DBClientConnectionStateUsed)...)

	instruments, err := newPoolStatsInstruments(conf.meter)
	if err != nil {
		return err
	}

	redisConf := rdb.Options()
	_, err = conf.meter.RegisterCallback(
		func(ctx context.Context, observer metric.Observer) error {
			stats := rdb.PoolStats()

			observer.ObserveInt64(instruments.connCount, int64(stats.IdleConns),
				metric.WithAttributeSet(idleAttrs))
			observer.ObserveInt64(instruments.connCount, int64(stats.TotalConns-stats.IdleConns),
				metric.WithAttributeSet(usedAttrs))
			observer.ObserveInt64(instruments.connIdleMax, int64(redisConf.MaxIdleConns),
				metric.WithAttributeSet(poolAttrs))
			observer.ObserveInt64(instruments.connIdleMin, int64(redisConf.MinIdleConns),
				metric.WithAttributeSet(poolAttrs))
			observer.ObserveInt64(instruments.connMax, int64(redisConf.PoolSize),
				metric.WithAttributeSet(poolAttrs))
			observer.ObserveInt64(instruments.connTimeouts, int64(stats.Timeouts),
				metric.WithAttributeSet(poolAttrs))
			// non-standard metrics start here
			observer.ObserveInt64(instruments.connHitCount, int64(stats.Hits),
				metric.WithAttributeSet(poolAttrs))
			observer.ObserveInt64(instruments.connMissCount, int64(stats.Misses),
				metric.WithAttributeSet(poolAttrs))
			observer.ObserveInt64(instruments.connWaitCount, int64(stats.WaitCount),
				metric.WithAttributeSet(poolAttrs))
			observer.ObserveFloat64(instruments.connWaitTimeTotal, time.Duration(stats.WaitDurationNs).Seconds(),
				metric.WithAttributeSet(poolAttrs))
			return nil
		},
		instruments.connCount,
		instruments.connIdleMax,
		instruments.connIdleMin,
		instruments.connMax,
		instruments.connTimeouts,
		instruments.connHitCount,
		instruments.connMissCount,
		instruments.connWaitCount,
		instruments.connWaitTimeTotal,
	)

	return err
}
