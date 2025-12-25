package controller

import (
	"strings"
	"testing"

	secretspizecomv1alpha1 "github.com/zeraholladay/gsm-operator/api/v1alpha1"
)

func TestMapKeysToSecretKeyMappings_LiteralKeyAndPointerValue(t *testing.T) {
	payload := []byte(`{"k":"ENV_KEY","v":"val"}`)
	mappings := []secretspizecomv1alpha1.SecretKeyMapping{
		{Key: "ENV_KEY", Value: "/v"},
	}

	res, err := mapKeysToSecretKeyMappings(payload, mappings)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(res) != 1 {
		t.Fatalf("expected 1 result, got %d", len(res))
	}
	if res[0].Key != "ENV_KEY" {
		t.Errorf("expected key ENV_KEY, got %q", res[0].Key)
	}
	if string(res[0].Value) != `"val"` {
		t.Errorf(`expected value "val", got %q`, string(res[0].Value))
	}
}

func TestMapKeysToSecretKeyMappings_PointerKeyAndValue(t *testing.T) {
	payload := []byte(`{"k":"ENV_KEY","v":"val"}`)
	mappings := []secretspizecomv1alpha1.SecretKeyMapping{
		{Key: "/k", Value: "/v"},
	}

	res, err := mapKeysToSecretKeyMappings(payload, mappings)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(res) != 1 {
		t.Fatalf("expected 1 result, got %d", len(res))
	}
	if res[0].Key != "ENV_KEY" {
		t.Errorf("expected key ENV_KEY, got %q", res[0].Key)
	}
	if string(res[0].Value) != `"val"` {
		t.Errorf(`expected value "val", got %q`, string(res[0].Value))
	}
}

func TestMapKeysToSecretKeyMappings_PointerKeyNotString(t *testing.T) {
	payload := []byte(`{"k":123,"v":"val"}`)
	mappings := []secretspizecomv1alpha1.SecretKeyMapping{
		{Key: "/k", Value: "/v"},
	}

	_, err := mapKeysToSecretKeyMappings(payload, mappings)
	if err == nil {
		t.Fatal("expected error for non-string pointer key, got nil")
	}
	if !strings.Contains(err.Error(), "not a string") {
		t.Fatalf("expected error mentioning not a string, got %v", err)
	}
}

func TestMapKeysToSecretKeyMappings_PointerKeyFailsRegex(t *testing.T) {
	payload := []byte(`{"k":"bad key with space","v":"val"}`)
	mappings := []secretspizecomv1alpha1.SecretKeyMapping{
		{Key: "/k", Value: "/v"},
	}

	_, err := mapKeysToSecretKeyMappings(payload, mappings)
	if err == nil {
		t.Fatal("expected error for key failing regex, got nil")
	}
	if !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("expected regex failure error, got %v", err)
	}
}
