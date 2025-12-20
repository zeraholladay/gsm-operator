package controller

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

// BuildOpaqueSecret constructs an in-memory Secret object from the provided payloads.
// Returns an error if any payload has an empty key.
func BuildOpaqueSecret(
	name, namespace string,
	payloads []KeyedSecretPayload,
) (*corev1.Secret, error) {
	log := logf.Log.WithName("k8s_secret").WithValues(
		"name", name,
		"namespace", namespace,
	)

	log.Info("building Kubernetes Opaque Secret from GSM payloads", "payloadCount", len(payloads))

	data := make(map[string][]byte, len(payloads))
	for _, p := range payloads {
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
			Name:      name,
			Namespace: namespace,
		},
		Type: corev1.SecretTypeOpaque,
		Data: data,
	}, nil
}
