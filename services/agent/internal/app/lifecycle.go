package app

import (
	"context"
	"os"
	"os/signal"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/rs/zerolog"
)

type PipelineTimeoutController struct {
	ctx       context.Context
	cancel    context.CancelFunc
	triggered atomic.Bool
}

func StartPipelineTimeout(raw string, logger *zerolog.Logger, onTimeout func(reason string)) *PipelineTimeoutController {
	controller := &PipelineTimeoutController{}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return controller
	}
	timeout, err := time.ParseDuration(raw)
	if err != nil {
		if logger != nil {
			logger.Error().Err(err).Msg("Invalid pipeline timeout duration")
		}
		return controller
	}
	if timeout <= 0 {
		return controller
	}

	controller.ctx, controller.cancel = context.WithTimeout(context.Background(), timeout)
	go func() {
		<-controller.ctx.Done()
		if controller.ctx.Err() == context.DeadlineExceeded {
			if controller.triggered.CompareAndSwap(false, true) {
				if logger != nil {
					logger.Error().Msg("Pipeline execution timed out. Cleaning up and exiting")
				}
				if onTimeout != nil {
					onTimeout("timeout")
				}
			}
		}
	}()
	return controller
}

func (c *PipelineTimeoutController) Context() context.Context {
	if c == nil || c.ctx == nil {
		return nil
	}
	return c.ctx
}

func (c *PipelineTimeoutController) ContextOrDefault(fallback context.Context) context.Context {
	if ctx := c.Context(); ctx != nil {
		return ctx
	}
	if fallback != nil {
		return fallback
	}
	return context.Background()
}

func (c *PipelineTimeoutController) Triggered() bool {
	return c != nil && c.triggered.Load()
}

func (c *PipelineTimeoutController) Stopping() bool {
	return c != nil && (c.Triggered() || (c.ctx != nil && c.ctx.Err() != nil))
}

func (c *PipelineTimeoutController) Stop() {
	if c != nil && c.cancel != nil {
		c.cancel()
	}
}

func StartTerminationSignalHandler(logger *zerolog.Logger, onSignal func(reason string), exit func(code int)) func() {
	if exit == nil {
		exit = os.Exit
	}
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGTERM, syscall.SIGINT)

	go func() {
		sig := <-signalChan
		if logger != nil {
			logger.Warn().Str("signal", sig.String()).Msg("Received termination signal")
		}
		if onSignal != nil {
			onSignal("signal")
		}
		exit(0)
	}()

	return func() {
		signal.Stop(signalChan)
	}
}
