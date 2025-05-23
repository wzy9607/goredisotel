package redisotel

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	semconv "go.opentelemetry.io/otel/semconv/v1.32.0"
)

func Test_newPoolStatsInstruments(t *testing.T) {
	t.Parallel()
	mr := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(mr))
	meter := mp.Meter(instrumName, metric.WithInstrumentationVersion(version), metric.WithSchemaURL(semconv.SchemaURL))
	got, err := newPoolStatsInstruments(meter)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.NotNil(t, got.connCount)
	require.NotNil(t, got.connIdleMax)
	require.NotNil(t, got.connIdleMin)
	require.NotNil(t, got.connMax)
	require.NotNil(t, got.connPendingRequests)
	require.NotNil(t, got.connTimeouts)
	require.NotNil(t, got.connWaitTime)
}
