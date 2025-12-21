package v1alpha1

import (
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

// Service account overrides should now live in annotations, not spec.
func TestGSMSecretSpecOmitsServiceAccountFields(t *testing.T) {
	specSchema := loadSpecSchema(t)

	if _, ok := specSchema.Properties["KSA"]; ok {
		t.Fatalf("unexpected KSA property in spec; service account overrides should move to annotations")
	}
	if _, ok := specSchema.Properties["GSA"]; ok {
		t.Fatalf("unexpected GSA property in spec; service account overrides should move to annotations")
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

// WIF audience should be provided via annotation, not spec.
func TestGSMSecretSpecOmitsWIFAudience(t *testing.T) {
	specSchema := loadSpecSchema(t)

	if _, ok := specSchema.Properties["wifAudience"]; ok {
		t.Fatalf("unexpected wifAudience property in spec; should be provided via annotation")
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

// gsmSecrets entries require key, projectId, secretId with minLength=1.
func TestGSMSecretEntryRequiredCoreFields(t *testing.T) {
	specSchema := loadSpecSchema(t)

	prop, ok := specSchema.Properties["gsmSecrets"]
	if !ok {
		t.Fatalf("gsmSecrets property missing from schema")
	}

	entry := prop.Items.Schema

	fields := []struct {
		name string
	}{
		{name: "key"},
		{name: "projectId"},
		{name: "secretId"},
	}

	required := requiredFields(entry.Required)
	for _, f := range fields {
		f := f
		t.Run(f.name, func(t *testing.T) {
			p, ok := entry.Properties[f.name]
			if !ok {
				t.Fatalf("%s property missing from gsmSecrets entry schema", f.name)
			}
			if p.MinLength == nil || *p.MinLength != 1 {
				t.Fatalf("%s minLength = %v, want 1", f.name, p.MinLength)
			}
			if _, ok := required[f.name]; !ok {
				t.Fatalf("%s is not marked as required; required fields: %v", f.name, entry.Required)
			}
		})
	}
}

// gsmSecrets entry key must match allowed pattern.
func TestGSMSecretEntryKeyPattern(t *testing.T) {
	specSchema := loadSpecSchema(t)

	prop, ok := specSchema.Properties["gsmSecrets"]
	if !ok {
		t.Fatalf("gsmSecrets property missing from schema")
	}
	entry := prop.Items.Schema

	keyProp, ok := entry.Properties["key"]
	if !ok {
		t.Fatalf("key property missing from gsmSecrets entry schema")
	}

	const expectedPattern = "^[A-Za-z0-9._-]+$"
	if keyProp.Pattern != expectedPattern {
		t.Fatalf("key pattern = %q, want %q", keyProp.Pattern, expectedPattern)
	}
}

// gsmSecrets entry projectId must match allowed pattern.
func TestGSMSecretEntryProjectIDPattern(t *testing.T) {
	specSchema := loadSpecSchema(t)

	prop, ok := specSchema.Properties["gsmSecrets"]
	if !ok {
		t.Fatalf("gsmSecrets property missing from schema")
	}
	entry := prop.Items.Schema

	projectProp, ok := entry.Properties["projectId"]
	if !ok {
		t.Fatalf("projectId property missing from gsmSecrets entry schema")
	}

	const expectedPattern = "^[a-z][a-z0-9-]{4,28}[a-z0-9]$"
	if projectProp.Pattern != expectedPattern {
		t.Fatalf("projectId pattern = %q, want %q", projectProp.Pattern, expectedPattern)
	}
}

// gsmSecrets entry secretId must match allowed pattern.
func TestGSMSecretEntrySecretIDPattern(t *testing.T) {
	specSchema := loadSpecSchema(t)

	prop, ok := specSchema.Properties["gsmSecrets"]
	if !ok {
		t.Fatalf("gsmSecrets property missing from schema")
	}
	entry := prop.Items.Schema

	secretProp, ok := entry.Properties["secretId"]
	if !ok {
		t.Fatalf("secretId property missing from gsmSecrets entry schema")
	}

	const expectedPattern = "^[A-Za-z][A-Za-z0-9_-]{0,253}[A-Za-z0-9]$"
	if secretProp.Pattern != expectedPattern {
		t.Fatalf("secretId pattern = %q, want %q", secretProp.Pattern, expectedPattern)
	}
}

// gsmSecrets entry version must match allowed pattern.
func TestGSMSecretEntryVersionPattern(t *testing.T) {
	specSchema := loadSpecSchema(t)

	prop, ok := specSchema.Properties["gsmSecrets"]
	if !ok {
		t.Fatalf("gsmSecrets property missing from schema")
	}
	entry := prop.Items.Schema

	versionProp, ok := entry.Properties["version"]
	if !ok {
		t.Fatalf("version property missing from gsmSecrets entry schema")
	}

	const expectedPattern = "^(latest|[1-9][0-9]*)$"
	if versionProp.Pattern != expectedPattern {
		t.Fatalf("version pattern = %q, want %q", versionProp.Pattern, expectedPattern)
	}
}

// targetSecret.name must match DNS-like pattern.
func TestTargetSecretNamePattern(t *testing.T) {
	specSchema := loadSpecSchema(t)

	target, ok := specSchema.Properties["targetSecret"]
	if !ok {
		t.Fatalf("targetSecret property missing from schema")
	}

	nameProp, ok := target.Properties["name"]
	if !ok {
		t.Fatalf("name property missing from targetSecret schema")
	}

	const expectedPattern = "^[a-z0-9]([-a-z0-9]*[a-z0-9])?$"
	if nameProp.Pattern != expectedPattern {
		t.Fatalf("targetSecret.name pattern = %q, want %q", nameProp.Pattern, expectedPattern)
	}
}

// targetSecret.name should require minLength=1.
func TestTargetSecretNameMinLength(t *testing.T) {
	specSchema := loadSpecSchema(t)

	target, ok := specSchema.Properties["targetSecret"]
	if !ok {
		t.Fatalf("targetSecret property missing from schema")
	}

	nameProp, ok := target.Properties["name"]
	if !ok {
		t.Fatalf("name property missing from targetSecret schema")
	}

	if nameProp.MinLength == nil || *nameProp.MinLength != 1 {
		t.Fatalf("targetSecret.name minLength = %v, want 1", nameProp.MinLength)
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
