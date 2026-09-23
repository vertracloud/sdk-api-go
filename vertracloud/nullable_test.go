package vertracloud

import (
	"encoding/json"
	"testing"
)

func TestNullableOmitsOrSendsNullExplicitly(t *testing.T) {
	type body struct {
		Omitted *Nullable[string] `json:"omitted,omitempty"`
		Cleared *Nullable[string] `json:"cleared,omitempty"`
		Set     *Nullable[string] `json:"set,omitempty"`
	}
	value := NullableValue("value")
	encoded, err := json.Marshal(body{Cleared: NullableNull[string](), Set: value})
	if err != nil {
		t.Fatal(err)
	}
	if string(encoded) != `{"cleared":null,"set":"value"}` {
		t.Fatalf("body = %s", encoded)
	}
}
