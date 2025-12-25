package v1alpha1

import (
	"os"
	"path/filepath"
	"testing"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/yaml"
)

const (
	testVersionV1alpha1 = "v1alpha1"
	testModifiedValue   = "MODIFIED"
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

// gsmSecrets entries require projectId, secretId with minLength=1.
// Note: key is now optional (mutually exclusive with keys via XOR validation).
func TestGSMSecretEntryRequiredCoreFields(t *testing.T) {
	specSchema := loadSpecSchema(t)

	prop, ok := specSchema.Properties["gsmSecrets"]
	if !ok {
		t.Fatalf("gsmSecrets property missing from schema")
	}

	entry := prop.Items.Schema

	// These fields are always required
	requiredFieldsList := []string{"projectId", "secretId"}

	required := requiredFields(entry.Required)
	for _, name := range requiredFieldsList {
		t.Run(name, func(t *testing.T) {
			p, ok := entry.Properties[name]
			if !ok {
				t.Fatalf("%s property missing from gsmSecrets entry schema", name)
			}
			if p.MinLength == nil || *p.MinLength != 1 {
				t.Fatalf("%s minLength = %v, want 1", name, p.MinLength)
			}
			if _, ok := required[name]; !ok {
				t.Fatalf("%s is not marked as required; required fields: %v", name, entry.Required)
			}
		})
	}
}

// gsmSecrets entry key is optional (mutually exclusive with keys).
func TestGSMSecretEntryKeyIsOptional(t *testing.T) {
	specSchema := loadSpecSchema(t)

	prop, ok := specSchema.Properties["gsmSecrets"]
	if !ok {
		t.Fatalf("gsmSecrets property missing from schema")
	}

	entry := prop.Items.Schema
	required := requiredFields(entry.Required)

	// key should NOT be in required list (it's optional due to XOR with keys)
	if _, ok := required["key"]; ok {
		t.Fatal("key should be optional (not in required list) due to XOR validation with keys")
	}

	// But key property should exist
	if _, ok := entry.Properties["key"]; !ok {
		t.Fatal("key property missing from gsmSecrets entry schema")
	}
}

// gsmSecrets entry keys is optional (mutually exclusive with key).
func TestGSMSecretEntryKeysIsOptional(t *testing.T) {
	specSchema := loadSpecSchema(t)

	prop, ok := specSchema.Properties["gsmSecrets"]
	if !ok {
		t.Fatalf("gsmSecrets property missing from schema")
	}

	entry := prop.Items.Schema
	required := requiredFields(entry.Required)

	// keys should NOT be in required list (it's optional due to XOR with key)
	if _, ok := required["keys"]; ok {
		t.Fatal("keys should be optional (not in required list) due to XOR validation with key")
	}

	// But keys property should exist
	keysProp, ok := entry.Properties["keys"]
	if !ok {
		t.Fatal("keys property missing from gsmSecrets entry schema")
	}

	// keys should be an array
	if keysProp.Type != "array" {
		t.Fatalf("keys type = %q, want 'array'", keysProp.Type)
	}
}

// gsmSecrets entry key must match allowed pattern (simple key name only).
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

// ==================== SecretKeyMapping Tests ====================

// SecretKeyMapping.key must match pattern allowing simple keys or JSON Pointer paths.
func TestSecretKeyMappingKeyPattern(t *testing.T) {
	specSchema := loadSpecSchema(t)

	prop, ok := specSchema.Properties["gsmSecrets"]
	if !ok {
		t.Fatalf("gsmSecrets property missing from schema")
	}

	entry := prop.Items.Schema
	keysProp, ok := entry.Properties["keys"]
	if !ok {
		t.Fatalf("keys property missing from gsmSecrets entry schema")
	}

	if keysProp.Items == nil || keysProp.Items.Schema == nil {
		t.Fatal("keys items schema missing")
	}

	mapping := keysProp.Items.Schema
	keyProp, ok := mapping.Properties["key"]
	if !ok {
		t.Fatal("key property missing from SecretKeyMapping schema")
	}

	// Pattern should allow simple keys OR JSON Pointer paths
	const expectedPattern = "^([A-Za-z0-9._-]+|(/[^/]*)+)$"
	if keyProp.Pattern != expectedPattern {
		t.Fatalf("SecretKeyMapping.key pattern = %q, want %q", keyProp.Pattern, expectedPattern)
	}

	if keyProp.MinLength == nil || *keyProp.MinLength != 1 {
		t.Fatalf("SecretKeyMapping.key minLength = %v, want 1", keyProp.MinLength)
	}
}

// SecretKeyMapping.value must be a JSON Pointer path (required).
func TestSecretKeyMappingValuePattern(t *testing.T) {
	specSchema := loadSpecSchema(t)

	prop, ok := specSchema.Properties["gsmSecrets"]
	if !ok {
		t.Fatalf("gsmSecrets property missing from schema")
	}

	entry := prop.Items.Schema
	keysProp, ok := entry.Properties["keys"]
	if !ok {
		t.Fatalf("keys property missing from gsmSecrets entry schema")
	}

	if keysProp.Items == nil || keysProp.Items.Schema == nil {
		t.Fatal("keys items schema missing")
	}

	mapping := keysProp.Items.Schema
	valueProp, ok := mapping.Properties["value"]
	if !ok {
		t.Fatal("value property missing from SecretKeyMapping schema")
	}

	// Pattern should require JSON Pointer format
	const expectedPattern = "^(/[^/]*)+$"
	if valueProp.Pattern != expectedPattern {
		t.Fatalf("SecretKeyMapping.value pattern = %q, want %q", valueProp.Pattern, expectedPattern)
	}

	if valueProp.MinLength == nil || *valueProp.MinLength != 1 {
		t.Fatalf("SecretKeyMapping.value minLength = %v, want 1", valueProp.MinLength)
	}
}

// SecretKeyMapping requires both key and value fields.
func TestSecretKeyMappingRequiredFields(t *testing.T) {
	specSchema := loadSpecSchema(t)

	prop, ok := specSchema.Properties["gsmSecrets"]
	if !ok {
		t.Fatalf("gsmSecrets property missing from schema")
	}

	entry := prop.Items.Schema
	keysProp, ok := entry.Properties["keys"]
	if !ok {
		t.Fatalf("keys property missing from gsmSecrets entry schema")
	}

	if keysProp.Items == nil || keysProp.Items.Schema == nil {
		t.Fatal("keys items schema missing")
	}

	mapping := keysProp.Items.Schema
	required := requiredFields(mapping.Required)

	if _, ok := required["key"]; !ok {
		t.Fatal("SecretKeyMapping.key should be required")
	}
	if _, ok := required["value"]; !ok {
		t.Fatal("SecretKeyMapping.value should be required")
	}
}

// GSMSecretEntry should have XOR validation for key/keys.
func TestGSMSecretEntryHasXORValidation(t *testing.T) {
	specSchema := loadSpecSchema(t)

	prop, ok := specSchema.Properties["gsmSecrets"]
	if !ok {
		t.Fatalf("gsmSecrets property missing from schema")
	}

	entry := prop.Items.Schema

	// Check for x-kubernetes-validations (CEL rules)
	if len(entry.XValidations) == 0 {
		t.Fatal("gsmSecrets entry should have XValidations for key/keys XOR")
	}

	// Look for the XOR rule
	foundXOR := false
	for _, v := range entry.XValidations {
		if v.Message == "exactly one of 'key' or 'keys' must be specified" {
			foundXOR = true
			break
		}
	}
	if !foundXOR {
		t.Fatal("XOR validation rule for key/keys not found")
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

	crdPath := filepath.Join("..", "..", "config", "crd", "bases", "secrets.gsm-operator.io_gsmsecrets.yaml")

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
		if crd.Spec.Versions[i].Name == testVersionV1alpha1 {
			version = &crd.Spec.Versions[i]
			break
		}
	}
	if version == nil {
		t.Fatalf("%s version not found in CRD", testVersionV1alpha1)
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

// ==================== Annotation Constants Tests ====================

func TestAnnotationConstants(t *testing.T) {
	tests := []struct {
		name     string
		constant string
		expected string
	}{
		{
			name:     "AnnotationKSA",
			constant: AnnotationKSA,
			expected: "secrets.gsm-operator.io/ksa",
		},
		{
			name:     "AnnotationGSA",
			constant: AnnotationGSA,
			expected: "secrets.gsm-operator.io/gsa",
		},
		{
			name:     "AnnotationWIFAudience",
			constant: AnnotationWIFAudience,
			expected: "secrets.gsm-operator.io/wif-audience",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.constant != tt.expected {
				t.Errorf("%s = %q, want %q", tt.name, tt.constant, tt.expected)
			}
		})
	}
}

func TestAnnotationConstantsHaveCorrectPrefix(t *testing.T) {
	const expectedPrefix = "secrets.gsm-operator.io/"

	annotations := []string{AnnotationKSA, AnnotationGSA, AnnotationWIFAudience}
	for _, ann := range annotations {
		if len(ann) < len(expectedPrefix) || ann[:len(expectedPrefix)] != expectedPrefix {
			t.Errorf("annotation %q does not have expected prefix %q", ann, expectedPrefix)
		}
	}
}

// ==================== Status Schema Tests ====================

func TestGSMSecretStatusSchema(t *testing.T) {
	statusSchema := loadStatusSchema(t)

	// observedGeneration should be present
	if _, ok := statusSchema.Properties["observedGeneration"]; !ok {
		t.Fatal("observedGeneration property missing from status schema")
	}

	// conditions should be present
	if _, ok := statusSchema.Properties["conditions"]; !ok {
		t.Fatal("conditions property missing from status schema")
	}
}

func TestGSMSecretStatusObservedGenerationFormat(t *testing.T) {
	statusSchema := loadStatusSchema(t)

	prop, ok := statusSchema.Properties["observedGeneration"]
	if !ok {
		t.Fatal("observedGeneration property missing from status schema")
	}

	if prop.Format != "int64" {
		t.Errorf("observedGeneration format = %q, want int64", prop.Format)
	}
}

func TestGSMSecretStatusConditionsIsListMap(t *testing.T) {
	statusSchema := loadStatusSchema(t)

	prop, ok := statusSchema.Properties["conditions"]
	if !ok {
		t.Fatal("conditions property missing from status schema")
	}

	// Check for x-kubernetes-list-type: map
	if prop.XListType == nil || *prop.XListType != "map" {
		t.Errorf("conditions x-kubernetes-list-type = %v, want 'map'", prop.XListType)
	}

	// Check for x-kubernetes-list-map-keys: [type]
	if len(prop.XListMapKeys) != 1 || prop.XListMapKeys[0] != "type" {
		t.Errorf("conditions x-kubernetes-list-map-keys = %v, want ['type']", prop.XListMapKeys)
	}
}

func TestGSMSecretStatusConditionsItems(t *testing.T) {
	statusSchema := loadStatusSchema(t)

	prop, ok := statusSchema.Properties["conditions"]
	if !ok {
		t.Fatal("conditions property missing from status schema")
	}

	if prop.Items == nil || prop.Items.Schema == nil {
		t.Fatal("conditions items schema missing")
	}

	conditionSchema := prop.Items.Schema

	// Verify required condition fields
	requiredConditionFields := []string{"type", "status", "lastTransitionTime", "reason", "message"}
	required := requiredFields(conditionSchema.Required)

	for _, field := range requiredConditionFields {
		if _, ok := required[field]; !ok {
			t.Errorf("condition field %q is not marked as required", field)
		}
	}
}

// ==================== Top-Level Schema Tests ====================

func TestGSMSecretSpecIsRequired(t *testing.T) {
	schema := loadRootSchema(t)

	required := requiredFields(schema.Required)
	if _, ok := required["spec"]; !ok {
		t.Fatalf("spec is not marked as required at root level; required: %v", schema.Required)
	}
}

func TestGSMSecretStatusIsOptional(t *testing.T) {
	schema := loadRootSchema(t)

	required := requiredFields(schema.Required)
	if _, ok := required["status"]; ok {
		t.Fatal("status should be optional (not in required list)")
	}

	// But status property should exist
	if _, ok := schema.Properties["status"]; !ok {
		t.Fatal("status property missing from root schema")
	}
}

func TestGSMSecretHasStatusSubresource(t *testing.T) {
	crd := loadCRD(t)

	var version *apiextensionsv1.CustomResourceDefinitionVersion
	for i := range crd.Spec.Versions {
		if crd.Spec.Versions[i].Name == testVersionV1alpha1 {
			version = &crd.Spec.Versions[i]
			break
		}
	}
	if version == nil {
		t.Fatalf("%s version not found", testVersionV1alpha1)
	}

	if version.Subresources == nil || version.Subresources.Status == nil {
		t.Fatal("status subresource is not enabled for v1alpha1")
	}
}

func TestGSMSecretIsNamespaced(t *testing.T) {
	crd := loadCRD(t)

	if crd.Spec.Scope != apiextensionsv1.NamespaceScoped {
		t.Errorf("CRD scope = %q, want Namespaced", crd.Spec.Scope)
	}
}

func TestGSMSecretGroupAndNames(t *testing.T) {
	crd := loadCRD(t)

	if crd.Spec.Group != "secrets.gsm-operator.io" {
		t.Errorf("group = %q, want 'secrets.gsm-operator.io'", crd.Spec.Group)
	}

	if crd.Spec.Names.Kind != "GSMSecret" {
		t.Errorf("kind = %q, want 'GSMSecret'", crd.Spec.Names.Kind)
	}

	if crd.Spec.Names.ListKind != "GSMSecretList" {
		t.Errorf("listKind = %q, want 'GSMSecretList'", crd.Spec.Names.ListKind)
	}

	if crd.Spec.Names.Plural != "gsmsecrets" {
		t.Errorf("plural = %q, want 'gsmsecrets'", crd.Spec.Names.Plural)
	}

	if crd.Spec.Names.Singular != "gsmsecret" {
		t.Errorf("singular = %q, want 'gsmsecret'", crd.Spec.Names.Singular)
	}
}

// ==================== Scheme Registration Tests ====================

func TestGSMSecretSchemeRegistration(t *testing.T) {
	scheme := runtime.NewScheme()
	err := AddToScheme(scheme)
	if err != nil {
		t.Fatalf("failed to add to scheme: %v", err)
	}

	// Check GSMSecret is registered
	gvk := GroupVersion.WithKind("GSMSecret")
	if !scheme.Recognizes(gvk) {
		t.Errorf("scheme does not recognize %v", gvk)
	}

	// Check GSMSecretList is registered
	gvkList := GroupVersion.WithKind("GSMSecretList")
	if !scheme.Recognizes(gvkList) {
		t.Errorf("scheme does not recognize %v", gvkList)
	}
}

func TestGroupVersionString(t *testing.T) {
	expected := "secrets.gsm-operator.io/v1alpha1"
	if GroupVersion.String() != expected {
		t.Errorf("GroupVersion.String() = %q, want %q", GroupVersion.String(), expected)
	}
}

// ==================== Deep Copy Tests ====================

func TestGSMSecretDeepCopy(t *testing.T) {
	original := &GSMSecret{
		TypeMeta: metav1.TypeMeta{
			Kind:       "GSMSecret",
			APIVersion: "secrets.gsm-operator.io/v1alpha1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-gsmsecret",
			Namespace: "test-namespace",
			Annotations: map[string]string{
				AnnotationKSA:         "test-ksa",
				AnnotationWIFAudience: "test-audience",
			},
		},
		Spec: GSMSecretSpec{
			TargetSecret: GSMSecretTargetSecret{Name: "target-secret"},
			Secrets: []GSMSecretEntry{
				{Key: "KEY1", ProjectID: "project-1", SecretID: "secret-1", Version: "latest"},
				{Key: "KEY2", ProjectID: "project-2", SecretID: "secret-2", Version: "1"},
			},
		},
		Status: GSMSecretStatus{
			ObservedGeneration: 5,
			Conditions: []metav1.Condition{
				{
					Type:   "Ready",
					Status: metav1.ConditionTrue,
					Reason: "Synced",
				},
			},
		},
	}

	copied := original.DeepCopy()

	// Verify it's a different pointer
	if copied == original {
		t.Fatal("DeepCopy returned same pointer")
	}

	// Verify values are equal
	if copied.Name != original.Name {
		t.Errorf("copied name = %q, want %q", copied.Name, original.Name)
	}
	if copied.Namespace != original.Namespace {
		t.Errorf("copied namespace = %q, want %q", copied.Namespace, original.Namespace)
	}

	// Verify spec is copied
	if len(copied.Spec.Secrets) != len(original.Spec.Secrets) {
		t.Errorf("copied spec.secrets length = %d, want %d", len(copied.Spec.Secrets), len(original.Spec.Secrets))
	}

	// Modify copy and verify original is unchanged
	copied.Spec.Secrets[0].Key = testModifiedValue
	if original.Spec.Secrets[0].Key == testModifiedValue {
		t.Error("modifying copy affected original - not a deep copy")
	}

	// Verify status is copied
	copied.Status.ObservedGeneration = 999
	if original.Status.ObservedGeneration == 999 {
		t.Error("modifying copied status affected original")
	}
}

func TestGSMSecretDeepCopyInto(t *testing.T) {
	original := &GSMSecret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "original",
			Namespace: "ns",
		},
		Spec: GSMSecretSpec{
			TargetSecret: GSMSecretTargetSecret{Name: "target"},
			Secrets: []GSMSecretEntry{
				{Key: "K", ProjectID: "p", SecretID: "s", Version: "1"},
			},
		},
	}

	target := &GSMSecret{}
	original.DeepCopyInto(target)

	if target.Name != original.Name {
		t.Errorf("target name = %q, want %q", target.Name, original.Name)
	}
	if len(target.Spec.Secrets) != len(original.Spec.Secrets) {
		t.Errorf("target secrets length = %d, want %d", len(target.Spec.Secrets), len(original.Spec.Secrets))
	}
}

