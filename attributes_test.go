package redisotel

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/otel/attribute"
	semconv "go.opentelemetry.io/otel/semconv/v1.30.0"

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
