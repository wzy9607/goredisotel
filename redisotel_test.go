package redisotel

import (
	"context"
	"testing"

	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"

	"github.com/redis/go-redis/v9"
)

type providerFunc func(name string, opts ...trace.TracerOption) trace.TracerProvider

func (fn providerFunc) TracerProvider(name string, opts ...trace.TracerOption) trace.TracerProvider {
	return fn(name, opts...)
}

func TestNewWithTracerProvider(t *testing.T) {
	t.Parallel()
	invoked := false

	tp := providerFunc(func(name string, opts ...trace.TracerOption) trace.TracerProvider {
		invoked = true
		return otel.GetTracerProvider()
	})

	_ = newTracingHook(&redis.Options{DB: 1}, WithTracerProvider(tp.TracerProvider("redis-test")))

	if !invoked {
		t.Fatalf("did not call custom TraceProvider")
	}
}

func TestWithDBStatement(t *testing.T) {
	t.Parallel()
	provider := sdktrace.NewTracerProvider()
	hook := newTracingHook(
		nil,
		WithTracerProvider(provider),
		WithDBStatement(false),
	)
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
	err := processHook(ctx, cmd)
	if err != nil {
		t.Fatal(err)
	}
}
