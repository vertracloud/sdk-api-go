package vertracloud

import (
	"context"
	"net/http"
	"reflect"
	"testing"

	"github.com/vertracloud/sdk-api-go/internal/vertratest"
)

func TestStatusGet(t *testing.T) {
	if got := reflect.TypeOf(StatusInfo{}.Status).Name(); got != "StatusType" {
		t.Fatalf("status field type = %s, want StatusType", got)
	}
	fake := vertratest.NewFakeRestClient(t)
	fake.EnqueueJSON(200, `{"status":"healthy","message":"ok"}`)
	got, err := New(fake).Status.Get(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "healthy" || got.Message != "ok" {
		t.Fatalf("status: %#v", got)
	}
	if len(fake.Requests) != 1 || fake.Requests[0].Method != http.MethodGet || fake.Requests[0].Path != "/v1/status" {
		t.Fatalf("requests: %#v", fake.Requests)
	}
}
