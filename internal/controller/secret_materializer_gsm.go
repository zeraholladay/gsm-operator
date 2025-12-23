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

	secretmanager "cloud.google.com/go/secretmanager/apiv1"

	"cloud.google.com/go/secretmanager/apiv1/secretmanagerpb"
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
		log.V(1).Info("fetching GSM secret payload",
			"key", e.Key,
			"projectID", e.ProjectID,
			"secretID", e.SecretID,
			"version", e.Version,
		)

		name := fmt.Sprintf("projects/%s/secrets/%s/versions/%s", e.ProjectID, e.SecretID, e.Version)

		data, err := accessSecretPayload(ctx, client, name)
		if err != nil {
			log.Error(err, "failed to fetch GSM secret payload",
				"key", e.Key,
				"projectID", e.ProjectID,
				"secretID", e.SecretID,
				"version", e.Version,
			)
			return nil, fmt.Errorf("fetch payload for key %q (project=%q, secret=%q, version=%q): %w",
				e.Key, e.ProjectID, e.SecretID, e.Version, err)
		}

		results = append(results, keyedSecretPayload{
			Key:   e.Key,
			Value: data,
		})
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
