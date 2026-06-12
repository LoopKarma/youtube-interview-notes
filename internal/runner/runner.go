// Package runner abstracts external process execution so that callers
// depending on tools like yt-dlp, ffmpeg and ffprobe can be unit-tested
// with a fake implementation instead of shelling out for real.
package runner

import (
	"context"
	"fmt"
	"os/exec"
)

// Runner executes an external command and returns its combined stdout.
// It mirrors the small slice of os/exec the application actually needs,
// which keeps fakes trivial to write in tests.
type Runner interface {
	Run(ctx context.Context, name string, args ...string) ([]byte, error)
}

// Func adapts an ordinary function to the Runner interface.
type Func func(ctx context.Context, name string, args ...string) ([]byte, error)

// Run calls the underlying function.
func (f Func) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	return f(ctx, name, args...)
}

// Exec is the production Runner. It runs the command and returns stdout,
// wrapping any failure with the command name and captured stderr.
var Exec Runner = Func(func(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	out, err := cmd.Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return out, fmt.Errorf("%s failed: %w: %s", name, err, ee.Stderr)
		}
		return out, fmt.Errorf("%s failed: %w", name, err)
	}
	return out, nil
})
