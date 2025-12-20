package controller

import (
	"context"
	"fmt"
	"time"

	secretmanager "cloud.google.com/go/secretmanager/apiv1"
	"cloud.google.com/go/secretmanager/apiv1/secretmanagerpb"

	secretspizecomv1alpha1 "github.com/zeraholladay/gsm-operator/api/v1alpha1"
	"google.golang.org/api/option"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

// AccessSecretPayload reads a Secret Manager secret version and returns the raw payload bytes.
// Pass version as "latest" or a numeric string like "1".
func AccessSecretPayload(
	ctx context.Context,
	client *secretmanager.Client,
	projectID, secretID, version string,
) ([]byte, error) {
	log := logf.FromContext(ctx).WithValues(
		"projectID", projectID,
		"secretID", secretID,
		"version", version,
	)

	if version == "" {
		version = "latest"
	}

	name := fmt.Sprintf("projects/%s/secrets/%s/versions/%s", projectID, secretID, version)
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

// KeyedSecretPayload holds a Kubernetes Secret data key and its corresponding GSM payload.
type KeyedSecretPayload struct {
	Key   string
	Value []byte
}

// FetchGSMSecretPayloads creates a Secret Manager client and fetches payloads
// for each GSMSecretEntry, returning the data keyed by the target Secret key.
// The call flow is:
//  1. Request a short-lived Kubernetes ServiceAccount token for the tenant KSA.
//  2. Exchange that token via Google's STS using the configured WIF audience.
//  3. Build a Secret Manager client with the resulting Google credentials.
//  4. Read each GSM secret version and map it into the target Secret's data keys.
func FetchGSMSecretPayloads(
	ctx context.Context,
	gsm secretspizecomv1alpha1.GSMSecret,
) ([]KeyedSecretPayload, error) {
	log := logf.FromContext(ctx).WithValues(
		"gsmsecret", gsm.Name,
		"namespace", gsm.Namespace,
	)

	// Nothing to do if the spec has no gsmSecrets entries.
	entries := gsm.Spec.Secrets
	if len(entries) == 0 {
		log.V(1).Info("GSMSecret has no entries; nothing to fetch")
		return nil, nil
	}

	log.Info("fetching GSM secret payloads",
		"entryCount", len(entries),
		"wifAudience", gsm.Spec.WIFAudience,
	)

	// Parameters describing the tenant ServiceAccount identity we want to assume.
	tokenRequestParams := KSATokenRequestParams{
		Namespace: gsm.Namespace,
		KSAName:   "gsm-reader",
		// Important: The audience of the KSA token must match the Workload
		// Identity Provider's expected audience (the same string used for
		// spec.wifAudience) so that STS accepts the token.
		Audience:   gsm.Spec.WIFAudience,
		Expiration: 10 * time.Minute,
		Timeout:    10 * time.Second,
	}

	// STEP 1: Request a short-lived JWT for the tenant KSA.
	log.Info("requesting Kubernetes ServiceAccount token for GSM payload fetch")
	token, err := RequestKSAToken(ctx, tokenRequestParams)
	if err != nil {
		log.Error(err, "failed to request Kubernetes ServiceAccount token")
		return nil, fmt.Errorf("request KSA token: %w", err)
	}

	// STEP 2: Exchange the KSA token for Google credentials via Workload Identity.
	// The WIF audience is configured per GSMSecret (spec.wifAudience).
	log.Info("exchanging Kubernetes ServiceAccount token via Workload Identity Federation")
	creds, err := GCPCredsFromK8sToken(ctx, token, gsm.Spec.WIFAudience, "")
	if err != nil {
		log.Error(err, "failed to exchange KSA token for Google credentials")
		return nil, fmt.Errorf("exchange KSA token for Google credentials: %w", err)
	}

	// STEP 3: Build a Secret Manager client bound to the tenant identity.
	log.Info("creating Google Secret Manager client with federated credentials")
	client, err := secretmanager.NewClient(ctx, option.WithCredentials(creds))
	if err != nil {
		log.Error(err, "failed to create Secret Manager client")
		return nil, fmt.Errorf("secretmanager.NewClient: %w", err)
	}
	defer client.Close()

	results := make([]KeyedSecretPayload, 0, len(entries))

	for _, e := range entries {
		// STEP 4: Read the requested GSM secret version and attach it under the
		// configured key in the target Kubernetes Secret.
		log.V(1).Info("fetching GSM secret payload",
			"key", e.Key,
			"projectID", e.ProjectID,
			"secretID", e.SecretID,
			"version", e.Version,
		)

		data, err := AccessSecretPayload(ctx, client, e.ProjectID, e.SecretID, e.Version)
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

		results = append(results, KeyedSecretPayload{
			Key:   e.Key,
			Value: data,
		})
	}

	return results, nil
}
