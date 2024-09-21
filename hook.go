package redisotel

import (
	"context"
	"errors"
	"fmt"
	"net"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"

	"github.com/redis/go-redis/v9"

	"github.com/wzy9607/goredisotel/internal/rediscmd"
)

type clientHook struct {
	conf *config

	dbNamespace string

	baseAttrSet      attribute.Set
	operationAttrs   []attribute.KeyValue
	operationAttrSet attribute.Set
	poolAttrSet      attribute.Set

	spanOpts []trace.SpanStartOption

	instruments *hookInstruments
}

var _ redis.Hook = (*clientHook)(nil)

// InstrumentClientWithHooks starts reporting OpenTelemetry Tracing and Metrics.
//
// Based on https://opentelemetry.io/docs/specs/semconv/database/.
func InstrumentClientWithHooks(rdb redis.UniversalClient, opts ...Option) error {
	conf := newConfig(opts...)
	newOpts := append(slices.Clone(opts), DisableMetrics())
	confMetricDisabled := newConfig(newOpts...)

	switch rdb := rdb.(type) {
	case *redis.Client:
		if err := addHook(rdb, rdb.Options(), conf); err != nil {
			return err
		}
		return nil
	case *redis.ClusterClient:
		if err := addHook(rdb, nil, confMetricDisabled); err != nil {
			return err
		}

		rdb.OnNewNode(func(rdb *redis.Client) {
			if err := addHook(rdb, rdb.Options(), conf); err != nil {
				otel.Handle(err)
			}
		})
		return nil
	case *redis.Ring:
		if err := addHook(rdb, nil, confMetricDisabled); err != nil {
			return err
		}

		rdb.OnNewNode(func(rdb *redis.Client) {
			if err := addHook(rdb, rdb.Options(), conf); err != nil {
				otel.Handle(err)
			}
		})
		return nil
	default:
		return fmt.Errorf("goredisotel: %T not supported", rdb)
	}
}

func addHook(rdb redis.UniversalClient, rdsOpt *redis.Options, conf *config) error {
	hook, err := newClientHook(rdsOpt, conf)
	if err != nil {
		return err
	}
	rdb.AddHook(hook)
	return nil
}

func newClientHook(rdsOpt *redis.Options, conf *config) (*clientHook, error) {
	var instruments *hookInstruments
	if conf.MetricsEnabled() {
		var err error
		if instruments, err = newHookInstruments(conf.meter); err != nil {
			return nil, err
		}
	}

	var dbNamespace string
	if rdsOpt != nil {
		dbNamespace = strconv.Itoa(rdsOpt.DB)
	}

	operationAttrSet := commonOperationAttrs(conf, rdsOpt)
	return &clientHook{
		conf: conf,

		dbNamespace: dbNamespace,

		baseAttrSet:      attribute.NewSet(conf.Attributes()...),
		operationAttrs:   operationAttrSet.ToSlice(),
		operationAttrSet: operationAttrSet,
		poolAttrSet:      commonPoolAttrs(conf, rdsOpt),

		spanOpts: []trace.SpanStartOption{
			trace.WithSpanKind(trace.SpanKindClient),
			trace.WithAttributes(conf.Attributes()...),
		},

		instruments: instruments,
	}, nil
}

func (ch *clientHook) DialHook(hook redis.DialHook) redis.DialHook {
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		start := time.Now()
		ctx, span := ch.conf.tracer.Start(ctx, "redis.dial", ch.spanOpts...)
		defer span.End()

		conn, err := hook(ctx, network, addr)

		dur := time.Since(start)
		realAddr := addr
		if err != nil {
			ch.recordDialError(span, err)
		} else {
			realAddr = conn.RemoteAddr().String() // for redis behind sentinel
		}

		if ch.conf.MetricsEnabled() {
			attrs := attribute.NewSet(
				semconv.DBClientConnectionsPoolName(realAddr+"/"+ch.dbNamespace),
				statusAttr(err),
			)

			ch.instruments.createTime.Record(ctx, dur.Seconds(),
				metric.WithAttributeSet(ch.baseAttrSet), metric.WithAttributeSet(attrs))
		}
		return conn, err
	}
}

