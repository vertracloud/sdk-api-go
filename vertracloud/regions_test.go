package vertracloud

import (
	"context"
	"testing"

	"github.com/vertracloud/sdk-api-go/internal/vertratest"
)

func TestApps_Get_DecodesRegionAndNode(t *testing.T) {
	fake := vertratest.NewFakeRestClient(t)
	fake.EnqueueJSON(200, `{"id":"1","name":"a","region":"sp","node":"amethyst"}`)
	app, err := newAppsService(fake).Get(context.Background(), "1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if app.Region != RegionSaoPaulo || app.Node != "amethyst" {
		t.Errorf("region/node = %q/%q", app.Region, app.Node)
	}
}

func TestDatabases_Get_DecodesRegionAndNode(t *testing.T) {
	fake := vertratest.NewFakeRestClient(t)
	fake.EnqueueJSON(200, `{"id":"1","name":"a","region":"ny","node":"amethyst"}`)
	db, err := newDatabasesService(fake).Get(context.Background(), "1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if db.Region != RegionNewYork || db.Node != "amethyst" {
		t.Errorf("region/node = %q/%q", db.Region, db.Node)
	}
}

func TestApps_Create_SendsRegionOnlyWhenSet(t *testing.T) {
	fake := vertratest.NewFakeRestClient(t)
	fake.EnqueueJSON(200, `{"id":"1","name":"a"}`)
	fake.EnqueueJSON(200, `{"id":"2","name":"b"}`)
	svc := newAppsService(fake)
	if _, err := svc.Create(context.Background(), ApplicationCreateParams{SnapshotID: "s", Name: "a", Region: RegionSaoPaulo}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := svc.Create(context.Background(), ApplicationCreateParams{SnapshotID: "s", Name: "b"}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	with, _, _ := parseMultipartRequest(t, fake.Requests[0])
	without, _, _ := parseMultipartRequest(t, fake.Requests[1])
	if with["region"] != "sp" {
		t.Errorf("fields = %v", with)
	}
	if _, ok := with["node"]; ok {
		t.Errorf("node must not be sent: %v", with)
	}
	if _, ok := without["region"]; ok {
		t.Errorf("region must be omitted: %v", without)
	}
}

func TestApps_CreateFromImage_SendsRegionOnlyWhenSet(t *testing.T) {
	fake := vertratest.NewFakeRestClient(t)
	fake.EnqueueJSON(202, `{"id":"1","name":"a","status":"building","custom_image":null}`)
	fake.EnqueueJSON(202, `{"id":"2","name":"b","status":"building","custom_image":null}`)
	svc := newAppsService(fake)
	if _, err := svc.CreateFromImage(context.Background(), ApplicationCreateFromImageParams{ImageID: testImageID, Name: "a", Memory: 256, Region: RegionNewYork}); err != nil {
		t.Fatalf("CreateFromImage: %v", err)
	}
	if _, err := svc.CreateFromImage(context.Background(), ApplicationCreateFromImageParams{ImageID: testImageID, Name: "b", Memory: 256}); err != nil {
		t.Fatalf("CreateFromImage: %v", err)
	}
	with := decodeJSONBody(t, fake.Requests[0])
	without := decodeJSONBody(t, fake.Requests[1])
	if with["region"] != "ny" {
		t.Errorf("body = %v", with)
	}
	if _, ok := with["node"]; ok {
		t.Errorf("node must not be sent: %v", with)
	}
	if _, ok := without["region"]; ok {
		t.Errorf("region must be omitted: %v", without)
	}
}

func TestDatabases_Create_SendsRegionOnlyWhenSet(t *testing.T) {
	fake := vertratest.NewFakeRestClient(t)
	fake.EnqueueJSON(200, `{"id":"1","name":"a","region":"sp","node":"amethyst"}`)
	fake.EnqueueJSON(200, `{"id":"2","name":"b"}`)
	svc := newDatabasesService(fake)
	dbType := DatabaseTypePostgreSQL
	db, err := svc.Create(context.Background(), DatabaseCreateBody{Name: "a", Type: &dbType, RAM: 512, Region: RegionSaoPaulo})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if db.Region != RegionSaoPaulo || db.Node != "amethyst" {
		t.Errorf("decoded region/node = %q/%q", db.Region, db.Node)
	}
	if _, err := svc.Create(context.Background(), DatabaseCreateBody{Name: "b", Type: &dbType, RAM: 512}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	with := decodeJSONBody(t, fake.Requests[0])
	without := decodeJSONBody(t, fake.Requests[1])
	if with["region"] != "sp" {
		t.Errorf("body = %v", with)
	}
	if _, ok := with["node"]; ok {
		t.Errorf("node must not be sent: %v", with)
	}
	if _, ok := without["region"]; ok {
		t.Errorf("region must be omitted: %v", without)
	}
}

func TestRegionErrorCodes(t *testing.T) {
	if ErrCodeRegionNotAvailable != "REGION_NOT_AVAILABLE" {
		t.Error("region error codes changed")
	}
}
