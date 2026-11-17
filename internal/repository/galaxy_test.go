package repository

import (
	"context"
	"errors"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/data/azcosmos"
	"net/http"
	"testing"
)

type noNetwork struct{ calls int }

func (n *noNetwork) Do(*http.Request) (*http.Response, error) {
	n.calls++
	return nil, errors.New("unexpected network request")
}
func TestNewDoesNotPerformIO(t *testing.T) {
	transport := &noNetwork{}
	key, err := azcosmos.NewKeyCredential("a2V5")
	if err != nil {
		t.Fatal(err)
	}
	options := &azcosmos.ClientOptions{}
	options.ClientOptions.Transport = transport
	options.ClientOptions.Retry = policy.RetryOptions{MaxRetries: -1}
	client, err := azcosmos.NewClientWithKey("https://example.documents.azure.com", key, options)
	if err != nil {
		t.Fatal(err)
	}
	g, err := New[testEntity](client, "db", "items", WithAutoProvision(true), WithConsistency(azcosmos.ConsistencyLevelStrong))
	if err != nil {
		t.Fatal(err)
	}
	if transport.calls != 0 {
		t.Fatal("constructor performed I/O")
	}
	g.options.autoProvision = false
	if err = g.EnsureProvisioned(context.Background()); err != nil || transport.calls != 0 {
		t.Fatal("disabled provisioning performed I/O")
	}
	if _, err = New[testEntity](nil, "db", "items"); !errors.Is(err, ErrValidation) {
		t.Fatal(err)
	}
	if _, err = New[testEntity](client, "", "items"); !errors.Is(err, ErrValidation) {
		t.Fatal(err)
	}
	if _, err = New[testEntity](client, "db", "items", WithConsistency(azcosmos.ConsistencyLevel("bad"))); !errors.Is(err, ErrValidation) {
		t.Fatal(err)
	}
	if _, err = New[overrideEntity](client, "db", "items", WithPartitionKeyCount(1)); err != nil {
		t.Fatal(err)
	}
	if _, err = New[overrideEntity](client, "db", "items", WithPartitionKeyCount(0)); !errors.Is(err, ErrValidation) {
		t.Fatal(err)
	}
}
