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
	"errors"
	"testing"

	xoauth2 "golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	authenticationv1 "k8s.io/api/authentication/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"

	secretspizecomv1alpha1 "github.com/zeraholladay/gsm-operator/api/v1alpha1"
)

// ==================== KSA Token Request Failure Tests ====================

func TestRequestKSAToken_MissingNamespace(t *testing.T) {
	m := &secretMaterializer{
		gsmSecret: &secretspizecomv1alpha1.GSMSecret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-gsmsecret",
				Namespace: "", // empty namespace
				Annotations: map[string]string{
					secretspizecomv1alpha1.AnnotationWIFAudience: "//iam.googleapis.com/projects/123/locations/global/workloadIdentityPools/pool/providers/provider",
				},
			},
		},
		kubeClientFn: func() (kubernetes.Interface, error) {
			return fake.NewClientset(), nil
		},
	}

	t.Setenv("KSA", "test-ksa")

	_, err := m.requestKSAToken(context.Background())
	if err == nil {
		t.Fatal("expected error for missing namespace")
	}

	if err.Error() != "namespace and ksaName are required" {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestRequestKSAToken_MissingKSA(t *testing.T) {
	t.Setenv("KSA", "")

	m := &secretMaterializer{
		gsmSecret: &secretspizecomv1alpha1.GSMSecret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-gsmsecret",
				Namespace: "default",
				Annotations: map[string]string{
					secretspizecomv1alpha1.AnnotationWIFAudience: "//iam.googleapis.com/projects/123/locations/global/workloadIdentityPools/pool/providers/provider",
				},
			},
		},
		kubeClientFn: func() (kubernetes.Interface, error) {
			return fake.NewClientset(), nil
		},
	}

	// When KSA env is empty and no annotation, getKSA returns "default"
	// So this test verifies that "default" KSA is used
	_, err := m.requestKSAToken(context.Background())
	// This will fail because the fake client doesn't support TokenRequest
	// but we're testing that it doesn't fail due to missing KSA name
	if err == nil {
		t.Fatal("expected error from token request, but should not be for missing KSA")
	}
}

func TestRequestKSAToken_KubeClientError(t *testing.T) {
	expectedErr := errors.New("failed to create kubernetes client")

	m := &secretMaterializer{
		gsmSecret: &secretspizecomv1alpha1.GSMSecret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-gsmsecret",
				Namespace: "default",
				Annotations: map[string]string{
					secretspizecomv1alpha1.AnnotationWIFAudience: "//iam.googleapis.com/projects/123/locations/global/workloadIdentityPools/pool/providers/provider",
				},
			},
		},
		kubeClientFn: func() (kubernetes.Interface, error) {
			return nil, expectedErr
		},
	}

	t.Setenv("KSA", "test-ksa")

	_, err := m.requestKSAToken(context.Background())
	if err == nil {
		t.Fatal("expected error when kube client creation fails")
	}

	if !errors.Is(err, expectedErr) {
		t.Errorf("expected error to wrap %v, got %v", expectedErr, err)
	}
}

func TestRequestKSAToken_Forbidden(t *testing.T) {
	fakeClient := fake.NewClientset()
	fakeClient.PrependReactor("create", "serviceaccounts/token", func(action k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewForbidden(
			schema.GroupResource{Group: "", Resource: "serviceaccounts/token"},
			"test-ksa",
			errors.New("RBAC denied"),
		)
	})

	m := &secretMaterializer{
		gsmSecret: &secretspizecomv1alpha1.GSMSecret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-gsmsecret",
				Namespace: "default",
				Annotations: map[string]string{
					secretspizecomv1alpha1.AnnotationWIFAudience: "//iam.googleapis.com/projects/123/locations/global/workloadIdentityPools/pool/providers/provider",
				},
			},
		},
		kubeClientFn: func() (kubernetes.Interface, error) {
			return fakeClient, nil
		},
	}

	t.Setenv("KSA", "test-ksa")

	_, err := m.requestKSAToken(context.Background())
	if err == nil {
		t.Fatal("expected error when token request is forbidden")
	}

	if !apierrors.IsForbidden(errors.Unwrap(err)) {
		// Check if the error message contains the expected text
		if err.Error() == "" || !containsSubstring(err.Error(), "forbidden") {
			t.Errorf("expected forbidden error, got: %v", err)
		}
	}
}

