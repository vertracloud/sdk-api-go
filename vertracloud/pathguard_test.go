package vertracloud

import (
	"context"
	"errors"
	"testing"

	"github.com/vertracloud/sdk-api-go/internal/vertratest"
)

func TestInvalidPathSegmentsNeverReachTheTransport(t *testing.T) {
	ctx := context.Background()
	calls := map[string]func(c *Client, bad string) error{
		"Apps.Get":                 func(c *Client, bad string) error { _, err := c.Apps.Get(ctx, bad); return err },
		"Apps.Delete":              func(c *Client, bad string) error { return c.Apps.Delete(ctx, bad) },
		"Apps.Domains.Remove":      func(c *Client, bad string) error { return c.Apps.Domains().Remove(ctx, "abc", bad) },
		"Apps.Domains.Remove(app)": func(c *Client, bad string) error { return c.Apps.Domains().Remove(ctx, bad, "dom") },
		"Apps.Domains.DNS":         func(c *Client, bad string) error { _, err := c.Apps.Domains().DNS(ctx, "abc", bad); return err },
		"Apps.Domains.Connect":     func(c *Client, bad string) error { _, err := c.Apps.Domains().Connect(ctx, "abc", bad); return err },
		"Apps.Domains.Update": func(c *Client, bad string) error {
			_, err := c.Apps.Domains().Update(ctx, "abc", bad, ApplicationDomainUpdateBody{})
			return err
		},
		"Apps.Envs.Delete":           func(c *Client, bad string) error { return c.Apps.Envs().Delete(ctx, "abc", bad) },
		"Apps.Releases.Log":          func(c *Client, bad string) error { _, err := c.Apps.Releases().Log(ctx, "abc", bad); return err },
		"Databases.Get":              func(c *Client, bad string) error { _, err := c.Databases.Get(ctx, bad); return err },
		"Databases.Delete":           func(c *Client, bad string) error { _, err := c.Databases.Delete(ctx, bad); return err },
		"Workspaces.Delete":          func(c *Client, bad string) error { return c.Workspaces.Delete(ctx, bad) },
		"Workspaces.Members.Remove":  func(c *Client, bad string) error { return c.Workspaces.Members().Remove(ctx, "ws", bad) },
		"Workspaces.Apps.Remove":     func(c *Client, bad string) error { return c.Workspaces.Apps().Remove(ctx, bad, "abc") },
		"Account.Folders.Delete":     func(c *Client, bad string) error { return c.Account.Folders().Delete(ctx, bad) },
		"Billing.Orders.Status":      func(c *Client, bad string) error { _, err := c.Billing.Orders().Status(ctx, bad); return err },
		"Snapshots.Restore":          func(c *Client, bad string) error { _, err := c.Snapshots.Restore(ctx, "abc", bad, ""); return err },
		"Snapshots.Download(stream)": func(c *Client, bad string) error { _, err := c.Snapshots.Download(ctx, "abc", bad, ""); return err },
		"Blob.Domains.Delete":        func(c *Client, bad string) error { return c.Blob.Domains.Delete(ctx, bad) },
	}
	invalid := []string{"", ".", "..", "%2e%2e", "%2E.", ".%2e", "%252e%252e"}

	for name, call := range calls {
		for _, bad := range invalid {
			t.Run(name+"/"+bad, func(t *testing.T) {
				fake := vertratest.NewFakeRestClient(t)
				err := call(New(fake), bad)
				if !errors.Is(err, ErrInvalidPathSegment) {
					t.Fatalf("err = %v, want ErrInvalidPathSegment", err)
				}
				if len(fake.Requests) != 0 {
					t.Fatalf("transport called: %+v", fake.Requests)
				}
			})
		}
	}
}

func TestDomainsRemoveDotDotDoesNotDeleteTheApp(t *testing.T) {
	fake := vertratest.NewFakeRestClient(t)
	err := newAppsService(fake).Domains().Remove(context.Background(), "abc", "..")
	if !errors.Is(err, ErrInvalidPathSegment) {
		t.Fatalf("err = %v, want ErrInvalidPathSegment", err)
	}
	if len(fake.Requests) != 0 {
		t.Fatalf("DELETE reached the transport: %+v", fake.Requests)
	}
}

func TestValidPathSegmentsStillEscaped(t *testing.T) {
	fake := vertratest.NewFakeRestClient(t)
	fake.EnqueueJSON(200, `{}`)
	if _, err := newAppsService(fake).Get(context.Background(), "a b/..c.d"); err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got, want := fake.Requests[0].Path, "/v1/apps/a%20b%2F..c.d"; got != want {
		t.Fatalf("path = %q, want %q", got, want)
	}
}
