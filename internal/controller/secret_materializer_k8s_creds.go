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
	"sync"
	"time"

	authenticationv1 "k8s.io/api/authentication/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

// requestKSAToken uses the Kubernetes TokenRequest API to obtain a signed JWT
// for the given ServiceAccount. The token is audience-restricted and
// short-lived according to the provided parameters.
func (m *secretMaterializer) requestKSAToken(ctx context.Context) (string, error) {
	namespace := m.gsmSecret.Namespace
	ksa := m.getKSA()
	wifAudience, err := m.getWIFAudience()
	if err != nil {
		return "", err
	}

	log := logf.FromContext(ctx).WithName("ksa_token").WithValues(
		"namespace", namespace,
		"ksa", ksa,
		"wifAudience", wifAudience,
	)

	if namespace == "" || ksa == "" {
		log.Error(fmt.Errorf("missing namespace or ksaName"), "namespace and ksaName are required")
		return "", fmt.Errorf("namespace and ksaName are required")
	}
	// STEP 1: Derive expiration and timeout from config, falling back to defaults when unset.
	expiration := time.Duration(m.getTokenExpSeconds()) * time.Second
	if expiration <= 0 {
		expiration = 10 * time.Minute
	}
	timeout := time.Duration(m.getHTTPRequestTimeoutSeconds()) * time.Second
	if timeout <= 0 {
		timeout = 10 * time.Second
	}

	// STEP 2: Scope the operation to the provided timeout so we don't hang
	// indefinitely on API calls.
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	log.Info("requesting Kubernetes ServiceAccount token",
		"audience", wifAudience,
		"expiration", expiration.String(),
	)

	// STEP 3: Build (or reuse) an in-cluster client for talking to the Kubernetes API.
	client, err := m.getKubeClient()
	if err != nil {
		log.Error(err, "failed to get in-cluster Kubernetes client")
		return "", err
	}

	expSeconds := int64(expiration.Seconds())

	// STEP 4: Construct a TokenRequest specifying audience and expiry.
	tokenReq := &authenticationv1.TokenRequest{
		Spec: authenticationv1.TokenRequestSpec{
			Audiences:         []string{wifAudience},
			ExpirationSeconds: &expSeconds,
		},
	}

	// STEP 5: Ask the Kubernetes API to mint a short-lived token for the
	// target ServiceAccount.
	resp, err := client.CoreV1().
		ServiceAccounts(namespace).
		CreateToken(ctx, ksa, tokenReq, metav1.CreateOptions{})
	if err != nil {
		// STEP 6: Shape common errors into more actionable messages.
		if apierrors.IsForbidden(err) {
			log.Error(err, "token request forbidden; missing RBAC permissions")
			return "", fmt.Errorf("token request forbidden (need RBAC create on serviceaccounts/token in namespace %q): %w", namespace, err)
		}
		if apierrors.IsNotFound(err) {
			log.Error(err, "ksa not found or token endpoint unsupported")
			return "", fmt.Errorf("serviceaccount %q not found in namespace %q (or token endpoint unsupported): %w", ksa, namespace, err)
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

var (
	kubeClientOnce sync.Once
	kubeClient     kubernetes.Interface
	kubeClientErr  error
)

// getInClusterKubeClient returns a process-wide shared in-cluster Kubernetes client.
// It is safe for concurrent use and only initializes the client once.
func getInClusterKubeClient() (kubernetes.Interface, error) {
	kubeClientOnce.Do(func() {
		cfg, err := rest.InClusterConfig()
		if err != nil {
			kubeClientErr = fmt.Errorf("build in-cluster config: %w", err)
			return
		}
		clientset, err := kubernetes.NewForConfig(cfg)
		if err != nil {
			kubeClientErr = fmt.Errorf("create Kubernetes client: %w", err)
			return
		}
		kubeClient = clientset
	})
	return kubeClient, kubeClientErr
}
