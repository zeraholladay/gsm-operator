package v1alpha1

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"sigs.k8s.io/yaml"
)

// Ensure the CRD schema keeps spec.targetSecret required as marked in gsmsecret_types.go.
func TestGSMSecretSpecTargetSecretIsRequired(t *testing.T) {
	specSchema := loadSpecSchema(t)
	required := requiredFields(specSchema.Required)

	if _, ok := required["targetSecret"]; !ok {
		t.Fatalf("spec.targetSecret is not marked as required; required fields: %v", specSchema.Required)
	}
}

// KSA default and validation should match the kubebuilder markers.
func TestGSMSecretSpecKSADefaultAndMinLength(t *testing.T) {
	specSchema := loadSpecSchema(t)

	prop, ok := specSchema.Properties["KSA"]
	if !ok {
		t.Fatalf("KSA property missing from schema")
	}

	if prop.MinLength == nil || *prop.MinLength != 1 {
		t.Fatalf("KSA minLength = %v, want 1", prop.MinLength)
	}

	if prop.Default == nil {
		t.Fatalf("KSA missing default value")
	}

	var defaultVal string
	if err := json.Unmarshal(prop.Default.Raw, &defaultVal); err != nil {
		t.Fatalf("failed to unmarshal KSA default: %v", err)
	}
	if defaultVal != "gsm-reader" {
		t.Fatalf("KSA default = %q, want %q", defaultVal, "gsm-reader")
	}
}

// GSA default and validation should match the kubebuilder markers.
func TestGSMSecretSpecGSADefaultAndMinLength(t *testing.T) {
	specSchema := loadSpecSchema(t)

	prop, ok := specSchema.Properties["GSA"]
	if !ok {
		t.Fatalf("GSA property missing from schema")
	}

	if prop.MinLength == nil || *prop.MinLength != 1 {
		t.Fatalf("GSA minLength = %v, want 1", prop.MinLength)
	}

	if prop.Default == nil {
		t.Fatalf("GSA missing default value")
	}

	var defaultVal string
	if err := json.Unmarshal(prop.Default.Raw, &defaultVal); err != nil {
		t.Fatalf("failed to unmarshal GSA default: %v", err)
	}
	if defaultVal != "gsm-reader" {
		t.Fatalf("GSA default = %q, want %q", defaultVal, "gsm-reader")
	}
}

// gsmSecrets should be required with at least one entry.
func TestGSMSecretSpecSecretsMinItemsAndRequired(t *testing.T) {
	specSchema := loadSpecSchema(t)

	prop, ok := specSchema.Properties["gsmSecrets"]
	if !ok {
		t.Fatalf("gsmSecrets property missing from schema")
	}

	if prop.MinItems == nil || *prop.MinItems != 1 {
		t.Fatalf("gsmSecrets minItems = %v, want 1", prop.MinItems)
	}

	required := requiredFields(specSchema.Required)
	if _, ok := required["gsmSecrets"]; !ok {
		t.Fatalf("gsmSecrets is not marked as required; required fields: %v", specSchema.Required)
	}
}

// wifAudience is optional but present as a string field.
func TestGSMSecretSpecWIFAudienceIsOptional(t *testing.T) {
	specSchema := loadSpecSchema(t)

	prop, ok := specSchema.Properties["wifAudience"]
	if !ok {
		t.Fatalf("wifAudience property missing from schema")
	}
	if prop.Type != "string" {
		t.Fatalf("wifAudience type = %q, want %q", prop.Type, "string")
	}

	required := requiredFields(specSchema.Required)
	if _, ok := required["wifAudience"]; ok {
		t.Fatalf("wifAudience unexpectedly marked as required")
	}
}

// gsmSecrets entries must require version, have minLength, and no default.
func TestGSMSecretEntryVersionRequiredNoDefault(t *testing.T) {
	specSchema := loadSpecSchema(t)

	prop, ok := specSchema.Properties["gsmSecrets"]
	if !ok {
		t.Fatalf("gsmSecrets property missing from schema")
	}

	if prop.Items == nil || prop.Items.Schema == nil {
		t.Fatalf("gsmSecrets items schema missing")
	}

	entry := prop.Items.Schema

	versionProp, ok := entry.Properties["version"]
	if !ok {
		t.Fatalf("version property missing from gsmSecrets entry schema")
	}

	if versionProp.MinLength == nil || *versionProp.MinLength != 1 {
		t.Fatalf("version minLength = %v, want 1", versionProp.MinLength)
	}

	if versionProp.Default != nil {
		t.Fatalf("version default present; expected none")
	}

	required := requiredFields(entry.Required)
	if _, ok := required["version"]; !ok {
		t.Fatalf("version is not marked as required; required fields: %v", entry.Required)
	}
}

func loadSpecSchema(t *testing.T) *apiextensionsv1.JSONSchemaProps {
	t.Helper()

	crdPath := filepath.Join("..", "..", "config", "crd", "bases", "secrets.pize.com_gsmsecrets.yaml")

	rawCRD, err := os.ReadFile(crdPath)
	if err != nil {
		t.Fatalf("failed to read CRD file %q: %v", crdPath, err)
	}

	var crd apiextensionsv1.CustomResourceDefinition
	if err := yaml.Unmarshal(rawCRD, &crd); err != nil {
		t.Fatalf("failed to unmarshal CRD yaml: %v", err)
	}

	var version *apiextensionsv1.CustomResourceDefinitionVersion
	for i := range crd.Spec.Versions {
		if crd.Spec.Versions[i].Name == "v1alpha1" {
			version = &crd.Spec.Versions[i]
			break
		}
	}
	if version == nil {
		t.Fatalf("v1alpha1 version not found in CRD")
	}

	if version.Schema == nil || version.Schema.OpenAPIV3Schema == nil {
		t.Fatalf("v1alpha1 version missing schema")
	}

	specSchema, ok := version.Schema.OpenAPIV3Schema.Properties["spec"]
	if !ok {
		t.Fatalf("spec property missing from schema")
	}

	return &specSchema
}

func requiredFields(required []string) map[string]struct{} {
	result := make(map[string]struct{}, len(required))
	for _, field := range required {
		result[field] = struct{}{}
	}
	return result
}
