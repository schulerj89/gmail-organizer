package config

import "testing"

func TestOAuthRedirectURLDefault(t *testing.T) {
	cfg := Config{Port: 8787}

	got := cfg.OAuthRedirectURL()
	want := "http://127.0.0.1:8787/api/auth/google/callback"
	if got != want {
		t.Fatalf("OAuthRedirectURL() = %q, want %q", got, want)
	}
}

func TestOAuthRedirectURLOverride(t *testing.T) {
	cfg := Config{
		Port:                  8787,
		OAuthRedirectURLValue: "http://localhost:8080/oauth2callback",
	}

	got := cfg.OAuthRedirectURL()
	want := "http://localhost:8080/oauth2callback"
	if got != want {
		t.Fatalf("OAuthRedirectURL() = %q, want %q", got, want)
	}
}
