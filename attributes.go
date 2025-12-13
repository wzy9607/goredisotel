package redisotel

import (
	"context"
	"errors"
	"net"
	"runtime"
	"slices"
	"strconv"
	"strings"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	semconv "go.opentelemetry.io/otel/semconv/v1.38.0"

	"github.com/redis/go-redis/v9"
)

func commonOperationAttrs(conf *config, opt *redis.Options) attribute.Set {
	attrs := append(conf.Attributes(), semconv.DBSystemNameRedis)
	if opt != nil {
		db := opt.DB
		if conf.db != nil {
			db = *conf.db
		}
		attrs = append(attrs, semconv.DBNamespace(strconv.Itoa(db)))
		attrs = append(attrs, serverAttributes(opt.Addr)...)
	}
	return attribute.NewSet(attrs...)
}

func commonPoolAttrs(conf *config, opt *redis.Options) attribute.Set {
	attrs := append(conf.Attributes(), semconv.DBSystemNameRedis)
	if opt != nil {
		// https://opentelemetry.io/docs/specs/semconv/attributes-registry/db/#general-database-attributes
		poolName := conf.poolName
		if poolName == "" {
			db := opt.DB
			if conf.db != nil {
				db = *conf.db
			}
			poolName = opt.Addr + "/" + strconv.Itoa(db)
		}
		attrs = append(attrs, semconv.DBClientConnectionPoolName(poolName))
	}
	return attribute.NewSet(attrs...)
}

func maybeStoredProcedureAttr(cmd redis.Cmder) (kv attribute.KeyValue, ok bool) {
	if !slices.Contains([]string{"evalsha", "evalsha_rd", "fcall", "fcall_rd"}, cmd.Name()) {
		return kv, false
	}
	args := cmd.Args()
	if len(args) < 2 {
		return kv, false
	}
	val, ok := args[1].(string)
	if !ok {
		return kv, false
	}
	kv = semconv.DBStoredProcedureName(val)
	return kv, true
}

// Database span attributes semantic conventions recommended server address and port
// https://opentelemetry.io/docs/specs/semconv/database/database-spans/#common-attributes,
// https://opentelemetry.io/docs/specs/semconv/database/database-metrics/#metric-dbclientoperationduration.
func serverAttributes(addr string) []attribute.KeyValue {
	host, portString, err := net.SplitHostPort(addr)
	if err != nil {
		otel.Handle(err)
		return []attribute.KeyValue{semconv.ServerAddress(addr)}
	}

	// Parse the port string to an integer
	port, err := strconv.Atoi(portString)
	if err != nil {
		otel.Handle(err)
		return []attribute.KeyValue{semconv.ServerAddress(host)}
	}

	return []attribute.KeyValue{semconv.ServerAddress(host), semconv.ServerPort(port)}
}

func errorKindAttr(err error) []attribute.KeyValue {
	var kind string
	switch {
	case errors.Is(err, redis.Nil):
		kind = "redis.Nil"
	case errors.Is(err, redis.TxFailedErr):
		kind = "redis.TxFailedErr"
	case errors.Is(err, context.Canceled):
		kind = "context.Canceled"
	case errors.Is(err, context.DeadlineExceeded):
		kind = "context.DeadlineExceeded"
	default:
		var redisErr redis.Error
		if errors.As(err, &redisErr) {
			first, _, _ := strings.Cut(redisErr.Error(), " ")
			kind = "redis." + first
			return []attribute.KeyValue{semconv.ErrorTypeKey.String(kind), semconv.DBResponseStatusCode(first)}
		}
		return []attribute.KeyValue{semconv.ErrorTypeOther}
	}
	return []attribute.KeyValue{semconv.ErrorTypeKey.String(kind)}
}

func statusAttr(err error) attribute.KeyValue {
	if err != nil {
		return attribute.String("status", "error")
	}
	return attribute.String("status", "ok")
}

func funcFileLine(pkg string) []attribute.KeyValue {
	const depth = 16
	var pcs [depth]uintptr
	n := runtime.Callers(3, pcs[:]) //nolint:mnd // ignore
	ff := runtime.CallersFrames(pcs[:n])

	var fnName, file string
	var line int
	for {
		f, ok := ff.Next()
		if !ok {
			break
		}
		fnName, file, line = f.Function, f.File, f.Line
		if !strings.Contains(fnName, pkg) {
			break
		}
	}

	if ind := strings.LastIndexByte(fnName, '/'); ind != -1 {
		fnName = fnName[ind+1:]
	}

	return []attribute.KeyValue{
		semconv.CodeFunctionName(fnName),
		semconv.CodeFilePath(file),
		semconv.CodeLineNumber(line),
	}
}
