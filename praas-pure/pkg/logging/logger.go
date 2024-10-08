package logging

import (
	"context"
	"go.uber.org/zap"
)

type logKey struct{}

func WithLogger(ctx context.Context) context.Context {
	logger, err := newLogger()
	if err != nil {
		panic("Could not create logger: " + err.Error())
	}
	return context.WithValue(ctx, logKey{}, logger)
}

func FromContext(ctx context.Context) *zap.SugaredLogger {
	untyped := ctx.Value(logKey{})
	if untyped == nil {
		panic("Could not get logger from context")
	}
	return untyped.(*zap.SugaredLogger)
}

func newLogger() (*zap.SugaredLogger, error) {
	logger, err := zap.NewProduction()
	if err != nil {
		return nil, err
	}
	logger = logger.Named("praas-control-plane")
	// TODO(gr): commit ID
	return logger.Sugar(), nil
}
