package redisotel

import (
	"testing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
)

type providerFunc func(name string, opts ...trace.TracerOption) trace.TracerProvider

func (fn providerFunc) TracerProvider(name string, opts ...trace.TracerOption) trace.TracerProvider {
	return fn(name, opts...)
}

func Test_newConfigWithTracerProvider(t *testing.T) {
	t.Parallel()
	invoked := false

	tp := providerFunc(func(name string, opts ...trace.TracerOption) trace.TracerProvider {
		invoked = true
		return otel.GetTracerProvider()
	})

	_ = newConfig(WithTracerProvider(tp.TracerProvider("redis-test")))

	if !invoked {
		t.Fatalf("did not call custom TraceProvider")
	}
}