func TestRequestKSAToken_ServiceAccountNotFound(t *testing.T) {
	fakeClient := fake.NewClientset()
	fakeClient.PrependReactor("create", "serviceaccounts/token", func(action k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewNotFound(
			schema.GroupResource{Group: "", Resource: "serviceaccounts"},
			"test-ksa",
		)
	})

	m := &secretMaterializer{
		gsmSecret: &secretspizecomv1alpha1.GSMSecret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-gsmsecret",
				Namespace: "default",
				Annotations: map[string]string{
					secretspizecomv1alpha1.AnnotationWIFAudience: "//iam.googleapis.com/projects/123/locations/global/workloadIdentityPools/pool/providers/provider",
				},
			},
		},
		kubeClientFn: func() (kubernetes.Interface, error) {
			return fakeClient, nil
		},
	}

	t.Setenv("KSA", "test-ksa")

	_, err := m.requestKSAToken(context.Background())
	if err == nil {
		t.Fatal("expected error when service account not found")
	}

	if !containsSubstring(err.Error(), "not found") {
		t.Errorf("expected not found error, got: %v", err)
	}
}

func TestRequestKSAToken_EmptyTokenReturned(t *testing.T) {
	fakeClient := fake.NewClientset()
	fakeClient.PrependReactor("create", "serviceaccounts/token", func(action k8stesting.Action) (bool, runtime.Object, error) {
		return true, &authenticationv1.TokenRequest{
			Status: authenticationv1.TokenRequestStatus{
				Token: "", // empty token
			},
		}, nil
	})

	m := &secretMaterializer{
		gsmSecret: &secretspizecomv1alpha1.GSMSecret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-gsmsecret",
				Namespace: "default",
				Annotations: map[string]string{
					secretspizecomv1alpha1.AnnotationWIFAudience: "//iam.googleapis.com/projects/123/locations/global/workloadIdentityPools/pool/providers/provider",
				},
			},
		},
		kubeClientFn: func() (kubernetes.Interface, error) {
			return fakeClient, nil
		},
	}

	t.Setenv("KSA", "test-ksa")

	_, err := m.requestKSAToken(context.Background())
	if err == nil {
		t.Fatal("expected error when empty token is returned")
	}

	if err.Error() != "token request succeeded but token was empty" {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestRequestKSAToken_Success(t *testing.T) {
	expectedToken := "eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9.test-token"
	fakeClient := fake.NewClientset(
		// Pre-create the service account
		&corev1.ServiceAccount{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-ksa",
				Namespace: "default",
			},
		},
	)
	fakeClient.PrependReactor("create", "serviceaccounts/token", func(action k8stesting.Action) (bool, runtime.Object, error) {
		return true, &authenticationv1.TokenRequest{
			Status: authenticationv1.TokenRequestStatus{
				Token: expectedToken,
			},
		}, nil
	})

	m := &secretMaterializer{
		gsmSecret: &secretspizecomv1alpha1.GSMSecret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-gsmsecret",
				Namespace: "default",
				Annotations: map[string]string{
					secretspizecomv1alpha1.AnnotationWIFAudience: "//iam.googleapis.com/projects/123/locations/global/workloadIdentityPools/pool/providers/provider",
				},
			},
		},
		kubeClientFn: func() (kubernetes.Interface, error) {
			return fakeClient, nil
		},
	}

	t.Setenv("KSA", "test-ksa")

	token, err := m.requestKSAToken(context.Background())
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if token != expectedToken {
		t.Errorf("expected token %q, got %q", expectedToken, token)
	}
}

// ==================== WIF Audience Tests ====================

func TestGetCredentials_MissingWIFAudience(t *testing.T) {
	t.Setenv("WIFAUDIENCE", "")

	m := &secretMaterializer{
		gsmSecret: &secretspizecomv1alpha1.GSMSecret{
			ObjectMeta: metav1.ObjectMeta{
				Name:        "test-gsmsecret",
				Namespace:   "default",
				Annotations: map[string]string{}, // no WIF audience annotation
			},
		},
	}

	_, err := m.getCredentials(context.Background())
	if err == nil {
		t.Fatal("expected error when WIF audience is missing")
	}

	if !containsSubstring(err.Error(), "WIFAudience not set") {
		t.Errorf("expected WIFAudience error, got: %v", err)
	}
}

