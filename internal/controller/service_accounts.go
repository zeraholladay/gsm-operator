// Package controller contains helpers used by the GSMSecret reconciler, including
// Kubernetes ServiceAccount token retrieval and Google STS / Workload Identity
// Federation integration.
package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	authenticationv1 "k8s.io/api/authentication/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

// ptr is a small generic helper to take the address of a value inline, e.g. for
// Kubernetes API fields that expect *int64 or similar pointer types.
func ptr[T any](v T) *T { return &v }

// KSATokenRequestParams holds the inputs needed to ask the Kubernetes API
// for a short‑lived token representing a specific ServiceAccount.
type KSATokenRequestParams struct {
	Namespace  string // e.g. "gsmsecret-test-ns"
	KSAName    string // e.g. "gsm-reader"
	Audience   string // e.g. "https://kubernetes.default.svc"
	Expiration time.Duration
	Timeout    time.Duration
}

// RequestKSAToken uses the Kubernetes TokenRequest API to obtain a signed JWT
// for the given ServiceAccount. The token is audience‑restricted and
// short‑lived according to the provided parameters.
func RequestKSAToken(ctx context.Context, p KSATokenRequestParams) (string, error) {
	log := logf.FromContext(ctx).WithName("ksa_token").WithValues(
		"namespace", p.Namespace,
		"serviceAccount", p.KSAName,
	)

	// STEP 1: Basic validation and defaulting of inputs.
	if p.Namespace == "" || p.KSAName == "" {
		log.Error(fmt.Errorf("missing namespace or serviceAccountName"), "namespace and serviceAccountName are required")
		return "", fmt.Errorf("namespace and serviceAccountName are required")
	}
	if p.Audience == "" {
		log.Error(fmt.Errorf("missing audience"), "audience is required")
		return "", fmt.Errorf("audience is required")
	}
	if p.Expiration <= 0 {
		p.Expiration = 10 * time.Minute
	}
	if p.Timeout <= 0 {
		p.Timeout = 10 * time.Second
	}

	// STEP 2: Scope the operation to the provided timeout so we don't hang
	// indefinitely on API calls.
	ctx, cancel := context.WithTimeout(ctx, p.Timeout)
	defer cancel()

	log.Info("requesting Kubernetes ServiceAccount token",
		"audience", p.Audience,
		"expiration", p.Expiration.String(),
	)

	// STEP 3: Build an in-cluster client for talking to the Kubernetes API.
	cfg, err := rest.InClusterConfig()
	if err != nil {
		log.Error(err, "failed to build in-cluster Kubernetes config")
		return "", fmt.Errorf("build in-cluster config: %w", err)
	}
	client, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		log.Error(err, "failed to create Kubernetes client")
		return "", fmt.Errorf("create Kubernetes client: %w", err)
	}

	expSeconds := int64(p.Expiration.Seconds())

	// STEP 4: Construct a TokenRequest specifying audience and expiry.
	tokenReq := &authenticationv1.TokenRequest{
		Spec: authenticationv1.TokenRequestSpec{
			Audiences:         []string{p.Audience},
			ExpirationSeconds: ptr(expSeconds),
		},
	}

	// STEP 5: Ask the Kubernetes API to mint a short-lived token for the
	// target ServiceAccount.
	resp, err := client.CoreV1().
		ServiceAccounts(p.Namespace).
		CreateToken(ctx, p.KSAName, tokenReq, metav1.CreateOptions{})
	if err != nil {
		// STEP 6: Shape common errors into more actionable messages.
		if apierrors.IsForbidden(err) {
			log.Error(err, "token request forbidden; missing RBAC permissions")
			return "", fmt.Errorf("token request forbidden (need RBAC create on serviceaccounts/token in namespace %q): %w", p.Namespace, err)
		}
		if apierrors.IsNotFound(err) {
			log.Error(err, "serviceAccount not found or token endpoint unsupported")
			return "", fmt.Errorf("serviceaccount %q not found in namespace %q (or token endpoint unsupported): %w", p.KSAName, p.Namespace, err)
		}
		log.Error(err, "token request failed for ServiceAccount")
		return "", fmt.Errorf("token request failed: %w", err)
	}

	// STEP 7: Ensure the API returned a non-empty token string.
	if resp.Status.Token == "" {
		log.Error(fmt.Errorf("empty token"), "token request succeeded but returned an empty token")
		return "", fmt.Errorf("token request succeeded but token was empty")
	}

	log.Info("successfully obtained Kubernetes ServiceAccount token")
	return resp.Status.Token, nil
}

// stsTokenResponse models the subset of fields we care about from Google's
// Security Token Service token exchange response.
type stsTokenResponse struct {
	AccessToken     string `json:"access_token"`
	ExpiresIn       int64  `json:"expires_in"`
	IssuedTokenType string `json:"issued_token_type"`
	TokenType       string `json:"token_type"`
}

// staticTokenSource is an oauth2.TokenSource that always returns the same token.
// This is sufficient for our use-case because each reconcile is short-lived and
// we request a fresh KSA token (and thus STS token) per call.
type staticTokenSource struct {
	token *oauth2.Token
}

func (s *staticTokenSource) Token() (*oauth2.Token, error) {
	// Because each reconcile path obtains a fresh STS token, we can simply
	// reuse the same token for the lifetime of the Google client.
	return s.token, nil
}

