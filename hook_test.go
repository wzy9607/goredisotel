package redisotel

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.opentelemetry.io/otel/sdk/metric/metricdata/metricdatatest"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	semconv "go.opentelemetry.io/otel/semconv/v1.34.0"
	"go.opentelemetry.io/otel/semconv/v1.34.0/dbconv"

	"github.com/redis/go-redis/v9"
)

type fakeConn struct {
	remoteAddr net.IPAddr
}

type fakeError string

type testClientHookFields struct {
	rdsOpt *redis.Options
	opts   []Option
}

func (c fakeConn) Read(b []byte) (n int, err error)   { return 0, nil }
func (c fakeConn) Write(b []byte) (n int, err error)  { return 0, nil }
func (c fakeConn) Close() error                       { return nil }
func (c fakeConn) LocalAddr() net.Addr                { return nil }
func (c fakeConn) RemoteAddr() net.Addr               { return &c.remoteAddr }
func (c fakeConn) SetDeadline(t time.Time) error      { return nil }
func (c fakeConn) SetReadDeadline(t time.Time) error  { return nil }
func (c fakeConn) SetWriteDeadline(t time.Time) error { return nil }
func (e fakeError) Error() string                     { return string(e) }
func (e fakeError) RedisError()                       {}

func testClientHooks(t *testing.T, fields testClientHookFields) (
	*clientHook, *tracetest.SpanRecorder, *sdktrace.TracerProvider, *sdkmetric.ManualReader,
) {
	t.Helper()
	sr := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(sr))
	mr := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(mr))
	ch, err := newClientHook(fields.rdsOpt,
		newConfig(append(fields.opts, WithTracerProvider(tp), WithMeterProvider(mp))...))
	require.NoError(t, err)
	return ch, sr, tp, mr
}

func attrMap(attrs []attribute.KeyValue) map[attribute.Key]attribute.KeyValue {
	m := make(map[attribute.Key]attribute.KeyValue, len(attrs))
	for _, kv := range attrs {
		m[kv.Key] = kv
	}
	return m
}

func assertOprDuration(t *testing.T, metrics metricdata.Metrics, wantAttrs []attribute.KeyValue) {
	t.Helper()
	metricdatatest.AssertEqual(t, metricdata.Metrics{
		Name:        dbconv.ClientOperationDuration{}.Name(),
		Description: dbconv.ClientOperationDuration{}.Description(),
		Unit:        dbconv.ClientOperationDuration{}.Unit(),
		Data: metricdata.Histogram[float64]{
			DataPoints: []metricdata.HistogramDataPoint[float64]{
				{Attributes: attribute.NewSet(wantAttrs...)},
			}, Temporality: metricdata.CumulativeTemporality,
		},
	}, metrics, metricdatatest.IgnoreTimestamp(), metricdatatest.IgnoreExemplars(),
		metricdatatest.IgnoreValue())
}

func assertCreateTime(t *testing.T, metrics metricdata.Metrics, wantAttrs []attribute.KeyValue) {
	t.Helper()
	metricdatatest.AssertEqual(t, metricdata.Metrics{
		Name:        dbconv.ClientConnectionCreateTime{}.Name(),
		Description: dbconv.ClientConnectionCreateTime{}.Description(),
		Unit:        dbconv.ClientConnectionCreateTime{}.Unit(),
		Data: metricdata.Histogram[float64]{
			DataPoints: []metricdata.HistogramDataPoint[float64]{
				{Attributes: attribute.NewSet(wantAttrs...)},
			}, Temporality: metricdata.CumulativeTemporality,
		},
	}, metrics, metricdatatest.IgnoreTimestamp(), metricdatatest.IgnoreExemplars(),
		metricdatatest.IgnoreValue())
}

func assertUseTime(t *testing.T, metrics metricdata.Metrics, wantAttrs []attribute.KeyValue) {
	t.Helper()
	metricdatatest.AssertEqual(t, metricdata.Metrics{
		Name:        dbconv.ClientConnectionUseTime{}.Name(),
		Description: dbconv.ClientConnectionUseTime{}.Description(),
		Unit:        dbconv.ClientConnectionUseTime{}.Unit(),
		Data: metricdata.Histogram[float64]{
			DataPoints: []metricdata.HistogramDataPoint[float64]{
				{Attributes: attribute.NewSet(wantAttrs...)},
			}, Temporality: metricdata.CumulativeTemporality,
		},
	}, metrics, metricdatatest.IgnoreTimestamp(), metricdatatest.IgnoreExemplars(),
		metricdatatest.IgnoreValue())
}

