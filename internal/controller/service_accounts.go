package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"golang.org/x/oauth2/google"
	authenticationv1 "k8s.io/api/authentication/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

func ptr[T any](v T) *T { return &v }

type KSATokenRequestParams struct {
	Namespace  string // e.g. "gsmsecret-test-ns"
	KSAName    string // e.g. "gsm-reader"
	Audience   string // e.g. "https://kubernetes.default.svc"
	Expiration time.Duration
	Timeout    time.Duration
}

func RequestKSAToken(ctx context.Context, client *kubernetes.Clientset, p KSATokenRequestParams) (string, error) {
	if p.Namespace == "" || p.KSAName == "" {
		return "", fmt.Errorf("namespace and serviceAccountName are required")
	}
	if p.Audience == "" {
		return "", fmt.Errorf("audience is required")
	}
	if p.Expiration <= 0 {
		p.Expiration = 10 * time.Minute
	}
	if p.Timeout <= 0 {
		p.Timeout = 10 * time.Second
	}

	ctx, cancel := context.WithTimeout(ctx, p.Timeout)
	defer cancel()

	expSeconds := int64(p.Expiration.Seconds())

	tokenReq := &authenticationv1.TokenRequest{
		Spec: authenticationv1.TokenRequestSpec{
			Audiences:         []string{p.Audience},
			ExpirationSeconds: ptr(expSeconds),
		},
	}

	resp, err := client.CoreV1().
		ServiceAccounts(p.Namespace).
		CreateToken(ctx, p.KSAName, tokenReq, metav1.CreateOptions{})
	if err != nil {
		// Helpful error shaping for debugging:
		if apierrors.IsForbidden(err) {
			return "", fmt.Errorf("token request forbidden (need RBAC create on serviceaccounts/token in namespace %q): %w", p.Namespace, err)
		}
		if apierrors.IsNotFound(err) {
			return "", fmt.Errorf("serviceaccount %q not found in namespace %q (or token endpoint unsupported): %w", p.KSAName, p.Namespace, err)
		}
		return "", fmt.Errorf("token request failed: %w", err)
	}

	if resp.Status.Token == "" {
		return "", fmt.Errorf("token request succeeded but token was empty")
	}
	return resp.Status.Token, nil
}

type ExternalAccountConfig struct {
	Type                           string `json:"type"` // "external_account"
	Audience                       string `json:"audience"`
	SubjectTokenType               string `json:"subject_token_type"` // "urn:ietf:params:oauth:token-type:jwt"
	TokenURL                       string `json:"token_url"`          // "https://sts.googleapis.com/v1/token"
	ServiceAccountImpersonationURL string `json:"service_account_impersonation_url,omitempty"`

	CredentialSource struct {
		SubjectToken string `json:"subject_token"`
	} `json:"credential_source"`
}

func GCPCredsFromK8sToken(
	ctx context.Context,
	k8sToken string,
	wifAudience string, // e.g. //iam.googleapis.com/projects/PROJECT_NUMBER/locations/global/workloadIdentityPools/POOL/providers/PROVIDER
	impersonateGSAEmail string, // optional; "" means no GSA impersonation
) (*google.Credentials, error) {

	cfg := ExternalAccountConfig{
		Type:             "external_account",
		Audience:         wifAudience,
		SubjectTokenType: "urn:ietf:params:oauth:token-type:jwt",
		TokenURL:         "https://sts.googleapis.com/v1/token",
	}
	cfg.CredentialSource.SubjectToken = k8sToken
	if impersonateGSAEmail != "" {
		cfg.ServiceAccountImpersonationURL =
			fmt.Sprintf("https://iamcredentials.googleapis.com/v1/projects/-/serviceAccounts/%s:generateAccessToken",
				impersonateGSAEmail)
	}

	raw, err := json.Marshal(cfg)
	if err != nil {
		return nil, fmt.Errorf("marshal external account config: %w", err)
	}

	creds, err := google.CredentialsFromJSON(ctx, raw, "https://www.googleapis.com/auth/cloud-platform")
	if err != nil {
		return nil, fmt.Errorf("build google credentials from external account config: %w", err)
	}

	return creds, err
}
