package main

import "testing"

func TestKnowledgeAuthorizationCompletesOfflineAnswer(t *testing.T) {
	if err := run(); err != nil {
		t.Fatal(err)
	}
}
