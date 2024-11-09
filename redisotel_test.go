package redisotel

import (
	"context"
	"testing"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.27.0"
	"go.opentelemetry.io/otel/trace"

	"github.com/redis/go-redis/v9"
)

func TestWithDBStatement(t *testing.T) {
	t.Parallel()
	provider := sdktrace.NewTracerProvider()
	conf := newConfig(WithTracerProvider(provider), WithDBStatement(false))
	hook, err := newClientHook(nil, conf)
	if err != nil {
		t.Fatal(err)
	}
	ctx, span := provider.Tracer("redis-test").Start(context.TODO(), "redis-test")
	cmd := redis.NewCmd(ctx, "ping")
	defer span.End()

	processHook := hook.ProcessHook(func(ctx context.Context, cmd redis.Cmder) error {
		attrs := trace.SpanFromContext(ctx).(sdktrace.ReadOnlySpan).Attributes()
		for _, attr := range attrs {
			if attr.Key == semconv.DBQueryTextKey {
				t.Fatal("Attribute with db statement should not exist")
			}
		}
		return nil
	})
	err = processHook(ctx, cmd)
	if err != nil {
		t.Fatal(err)
	}
}
