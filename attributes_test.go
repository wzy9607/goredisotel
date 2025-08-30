package redisotel

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/otel/attribute"
	semconv "go.opentelemetry.io/otel/semconv/v1.37.0"

	"github.com/redis/go-redis/v9"
)

func Test_commonOperationAttrs(t *testing.T) {
	t.Parallel()
	type args struct {
		conf *config
		opt  *redis.Options
	}
	tests := []struct {
		name string
		args args
		want attribute.Set
	}{
		{
			name: "addr parsed",
			args: args{
				conf: &config{
					attrs: []attribute.KeyValue{attribute.Key("foo").String("bar")},
				},
				opt: &redis.Options{
					Addr: "10.1.1.1:6379",
					DB:   2,
				},
			},
			want: attribute.NewSet(
				attribute.Key("foo").String("bar"),
				semconv.DBSystemNameRedis,
				semconv.DBNamespace("2"),
				semconv.ServerAddress("10.1.1.1"),
				semconv.ServerPort(6379),
			),
		}, {
			name: "cannot parse addr",
			args: args{
				conf: &config{
					attrs: []attribute.KeyValue{attribute.Key("foo").String("bar")},
				},
				opt: &redis.Options{
					Addr: "FailoverClient",
					DB:   2,
				},
			},
			want: attribute.NewSet(
				attribute.Key("foo").String("bar"),
				semconv.DBSystemNameRedis,
				semconv.DBNamespace("2"),
				semconv.ServerAddress("FailoverClient"),
			),
		}, {
			name: "cannot parse port",
			args: args{
				conf: &config{
					attrs: []attribute.KeyValue{attribute.Key("foo").String("bar")},
				},
				opt: &redis.Options{
					Addr: "10.1.1.1:a",
					DB:   2,
				},
			},
			want: attribute.NewSet(
				attribute.Key("foo").String("bar"),
				semconv.DBSystemNameRedis,
				semconv.DBNamespace("2"),
				semconv.ServerAddress("10.1.1.1"),
			),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equalf(t, tt.want, commonOperationAttrs(tt.args.conf, tt.args.opt), "commonOperationAttrs(%v, %v)",
				tt.args.conf, tt.args.opt)
		})
	}
}

func Test_maybeStoredProcedureAttr(t *testing.T) {
	t.Parallel()
	type args struct {
		cmd redis.Cmder
	}
	tests := []struct {
		name   string
		args   args
		wantKv attribute.KeyValue
		wantOk bool
	}{
		{
			name: "normal evalsha",
			args: args{
				cmd: redis.NewCmd(context.Background(), "evalsha", "sha1", 1, "key1", "key2"),
			},
			wantKv: semconv.DBStoredProcedureName("sha1"),
			wantOk: true,
		}, {
			name: "normal evalsha_rd",
			args: args{
				cmd: redis.NewCmd(context.Background(), "evalsha_rd", "sha1", 1, "key1", "key2"),
			},
			wantKv: semconv.DBStoredProcedureName("sha1"),
			wantOk: true,
		}, {
			name: "normal fcall",
			args: args{
				cmd: redis.NewCmd(context.Background(), "fcall", "funcname", 1, "key1", "key2"),
			},
			wantKv: semconv.DBStoredProcedureName("funcname"),
			wantOk: true,
		}, {
			name: "normal fcall_rd",
			args: args{
				cmd: redis.NewCmd(context.Background(), "fcall_rd", "funcname", 1, "key1", "key2"),
			},
			wantKv: semconv.DBStoredProcedureName("funcname"),
			wantOk: true,
		}, {
			name: "other cmd, not stored procedure",
			args: args{
				cmd: redis.NewCmd(context.Background(), "set", "key", "value"),
			},
			wantKv: attribute.KeyValue{},
			wantOk: false,
		}, {
			name: "evalsha, but no sha1",
			args: args{
				cmd: redis.NewCmd(context.Background(), "evalsha"),
			},
			wantKv: attribute.KeyValue{},
			wantOk: false,
		}, {
			name: "evalsha, but broken sha1",
			args: args{
				cmd: redis.NewCmd(context.Background(), "evalsha", 123),
			},
			wantKv: attribute.KeyValue{},
			wantOk: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			gotKv, gotOk := maybeStoredProcedureAttr(tt.args.cmd)
			assert.Equalf(t, tt.wantKv, gotKv, "maybeStoredProcedureAttr(%v)", tt.args.cmd)
			assert.Equalf(t, tt.wantOk, gotOk, "maybeStoredProcedureAttr(%v)", tt.args.cmd)
		})
	}
}