func Test_clientHook_DialHook(t *testing.T) {
	t.Parallel()
	type fields = testClientHookFields
	type args struct {
		hook    redis.DialHook
		network string
		addr    string
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		wantErr bool

		checkSpan    func(t *testing.T, span sdktrace.ReadOnlySpan)
		checkMetrics func(t *testing.T, sm metricdata.ScopeMetrics)
	}{
		{
			name: "success",
			fields: fields{
				rdsOpt: &redis.Options{
					DB: 3,
				},
				opts: []Option{},
			},
			args: args{
				hook: func(ctx context.Context, network, addr string) (net.Conn, error) {
					return fakeConn{remoteAddr: net.IPAddr{IP: net.ParseIP("10.1.1.1")}}, nil
				},
				network: "tcp",
				addr:    "FailoverClient",
			},
			checkSpan: func(t *testing.T, span sdktrace.ReadOnlySpan) {
				t.Helper()
				assert.Equal(t, "redis.dial", span.Name())
				assert.Equal(t, sdktrace.Status{Code: codes.Unset}, span.Status())
				t.Logf("attrs: %v", span.Attributes())

				wantAttrs := []attribute.KeyValue{
					semconv.DBSystemNameRedis,
					semconv.DBClientConnectionPoolName("10.1.1.1/3"),
				}
				assert.Subset(t, span.Attributes(), wantAttrs)
			},
			checkMetrics: func(t *testing.T, sm metricdata.ScopeMetrics) {
				t.Helper()
				require.Len(t, sm.Metrics, 1)

				assertCreateTime(t, sm.Metrics[0], []attribute.KeyValue{
					semconv.DBSystemNameRedis,
					semconv.DBClientConnectionPoolName("10.1.1.1/3"),
					attribute.String("status", "ok"),
				})
			},
		}, {
			name: "error",
			fields: fields{
				rdsOpt: &redis.Options{
					DB: 3,
				},
				opts: []Option{},
			},
			args: args{
				hook: func(ctx context.Context, network, addr string) (net.Conn, error) {
					return nil, errors.New("some error")
				},
				network: "tcp",
				addr:    "FailoverClient",
			},
			wantErr: true,
			checkSpan: func(t *testing.T, span sdktrace.ReadOnlySpan) {
				t.Helper()
				assert.Equal(t, "redis.dial", span.Name())
				assert.Equal(t, sdktrace.Status{Code: codes.Error, Description: "some error"}, span.Status())
				t.Logf("attrs: %v", span.Attributes())

				wantAttrs := []attribute.KeyValue{
					semconv.DBSystemNameRedis,
					semconv.DBClientConnectionPoolName("FailoverClient/3"),
				}
				assert.Subset(t, span.Attributes(), wantAttrs)
			},
			checkMetrics: func(t *testing.T, sm metricdata.ScopeMetrics) {
				t.Helper()
				require.Len(t, sm.Metrics, 1)

				assertCreateTime(t, sm.Metrics[0], []attribute.KeyValue{
					semconv.DBSystemNameRedis,
					semconv.DBClientConnectionPoolName("FailoverClient/3"),
					attribute.String("status", "error"),
				})
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ch, sr, tp, mr := testClientHooks(t, tt.fields)

			ctx, span := tp.Tracer("redis-test").Start(context.Background(), "redis-test")
			_, err := ch.DialHook(tt.args.hook)(ctx, tt.args.network, tt.args.addr)
			span.End()
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}

			spans := sr.Ended()
			assert.Len(t, spans, 2)
			tt.checkSpan(t, spans[0])

			rm := metricdata.ResourceMetrics{}
			require.NoError(t, mr.Collect(context.Background(), &rm))
			require.Len(t, rm.ScopeMetrics, 1)
			tt.checkMetrics(t, rm.ScopeMetrics[0])
		})
	}
}

