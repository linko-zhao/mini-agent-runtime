package calculator

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"testing"
)

func TestInputBoundary(t *testing.T) {
	c := &Calculator{}
	for _, input := range []string{`{}`, `{"a":1}`, `null`, `[]`, `{"a":null,"b":1}`, `{"a":1,"b":2,"c":3}`, `{"a":1,"b":2} {}`, `{"a":1.5,"b":2}`, fmt.Sprintf(`{"a":%d,"b":1}`, math.MaxInt), fmt.Sprintf(`{"a":%d,"b":-1}`, math.MinInt)} {
		t.Run(input, func(t *testing.T) {
			if _, err := c.Execute(context.Background(), json.RawMessage(input)); err == nil {
				t.Fatal("accepted invalid input")
			}
		})
	}
	out, err := c.Execute(context.Background(), json.RawMessage(`{"a":0,"b":2}`))
	if err != nil || out != "2" {
		t.Fatalf("%q %v", out, err)
	}
}
