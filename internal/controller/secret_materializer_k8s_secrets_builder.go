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

	corev1 "k8s.io/api/core/v1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

// buildOpaqueSecret constructs a Kubernetes Opaque Secret from the
// secretMaterializer's in-memory payloads and associated GSMSecret metadata.
func (m *secretMaterializer) buildOpaqueSecret(ctx context.Context) (*corev1.Secret, error) {
	if m == nil || m.gsmSecret == nil {
		return nil, fmt.Errorf("secretMaterializer or gsmSecret is nil")
	}

	log := logf.FromContext(ctx).WithValues("gsmsecret", m.gsmSecret.Name, "namespace", m.gsmSecret.Namespace)

	log.Info("building Kubernetes Opaque Secret from GSM payloads", "payloadCount", len(m.payloads))

	data := make(map[string][]byte, len(m.payloads))
	for _, p := range m.payloads {
		if p.Key == "" {
			log.Error(fmt.Errorf("empty key"), "encountered payload with empty key while building Secret")
			return nil, fmt.Errorf("payload has empty key")
		}
		data[p.Key] = p.Value
	}

	return &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Secret",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      m.gsmSecret.Spec.TargetSecret.Name,
			Namespace: m.gsmSecret.Namespace,
		},
		Type: corev1.SecretTypeOpaque,
		Data: data,
	}, nil
}
