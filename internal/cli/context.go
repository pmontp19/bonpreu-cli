package cli

import (
	"context"
	"log"
)

type ctxKey int

const (
	keyFlags ctxKey = iota
	keyLog
	keyRT
)

type Ctx struct {
	Flags  *Flags
	Logger *log.Logger
}

// rtHolder memoizes the per-command runtime so the client (and its cookie jar /
// rotated CSRF token) is built once and can be persisted after the command runs.
type rtHolder struct{ rt *runtime }

func WithFlags(ctx context.Context, f *Flags, logger *log.Logger) context.Context {
	ctx = context.WithValue(ctx, keyFlags, f)
	ctx = context.WithValue(ctx, keyLog, logger)
	return context.WithValue(ctx, keyRT, &rtHolder{})
}

func holderFrom(ctx context.Context) *rtHolder {
	h, _ := ctx.Value(keyRT).(*rtHolder)
	return h
}

func FromContext(ctx context.Context) Ctx {
	var f *Flags
	if v, ok := ctx.Value(keyFlags).(*Flags); ok {
		f = v
	}
	var lg *log.Logger
	if v, ok := ctx.Value(keyLog).(*log.Logger); ok {
		lg = v
	}
	return Ctx{Flags: f, Logger: lg}
}
