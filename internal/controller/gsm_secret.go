package controller

import (
	"context"
	"fmt"

	secretmanager "cloud.google.com/go/secretmanager/apiv1"
	"cloud.google.com/go/secretmanager/apiv1/secretmanagerpb"

	secretswayfaircomv1alpha1 "github.com/wayfair-shared/gsm-operator/api/v1alpha1"
)

// AccessSecretPayload reads a Secret Manager secret version and returns the raw payload bytes.
// Pass version as "latest" or a numeric string like "1".
func AccessSecretPayload(
	ctx context.Context,
	client *secretmanager.Client,
	projectID, secretID, version string,
) ([]byte, error) {
	if version == "" {
		version = "latest"
	}

	name := fmt.Sprintf("projects/%s/secrets/%s/versions/%s", projectID, secretID, version)
	resp, err := client.AccessSecretVersion(ctx, &secretmanagerpb.AccessSecretVersionRequest{
		Name: name,
	})
	if err != nil {
		return nil, fmt.Errorf("AccessSecretVersion(%s): %w", name, err)
	}

	return resp.GetPayload().GetData(), nil
}

// KeyedSecretPayload holds a Kubernetes Secret data key and its corresponding GSM payload.
type KeyedSecretPayload struct {
	Key   string
	Value []byte
}

// FetchGSMSecretPayloads creates a Secret Manager client and fetches payloads
// for each GSMSecretEntry, returning the data keyed by the target Secret key.
func FetchGSMSecretPayloads(
	ctx context.Context,
	entries []secretswayfaircomv1alpha1.GSMSecretEntry,
) ([]KeyedSecretPayload, error) {
	if len(entries) == 0 {
		return nil, nil
	}

	client, err := secretmanager.NewClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("secretmanager.NewClient: %w", err)
	}
	defer client.Close()

	results := make([]KeyedSecretPayload, 0, len(entries))

	for _, e := range entries {
		data, err := AccessSecretPayload(ctx, client, e.ProjectID, e.SecretID, e.Version)
		if err != nil {
			return nil, fmt.Errorf("fetch payload for key %q (project=%q, secret=%q, version=%q): %w",
				e.Key, e.ProjectID, e.SecretID, e.Version, err)
		}

		results = append(results, KeyedSecretPayload{
			Key:   e.Key,
			Value: data,
		})
	}

	return results, nil
}
