package main

import "testing"

func TestLocalWebDoesNotRequireLegacyPassword(t *testing.T) {
	t.Setenv("JANA_WEB_AUTH", "")
	t.Setenv("JANA_WEB_TOKEN", "legacy-installed-token")
	t.Setenv("JANA_WEB_TOKEN_FILE", "/missing/unused-secret")
	token, disabled, err := webAuthentication(false)
	if err != nil || !disabled || token != "" {
		t.Fatalf("local login still enabled: %q %v %v", token, disabled, err)
	}
}

func TestWebAuthenticationRemainsExplicitlyConfigurable(t *testing.T) {
	t.Setenv("JANA_WEB_TOKEN_FILE", "")
	t.Setenv("JANA_WEB_TOKEN", "synthetic-long-test-password")
	t.Setenv("JANA_WEB_AUTH", "password")
	token, disabled, err := webAuthentication(false)
	if err != nil || disabled || token != "synthetic-long-test-password" {
		t.Fatal(token, disabled, err)
	}
	t.Setenv("JANA_WEB_AUTH", "")
	t.Setenv("JANA_WEB_TOKEN", "")
	if _, _, err = webAuthentication(true); err == nil {
		t.Fatal("network listener silently disabled authentication")
	}
	t.Setenv("JANA_WEB_AUTH", "none")
	if _, disabled, err = webAuthentication(true); err != nil || !disabled {
		t.Fatal(disabled, err)
	}
	t.Setenv("JANA_WEB_AUTH", "invalid")
	if _, _, err = webAuthentication(false); err == nil {
		t.Fatal("invalid authentication mode accepted")
	}
}
