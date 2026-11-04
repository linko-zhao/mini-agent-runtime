package main

import "testing"

func TestHiddenWriteRequestIsDeniedWithoutExecution(t *testing.T) {
	if err := run(); err != nil {
		t.Fatal(err)
	}
}