func (ch *clientHook) ProcessHook(hook redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, cmd redis.Cmder) error {
		fn, file, line := funcFileLine("github.com/redis/go-redis")

		attrs := make([]attribute.KeyValue, 0, 8) //nolint:mnd // ignore
		metricAttrs := make([]attribute.KeyValue, 0, 2)
		attrs = append(attrs,
			semconv.CodeFunction(fn),
			semconv.CodeFilepath(file),
			semconv.CodeLineNumber(line),
			semconv.DBOperationName(cmd.FullName()),
		)
		metricAttrs = append(metricAttrs,
			semconv.DBOperationName(cmd.FullName()),
		)

		if ch.conf.dbStmtEnabled {
			cmdString := rediscmd.CmdString(cmd)
			attrs = append(attrs, semconv.DBQueryText(cmdString))
		}

		opts := ch.spanOpts
		opts = append(opts, trace.WithAttributes(ch.operationAttrs...), trace.WithAttributes(attrs...))

		start := time.Now()
		ctx, span := ch.conf.tracer.Start(ctx, cmd.FullName(), opts...)
		defer span.End()

		err := hook(ctx, cmd)

		dur := time.Since(start)
		if err != nil {
			metricAttrs = ch.recordError(span, metricAttrs, err)
		}

		if ch.conf.MetricsEnabled() {
			ch.instruments.oprDuration.Record(ctx, dur.Seconds(), metric.WithAttributeSet(ch.operationAttrSet),
				metric.WithAttributes(metricAttrs...))
			ch.instruments.useTime.Record(ctx, dur.Seconds(), metric.WithAttributeSet(ch.poolAttrSet),
				metric.WithAttributes(statusAttr(err)))
		}
		return err
	}
}

func (ch *clientHook) ProcessPipelineHook(
	hook redis.ProcessPipelineHook,
) redis.ProcessPipelineHook {
	return func(ctx context.Context, cmds []redis.Cmder) error {
		fn, file, line := funcFileLine("github.com/redis/go-redis")

		attrs := make([]attribute.KeyValue, 0, 8) //nolint:mnd // ignore
		metricAttrs := make([]attribute.KeyValue, 0, 3)
		attrs = append(attrs,
			semconv.CodeFunction(fn),
			semconv.CodeFilepath(file),
			semconv.CodeLineNumber(line),
			semconv.DBOperationName("pipeline"),
			attribute.Int("db.redis.num_cmd", len(cmds)),
		)
		metricAttrs = append(metricAttrs,
			semconv.DBOperationName("pipeline"),
			attribute.Int("db.redis.num_cmd", len(cmds)),
		)

		summary, cmdsString := rediscmd.CmdsString(cmds)
		if ch.conf.dbStmtEnabled {
			attrs = append(attrs, semconv.DBQueryText(cmdsString))
		}

		opts := ch.spanOpts
		opts = append(opts, trace.WithAttributes(ch.operationAttrs...), trace.WithAttributes(attrs...))

		start := time.Now()
		ctx, span := ch.conf.tracer.Start(ctx, "redis.pipeline "+summary, opts...)
		defer span.End()

		err := hook(ctx, cmds)

		dur := time.Since(start)
		if err != nil {
			metricAttrs = ch.recordError(span, metricAttrs, err)
		}

		if ch.conf.MetricsEnabled() {
			ch.instruments.oprDuration.Record(ctx, dur.Seconds(), metric.WithAttributeSet(ch.operationAttrSet),
				metric.WithAttributes(metricAttrs...))
			ch.instruments.useTime.Record(ctx, dur.Seconds(), metric.WithAttributeSet(ch.poolAttrSet),
				metric.WithAttributes(statusAttr(err)))
		}
		return err
	}
}

func (ch *clientHook) recordDialError(span trace.Span, err error) {
	errorKind := errorKindAttr(err)
	span.SetAttributes(errorKind)
	if !errors.Is(err, redis.Nil) {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
}

func (ch *clientHook) recordError(
	span trace.Span, metricAttrs []attribute.KeyValue, err error,
) (newMetricAttrs []attribute.KeyValue) {
	errorKind := errorKindAttr(err)
	span.SetAttributes(errorKind)
	metricAttrs = append(metricAttrs, errorKind)
	if !errors.Is(err, redis.Nil) {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
	return metricAttrs
}

func funcFileLine(pkg string) (fnName, file string, line int) {
	const depth = 16
	var pcs [depth]uintptr
	n := runtime.Callers(3, pcs[:]) //nolint:mnd // ignore
	ff := runtime.CallersFrames(pcs[:n])

	for {
		f, ok := ff.Next()
		if !ok {
			break
		}
		fnName, file, line = f.Function, f.File, f.Line
		if !strings.Contains(fnName, pkg) {
			break
		}
	}

	if ind := strings.LastIndexByte(fnName, '/'); ind != -1 {
		fnName = fnName[ind+1:]
	}

	return fnName, file, line
}
