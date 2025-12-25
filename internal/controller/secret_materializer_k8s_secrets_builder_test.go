/*
Copyright 2025 Zera Holladay.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	secretspizecomv1alpha1 "github.com/zeraholladay/gsm-operator/api/v1alpha1"
)

func newTestPayload(t *testing.T, key string, value []byte) keyedSecretPayload {
	t.Helper()
	p, err := newKeyedSecretPayload(key, value)
	if err != nil {
		t.Fatalf("newKeyedSecretPayload(%q) failed: %v", key, err)
	}
	return p
}

func TestBuildOpaqueSecret_Success(t *testing.T) {
	m := &secretMaterializer{
		gsmSecret: &secretspizecomv1alpha1.GSMSecret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-gsmsecret",
				Namespace: "test-namespace",
			},
			Spec: secretspizecomv1alpha1.GSMSecretSpec{
				TargetSecret: secretspizecomv1alpha1.GSMSecretTargetSecret{
					Name: "my-target-secret",
				},
			},
		},
		payloads: []keyedSecretPayload{
			newTestPayload(t, "DB_PASSWORD", []byte("super-secret")),
			newTestPayload(t, "API_KEY", []byte("api-key-value")),
		},
	}

	secret, err := m.buildOpaqueSecret(context.Background())
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if secret.Name != "my-target-secret" {
		t.Errorf("expected secret name 'my-target-secret', got %q", secret.Name)
	}
	if secret.Namespace != "test-namespace" {
		t.Errorf("expected namespace 'test-namespace', got %q", secret.Namespace)
	}
	if secret.Type != corev1.SecretTypeOpaque {
		t.Errorf("expected secret type Opaque, got %q", secret.Type)
	}
	if len(secret.Data) != 2 {
		t.Errorf("expected 2 data entries, got %d", len(secret.Data))
	}
	if string(secret.Data["DB_PASSWORD"]) != "super-secret" {
		t.Errorf("expected DB_PASSWORD='super-secret', got %q", string(secret.Data["DB_PASSWORD"]))
	}
	if string(secret.Data["API_KEY"]) != "api-key-value" {
		t.Errorf("expected API_KEY='api-key-value', got %q", string(secret.Data["API_KEY"]))
	}
}

func TestBuildOpaqueSecret_EmptyPayloads(t *testing.T) {
	m := &secretMaterializer{
		gsmSecret: &secretspizecomv1alpha1.GSMSecret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-gsmsecret",
				Namespace: "test-namespace",
			},
			Spec: secretspizecomv1alpha1.GSMSecretSpec{
				TargetSecret: secretspizecomv1alpha1.GSMSecretTargetSecret{
					Name: "my-target-secret",
				},
			},
		},
		payloads: []keyedSecretPayload{},
	}

	secret, err := m.buildOpaqueSecret(context.Background())
	if err != nil {
		t.Fatalf("expected no error for empty payloads, got %v", err)
	}

	if len(secret.Data) != 0 {
		t.Errorf("expected empty data map, got %d entries", len(secret.Data))
	}
}

func TestBuildOpaqueSecret_DuplicateKey_LastWins(t *testing.T) {
	m := &secretMaterializer{
		gsmSecret: &secretspizecomv1alpha1.GSMSecret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-gsmsecret",
				Namespace: "test-namespace",
			},
			Spec: secretspizecomv1alpha1.GSMSecretSpec{
				TargetSecret: secretspizecomv1alpha1.GSMSecretTargetSecret{
					Name: "my-target-secret",
				},
			},
		},
		payloads: []keyedSecretPayload{
			newTestPayload(t, "DUPLICATE_KEY", []byte("value1")),
			newTestPayload(t, "OTHER_KEY", []byte("other-value")),
			newTestPayload(t, "DUPLICATE_KEY", []byte("value2")), // duplicate - should win
		},
	}

	secret, err := m.buildOpaqueSecret(context.Background())
	if err != nil {
		t.Fatalf("expected no error for duplicate key (last wins), got %v", err)
	}

	// Last value should win
	if string(secret.Data["DUPLICATE_KEY"]) != "value2" {
		t.Errorf("expected DUPLICATE_KEY='value2' (last wins), got %q", string(secret.Data["DUPLICATE_KEY"]))
	}
	if string(secret.Data["OTHER_KEY"]) != "other-value" {
		t.Errorf("expected OTHER_KEY='other-value', got %q", string(secret.Data["OTHER_KEY"]))
	}
	if len(secret.Data) != 2 {
		t.Errorf("expected 2 data entries, got %d", len(secret.Data))
	}
}

func TestBuildOpaqueSecret_EmptyKey(t *testing.T) {
	m := &secretMaterializer{
		gsmSecret: &secretspizecomv1alpha1.GSMSecret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-gsmsecret",
				Namespace: "test-namespace",
			},
			Spec: secretspizecomv1alpha1.GSMSecretSpec{
				TargetSecret: secretspizecomv1alpha1.GSMSecretTargetSecret{
					Name: "my-target-secret",
				},
			},
		},
		payloads: []keyedSecretPayload{
			newTestPayload(t, "VALID_KEY", []byte("valid-value")),
			// Use constructor directly to inject invalid key for this negative test.
			{Key: "", Value: []byte("empty-key-value")}, // empty key
		},
	}

	_, err := m.buildOpaqueSecret(context.Background())
	if err == nil {
		t.Fatal("expected error for empty key, got nil")
	}

	expectedMsg := "payload has empty key"
	if err.Error() != expectedMsg {
		t.Errorf("expected error message %q, got %q", expectedMsg, err.Error())
	}
}

func TestBuildOpaqueSecret_NilMaterializer(t *testing.T) {
	var m *secretMaterializer = nil

	_, err := m.buildOpaqueSecret(context.Background())
	if err == nil {
		t.Fatal("expected error for nil materializer, got nil")
	}
}

func TestBuildOpaqueSecret_NilGSMSecret(t *testing.T) {
	m := &secretMaterializer{
		gsmSecret: nil,
		payloads:  []keyedSecretPayload{},
	}

	_, err := m.buildOpaqueSecret(context.Background())
	if err == nil {
		t.Fatal("expected error for nil gsmSecret, got nil")
	}
}

func TestBuildOpaqueSecret_EmptyValue(t *testing.T) {
	m := &secretMaterializer{
		gsmSecret: &secretspizecomv1alpha1.GSMSecret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-gsmsecret",
				Namespace: "test-namespace",
			},
			Spec: secretspizecomv1alpha1.GSMSecretSpec{
				TargetSecret: secretspizecomv1alpha1.GSMSecretTargetSecret{
					Name: "my-target-secret",
				},
			},
		},
		payloads: []keyedSecretPayload{
			newTestPayload(t, "EMPTY_VALUE_KEY", []byte{}),
			newTestPayload(t, "NIL_VALUE_KEY", nil),
		},
	}

	secret, err := m.buildOpaqueSecret(context.Background())
	if err != nil {
		t.Fatalf("expected no error for empty/nil values, got %v", err)
	}

	// Empty and nil values should still be stored in the secret
	if len(secret.Data) != 2 {
		t.Errorf("expected 2 data entries, got %d", len(secret.Data))
	}
	if len(secret.Data["EMPTY_VALUE_KEY"]) != 0 {
		t.Errorf("expected empty value for EMPTY_VALUE_KEY, got %d bytes", len(secret.Data["EMPTY_VALUE_KEY"]))
	}
}

func TestBuildOpaqueSecret_BinaryPayload(t *testing.T) {
	binaryData := []byte{0x00, 0x01, 0x02, 0xff, 0xfe}
	m := &secretMaterializer{
		gsmSecret: &secretspizecomv1alpha1.GSMSecret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-gsmsecret",
				Namespace: "test-namespace",
			},
			Spec: secretspizecomv1alpha1.GSMSecretSpec{
				TargetSecret: secretspizecomv1alpha1.GSMSecretTargetSecret{
					Name: "my-target-secret",
				},
			},
		},
		payloads: []keyedSecretPayload{
			newTestPayload(t, "BINARY_DATA", binaryData),
		},
	}

	secret, err := m.buildOpaqueSecret(context.Background())
	if err != nil {
		t.Fatalf("expected no error for binary payload, got %v", err)
	}

	if string(secret.Data["BINARY_DATA"]) != string(binaryData) {
		t.Errorf("binary data not preserved correctly")
	}
}

func TestBuildOpaqueSecret_SpecialCharacterKeys(t *testing.T) {
	m := &secretMaterializer{
		gsmSecret: &secretspizecomv1alpha1.GSMSecret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-gsmsecret",
				Namespace: "test-namespace",
			},
			Spec: secretspizecomv1alpha1.GSMSecretSpec{
				TargetSecret: secretspizecomv1alpha1.GSMSecretTargetSecret{
					Name: "my-target-secret",
				},
			},
		},
		payloads: []keyedSecretPayload{
			newTestPayload(t, "KEY_WITH.DOT", []byte("value1")),
			newTestPayload(t, "KEY-WITH-DASH", []byte("value2")),
			newTestPayload(t, "KEY_WITH_UNDERSCORE", []byte("value3")),
		},
	}

	secret, err := m.buildOpaqueSecret(context.Background())
	if err != nil {
		t.Fatalf("expected no error for special character keys, got %v", err)
	}

	if len(secret.Data) != 3 {
		t.Errorf("expected 3 data entries, got %d", len(secret.Data))
	}
}
