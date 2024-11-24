package redisotel

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	semconv "go.opentelemetry.io/otel/semconv/v1.27.0"
)

type fakeConn struct {
	remoteAddr net.IPAddr
}

type fakeError string

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

func attrMap(attrs []attribute.KeyValue) map[attribute.Key]attribute.KeyValue {
	m := make(map[attribute.Key]attribute.KeyValue, len(attrs))
	for _, kv := range attrs {
		m[kv.Key] = kv
	}
	return m
}

func Test_clientHook_DialHook(t *testing.T) {
	t.Parallel()
	type fields struct {
		rdsOpt *redis.Options
		opts   []Option
	}
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
		checkFn func(t *testing.T, span sdktrace.ReadOnlySpan)
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
			checkFn: func(t *testing.T, span sdktrace.ReadOnlySpan) {
				t.Helper()
				assert.Equal(t, "redis.dial", span.Name())
				assert.Equal(t, sdktrace.Status{Code: codes.Unset}, span.Status())
				attrs := attrMap(span.Attributes())
				t.Logf("attrs: %v", attrs)

				kv, ok := attrs[semconv.DBSystemKey]
				assert.True(t, ok)
				assert.Equal(t, semconv.DBSystemRedis, kv)

				kv, ok = attrs[semconv.DBClientConnectionPoolNameKey]
				assert.True(t, ok)
				assert.Equal(t, "10.1.1.1/3", kv.Value.AsString())
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
			checkFn: func(t *testing.T, span sdktrace.ReadOnlySpan) {
				t.Helper()
				assert.Equal(t, "redis.dial", span.Name())
				assert.Equal(t, sdktrace.Status{Code: codes.Error, Description: "some error"}, span.Status())
				attrs := attrMap(span.Attributes())
				t.Logf("attrs: %v", attrs)

				kv, ok := attrs[semconv.DBSystemKey]
				assert.True(t, ok)
				assert.Equal(t, semconv.DBSystemRedis, kv)

				kv, ok = attrs[semconv.DBClientConnectionPoolNameKey]
				assert.True(t, ok)
				assert.Equal(t, "FailoverClient/3", kv.Value.AsString())
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sr := tracetest.NewSpanRecorder()
			tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(sr))
			ch, err := newClientHook(tt.fields.rdsOpt, newConfig(append(tt.fields.opts, WithTracerProvider(tp))...))
			require.NoError(t, err)

			ctx, span := tp.Tracer("redis-test").Start(context.Background(), "redis-test")
			_, err = ch.DialHook(tt.args.hook)(ctx, tt.args.network, tt.args.addr)
			span.End()

			spans := sr.Ended()
			assert.Len(t, spans, 2)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			tt.checkFn(t, spans[0])
		})
	}
}

