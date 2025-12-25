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
	"encoding/json"
	"fmt"
	"strings"

	secretmanager "cloud.google.com/go/secretmanager/apiv1"

	"cloud.google.com/go/secretmanager/apiv1/secretmanagerpb"
	"github.com/kaptinlin/jsonpointer"
	secretspizecomv1alpha1 "github.com/zeraholladay/gsm-operator/api/v1alpha1"
	"google.golang.org/api/option"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

// resolvePayloads populates the secretMaterializer's payloads slice by
// fetching data from Google Secret Manager for the associated GSMSecret.
func (m *secretMaterializer) resolvePayloads(ctx context.Context) error {
	log := logf.FromContext(ctx).WithValues(
		"gsmsecret", m.gsmSecret.Name,
		"namespace", m.gsmSecret.Namespace,
	)

	// Ensure the enriched logger is available via context for helper calls.
	ctx = logf.IntoContext(ctx, log)

	// Nothing to do if the spec has no gsmSecrets entries.
	if len(m.gsmSecret.Spec.Secrets) == 0 {
		log.V(1).Info("GSMSecret has no entries; nothing to fetch")
		return nil
	}

	// STEP 1: Build a Secret Manager client bound to the tenant identity via WIF.
	client, err := m.newGsmClient(ctx)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := client.Close(); cerr != nil {
			log.Error(cerr, "failed to close Secret Manager client")
		}
	}()

	// STEP 2: Read each configured GSM secret entry and collect their payloads
	// so they can be materialized into the target Kubernetes Secret.
	results, err := m.fetchSecretEntriesPayloads(ctx, client)
	if err != nil {
		log.Error(err, "failed to fetch GSM secret entry payloads")
		return err
	}

	m.payloads = results

	return nil
}

// newGsmClient exchanges the Kubernetes ServiceAccount token for Google credentials
// via Workload Identity Federation and returns a Secret Manager client.
func (m *secretMaterializer) newGsmClient(ctx context.Context) (*secretmanager.Client, error) {
	log := logf.FromContext(ctx)

	// Is in "Trusted Subsystem" mode?
	if m.isTrustedSubsystem() {
		log.Info("using trusted subsystem mode: operator acting as its own IAM principal")
		client, err := secretmanager.NewClient(ctx)
		if err != nil {
			log.Error(err, "failed to create Secret Manager client in trusted subsystem mode")
			return nil, fmt.Errorf("secretmanager.NewClient (trusted subsystem): %w", err)
		}
		return client, nil
	}

	// Exchange the KSA token for Google credentials via WIF.
	log.Info("exchanging Kubernetes ServiceAccount token via Workload Identity Federation")
	creds, err := m.getGcpCreds(ctx)
	if err != nil {
		log.Error(err, "failed to exchange KSA token for Google credentials")
		return nil, fmt.Errorf("exchange KSA token for Google credentials: %w", err)
	}

	// Build a Secret Manager client bound to the tenant identity.
	log.Info("creating Google Secret Manager client with federated credentials")
	client, err := secretmanager.NewClient(ctx, option.WithCredentials(creds))
	if err != nil {
		log.Error(err, "failed to create Secret Manager client")
		return nil, fmt.Errorf("secretmanager.NewClient WithCredentials: %w", err)
	}

	return client, nil
}

// fetchSecretEntriesPayloads reads each configured GSM secret entry from Google
// Secret Manager and returns the payloads keyed by the target Secret data key.
func (m *secretMaterializer) fetchSecretEntriesPayloads(
	ctx context.Context,
	client *secretmanager.Client,
) ([]keyedSecretPayload, error) {
	log := logf.FromContext(ctx)

	results := make([]keyedSecretPayload, 0, len(m.gsmSecret.Spec.Secrets))

	for _, e := range m.gsmSecret.Spec.Secrets {
		// Validation: reject entries that try to use both single key and multi-key forms.
		if e.Key != "" && len(e.Keys) > 0 {
			return nil, fmt.Errorf("invalid GSMSecret entry: cannot set both key and keys")
		}

		// Fetch the secret payload from GSM for the requested project/secret/version.
		log.V(1).Info("fetching GSM secret payload",
			"projectID", e.ProjectID,
			"secretID", e.SecretID,
			"version", e.Version,
		)

		name := fmt.Sprintf("projects/%s/secrets/%s/versions/%s", e.ProjectID, e.SecretID, e.Version)

		data, err := accessSecretPayload(ctx, client, name)
		if err != nil {
			log.Error(err, "failed to fetch GSM secret payload",
				"projectID", e.ProjectID,
				"secretID", e.SecretID,
				"version", e.Version,
			)
			return nil, fmt.Errorf("fetch payload for key %q (project=%q, secret=%q, version=%q): %w",
				e.Key, e.ProjectID, e.SecretID, e.Version, err)
		}

		// Materialize the payload either as a single key or via multi-key mappings.
		switch {
		case e.Key != "":
			payload, err := newKeyedSecretPayload(e.Key, data)
			if err != nil {
				return nil, fmt.Errorf("validate key %q: %w", e.Key, err)
			}
			results = append(results, payload)
		case len(e.Keys) > 0:
			mapped, err := mapKeysToSecretKeyMappings(data, e.Keys)
			if err != nil {
				return nil, fmt.Errorf("map key mappings for secret %q: %w", e.SecretID, err)
			}
			results = append(results, mapped...)
		default:
			// Spec requires exactly one of key or keys.
			return nil, fmt.Errorf("invalid GSMSecret entry: either key or keys must be set")
		}
	}

	return results, nil
}

