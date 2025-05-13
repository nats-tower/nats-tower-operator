package natstower

import (
	"context"
	"testing"
)

func TestNATSTowerClient(t *testing.T) {
	nt, err := CreateNATSTowerClient(context.Background(), NATSTowerClientConfig{
		ClusterID:       "test",
		NATSTowerURL:    "http://192.168.0.95:8090",
		NATSTowerAPIKey: "xxx",
	})
	if err != nil {
		t.Fatalf("error creating NATSTowerClient: %v", err)
	}

	creds, err := nt.CreateOrGetUserAuth(context.Background(),
		"test",
		"nats://192.168.0.95:4222",
		"test_acc",
		"test_secret",
		"desc")
	if err != nil {
		t.Fatalf("error creating or getting user auth: %v", err)
	}

	t.Logf("creds: %+v", *creds)

	err = nt.RemoveUserAuth(context.Background(),
		"test",
		"nats://192.168.0.95:4222",
		"test_acc",
		"test_secret")
	if err != nil {
		t.Fatalf("error removing user auth: %v", err)
	}
}