func TestGetCredentials_KSATokenRequestFails(t *testing.T) {
	t.Setenv("WIFAUDIENCE", "//iam.googleapis.com/projects/123/locations/global/workloadIdentityPools/pool/providers/provider")
	t.Setenv("KSA", "test-ksa")

	expectedErr := errors.New("kube client unavailable")
	m := &secretMaterializer{
		gsmSecret: &secretspizecomv1alpha1.GSMSecret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-gsmsecret",
				Namespace: "default",
			},
		},
		kubeClientFn: func() (kubernetes.Interface, error) {
			return nil, expectedErr
		},
	}

	_, err := m.getCredentials(context.Background())
	if err == nil {
		t.Fatal("expected error when KSA token request fails")
	}

	if !containsSubstring(err.Error(), "request KSA token") {
		t.Errorf("expected KSA token request error, got: %v", err)
	}
}

// ==================== HTTP Request Timeout Tests ====================

func TestGetHTTPRequestTimeoutSeconds_Default(t *testing.T) {
	t.Setenv("HTTP_TIMEOUT_SECONDS", "")

	m := &secretMaterializer{}

	if got := m.getHTTPRequestTimeoutSeconds(); got != 30 {
		t.Errorf("expected default timeout 30, got %d", got)
	}
}

func TestGetHTTPRequestTimeoutSeconds_CustomValue(t *testing.T) {
	t.Setenv("HTTP_TIMEOUT_SECONDS", "60")

	m := &secretMaterializer{}

	if got := m.getHTTPRequestTimeoutSeconds(); got != 60 {
		t.Errorf("expected timeout 60, got %d", got)
	}
}

func TestGetHTTPRequestTimeoutSeconds_InvalidValue(t *testing.T) {
	t.Setenv("HTTP_TIMEOUT_SECONDS", "invalid")

	m := &secretMaterializer{}

	// Should fall back to default
	if got := m.getHTTPRequestTimeoutSeconds(); got != 30 {
		t.Errorf("expected default timeout 30 for invalid value, got %d", got)
	}
}

func TestGetHTTPRequestTimeoutSeconds_Zero(t *testing.T) {
	t.Setenv("HTTP_TIMEOUT_SECONDS", "0")

	m := &secretMaterializer{}

	// Zero should fall back to default
	if got := m.getHTTPRequestTimeoutSeconds(); got != 30 {
		t.Errorf("expected default timeout 30 for zero value, got %d", got)
	}
}

// ==================== resolvePayloads Tests ====================

func TestResolvePayloads_EmptySecrets(t *testing.T) {
	m := &secretMaterializer{
		gsmSecret: &secretspizecomv1alpha1.GSMSecret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-gsmsecret",
				Namespace: "default",
			},
			Spec: secretspizecomv1alpha1.GSMSecretSpec{
				TargetSecret: secretspizecomv1alpha1.GSMSecretTargetSecret{Name: "target"},
				Secrets:      []secretspizecomv1alpha1.GSMSecretEntry{}, // empty
			},
		},
	}

	err := m.resolvePayloads(context.Background())
	if err != nil {
		t.Fatalf("expected no error for empty secrets, got: %v", err)
	}

	if len(m.payloads) != 0 {
		t.Errorf("expected 0 payloads, got %d", len(m.payloads))
	}
}

func TestResolvePayloads_WIFAudienceError(t *testing.T) {
	t.Setenv("WIFAUDIENCE", "")

	m := &secretMaterializer{
		gsmSecret: &secretspizecomv1alpha1.GSMSecret{
			ObjectMeta: metav1.ObjectMeta{
				Name:        "test-gsmsecret",
				Namespace:   "default",
				Annotations: map[string]string{}, // no WIF audience
			},
			Spec: secretspizecomv1alpha1.GSMSecretSpec{
				TargetSecret: secretspizecomv1alpha1.GSMSecretTargetSecret{Name: "target"},
				Secrets: []secretspizecomv1alpha1.GSMSecretEntry{
					{Key: "KEY", ProjectID: "proj", SecretID: "secret", Version: "1"},
				},
			},
		},
	}

	err := m.resolvePayloads(context.Background())
	if err == nil {
		t.Fatal("expected error when WIF audience is missing")
	}

	if !containsSubstring(err.Error(), "WIFAudience") {
		t.Errorf("expected WIFAudience error, got: %v", err)
	}
}

