package redisotel

import (
	"slices"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/metric/noop"
	semconv "go.opentelemetry.io/otel/semconv/v1.30.0"
	"go.opentelemetry.io/otel/trace"
)

type config struct {
	// Common options.
	attrs []attribute.KeyValue

	// Tracing options.

	tp     trace.TracerProvider
	tracer trace.Tracer

	dbQueryTextEnabled bool

	// Metrics options.

	metricsEnabled bool

	mp    metric.MeterProvider
	meter metric.Meter

	poolName string
}

// Option configures the instrumentation.
type Option interface {
	apply(conf *config)
}

type option func(conf *config)

func (fn option) apply(conf *config) {
	fn(conf)
}

func newConfig(opts ...Option) *config {
	conf := &config{
		attrs: []attribute.KeyValue{},

		tp:     otel.GetTracerProvider(),
		tracer: nil,

		dbQueryTextEnabled: false,

		metricsEnabled: true,

		mp:    otel.GetMeterProvider(),
		meter: nil,

		poolName: "",
	}

	for _, opt := range opts {
		opt.apply(conf)
	}

	conf.attrs = append(conf.attrs, semconv.DBSystemNameRedis)

	if !conf.metricsEnabled { // use noop to disable metrics recording.
		conf.mp = noop.NewMeterProvider()
	}
	conf.meter = conf.mp.Meter(
		instrumName,
		metric.WithInstrumentationVersion(version),
		metric.WithSchemaURL(semconv.SchemaURL),
	)

	conf.tracer = conf.tp.Tracer(
		instrumName,
		trace.WithInstrumentationVersion(version),
		trace.WithSchemaURL(semconv.SchemaURL),
	)

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

// WithTracerProvider specifies a tracer provider to use for creating a tracer.
// If none is specified, the global provider is used.
func WithTracerProvider(provider trace.TracerProvider) Option {
	return option(func(conf *config) {
		conf.tp = provider
	})
}

// EnableDBQueryText tells the tracing hook to log raw redis commands.
func EnableDBQueryText() Option {
	return option(func(conf *config) {
		conf.dbQueryTextEnabled = true
	})
}

// WithMeterProvider configures a metric.Meter used to create instruments.
func WithMeterProvider(mp metric.MeterProvider) Option {
	return option(func(conf *config) {
		conf.mp = mp
	})
}

// DisableMetrics tells the hook not to record metrics.
func DisableMetrics() Option {
	return option(func(conf *config) {
		conf.metricsEnabled = false
	})
}

// Attributes returns the common attributes.
func (conf *config) Attributes() []attribute.KeyValue {
	return slices.Clone(conf.attrs)
}