func Test_clientHook_ProcessHook(t *testing.T) { //nolint:maintidx //table driven tests
	t.Parallel()
	type fields = testClientHookFields
	type args struct {
		hook redis.ProcessHook
		cmd  redis.Cmder
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		wantErr bool

		checkSpan    func(t *testing.T, span sdktrace.ReadOnlySpan)
		checkMetrics func(t *testing.T, sm metricdata.ScopeMetrics)
	}{
		{
			name: "default config, success",
			fields: fields{
				rdsOpt: &redis.Options{Addr: "10.1.1.1:6379", DB: 3},
			},
			args: args{
				hook: func(ctx context.Context, cmd redis.Cmder) error { return nil },
				cmd:  redis.NewCmd(context.Background(), "set", "key", "value"),
			},
			checkSpan: func(t *testing.T, span sdktrace.ReadOnlySpan) {
				t.Helper()
				assert.Equal(t, "set", span.Name())
				assert.Equal(t, sdktrace.Status{Code: codes.Unset}, span.Status())
				t.Logf("attrs: %v", span.Attributes())

				wantAttrs := []attribute.KeyValue{
					semconv.DBSystemNameRedis,
					semconv.DBNamespace("3"),
					semconv.DBOperationName("set"),
					semconv.ServerAddress("10.1.1.1"),
					semconv.ServerPort(6379),
				}
				assert.Subset(t, span.Attributes(), wantAttrs)

				wantNotExistAttrs := []attribute.Key{semconv.DBResponseStatusCodeKey, semconv.DBQueryTextKey}
				attrs := attrMap(span.Attributes())
				for _, key := range wantNotExistAttrs {
					assert.NotContains(t, attrs, key)
				}
			},
			checkMetrics: func(t *testing.T, sm metricdata.ScopeMetrics) {
				t.Helper()
				require.Len(t, sm.Metrics, 2)

				assertOprDuration(t, sm.Metrics[0], []attribute.KeyValue{
					semconv.DBSystemNameRedis,
					semconv.DBNamespace("3"),
					semconv.DBOperationName("set"),
					semconv.ServerAddress("10.1.1.1"),
					semconv.ServerPort(6379),
				})

				assertUseTime(t, sm.Metrics[1], []attribute.KeyValue{
					semconv.DBSystemNameRedis,
					semconv.DBClientConnectionPoolName("10.1.1.1:6379/3"),
					attribute.String("status", "ok"),
				})
			},
		}, {
			name: "default config, evalsha, success",
			fields: fields{
				rdsOpt: &redis.Options{Addr: "10.1.1.1:6379", DB: 3},
			},
			args: args{
				hook: func(ctx context.Context, cmd redis.Cmder) error { return nil },
				cmd:  redis.NewCmd(context.Background(), "evalsha", "sha1", 1, "key1", "key2"),
			},
			checkSpan: func(t *testing.T, span sdktrace.ReadOnlySpan) {
				t.Helper()
				assert.Equal(t, "evalsha", span.Name())
				assert.Equal(t, sdktrace.Status{Code: codes.Unset}, span.Status())
				t.Logf("attrs: %v", span.Attributes())

				wantAttrs := []attribute.KeyValue{
					semconv.DBSystemNameRedis,
					semconv.DBNamespace("3"),
					semconv.DBOperationName("evalsha"),
					semconv.ServerAddress("10.1.1.1"),
					semconv.ServerPort(6379),
					semconv.DBStoredProcedureName("sha1"),
				}
				assert.Subset(t, span.Attributes(), wantAttrs)

				wantNotExistAttrs := []attribute.Key{semconv.DBResponseStatusCodeKey, semconv.DBQueryTextKey}
				attrs := attrMap(span.Attributes())
				for _, key := range wantNotExistAttrs {
					assert.NotContains(t, attrs, key)
				}
			},
			checkMetrics: func(t *testing.T, sm metricdata.ScopeMetrics) {
				t.Helper()
				require.Len(t, sm.Metrics, 2)

				assertOprDuration(t, sm.Metrics[0], []attribute.KeyValue{
					semconv.DBSystemNameRedis,
					semconv.DBNamespace("3"),
					semconv.DBOperationName("evalsha"),
					semconv.ServerAddress("10.1.1.1"),
					semconv.ServerPort(6379),
					semconv.DBStoredProcedureName("sha1"),
				})

				assertUseTime(t, sm.Metrics[1], []attribute.KeyValue{
					semconv.DBSystemNameRedis,
					semconv.DBClientConnectionPoolName("10.1.1.1:6379/3"),
					attribute.String("status", "ok"),
				})
			},
		}, {
			name: "default config, return nil",
			fields: fields{
				rdsOpt: &redis.Options{Addr: "10.1.1.1:6379", DB: 3},
			},
			args: args{
				hook: func(ctx context.Context, cmd redis.Cmder) error { return redis.Nil },
				cmd:  redis.NewCmd(context.Background(), "get", "key"),
			},
			wantErr: true,
			checkSpan: func(t *testing.T, span sdktrace.ReadOnlySpan) {
				t.Helper()
				assert.Equal(t, "get", span.Name())
				assert.Equal(t, sdktrace.Status{Code: codes.Unset}, span.Status())
				t.Logf("attrs: %v", span.Attributes())

				wantAttrs := []attribute.KeyValue{
					semconv.DBSystemNameRedis,
					semconv.DBNamespace("3"),
					semconv.DBOperationName("get"),
					semconv.ServerAddress("10.1.1.1"),
					semconv.ServerPort(6379),
					semconv.ErrorTypeKey.String("redis.Nil"),
				}
				assert.Subset(t, span.Attributes(), wantAttrs)

				wantNotExistAttrs := []attribute.Key{semconv.DBResponseStatusCodeKey, semconv.DBQueryTextKey}
				attrs := attrMap(span.Attributes())
				for _, key := range wantNotExistAttrs {
					assert.NotContains(t, attrs, key)
				}
			},
			checkMetrics: func(t *testing.T, sm metricdata.ScopeMetrics) {
				t.Helper()
				require.Len(t, sm.Metrics, 2)

				assertOprDuration(t, sm.Metrics[0], []attribute.KeyValue{
					semconv.DBSystemNameRedis,
					semconv.DBNamespace("3"),
					semconv.DBOperationName("get"),
					semconv.ServerAddress("10.1.1.1"),
					semconv.ServerPort(6379),
					semconv.ErrorTypeKey.String("redis.Nil"),
				})

				assertUseTime(t, sm.Metrics[1], []attribute.KeyValue{
					semconv.DBSystemNameRedis,
					semconv.DBClientConnectionPoolName("10.1.1.1:6379/3"),
					attribute.String("status", "error"),
				})
			},
		}, {
			name: "default config, return error",
			fields: fields{
				rdsOpt: &redis.Options{Addr: "10.1.1.1:6379", DB: 3},
			},
			args: args{
				hook: func(ctx context.Context, cmd redis.Cmder) error { return fakeError("READONLY aaa") },
				cmd:  redis.NewCmd(context.Background(), "incr", "key"),
			},
			wantErr: true,
			checkSpan: func(t *testing.T, span sdktrace.ReadOnlySpan) {
				t.Helper()
				assert.Equal(t, "incr", span.Name())
				assert.Equal(t, sdktrace.Status{Code: codes.Error, Description: "READONLY aaa"}, span.Status())
				t.Logf("attrs: %v", span.Attributes())

				wantAttrs := []attribute.KeyValue{
					semconv.DBSystemNameRedis,
					semconv.DBNamespace("3"),
					semconv.DBOperationName("incr"),
					semconv.ServerAddress("10.1.1.1"),
					semconv.ServerPort(6379),
					semconv.DBResponseStatusCode("READONLY"),
					semconv.ErrorTypeKey.String("redis.READONLY"),
				}
				assert.Subset(t, span.Attributes(), wantAttrs)

				wantNotExistAttrs := []attribute.Key{semconv.DBQueryTextKey}
				attrs := attrMap(span.Attributes())
				for _, key := range wantNotExistAttrs {
					assert.NotContains(t, attrs, key)
				}
			},
			checkMetrics: func(t *testing.T, sm metricdata.ScopeMetrics) {
				t.Helper()
				require.Len(t, sm.Metrics, 2)

				assertOprDuration(t, sm.Metrics[0], []attribute.KeyValue{
					semconv.DBSystemNameRedis,
					semconv.DBNamespace("3"),
					semconv.DBOperationName("incr"),
					semconv.ServerAddress("10.1.1.1"),
					semconv.ServerPort(6379),
					semconv.DBResponseStatusCode("READONLY"),
					semconv.ErrorTypeKey.String("redis.READONLY"),
				})

				assertUseTime(t, sm.Metrics[1], []attribute.KeyValue{
					semconv.DBSystemNameRedis,
					semconv.DBClientConnectionPoolName("10.1.1.1:6379/3"),
					attribute.String("status", "error"),
				})
			},
		}, {
			name: "enable DBQueryText option",
			fields: fields{
				rdsOpt: &redis.Options{Addr: "10.1.1.1:6379", DB: 3},
				opts:   []Option{EnableDBQueryText()},
			},
			args: args{
				hook: func(ctx context.Context, cmd redis.Cmder) error { return nil },
				cmd:  redis.NewCmd(context.Background(), "set", "key", "value"),
			},
			checkSpan: func(t *testing.T, span sdktrace.ReadOnlySpan) {
				t.Helper()
				t.Logf("attrs: %v", span.Attributes())

				wantAttrs := []attribute.KeyValue{
					semconv.DBSystemNameRedis,
					semconv.DBNamespace("3"),
					semconv.DBOperationName("set"),
					semconv.ServerAddress("10.1.1.1"),
					semconv.ServerPort(6379),
					semconv.DBQueryText("set key value"),
				}
				assert.Subset(t, span.Attributes(), wantAttrs)

				wantNotExistAttrs := []attribute.Key{semconv.DBResponseStatusCodeKey}
				attrs := attrMap(span.Attributes())
				for _, key := range wantNotExistAttrs {
					assert.NotContains(t, attrs, key)
				}
			},
			checkMetrics: func(t *testing.T, sm metricdata.ScopeMetrics) {
				t.Helper()
				require.Len(t, sm.Metrics, 2)

				assertOprDuration(t, sm.Metrics[0], []attribute.KeyValue{
					semconv.DBSystemNameRedis,
					semconv.DBNamespace("3"),
					semconv.DBOperationName("set"),
					semconv.ServerAddress("10.1.1.1"),
					semconv.ServerPort(6379),
				})

				assertUseTime(t, sm.Metrics[1], []attribute.KeyValue{
					semconv.DBSystemNameRedis,
					semconv.DBClientConnectionPoolName("10.1.1.1:6379/3"),
					attribute.String("status", "ok"),
				})
			},
		}, {
			name: "DisableMetrics option",
			fields: fields{
				rdsOpt: &redis.Options{Addr: "10.1.1.1:6379", DB: 3},
				opts:   []Option{DisableMetrics()},
			},
			args: args{
				hook: func(ctx context.Context, cmd redis.Cmder) error { return nil },
				cmd:  redis.NewCmd(context.Background(), "set", "key", "value"),
			},
			checkSpan: func(t *testing.T, span sdktrace.ReadOnlySpan) {
				t.Helper()
				assert.Equal(t, "set", span.Name())
				assert.Equal(t, sdktrace.Status{Code: codes.Unset}, span.Status())
				t.Logf("attrs: %v", span.Attributes())

				wantAttrs := []attribute.KeyValue{
					semconv.DBSystemNameRedis,
					semconv.DBNamespace("3"),
					semconv.DBOperationName("set"),
					semconv.ServerAddress("10.1.1.1"),
					semconv.ServerPort(6379),
				}
				assert.Subset(t, span.Attributes(), wantAttrs)

				wantNotExistAttrs := []attribute.Key{semconv.DBResponseStatusCodeKey, semconv.DBQueryTextKey}
				attrs := attrMap(span.Attributes())
				for _, key := range wantNotExistAttrs {
					assert.NotContains(t, attrs, key)
				}
			},
		}, {
			name: "WithDB option",
			fields: fields{
				rdsOpt: &redis.Options{Addr: "10.1.1.1:6379", DB: 0},
				opts:   []Option{WithDB(3)},
			},
			args: args{
				hook: func(ctx context.Context, cmd redis.Cmder) error { return nil },
				cmd:  redis.NewCmd(context.Background(), "set", "key", "value"),
			},
			checkSpan: func(t *testing.T, span sdktrace.ReadOnlySpan) {
				t.Helper()
				assert.Equal(t, "set", span.Name())
				assert.Equal(t, sdktrace.Status{Code: codes.Unset}, span.Status())
				t.Logf("attrs: %v", span.Attributes())

				wantAttrs := []attribute.KeyValue{
					semconv.DBSystemNameRedis,
					semconv.DBNamespace("3"),
					semconv.DBOperationName("set"),
					semconv.ServerAddress("10.1.1.1"),
					semconv.ServerPort(6379),
				}
				assert.Subset(t, span.Attributes(), wantAttrs)

				wantNotExistAttrs := []attribute.Key{semconv.DBResponseStatusCodeKey, semconv.DBQueryTextKey}
				attrs := attrMap(span.Attributes())
				for _, key := range wantNotExistAttrs {
					assert.NotContains(t, attrs, key)
				}
			},
			checkMetrics: func(t *testing.T, sm metricdata.ScopeMetrics) {
				t.Helper()
				require.Len(t, sm.Metrics, 2)

				assertOprDuration(t, sm.Metrics[0], []attribute.KeyValue{
					semconv.DBSystemNameRedis,
					semconv.DBNamespace("3"),
					semconv.DBOperationName("set"),
					semconv.ServerAddress("10.1.1.1"),
					semconv.ServerPort(6379),
				})

				assertUseTime(t, sm.Metrics[1], []attribute.KeyValue{
					semconv.DBSystemNameRedis,
					semconv.DBClientConnectionPoolName("10.1.1.1:6379/3"),
					attribute.String("status", "ok"),
				})
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ch, sr, tp, mr := testClientHooks(t, tt.fields)

			ctx, span := tp.Tracer("redis-test").Start(context.Background(), "redis-test")
			err := ch.ProcessHook(tt.args.hook)(ctx, tt.args.cmd)
			span.End()
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}

			spans := sr.Ended()
			assert.Len(t, spans, 2)
			tt.checkSpan(t, spans[0])

			rm := metricdata.ResourceMetrics{}
			require.NoError(t, mr.Collect(context.Background(), &rm))
			if tt.checkMetrics == nil { // DisableMetrics
				require.Empty(t, rm.ScopeMetrics)
				return
			}
			require.Len(t, rm.ScopeMetrics, 1)
			tt.checkMetrics(t, rm.ScopeMetrics[0])
		})
	}
}

