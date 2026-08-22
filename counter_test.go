package htxt

import (
	"encoding/json"
	"testing"
)

func TestCounterMarshalJSON(t *testing.T) {
	counter := Counter{}
	counter.Increment(2, "Pages")
	encoded, err := json.Marshal(&counter)
	if err != nil {
		t.Fatal(err)
	}
	if actual, expected := string(encoded), `{"pages":2}`; actual != expected {
		t.Fatalf("JSON = %s, want %s", actual, expected)
	}
}

func TestBuildStatusMarshalJSON(t *testing.T) {
	status := BuildStatus{}
	status.Assets.Increment(2, "Images")
	encoded, err := json.Marshal(&status)
	if err != nil {
		t.Fatal(err)
	}
	if actual, expected := string(encoded), `{"assets":{"images":2},"tags":null,"bytes":null}`; actual != expected {
		t.Fatalf("JSON = %s, want %s", actual, expected)
	}
}