func TestGSMSecretDeepCopyObject(t *testing.T) {
	original := &GSMSecret{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test",
		},
	}

	obj := original.DeepCopyObject()
	copied, ok := obj.(*GSMSecret)
	if !ok {
		t.Fatal("DeepCopyObject did not return *GSMSecret")
	}

	if copied == original {
		t.Fatal("DeepCopyObject returned same pointer")
	}
	if copied.Name != original.Name {
		t.Errorf("copied name = %q, want %q", copied.Name, original.Name)
	}
}

func TestGSMSecretListDeepCopy(t *testing.T) {
	original := &GSMSecretList{
		Items: []GSMSecret{
			{
				ObjectMeta: metav1.ObjectMeta{Name: "item1"},
				Spec: GSMSecretSpec{
					TargetSecret: GSMSecretTargetSecret{Name: "target1"},
					Secrets:      []GSMSecretEntry{{Key: "K1", ProjectID: "p", SecretID: "s", Version: "1"}},
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{Name: "item2"},
				Spec: GSMSecretSpec{
					TargetSecret: GSMSecretTargetSecret{Name: "target2"},
					Secrets:      []GSMSecretEntry{{Key: "K2", ProjectID: "p", SecretID: "s", Version: "2"}},
				},
			},
		},
	}

	copied := original.DeepCopy()

	if copied == original {
		t.Fatal("DeepCopy returned same pointer")
	}
	if len(copied.Items) != len(original.Items) {
		t.Errorf("copied items length = %d, want %d", len(copied.Items), len(original.Items))
	}

	// Modify copy and verify original unchanged
	copied.Items[0].Name = testModifiedValue
	if original.Items[0].Name == testModifiedValue {
		t.Error("modifying copy affected original")
	}
}

func TestGSMSecretSpecDeepCopy(t *testing.T) {
	original := GSMSecretSpec{
		TargetSecret: GSMSecretTargetSecret{Name: "target"},
		Secrets: []GSMSecretEntry{
			{Key: "K1", ProjectID: "p1", SecretID: "s1", Version: "1"},
			{Key: "K2", ProjectID: "p2", SecretID: "s2", Version: "latest"},
		},
	}

	copied := original.DeepCopy()

	if copied == &original {
		t.Fatal("DeepCopy returned same pointer")
	}
	if len(copied.Secrets) != len(original.Secrets) {
		t.Errorf("copied secrets length = %d, want %d", len(copied.Secrets), len(original.Secrets))
	}

	// Modify copy
	copied.Secrets[0].Key = testModifiedValue
	if original.Secrets[0].Key == testModifiedValue {
		t.Error("modifying copy affected original")
	}
}

func TestGSMSecretStatusDeepCopy(t *testing.T) {
	original := GSMSecretStatus{
		ObservedGeneration: 10,
		Conditions: []metav1.Condition{
			{Type: "Ready", Status: metav1.ConditionTrue, Reason: "Synced", Message: "OK"},
			{Type: "Degraded", Status: metav1.ConditionFalse, Reason: "OK", Message: ""},
		},
	}

	copied := original.DeepCopy()

	if copied.ObservedGeneration != original.ObservedGeneration {
		t.Errorf("copied ObservedGeneration = %d, want %d", copied.ObservedGeneration, original.ObservedGeneration)
	}
	if len(copied.Conditions) != len(original.Conditions) {
		t.Errorf("copied conditions length = %d, want %d", len(copied.Conditions), len(original.Conditions))
	}

	// Modify copy
	copied.Conditions[0].Message = testModifiedValue
	if original.Conditions[0].Message == testModifiedValue {
		t.Error("modifying copy affected original")
	}
}

func TestGSMSecretEntryDeepCopy(t *testing.T) {
	original := GSMSecretEntry{
		Key:       "MY_KEY",
		ProjectID: "my-project",
		SecretID:  "my-secret",
		Version:   "latest",
	}

	copied := original.DeepCopy()

	if copied.Key != original.Key {
		t.Errorf("copied Key = %q, want %q", copied.Key, original.Key)
	}
	if copied.ProjectID != original.ProjectID {
		t.Errorf("copied ProjectID = %q, want %q", copied.ProjectID, original.ProjectID)
	}
	if copied.SecretID != original.SecretID {
		t.Errorf("copied SecretID = %q, want %q", copied.SecretID, original.SecretID)
	}
	if copied.Version != original.Version {
		t.Errorf("copied Version = %q, want %q", copied.Version, original.Version)
	}
}

func TestGSMSecretEntryWithKeysDeepCopy(t *testing.T) {
	original := GSMSecretEntry{
		Keys: []SecretKeyMapping{
			{Key: "DB_HOST", Value: "/host"},
			{Key: "DB_PORT", Value: "/port"},
		},
		ProjectID: "my-project",
		SecretID:  "db-config",
		Version:   "1",
	}

	copied := original.DeepCopy()

	if len(copied.Keys) != len(original.Keys) {
		t.Fatalf("copied Keys length = %d, want %d", len(copied.Keys), len(original.Keys))
	}

	// Verify values are equal
	for i, k := range original.Keys {
		if copied.Keys[i].Key != k.Key {
			t.Errorf("copied Keys[%d].Key = %q, want %q", i, copied.Keys[i].Key, k.Key)
		}
		if copied.Keys[i].Value != k.Value {
			t.Errorf("copied Keys[%d].Value = %q, want %q", i, copied.Keys[i].Value, k.Value)
		}
	}

	// Modify copy and verify original unchanged
	copied.Keys[0].Key = testModifiedValue
	if original.Keys[0].Key == testModifiedValue {
		t.Error("modifying copy affected original - not a deep copy")
	}
}

func TestSecretKeyMappingDeepCopy(t *testing.T) {
	original := SecretKeyMapping{
		Key:   "MY_KEY",
		Value: "/data/password",
	}

	copied := original.DeepCopy()

	if copied.Key != original.Key {
		t.Errorf("copied Key = %q, want %q", copied.Key, original.Key)
	}
	if copied.Value != original.Value {
		t.Errorf("copied Value = %q, want %q", copied.Value, original.Value)
	}
}

func TestNilSecretKeyMappingDeepCopy(t *testing.T) {
	var nilMapping *SecretKeyMapping
	if nilMapping.DeepCopy() != nil {
		t.Error("DeepCopy of nil SecretKeyMapping should return nil")
	}
}

func TestGSMSecretTargetSecretDeepCopy(t *testing.T) {
	original := GSMSecretTargetSecret{Name: "my-target"}
	copied := original.DeepCopy()

	if copied.Name != original.Name {
		t.Errorf("copied Name = %q, want %q", copied.Name, original.Name)
	}
}

func TestGSMSecretListDeepCopyObject(t *testing.T) {
	original := &GSMSecretList{
		Items: []GSMSecret{
			{ObjectMeta: metav1.ObjectMeta{Name: "item1"}},
		},
	}

	obj := original.DeepCopyObject()
	copied, ok := obj.(*GSMSecretList)
	if !ok {
		t.Fatal("DeepCopyObject did not return *GSMSecretList")
	}

	if copied == original {
		t.Fatal("DeepCopyObject returned same pointer")
	}
	if len(copied.Items) != len(original.Items) {
		t.Errorf("copied items length = %d, want %d", len(copied.Items), len(original.Items))
	}
}

func TestNilDeepCopy(t *testing.T) {
	var nilGSMSecret *GSMSecret
	if nilGSMSecret.DeepCopy() != nil {
		t.Error("DeepCopy of nil should return nil")
	}

	var nilSpec *GSMSecretSpec
	if nilSpec.DeepCopy() != nil {
		t.Error("DeepCopy of nil spec should return nil")
	}

	var nilStatus *GSMSecretStatus
	if nilStatus.DeepCopy() != nil {
		t.Error("DeepCopy of nil status should return nil")
	}

	var nilEntry *GSMSecretEntry
	if nilEntry.DeepCopy() != nil {
		t.Error("DeepCopy of nil entry should return nil")
	}

	var nilTarget *GSMSecretTargetSecret
	if nilTarget.DeepCopy() != nil {
		t.Error("DeepCopy of nil target should return nil")
	}

	var nilList *GSMSecretList
	if nilList.DeepCopy() != nil {
		t.Error("DeepCopy of nil list should return nil")
	}
}

// ==================== Helper Functions ====================

func loadStatusSchema(t *testing.T) *apiextensionsv1.JSONSchemaProps {
	t.Helper()

	rootSchema := loadRootSchema(t)
	statusSchema, ok := rootSchema.Properties["status"]
	if !ok {
		t.Fatal("status property missing from schema")
	}
	return &statusSchema
}

func loadRootSchema(t *testing.T) *apiextensionsv1.JSONSchemaProps {
	t.Helper()

	crd := loadCRD(t)

	var version *apiextensionsv1.CustomResourceDefinitionVersion
	for i := range crd.Spec.Versions {
		if crd.Spec.Versions[i].Name == "v1alpha1" {
			version = &crd.Spec.Versions[i]
			break
		}
	}
	if version == nil {
		t.Fatal("v1alpha1 version not found in CRD")
	}

	if version.Schema == nil || version.Schema.OpenAPIV3Schema == nil {
		t.Fatal("v1alpha1 version missing schema")
	}

	return version.Schema.OpenAPIV3Schema
}

func loadCRD(t *testing.T) *apiextensionsv1.CustomResourceDefinition {
	t.Helper()

	crdPath := filepath.Join("..", "..", "config", "crd", "bases", "secrets.gsm-operator.io_gsmsecrets.yaml")

	rawCRD, err := os.ReadFile(crdPath)
	if err != nil {
		t.Fatalf("failed to read CRD file %q: %v", crdPath, err)
	}

	var crd apiextensionsv1.CustomResourceDefinition
	if err := yaml.Unmarshal(rawCRD, &crd); err != nil {
		t.Fatalf("failed to unmarshal CRD yaml: %v", err)
	}

	return &crd
}
