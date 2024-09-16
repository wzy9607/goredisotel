package redisotel

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"

	"github.com/redis/go-redis/v9"
)

// InstrumentMetrics starts reporting OpenTelemetry Metrics.
//
// Based on https://opentelemetry.io/docs/specs/semconv/database/.
func InstrumentMetrics(rdb redis.UniversalClient, opts ...MetricsOption) error {
	baseOpts := make([]baseOption, len(opts))
	for i, opt := range opts {
		baseOpts[i] = opt
	}
	conf := newConfig(baseOpts...)

	if conf.meter == nil {
		conf.meter = conf.mp.Meter(
			instrumName,
			metric.WithInstrumentationVersion("semver:"+redis.Version()),
		)
	}

	switch rdb := rdb.(type) {
	case *redis.Client:
		if err := reportPoolStats(rdb, conf); err != nil {
			return err
		}
		if err := addMetricsHook(rdb, conf); err != nil {
			return err
		}
		return nil
	case *redis.ClusterClient:
		rdb.OnNewNode(func(rdb *redis.Client) {
			if err := reportPoolStats(rdb, conf); err != nil {
				otel.Handle(err)
			}
			if err := addMetricsHook(rdb, conf); err != nil {
				otel.Handle(err)
			}
		})
		return nil
	case *redis.Ring:
		rdb.OnNewNode(func(rdb *redis.Client) {
			if err := reportPoolStats(rdb, conf); err != nil {
				otel.Handle(err)
			}
			if err := addMetricsHook(rdb, conf); err != nil {
				otel.Handle(err)
			}
		})
		return nil
	default:
		return fmt.Errorf("goredisotel: %T not supported", rdb)
	}
}

// reportPoolStats reports connection pool stats.
// todo db.client.connection.pending_requests, db.client.connection.wait_time
func reportPoolStats(rdb *redis.Client, conf *config) error {
	poolAttrs := commonPoolAttrs(conf, rdb.Options())
	idleAttrs := attribute.NewSet(append(poolAttrs.ToSlice(), semconv.DBClientConnectionsStateIdle)...)
	usedAttrs := attribute.NewSet(append(poolAttrs.ToSlice(), semconv.DBClientConnectionsStateUsed)...)

	idleMax, err := conf.meter.Int64ObservableUpDownCounter(
		semconv.DBClientConnectionIdleMaxName,
		metric.WithDescription(semconv.DBClientConnectionIdleMaxDescription),
		metric.WithUnit(semconv.DBClientConnectionIdleMaxUnit),
	)
	if err != nil {
		return err
	}

	idleMin, err := conf.meter.Int64ObservableUpDownCounter(
		semconv.DBClientConnectionIdleMinName,
		metric.WithDescription(semconv.DBClientConnectionIdleMinDescription),
		metric.WithUnit(semconv.DBClientConnectionIdleMinUnit),
	)
	if err != nil {
		return err
	}

	connMax, err := conf.meter.Int64ObservableUpDownCounter(
		semconv.DBClientConnectionMaxName,
		metric.WithDescription(semconv.DBClientConnectionMaxDescription),
		metric.WithUnit(semconv.DBClientConnectionMaxUnit),
	)
	if err != nil {
		return err
	}

	connCount, err := conf.meter.Int64ObservableUpDownCounter(
		semconv.DBClientConnectionCountName,
		metric.WithDescription(semconv.DBClientConnectionCountDescription),
		metric.WithUnit(semconv.DBClientConnectionCountUnit),
	)
	if err != nil {
		return err
	}

	timeouts, err := conf.meter.Int64ObservableCounter(
		semconv.DBClientConnectionTimeoutsName,
		metric.WithDescription(semconv.DBClientConnectionTimeoutsDescription),
		metric.WithUnit(semconv.DBClientConnectionTimeoutsUnit),
	)
	if err != nil {
		return err
	}

	redisConf := rdb.Options()
	_, err = conf.meter.RegisterCallback(
		func(ctx context.Context, o metric.Observer) error {
			stats := rdb.PoolStats()

			o.ObserveInt64(idleMax, int64(redisConf.MaxIdleConns), metric.WithAttributeSet(poolAttrs))
			o.ObserveInt64(idleMin, int64(redisConf.MinIdleConns), metric.WithAttributeSet(poolAttrs))
			o.ObserveInt64(connMax, int64(redisConf.PoolSize), metric.WithAttributeSet(poolAttrs))

			o.ObserveInt64(connCount, int64(stats.IdleConns), metric.WithAttributeSet(idleAttrs))
			o.ObserveInt64(connCount, int64(stats.TotalConns-stats.IdleConns), metric.WithAttributeSet(usedAttrs))

			o.ObserveInt64(timeouts, int64(stats.Timeouts), metric.WithAttributeSet(poolAttrs))
			return nil
		},
		connCount,
		idleMax,
		idleMin,
		connMax,
		timeouts,
	)

	return err
}

