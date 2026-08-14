package cli

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"zeroclaw/internal/agent"
)

// RaceResult holds benchmark telemetry for a single driver backend turn.
type RaceResult struct {
	Backend    string
	Duration   time.Duration
	Result     agent.TurnResult
	EventCount int
	Error      error
}

// RunRace executes a prompt concurrently against multiple backends and outputs a comparison benchmark.
func RunRace(w io.Writer, prompt string, backends []string) error {
	if len(backends) == 0 {
		backends = []string{"zero", "zero"}
	}

	fmt.Fprintf(w, "%s %s %s\n\n", badge(" zeroclaw race "), boldInk("Multi-Driver Benchmark"), faint("prompt: "+prompt))

	results := make([]RaceResult, len(backends))
	var wg sync.WaitGroup

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	for i, backend := range backends {
		wg.Add(1)
		go func(idx int, b string) {
			defer wg.Done()
			driver, err := agent.NewDriver(b)
			if err != nil {
				results[idx] = RaceResult{Backend: b, Error: err}
				return
			}

			eventCount := 0
			start := time.Now()
			opts := agent.TurnOptions{
				Prompt:   prompt,
				Autonomy: "high",
				Attended: true,
			}

			res, err := driver.Turn(ctx, opts, func(ev agent.Event) {
				eventCount++
			})

			duration := time.Since(start)
			results[idx] = RaceResult{
				Backend:    b,
				Duration:   duration,
				Result:     res,
				EventCount: eventCount,
				Error:      err,
			}
		}(i, backend)
	}

	wg.Wait()

	// Output Benchmark Telemetry Table
	fmt.Fprintln(w, "--------------------------------------------------------------------------------")
	fmt.Fprintf(w, "%-15s %-10s %-12s %-15s %-12s\n", "BACKEND", "STATUS", "DURATION", "OUTPUT SIZE", "EVENTS")
	fmt.Fprintln(w, "--------------------------------------------------------------------------------")

	for _, res := range results {
		statusStr := green("✓ OK")
		if res.Error != nil || res.Result.Status == "FAIL" {
			statusStr = red("✗ FAIL")
		}

		outputLen := fmt.Sprintf("%d chars (%d words)", len(res.Result.Final), len(strings.Fields(res.Result.Final)))
		if res.Error != nil {
			outputLen = "error"
		}

		fmt.Fprintf(w, "%-15s %-10s %-12s %-15s %-12d\n",
			res.Backend,
			statusStr,
			res.Duration.Round(time.Millisecond).String(),
			outputLen,
			res.EventCount,
		)
	}
	fmt.Fprintln(w, "--------------------------------------------------------------------------------")

	// Output Driver Response Previews
	fmt.Fprintln(w, "\n--- Output Comparison ---")
	for _, res := range results {
		fmt.Fprintf(w, "\n[%s response (%s)]:\n", res.Backend, res.Duration.Round(time.Millisecond))
		if res.Error != nil {
			fmt.Fprintf(w, "%s %v\n", red("Error:"), res.Error)
		} else if res.Result.Final != "" {
			fmt.Fprintln(w, res.Result.Final)
		} else {
			fmt.Fprintln(w, faint("(no text output produced)"))
		}
	}

	// Compute basic diff telemetry if 2 backends were run
	if len(results) == 2 && results[0].Error == nil && results[1].Error == nil {
		diffLen := len(results[0].Result.Final) - len(results[1].Result.Final)
		if diffLen < 0 {
			diffLen = -diffLen
		}
		fmt.Fprintf(w, "\n%s Response length difference: %d characters\n", faint("Telemetry:"), diffLen)
	}

	return nil
}