func Test_clientHook_ProcessHook(t *testing.T) {
	t.Parallel()
	type fields struct {
		rdsOpt *redis.Options
		opts   []Option
	}
	type args struct {
		hook redis.ProcessHook
		cmd  redis.Cmder
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		wantErr bool
		checkFn func(t *testing.T, span sdktrace.ReadOnlySpan)
	}{
		{
			name: "default config, success",
			fields: fields{
				rdsOpt: &redis.Options{
					Addr: "10.1.1.1:6379",
					DB:   3,
				},
			},
			args: args{
				hook: func(ctx context.Context, cmd redis.Cmder) error { return nil },
				cmd:  redis.NewCmd(context.Background(), "set", "key", "value"),
			},
			checkFn: func(t *testing.T, span sdktrace.ReadOnlySpan) {
				t.Helper()
				assert.Equal(t, "set 3", span.Name())
				assert.Equal(t, sdktrace.Status{Code: codes.Unset}, span.Status())
				attrs := attrMap(span.Attributes())
				t.Logf("attrs: %v", attrs)

				kv, ok := attrs[semconv.DBSystemKey]
				assert.True(t, ok)
				assert.Equal(t, semconv.DBSystemRedis, kv)

				kv, ok = attrs[semconv.DBNamespaceKey]
				assert.True(t, ok)
				assert.Equal(t, "3", kv.Value.AsString())

				kv, ok = attrs[semconv.DBOperationNameKey]
				assert.True(t, ok)
				assert.Equal(t, "set", kv.Value.AsString())

				_, ok = attrs[semconv.DBQueryTextKey]
				assert.Falsef(t, ok, "DBQueryText attribute should not exist")

				kv, ok = attrs[semconv.ServerAddressKey]
				assert.True(t, ok)
				assert.Equal(t, "10.1.1.1", kv.Value.AsString())

				kv, ok = attrs[semconv.ServerPortKey]
				assert.True(t, ok)
				assert.Equal(t, int64(6379), kv.Value.AsInt64())
			},
		}, {
			name: "default config, nil",
			fields: fields{
				rdsOpt: &redis.Options{
					DB: 3,
				},
			},
			args: args{
				hook: func(ctx context.Context, cmd redis.Cmder) error { return redis.Nil },
				cmd:  redis.NewCmd(context.Background(), "get", "key"),
			},
			wantErr: true,
			checkFn: func(t *testing.T, span sdktrace.ReadOnlySpan) {
				t.Helper()
				assert.Equal(t, "get 3", span.Name())
				assert.Equal(t, sdktrace.Status{Code: codes.Unset}, span.Status())
				attrs := attrMap(span.Attributes())
				t.Logf("attrs: %v", attrs)

				kv, ok := attrs[semconv.DBSystemKey]
				assert.True(t, ok)
				assert.Equal(t, semconv.DBSystemRedis, kv)

				kv, ok = attrs[semconv.DBNamespaceKey]
				assert.True(t, ok)
				assert.Equal(t, "3", kv.Value.AsString())

				kv, ok = attrs[semconv.DBOperationNameKey]
				assert.True(t, ok)
				assert.Equal(t, "get", kv.Value.AsString())

				_, ok = attrs[semconv.DBQueryTextKey]
				assert.Falsef(t, ok, "DBQueryText attribute should not exist")

				kv, ok = attrs[semconv.ErrorTypeKey]
				assert.True(t, ok)
				assert.Equal(t, "redis.Nil", kv.Value.AsString())
			},
		}, {
			name: "default config, error",
			fields: fields{
				rdsOpt: &redis.Options{
					DB: 3,
				},
			},
			args: args{
				hook: func(ctx context.Context, cmd redis.Cmder) error { return fakeError("READONLY aaa") },
				cmd:  redis.NewCmd(context.Background(), "incr", "key"),
			},
			wantErr: true,
			checkFn: func(t *testing.T, span sdktrace.ReadOnlySpan) {
				t.Helper()
				assert.Equal(t, "incr 3", span.Name())
				assert.Equal(t, sdktrace.Status{Code: codes.Error, Description: "READONLY aaa"}, span.Status())
				attrs := attrMap(span.Attributes())
				t.Logf("attrs: %v", attrs)

				kv, ok := attrs[semconv.DBSystemKey]
				assert.True(t, ok)
				assert.Equal(t, semconv.DBSystemRedis, kv)

				kv, ok = attrs[semconv.DBNamespaceKey]
				assert.True(t, ok)
				assert.Equal(t, "3", kv.Value.AsString())

				kv, ok = attrs[semconv.DBOperationNameKey]
				assert.True(t, ok)
				assert.Equal(t, "incr", kv.Value.AsString())

				_, ok = attrs[semconv.DBQueryTextKey]
				assert.Falsef(t, ok, "DBQueryText attribute should not exist")

				kv, ok = attrs[semconv.ErrorTypeKey]
				assert.True(t, ok)
				assert.Equal(t, "redis.READONLY", kv.Value.AsString())
			},
		}, {
			name: "enable WithDBStatement by option",
			fields: fields{
				rdsOpt: &redis.Options{},
				opts:   []Option{WithDBStatement(true)},
			},
			args: args{
				hook: func(ctx context.Context, cmd redis.Cmder) error { return nil },
				cmd:  redis.NewCmd(context.Background(), "set", "key", "value"),
			},
			checkFn: func(t *testing.T, span sdktrace.ReadOnlySpan) {
				t.Helper()
				attrs := attrMap(span.Attributes())
				t.Logf("attrs: %v", attrs)

				kv, ok := attrs[semconv.DBQueryTextKey]
				assert.Truef(t, ok, "should have DBQueryText attribute when enabled")
				assert.Equal(t, "set key value", kv.Value.AsString())
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sr := tracetest.NewSpanRecorder()
			tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(sr))
			ch, err := newClientHook(tt.fields.rdsOpt, newConfig(append(tt.fields.opts, WithTracerProvider(tp))...))
			require.NoError(t, err)

			ctx, span := tp.Tracer("redis-test").Start(context.Background(), "redis-test")
			err = ch.ProcessHook(tt.args.hook)(ctx, tt.args.cmd)
			span.End()

			spans := sr.Ended()
			assert.Len(t, spans, 2)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			tt.checkFn(t, spans[0])
		})
	}
}
