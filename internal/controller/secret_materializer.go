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
	"fmt"
	"os"
	"strconv"
	"strings"

	"k8s.io/client-go/kubernetes"

	secretspizecomv1alpha1 "github.com/zeraholladay/gsm-operator/api/v1alpha1"
)

const (
	// Kubernetes TokenRequest API enforces a minimum expiration of 10 minutes.
	minTokenExpSeconds uint64 = 10 * 60
	// Default to the Kubernetes minimum unless explicitly overridden higher.
	defaultTokenExpSeconds uint64 = minTokenExpSeconds
	// defaultKSAName is the default Kubernetes ServiceAccount name when not specified.
	defaultKSAName = "default"
)

// secretMaterializer holds the dependencies and state required to materialize
// a Kubernetes Secret from a GSMSecret resource.
type secretMaterializer struct {
	gsmSecret    *secretspizecomv1alpha1.GSMSecret
	payloads     []keyedSecretPayload
	kubeClientFn func() (kubernetes.Interface, error)
}

// keyedSecretPayload holds a Kubernetes Secret data key and its corresponding GSM payload.
type keyedSecretPayload struct {
	Key   string
	Value []byte
}

func (m *secretMaterializer) getKSA() string {
	if v := os.Getenv("KSA"); v != "" {
		return v
	}

	if ann := m.gsmSecret.GetAnnotations(); ann != nil {
		if v := strings.TrimSpace(ann[secretspizecomv1alpha1.AnnotationKSA]); v != "" {
			return v
		}
	}
	return defaultKSAName
}

func (m *secretMaterializer) getWIFAudience() (string, error) {
	if v := os.Getenv("WIFAUDIENCE"); v != "" {
		return v, nil
	}

	if ann := m.gsmSecret.GetAnnotations(); ann != nil {
		if v := strings.TrimSpace(ann[secretspizecomv1alpha1.AnnotationWIFAudience]); v != "" {
			return v, nil
		}
	}
	return "", fmt.Errorf("WIFAudience not set: set WIFAUDIENCE env var or annotation %q", secretspizecomv1alpha1.AnnotationWIFAudience)
}

// The token may not specify a duration less than 10 minutes
func (m *secretMaterializer) getTokenExpSeconds() uint64 {
	if v := os.Getenv("TOKEN_EXP_SECONDS"); v != "" {
		if parsed, err := strconv.ParseUint(v, 10, 64); err == nil && parsed > 0 {
			if parsed < minTokenExpSeconds {
				return minTokenExpSeconds
			}
			return parsed
		}
	}
	return defaultTokenExpSeconds
}

func (m *secretMaterializer) getHTTPRequestTimeoutSeconds() uint64 {
	if v := os.Getenv("HTTP_TIMEOUT_SECONDS"); v != "" {
		if parsed, err := strconv.ParseUint(v, 10, 64); err == nil && parsed > 0 {
			return parsed
		}
	}
	return 30
}

func (m *secretMaterializer) getKubeClient() (kubernetes.Interface, error) {
	if m.kubeClientFn != nil {
		return m.kubeClientFn()
	}
	return getInClusterKubeClient()
}