func Test_clientHook_ProcessPipelineHook(t *testing.T) {
	t.Parallel()
	type fields = testClientHookFields
	type args struct {
		hook redis.ProcessPipelineHook
		cmds []redis.Cmder
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		wantErr bool

		checkSpan    func(t *testing.T, span sdktrace.ReadOnlySpan)
		checkMetrics func(t *testing.T, sm metricdata.ScopeMetrics)
	}{
		{
			name: "default config, success",
			fields: fields{
				rdsOpt: &redis.Options{Addr: "10.1.1.1:6379", DB: 3},
			},
			args: args{
				hook: func(ctx context.Context, cmds []redis.Cmder) error { return nil },
				cmds: []redis.Cmder{
					redis.NewCmd(context.Background(), "set", "key", "value"),
					redis.NewCmd(context.Background(), "get", "key1"),
					redis.NewCmd(context.Background(), "get", "key2"),
				},
			},
			checkSpan: func(t *testing.T, span sdktrace.ReadOnlySpan) {
				t.Helper()
				assert.Equal(t, "PIPELINE set get", span.Name())
				assert.Equal(t, sdktrace.Status{Code: codes.Unset}, span.Status())
				t.Logf("attrs: %v", span.Attributes())

				wantAttrs := []attribute.KeyValue{
					semconv.DBSystemNameRedis,
					semconv.DBNamespace("3"),
					semconv.DBOperationName("PIPELINE set get"),
					semconv.ServerAddress("10.1.1.1"),
					semconv.ServerPort(6379),
				}
				assert.Subset(t, span.Attributes(), wantAttrs)

				wantNotExistAttrs := []attribute.Key{semconv.DBResponseStatusCodeKey, semconv.DBQueryTextKey}
				attrs := attrMap(span.Attributes())
				for _, key := range wantNotExistAttrs {
					assert.NotContains(t, attrs, key)
				}
			},
			checkMetrics: func(t *testing.T, sm metricdata.ScopeMetrics) {
				t.Helper()
				require.Len(t, sm.Metrics, 2)

				assertOprDuration(t, sm.Metrics[0], []attribute.KeyValue{
					semconv.DBSystemNameRedis,
					semconv.DBNamespace("3"),
					semconv.DBOperationName("PIPELINE set get"),
					semconv.DBOperationBatchSize(3),
					semconv.ServerAddress("10.1.1.1"),
					semconv.ServerPort(6379),
				})

				assertUseTime(t, sm.Metrics[1], []attribute.KeyValue{
					semconv.DBSystemNameRedis,
					semconv.DBClientConnectionPoolName("10.1.1.1:6379/3"),
					attribute.String("status", "ok"),
				})
			},
		}, {
			name: "enable DBQueryText option",
			fields: fields{
				rdsOpt: &redis.Options{Addr: "10.1.1.1:6379", DB: 3},
				opts:   []Option{EnableDBQueryText()},
			},
			args: args{
				hook: func(ctx context.Context, cmds []redis.Cmder) error { return nil },
				cmds: []redis.Cmder{
					redis.NewCmd(context.Background(), "set", "key", "value"),
					redis.NewCmd(context.Background(), "get", "key1"),
					redis.NewCmd(context.Background(), "get", "key2"),
				},
			},
			checkSpan: func(t *testing.T, span sdktrace.ReadOnlySpan) {
				t.Helper()
				assert.Equal(t, "PIPELINE set get", span.Name())
				assert.Equal(t, sdktrace.Status{Code: codes.Unset}, span.Status())
				t.Logf("attrs: %v", span.Attributes())

				wantAttrs := []attribute.KeyValue{
					semconv.DBSystemNameRedis,
					semconv.DBNamespace("3"),
					semconv.DBOperationName("PIPELINE set get"),
					semconv.ServerAddress("10.1.1.1"),
					semconv.ServerPort(6379),
					semconv.DBQueryText("set key value\nget key1\nget key2"),
				}
				assert.Subset(t, span.Attributes(), wantAttrs)

				wantNotExistAttrs := []attribute.Key{semconv.DBResponseStatusCodeKey}
				attrs := attrMap(span.Attributes())
				for _, key := range wantNotExistAttrs {
					assert.NotContains(t, attrs, key)
				}
			},
			checkMetrics: func(t *testing.T, sm metricdata.ScopeMetrics) {
				t.Helper()
				require.Len(t, sm.Metrics, 2)

				assertOprDuration(t, sm.Metrics[0], []attribute.KeyValue{
					semconv.DBSystemNameRedis,
					semconv.DBNamespace("3"),
					semconv.DBOperationName("PIPELINE set get"),
					semconv.DBOperationBatchSize(3),
					semconv.ServerAddress("10.1.1.1"),
					semconv.ServerPort(6379),
				})

				assertUseTime(t, sm.Metrics[1], []attribute.KeyValue{
					semconv.DBSystemNameRedis,
					semconv.DBClientConnectionPoolName("10.1.1.1:6379/3"),
					attribute.String("status", "ok"),
				})
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ch, sr, tp, mr := testClientHooks(t, tt.fields)

			ctx, span := tp.Tracer("redis-test").Start(context.Background(), "redis-test")
			err := ch.ProcessPipelineHook(tt.args.hook)(ctx, tt.args.cmds)
			span.End()
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}

			spans := sr.Ended()
			assert.Len(t, spans, 2)
			tt.checkSpan(t, spans[0])

			rm := metricdata.ResourceMetrics{}
			require.NoError(t, mr.Collect(context.Background(), &rm))
			require.Len(t, rm.ScopeMetrics, 1)
			tt.checkMetrics(t, rm.ScopeMetrics[0])
		})
	}
}
