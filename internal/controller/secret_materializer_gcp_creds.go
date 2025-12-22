package controller

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

import (
	"context"
	"fmt"
	"time"

	xoauth2 "golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/option"
	"google.golang.org/api/sts/v1"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

// getCredentials builds Google credentials for the current GSMSecret by
// requesting a KSA token and exchanging it via Workload Identity Federation.
func (m *secretMaterializer) getCredentials(ctx context.Context) (*google.Credentials, error) {
	log := logf.FromContext(ctx).WithValues(
		"gsmsecret", m.gsmSecret.Name,
		"namespace", m.gsmSecret.Namespace,
	)

	// STEP 0: Resolve WIF audience upfront so we fail fast if misconfigured.
	wifAudience, err := m.getWIFAudience()
	if err != nil {
		log.Error(err, "failed to get WIF audience")
		return nil, fmt.Errorf("get WIF audience: %w", err)
	}

	// STEP 1: Request a short-lived JWT for the tenant KSA.
	log.Info("requesting Kubernetes ServiceAccount token for GSM payload fetch")
	token, err := m.requestKSAToken(ctx)
	if err != nil {
		log.Error(err, "failed to request Kubernetes ServiceAccount token")
		return nil, fmt.Errorf("request KSA token: %w", err)
	}

	// STEP 2: Exchange the KSA token for Google credentials via Workload Identity.
	log.Info("exchanging Kubernetes ServiceAccount token via Workload Identity Federation")
	creds, err := m.gcpCredsFromK8sToken(ctx, token, wifAudience)
	if err != nil {
		log.Error(err, "failed to exchange KSA token for Google credentials")
		return nil, fmt.Errorf("exchange KSA token for Google credentials: %w", err)
	}
	return creds, nil
}

// gcpCredsFromK8sToken turns a Kubernetes ServiceAccount JWT plus a Workload
// Identity Audience into a google.Credentials object that can be passed to
// Google client libraries (e.g. Secret Manager). The current implementation
// performs a direct STS token exchange and does not support GSA impersonation.
func (m *secretMaterializer) gcpCredsFromK8sToken(
	ctx context.Context,
	k8sToken string,
	wifAudience string,
) (*google.Credentials, error) {
	log := logf.FromContext(ctx).WithName("gcp_creds_from_k8s").WithValues(
		"wifAudience", wifAudience,
	)

	// STEP 1: Exchange the Kubernetes ServiceAccount token for a Google access
	// token via the Workload Identity Federation provider.
	log.Info("exchanging Kubernetes ServiceAccount token for Google access token via WIF")
	stsResp, err := m.exchangeK8sTokenWithSTS(ctx, k8sToken, wifAudience)
	if err != nil {
		log.Error(err, "failed to exchange Kubernetes token via STS")
		return nil, fmt.Errorf("exchange KSA token via STS: %w", err)
	}

	// STEP 2: Convert the STS response into an oauth2.Token with an explicit
	// expiry timestamp.
	expiry := time.Now().Add(time.Duration(stsResp.ExpiresIn) * time.Second)
	token := &xoauth2.Token{
		AccessToken: stsResp.AccessToken,
		TokenType:   stsResp.TokenType,
		Expiry:      expiry,
	}

	// STEP 3: Wrap the token in a google.Credentials instance so it can be
	// passed to Google client constructors (e.g. Secret Manager).
	creds := &google.Credentials{
		TokenSource: xoauth2.StaticTokenSource(token),
	}
	log.Info("successfully constructed google.Credentials from Kubernetes ServiceAccount token")
	return creds, nil
}

// exchangeK8sTokenWithSTS exchanges a Kubernetes ServiceAccount JWT for a
// Google access token using the official STS library.
func (m *secretMaterializer) exchangeK8sTokenWithSTS(ctx context.Context, k8sToken, wifAudience string) (*sts.GoogleIdentityStsV1ExchangeTokenResponse, error) {
	// Initialize the STS service.
	// Note: We use WithoutAuthentication() because we are calling the token
	// exchange endpoint to *get* credentials. We don't have them yet.
	stsService, err := sts.NewService(ctx, option.WithoutAuthentication())
	if err != nil {
		return nil, fmt.Errorf("failed to create STS service: %w", err)
	}

	// Construct the request using the library's types.
	req := &sts.GoogleIdentityStsV1ExchangeTokenRequest{
		GrantType:          "urn:ietf:params:oauth:grant-type:token-exchange",
		Audience:           wifAudience,
		Scope:              "https://www.googleapis.com/auth/cloud-platform",
		RequestedTokenType: "urn:ietf:params:oauth:token-type:access_token",
		SubjectTokenType:   "urn:ietf:params:oauth:token-type:jwt",
		SubjectToken:       k8sToken,
	}

	// Execute the request.
	resp, err := stsService.V1.Token(req).Do()
	if err != nil {
		return nil, fmt.Errorf("STS exchange failed: %w", err)
	}

	// Map the library response to your internal struct.
	return resp, nil
}