// exchangeK8sTokenWithSTS exchanges a Kubernetes ServiceAccount JWT for a
// Google access token using Workload Identity Federation.
func exchangeK8sTokenWithSTS(ctx context.Context, k8sToken, wifAudience string) (*stsTokenResponse, error) {
	log := logf.FromContext(ctx).WithName("sts_exchange").WithValues(
		"wifAudience", wifAudience,
	)

	if k8sToken == "" {
		log.Error(fmt.Errorf("missing k8sToken"), "k8sToken is required for STS exchange")
		return nil, fmt.Errorf("k8sToken is required")
	}
	if wifAudience == "" {
		log.Error(fmt.Errorf("missing wifAudience"), "wifAudience is required for STS exchange")
		return nil, fmt.Errorf("wifAudience is required")
	}

	// STEP 1: Prepare the OAuth 2.0 token exchange form payload for STS.
	values := url.Values{}
	values.Set("grant_type", "urn:ietf:params:oauth:grant-type:token-exchange")
	values.Set("audience", wifAudience)
	values.Set("scope", "https://www.googleapis.com/auth/cloud-platform")
	values.Set("requested_token_type", "urn:ietf:params:oauth:token-type:access_token")
	values.Set("subject_token_type", "urn:ietf:params:oauth:token-type:jwt")
	values.Set("subject_token", k8sToken)

	// STEP 2: Construct the HTTP POST request against the Google STS endpoint.
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://sts.googleapis.com/v1/token", strings.NewReader(values.Encode()))
	if err != nil {
		log.Error(err, "failed to build STS HTTP request")
		return nil, fmt.Errorf("build STS request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	// Use a bounded-timeout HTTP client to avoid hanging reconciles if STS is
	// slow or unreachable but not failing fast.
	httpClient := &http.Client{
		Timeout: 10 * time.Second,
	}

	// STEP 3: Execute the HTTP request to STS.
	resp, err := httpClient.Do(req)
	if err != nil {
		log.Error(err, "failed to call STS endpoint")
		return nil, fmt.Errorf("call STS: %w", err)
	}
	defer resp.Body.Close()

	// STEP 4: Handle non-success HTTP responses with a concise error payload
	// to aid debugging (e.g. invalid_grant, audience mismatch, etc.).
	if resp.StatusCode != http.StatusOK {
		var bodySnippet struct {
			Error            string `json:"error,omitempty"`
			ErrorDescription string `json:"error_description,omitempty"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&bodySnippet)
		log.Error(fmt.Errorf("STS exchange failed"), "STS token exchange failed",
			"status", resp.Status,
			"error", bodySnippet.Error,
			"description", bodySnippet.ErrorDescription,
		)
		return nil, fmt.Errorf("STS token exchange failed: status=%s error=%q description=%q",
			resp.Status, bodySnippet.Error, bodySnippet.ErrorDescription)
	}

	// STEP 5: Decode the successful STS response into a typed struct.
	var tr stsTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tr); err != nil {
		log.Error(err, "failed to decode STS response body")
		return nil, fmt.Errorf("decode STS response: %w", err)
	}
	if tr.AccessToken == "" {
		log.Error(fmt.Errorf("missing access_token"), "STS response missing access_token")
		return nil, fmt.Errorf("STS response missing access_token")
	}

	log.Info("successfully exchanged Kubernetes token via STS",
		"tokenType", tr.TokenType,
		"expiresIn", tr.ExpiresIn,
	)
	return &tr, nil
}

// GCPCredsFromK8sToken turns a Kubernetes ServiceAccount JWT plus a Workload
// Identity Audience into a google.Credentials object that can be passed to
// Google client libraries (e.g. Secret Manager). The current implementation
// performs a direct STS token exchange and does not support GSA impersonation.
func GCPCredsFromK8sToken(
	ctx context.Context,
	k8sToken string,
	wifAudience string, // e.g. //iam.googleapis.com/projects/PROJECT_NUMBER/locations/global/workloadIdentityPools/POOL/providers/PROVIDER
	impersonateGSAEmail string, // optional; "" means no GSA impersonation
) (*google.Credentials, error) {
	log := logf.FromContext(ctx).WithName("gcp_creds_from_k8s").WithValues(
		"wifAudience", wifAudience,
		"impersonateGSAEmail", impersonateGSAEmail,
	)

	// STEP 0: Guardrail – we intentionally do not support GSA impersonation yet.
	if impersonateGSAEmail != "" {
		// Not implemented yet; the reconciler never passes a non-empty value.
		log.Error(fmt.Errorf("impersonation not implemented"), "GSA impersonation is not implemented")
		return nil, fmt.Errorf("GSA impersonation is not implemented")
	}

	// STEP 1: Exchange the Kubernetes ServiceAccount token for a Google access
	// token via the Workload Identity Federation provider.
	log.Info("exchanging Kubernetes ServiceAccount token for Google access token via WIF")
	stsResp, err := exchangeK8sTokenWithSTS(ctx, k8sToken, wifAudience)
	if err != nil {
		log.Error(err, "failed to exchange Kubernetes token via STS")
		return nil, fmt.Errorf("exchange KSA token via STS: %w", err)
	}

	// STEP 2: Convert the STS response into an oauth2.Token with an explicit
	// expiry timestamp.
	expiry := time.Now().Add(time.Duration(stsResp.ExpiresIn) * time.Second)
	token := &oauth2.Token{
		AccessToken: stsResp.AccessToken,
		TokenType:   stsResp.TokenType,
		Expiry:      expiry,
	}

	// STEP 3: Wrap the token in a google.Credentials instance so it can be
	// passed to Google client constructors (e.g. Secret Manager).
	creds := &google.Credentials{
		TokenSource: &staticTokenSource{token: token},
	}
	log.Info("successfully constructed google.Credentials from Kubernetes ServiceAccount token")
	return creds, nil
}
