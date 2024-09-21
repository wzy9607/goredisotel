package redisotel

import (
	"context"
	"errors"
	"fmt"
	"net"
	"runtime"
	"strings"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"

	"github.com/redis/go-redis/v9"

	"github.com/wzy9607/goredisotel/internal/rediscmd"
)

type clientHook struct {
	conf *config

	operationAttrs []attribute.KeyValue

	spanOpts []trace.SpanStartOption
}

var _ redis.Hook = (*clientHook)(nil)

func InstrumentClientWithHooks(rdb redis.UniversalClient, opts ...TracingOption) error {
	baseOpts := make([]baseOption, len(opts))
	for i, opt := range opts {
		baseOpts[i] = opt
	}
	conf := newConfig(baseOpts...)

	switch rdb := rdb.(type) {
	case *redis.Client:
		rdb.AddHook(newClientHook(rdb.Options(), conf))
		return nil
	case *redis.ClusterClient:
		rdb.AddHook(newClientHook(nil, conf))

		rdb.OnNewNode(func(rdb *redis.Client) {
			rdb.AddHook(newClientHook(rdb.Options(), conf))
		})
		return nil
	case *redis.Ring:
		rdb.AddHook(newClientHook(nil, conf))

		rdb.OnNewNode(func(rdb *redis.Client) {
			rdb.AddHook(newClientHook(rdb.Options(), conf))
		})
		return nil
	default:
		return fmt.Errorf("goredisotel: %T not supported", rdb)
	}
}

func newClientHook(rdsOpt *redis.Options, conf *config) *clientHook {
	operationAttrs := commonOperationAttrs(conf, rdsOpt)
	return &clientHook{
		conf: conf,

		operationAttrs: operationAttrs.ToSlice(),

		spanOpts: []trace.SpanStartOption{
			trace.WithSpanKind(trace.SpanKindClient),
			trace.WithAttributes(conf.Attributes()...),
		},
	}
}

func (th *clientHook) DialHook(hook redis.DialHook) redis.DialHook {
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		ctx, span := th.conf.tracer.Start(ctx, "redis.dial", th.spanOpts...)
		defer span.End()

		conn, err := hook(ctx, network, addr)
		if err != nil {
			recordError(span, err)
			return nil, err
		}
		return conn, nil
	}
}

func (th *clientHook) ProcessHook(hook redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, cmd redis.Cmder) error {
		fn, file, line := funcFileLine("github.com/redis/go-redis")

		attrs := make([]attribute.KeyValue, 0, 8) //nolint:mnd // ignore
		attrs = append(attrs,
			semconv.CodeFunction(fn),
			semconv.CodeFilepath(file),
			semconv.CodeLineNumber(line),
			semconv.DBOperationName(cmd.FullName()),
		)

		if th.conf.dbStmtEnabled {
			cmdString := rediscmd.CmdString(cmd)
			attrs = append(attrs, semconv.DBQueryText(cmdString))
		}

		opts := th.spanOpts
		opts = append(opts, trace.WithAttributes(th.operationAttrs...), trace.WithAttributes(attrs...))

		ctx, span := th.conf.tracer.Start(ctx, cmd.FullName(), opts...)
		defer span.End()

		if err := hook(ctx, cmd); err != nil {
			recordError(span, err)
			return err
		}
		return nil
	}
}

func (th *clientHook) ProcessPipelineHook(
	hook redis.ProcessPipelineHook,
) redis.ProcessPipelineHook {
	return func(ctx context.Context, cmds []redis.Cmder) error {
		fn, file, line := funcFileLine("github.com/redis/go-redis")

		attrs := make([]attribute.KeyValue, 0, 8) //nolint:mnd // ignore
		attrs = append(attrs,
			semconv.CodeFunction(fn),
			semconv.CodeFilepath(file),
			semconv.CodeLineNumber(line),
			semconv.DBOperationName("pipeline"),
			attribute.Int("db.redis.num_cmd", len(cmds)),
		)

		summary, cmdsString := rediscmd.CmdsString(cmds)
		if th.conf.dbStmtEnabled {
			attrs = append(attrs, semconv.DBQueryText(cmdsString))
		}

		opts := th.spanOpts
		opts = append(opts, trace.WithAttributes(th.operationAttrs...), trace.WithAttributes(attrs...))

		ctx, span := th.conf.tracer.Start(ctx, "redis.pipeline "+summary, opts...)
		defer span.End()

		if err := hook(ctx, cmds); err != nil {
			recordError(span, err)
			return err
		}
		return nil
	}
}

func recordError(span trace.Span, err error) {
	span.SetAttributes(errorKindAttr(err))
	if !errors.Is(err, redis.Nil) {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
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
