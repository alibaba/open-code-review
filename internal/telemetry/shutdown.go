package telemetry

import (
	"context"
	"fmt"
	"os"
	"time"
)

// Shutdown flushes and shuts down all initialized providers.
// It should be called before process exit to ensure buffered data is exported.
func Shutdown(ctx context.Context) error {
	if len(shutdownFuncs) == 0 {
		return nil
	}

	var errs []error
	for _, fn := range shutdownFuncs {
		if err := fn(ctx); err != nil {
			errs = append(errs, err)
		}
	}

	shutdownFuncs = nil

	if len(errs) > 0 {
		return fmt.Errorf("telemetry shutdown: %v", errs)
	}
	return nil
}

// ShutdownWithTimeout creates a timeout context and calls Shutdown.
func ShutdownWithTimeout(ctx context.Context, timeout time.Duration) {
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	if err := Shutdown(cctx); err != nil {
		fmt.Fprintf(os.Stderr, "[ocr] WARNING: telemetry shutdown error: %v\n", err)
	}
}
