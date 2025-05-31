package redisotel

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/metric/noop"
)

func Test_newPoolStatsInstruments(t *testing.T) {
	t.Parallel()
	meter := noop.NewMeterProvider().Meter("test")
	got, err := newPoolStatsInstruments(meter)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.NotNil(t, got.connCount)
	assert.NotNil(t, got.connIdleMax)
	assert.NotNil(t, got.connIdleMin)
	assert.NotNil(t, got.connMax)
	assert.NotNil(t, got.connPendingRequests)
	assert.NotNil(t, got.connTimeouts)
	assert.NotNil(t, got.connWaitTime)
	assert.NotNil(t, got.connHitCount)
	assert.NotNil(t, got.connMissCount)
	assert.NotNil(t, got.connWaitCount)
	assert.NotNil(t, got.connWaitTimeTotal)
}