func addMetricsHook(rdb *redis.Client, conf *config) error {
	oprDuration, err := conf.meter.Float64Histogram(
		semconv.DBClientOperationDurationName,
		metric.WithDescription(semconv.DBClientOperationDurationDescription),
		metric.WithUnit(semconv.DBClientOperationDurationUnit),
	)
	if err != nil {
		return err
	}

	createTime, err := conf.meter.Float64Histogram(
		semconv.DBClientConnectionCreateTimeName,
		metric.WithDescription(semconv.DBClientConnectionCreateTimeDescription),
		metric.WithUnit(semconv.DBClientConnectionCreateTimeUnit),
	)
	if err != nil {
		return err
	}

	useTime, err := conf.meter.Float64Histogram(
		semconv.DBClientConnectionUseTimeName,
		metric.WithDescription(semconv.DBClientConnectionUseTimeDescription),
		metric.WithUnit(semconv.DBClientConnectionUseTimeUnit),
	)
	if err != nil {
		return err
	}

	opt := rdb.Options()
	rdb.AddHook(&metricsHook{
		oprDuration: oprDuration,

		createTime: createTime,
		useTime:    useTime,

		dbNamespace: strconv.Itoa(opt.DB),

		baseAttrs:      attribute.NewSet(conf.Attributes()...),
		operationAttrs: commonOperationAttrs(conf, opt),
		poolAttrs:      commonPoolAttrs(conf, opt),
	})
	return nil
}

type metricsHook struct {
	oprDuration metric.Float64Histogram

	createTime metric.Float64Histogram
	useTime    metric.Float64Histogram

	dbNamespace string

	baseAttrs      attribute.Set
	operationAttrs attribute.Set
	poolAttrs      attribute.Set
}

var _ redis.Hook = (*metricsHook)(nil)

func (mh *metricsHook) DialHook(hook redis.DialHook) redis.DialHook {
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		start := time.Now()

		conn, err := hook(ctx, network, addr)

		dur := time.Since(start)
		realAddr := addr
		if err == nil {
			realAddr = conn.RemoteAddr().String() // for redis behind sentinel
		}
		attrs := attribute.NewSet(
			semconv.DBClientConnectionsPoolName(realAddr+"/"+mh.dbNamespace),
			statusAttr(err),
		)

		mh.createTime.Record(ctx, dur.Seconds(), metric.WithAttributeSet(mh.baseAttrs), metric.WithAttributeSet(attrs))
		return conn, err
	}
}

func (mh *metricsHook) operationAttributes(name string, err error) attribute.Set {
	if err == nil {
		return attribute.NewSet(semconv.DBOperationName(name))
	}
	return attribute.NewSet(semconv.DBOperationName(name), errorKindAttr(err))
}

func (mh *metricsHook) ProcessHook(hook redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, cmd redis.Cmder) error {
		start := time.Now()

		err := hook(ctx, cmd)

		dur := time.Since(start)

		mh.oprDuration.Record(ctx, dur.Seconds(), metric.WithAttributeSet(mh.operationAttrs),
			metric.WithAttributeSet(mh.operationAttributes(cmd.FullName(), err)))

		mh.useTime.Record(ctx, dur.Seconds(), metric.WithAttributeSet(mh.poolAttrs),
			metric.WithAttributeSet(attribute.NewSet(statusAttr(err))))

		return err
	}
}

func (mh *metricsHook) ProcessPipelineHook(
	hook redis.ProcessPipelineHook,
) redis.ProcessPipelineHook {
	return func(ctx context.Context, cmds []redis.Cmder) error {
		start := time.Now()

		err := hook(ctx, cmds)

		dur := time.Since(start)

		mh.oprDuration.Record(ctx, dur.Seconds(), metric.WithAttributeSet(mh.operationAttrs),
			metric.WithAttributeSet(mh.operationAttributes("pipeline", err)))

		mh.useTime.Record(ctx, dur.Seconds(), metric.WithAttributeSet(mh.poolAttrs),
			metric.WithAttributeSet(attribute.NewSet(statusAttr(err))))

		return err
	}
}
