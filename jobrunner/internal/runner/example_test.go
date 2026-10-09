package runner_test

import (
	"context"
	"errors"
	"example.com/jobrunner/internal/runner"
	"fmt"
)

func ExampleRun() {
	jobs := []runner.Job{
		runner.JobFunc(func(context.Context) error { return nil }),
		runner.JobFunc(func(context.Context) error { return errors.New("try later") }),
	}
	results, err := runner.Run(context.Background(), 2, jobs)
	fmt.Println("batch error:", err)
	// Ordered results do not imply ordered execution.
	for _, result := range results {
		fmt.Printf("job %d: started=%t error=%v\n", result.Index, result.Started, result.Err)
	}
	// Output:
	// batch error: <nil>
	// job 0: started=true error=<nil>
	// job 1: started=true error=try later
}