func accessSecretPayload(
	ctx context.Context,
	client *secretmanager.Client,
	name string,
) ([]byte, error) {
	log := logf.FromContext(ctx).WithValues(
		"name", name,
	)

	log.V(1).Info("accessing GSM secret version", "resource", name)

	resp, err := client.AccessSecretVersion(ctx, &secretmanagerpb.AccessSecretVersionRequest{
		Name: name,
	})
	if err != nil {
		log.Error(err, "failed to access GSM secret version", "resource", name)
		return nil, fmt.Errorf("AccessSecretVersion(%s): %w", name, err)
	}

	log.V(1).Info("successfully accessed GSM secret version", "resource", name)
	return resp.GetPayload().GetData(), nil
}

// mapKeysToSecretKeyMappings expands a multi-key mapping entry into individual keyed payloads.
// Each mapping.value is treated as a JSON Pointer (RFC 6901) into the secret payload.
func mapKeysToSecretKeyMappings(data []byte, mappings []secretspizecomv1alpha1.SecretKeyMapping) ([]keyedSecretPayload, error) {
	var payload interface{}
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, fmt.Errorf("decode secret payload as JSON: %w", err)
	}

	results := make([]keyedSecretPayload, 0, len(mappings))
	for _, mapping := range mappings {
		if strings.TrimSpace(mapping.Key) == "" {
			return nil, fmt.Errorf("mapping key cannot be empty")
		}
		if strings.TrimSpace(mapping.Value) == "" {
			return nil, fmt.Errorf("mapping value cannot be empty")
		}

		var targetKey string
		if strings.HasPrefix(mapping.Key, "/") {
			// Key is a JSON Pointer: resolve to a string and use that as the target key.
			resolvedKey, err := extractStringAtPointer(payload, mapping.Key)
			if err != nil {
				return nil, fmt.Errorf("resolve key pointer %q: %w", mapping.Key, err)
			}
			if !secretKeyRegex.MatchString(resolvedKey) {
				return nil, fmt.Errorf("resolved key %q does not match %q", resolvedKey, secretKeyRegex.String())
			}
			targetKey = resolvedKey
		} else {
			// Key is a literal string.
			targetKey = mapping.Key
		}

		value, err := jsonpointer.GetByPointer(payload, mapping.Value)
		if err != nil {
			return nil, fmt.Errorf("extract %q: %w", mapping.Value, err)
		}

		// Marshal back to bytes to align with the rest of the payload handling.
		encoded, err := json.Marshal(value)
		if err != nil {
			return nil, fmt.Errorf("marshal extracted value for key %q: %w", mapping.Key, err)
		}

		payload, err := newKeyedSecretPayload(targetKey, encoded)
		if err != nil {
			return nil, fmt.Errorf("validate key %q: %w", targetKey, err)
		}
		results = append(results, payload)
	}

	return results, nil
}

// extractStringAtPointer decodes the payload and resolves the given JSON Pointer.
// It returns the value if it exists and is a JSON string.
func extractStringAtPointer(payload interface{}, pointer string) (string, error) {
	val, err := jsonpointer.GetByPointer(payload, pointer)
	if err != nil {
		return "", err
	}

	s, ok := val.(string)
	if !ok {
		return "", fmt.Errorf("value at %q is not a string", pointer)
	}

	return s, nil
}
