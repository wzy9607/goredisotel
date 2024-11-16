package redisotel

import (
	"context"
	"testing"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.27.0"
	"go.opentelemetry.io/otel/trace"
)

func Test_clientHook_ProcessHookWithDBStatement(t *testing.T) {
	t.Parallel()
	provider := sdktrace.NewTracerProvider()
	type fields struct {
		conf *config
	}
	tests := []struct {
		name    string
		fields  fields
		checkFn func(t *testing.T) func(ctx context.Context, cmd redis.Cmder) error
	}{
		{
			name: "disabled by default",
			fields: fields{
				conf: newConfig(WithTracerProvider(provider)),
			},
			checkFn: func(t *testing.T) func(ctx context.Context, cmd redis.Cmder) error {
				t.Helper()
				return func(ctx context.Context, cmd redis.Cmder) error {
					attrs := trace.SpanFromContext(ctx).(sdktrace.ReadOnlySpan).Attributes()
					for _, attr := range attrs {
						if attr.Key == semconv.DBQueryTextKey {
							t.Fatal("DBQueryText attribute should not exist")
						}
					}
					return nil
				}
			},
		}, {
			name: "enable by option",
			fields: fields{
				conf: newConfig(WithTracerProvider(provider), WithDBStatement(true)),
			},
			checkFn: func(t *testing.T) func(ctx context.Context, cmd redis.Cmder) error {
				t.Helper()
				return func(ctx context.Context, cmd redis.Cmder) error {
					attrs := trace.SpanFromContext(ctx).(sdktrace.ReadOnlySpan).Attributes()
					contains := false
					for _, attr := range attrs {
						if attr.Key == semconv.DBQueryTextKey {
							assert.Equal(t, attr, semconv.DBQueryText("set key value"))
							contains = true
						}
					}
					assert.Truef(t, contains, "should have DBQueryText attribute when enabled")
					return nil
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hook, err := newClientHook(nil, tt.fields.conf)
			if err != nil {
				t.Fatal(err)
			}
			ctx, span := provider.Tracer("redis-test").Start(context.Background(), "redis-test")
			cmd := redis.NewCmd(ctx, "set", "key", "value")
			defer span.End()

			processHook := hook.ProcessHook(tt.checkFn(t))
			err = processHook(ctx, cmd)
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}
