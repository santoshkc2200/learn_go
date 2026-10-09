package main

import (
	"context"
	"errors"
	"testing"
)

func TestDemoTransientError(t *testing.T) {
	job := &demoJob{failFirst: 1}
	err := job.Run(context.Background())
	var temporary *TemporaryError
	if !errors.As(err, &temporary) || temporary.Attempt != 1 {
		t.Fatalf("custom error lost: %v", err)
	}
	if err := job.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
}