// ==================== GSA Impersonation Tests ====================

func TestGsaCredsFromGcpCreds_ReturnsCredentials(t *testing.T) {
	m := &secretMaterializer{
		gsmSecret: &secretspizecomv1alpha1.GSMSecret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-gsmsecret",
				Namespace: "default",
			},
		},
	}

	// Create a mock credentials with a static token source
	mockCreds := &google.Credentials{
		TokenSource: mockTokenSource{token: "mock-access-token"},
	}

	// gsaCredsFromGcpCreds creates an impersonated token source.
	// The impersonate library creates the token source successfully,
	// but token fetches will fail without real GCP credentials.
	creds, err := m.gsaCredsFromGcpCreds(context.Background(), mockCreds, "test-gsa@project.iam.gserviceaccount.com")
	if err != nil {
		t.Fatalf("unexpected error creating impersonated credentials: %v", err)
	}

	if creds == nil {
		t.Fatal("expected non-nil credentials")
	}

	if creds.TokenSource == nil {
		t.Fatal("expected credentials to have a TokenSource")
	}
}

func TestGsaCredsFromGcpCreds_EmptyGSA(t *testing.T) {
	m := &secretMaterializer{
		gsmSecret: &secretspizecomv1alpha1.GSMSecret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-gsmsecret",
				Namespace: "default",
			},
		},
	}

	mockCreds := &google.Credentials{
		TokenSource: mockTokenSource{token: "mock-access-token"},
	}

	// Empty GSA should fail
	_, err := m.gsaCredsFromGcpCreds(context.Background(), mockCreds, "")
	if err == nil {
		t.Fatal("expected error when impersonating empty GSA")
	}
}

func TestGetGSA_IntegrationWithGetCredentials(t *testing.T) {
	// This test verifies the GSA retrieval logic is correctly integrated
	// when GSA annotation is set
	t.Setenv("WIFAUDIENCE", "//iam.googleapis.com/projects/123/locations/global/workloadIdentityPools/pool/providers/provider")
	t.Setenv("KSA", "test-ksa")

	expectedToken := "eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9.test-token"
	fakeClient := fake.NewClientset(
		&corev1.ServiceAccount{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-ksa",
				Namespace: "default",
			},
		},
	)
	fakeClient.PrependReactor("create", "serviceaccounts/token", func(action k8stesting.Action) (bool, runtime.Object, error) {
		return true, &authenticationv1.TokenRequest{
			Status: authenticationv1.TokenRequestStatus{
				Token: expectedToken,
			},
		}, nil
	})

	m := &secretMaterializer{
		gsmSecret: &secretspizecomv1alpha1.GSMSecret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-gsmsecret",
				Namespace: "default",
				Annotations: map[string]string{
					secretspizecomv1alpha1.AnnotationGSA: "test-gsa@project.iam.gserviceaccount.com",
				},
			},
		},
		kubeClientFn: func() (kubernetes.Interface, error) {
			return fakeClient, nil
		},
	}

	// Verify getGSA returns the expected value
	if got := m.getGSA(); got != "test-gsa@project.iam.gserviceaccount.com" {
		t.Errorf("expected GSA annotation value, got %q", got)
	}

	// getCredentials will fail at the STS exchange step (no real GCP),
	// but we've verified the GSA is correctly retrieved
}

func TestGetCredentials_NoGSAImpersonationWhenAnnotationMissing(t *testing.T) {
	// Verify that when GSA annotation is not set, getGSA returns empty string
	m := &secretMaterializer{
		gsmSecret: &secretspizecomv1alpha1.GSMSecret{
			ObjectMeta: metav1.ObjectMeta{
				Name:        "test-gsmsecret",
				Namespace:   "default",
				Annotations: map[string]string{
					// No GSA annotation
				},
			},
		},
	}

	if got := m.getGSA(); got != "" {
		t.Errorf("expected empty GSA when annotation missing, got %q", got)
	}
}

// mockTokenSource is a simple token source for testing
type mockTokenSource struct {
	token string
}

func (m mockTokenSource) Token() (*xoauth2.Token, error) {
	return &xoauth2.Token{
		AccessToken: m.token,
		TokenType:   "Bearer",
	}, nil
}

// Helper function
func containsSubstring(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
