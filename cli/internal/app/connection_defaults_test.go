package app

import (
	"encoding/json"
	"example.com/jana-locate-cli/internal/api"
	"strings"
	"testing"
)

func TestSavedConnectionDefaultsAndSecretReuse(t *testing.T) {
	dsn := "postgresql://demo:never-return-this-secret@localhost:15432/orders?sslmode=disable&application_name=jana-defaults"
	saved, err := DescribeConnection(dsn, &api.Source{Schema: "public", Table: "orders", Column: "delivery_address"}, "2026-10-17T10:00:00Z")
	if err != nil || !saved.Configured || !saved.HasPassword || saved.Port != 15432 || saved.Sslmode != "disable" {
		t.Fatalf("defaults: %+v %v", saved, err)
	}
	b, _ := json.Marshal(saved)
	if strings.Contains(string(b), "never-return") || strings.Contains(string(b), "postgresql://") {
		t.Fatal("profile exposed credentials")
	}
	server := Server{Connections: &Connections{Load: func() (string, api.ConnectionProfile, error) { return dsn, saved, nil }}}
	yes := true
	empty := ""
	req := api.ConnectionRequest{Host: "localhost", Port: 15432, Database: "orders", User: "demo", Sslmode: "disable", Password: &empty, UseSavedPassword: &yes}
	actual, err := server.requestConnectionDSN(req)
	if err != nil || actual != dsn {
		t.Fatal("saved DSN/password/custom options not reused", err)
	}
	req.Host = "another-host"
	if _, err = server.requestConnectionDSN(req); err == nil {
		t.Fatal("sent saved password to a different host")
	}
	req.Host = "localhost"
	req.User = "other-user"
	if _, err = server.requestConnectionDSN(req); err == nil {
		t.Fatal("reused secret for another user")
	}
	replacement := "new-password"
	req.Password = &replacement
	actual, err = server.requestConnectionDSN(req)
	if err != nil || !strings.Contains(actual, "new-password") {
		t.Fatal("explicit replacement ignored", err)
	}
	req.Password = &empty
	no := false
	req.UseSavedPassword = &no
	actual, err = server.requestConnectionDSN(req)
	if err != nil || strings.Contains(actual, "never-return") {
		t.Fatal("could not explicitly clear password", err)
	}
}
