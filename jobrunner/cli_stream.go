package main

import (
	"context"
	"errors"
	"fmt"

	"example.com/jobrunner/internal/runner"
)

// This scope owns the producer and runner. A failed output cancels both and
// joins them before returning. Nothing allocates a slice proportional to count.
func streamCLI(parent context.Context, workers, count int, o runner.Options, newJob func(int) runner.Job, emit func(runner.Result) error) error {
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	input := make(chan runner.Job)
	output := make(chan runner.Result)
	producerDone := make(chan struct{})
	go func() {
		defer close(producerDone)
		defer close(input)
		for i := range count {
			select {
			case input <- newJob(i):
			case <-ctx.Done():
				return
			}
		}
	}()
	runnerDone := make(chan error, 1)
	go func() { err := runner.Stream(ctx, workers, input, output, o); close(output); runnerDone <- err }()
	var writeErr error
	for result := range output {
		if err := emit(result); err != nil {
			writeErr = fmt.Errorf("write summary: %w", err)
			cancel()
			break
		}
	}
	err := <-runnerDone
	<-producerDone
	return errors.Join(writeErr, err)
}
