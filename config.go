package redisotel

import (
	"slices"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
)

type config struct {
	// Common options.
	attrs []attribute.KeyValue

	// Tracing options.

	tp     trace.TracerProvider
	tracer trace.Tracer

	dbStmtEnabled bool

	// Metrics options.

	mp    metric.MeterProvider
	meter metric.Meter

	poolName string
}

type baseOption interface {
	apply(conf *config)
}

type Option interface {
	baseOption
	tracing()
	metrics()
}

type option func(conf *config)

func (fn option) apply(conf *config) {
	fn(conf)
}

func (fn option) tracing() {}

func (fn option) metrics() {}

func newConfig(opts ...baseOption) *config {
	conf := &config{
		attrs: []attribute.KeyValue{},

		tp:     otel.GetTracerProvider(),
		tracer: nil,

		dbStmtEnabled: true,

		mp:    otel.GetMeterProvider(),
		meter: nil,

		poolName: "",
	}

	for _, opt := range opts {
		opt.apply(conf)
	}

	conf.attrs = append(conf.attrs, semconv.DBSystemRedis)

	return conf
}

// WithPoolName specifies the pool name to use in the attributes.
func WithPoolName(poolName string) Option {
	return option(func(conf *config) {
		conf.poolName = poolName
	})
}

// WithAttributes specifies additional attributes to be added to the span.
func WithAttributes(attrs ...attribute.KeyValue) Option {
	return option(func(conf *config) {
		conf.attrs = append(conf.attrs, attrs...)
	})
}

// ------------------------------------------------------------------------------

type TracingOption interface {
	baseOption
	tracing()
}

type tracingOption func(conf *config)

var _ TracingOption = (*tracingOption)(nil)

func (fn tracingOption) apply(conf *config) {
	fn(conf)
}

func (fn tracingOption) tracing() {}

// WithTracerProvider specifies a tracer provider to use for creating a tracer.
// If none is specified, the global provider is used.
func WithTracerProvider(provider trace.TracerProvider) TracingOption {
	return tracingOption(func(conf *config) {
		conf.tp = provider
	})
}

// WithDBStatement tells the tracing hook not to log raw redis commands.
func WithDBStatement(on bool) TracingOption {
	return tracingOption(func(conf *config) {
		conf.dbStmtEnabled = on
	})
}

// ------------------------------------------------------------------------------

type MetricsOption interface {
	baseOption
	metrics()
}

type metricsOption func(conf *config)

var _ MetricsOption = (*metricsOption)(nil)

func (fn metricsOption) apply(conf *config) {
	fn(conf)
}

func (fn metricsOption) metrics() {}

// WithMeterProvider configures a metric.Meter used to create instruments.
func WithMeterProvider(mp metric.MeterProvider) MetricsOption {
	return metricsOption(func(conf *config) {
		conf.mp = mp
	})
}

func (conf *config) Attributes() []attribute.KeyValue {
	return slices.Clone(conf.attrs)
}
