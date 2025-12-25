package controller

import (
	"bytes"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	secretspizecomv1alpha1 "github.com/zeraholladay/gsm-operator/api/v1alpha1"
)

func TestGetTokenExpSecondsDefault(t *testing.T) {
	t.Setenv("TOKEN_EXP_SECONDS", "")
	m := &secretMaterializer{}

	if got := m.getTokenExpSeconds(); got != defaultTokenExpSeconds {
		t.Fatalf("expected defaultTokenExpSeconds %d, got %d", defaultTokenExpSeconds, got)
	}
}

func TestGetTokenExpSecondsClampsToMinimum(t *testing.T) {
	t.Setenv("TOKEN_EXP_SECONDS", "300")
	m := &secretMaterializer{}

	if got := m.getTokenExpSeconds(); got != minTokenExpSeconds {
		t.Fatalf("expected minimum %d when value below minimum provided, got %d", minTokenExpSeconds, got)
	}
}

func TestGetTokenExpSecondsRespectsHigherValue(t *testing.T) {
	t.Setenv("TOKEN_EXP_SECONDS", "1200")
	m := &secretMaterializer{}

	if got := m.getTokenExpSeconds(); got != 1200 {
		t.Fatalf("expected 1200 from env override, got %d", got)
	}
}

func TestGetKSAFromEnvOverridesAnnotation(t *testing.T) {
	t.Setenv("KSA", "env-ksa")
	m := &secretMaterializer{
		gsmSecret: &secretspizecomv1alpha1.GSMSecret{
			ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{
					secretspizecomv1alpha1.AnnotationKSA: "annotated-ksa",
				},
			},
		},
	}

	if got := m.getKSA(); got != "env-ksa" {
		t.Fatalf("expected env KSA to win, got %q", got)
	}
}

func TestGetKSAFromAnnotation(t *testing.T) {
	t.Setenv("KSA", "")
	m := &secretMaterializer{
		gsmSecret: &secretspizecomv1alpha1.GSMSecret{
			ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{
					secretspizecomv1alpha1.AnnotationKSA: "annotated-ksa",
				},
			},
		},
	}

	if got := m.getKSA(); got != "annotated-ksa" {
		t.Fatalf("expected annotated KSA, got %q", got)
	}
}

func TestGetKSADefault(t *testing.T) {
	t.Setenv("KSA", "")
	m := &secretMaterializer{
		gsmSecret: &secretspizecomv1alpha1.GSMSecret{
			ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{},
			},
		},
	}

	if got := m.getKSA(); got != defaultKSAName {
		t.Fatalf("expected default KSA %q, got %q", defaultKSAName, got)
	}
}

func TestGetWIFAudienceFromEnv(t *testing.T) {
	t.Setenv("WIFAUDIENCE", "env-aud")
	m := &secretMaterializer{
		gsmSecret: &secretspizecomv1alpha1.GSMSecret{
			ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{
					secretspizecomv1alpha1.AnnotationWIFAudience: "annotated-aud",
				},
			},
		},
	}

	got, err := m.getWIFAudience()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "env-aud" {
		t.Fatalf("expected env audience to win, got %q", got)
	}
}

func TestGetWIFAudienceFromAnnotation(t *testing.T) {
	t.Setenv("WIFAUDIENCE", "")
	m := &secretMaterializer{
		gsmSecret: &secretspizecomv1alpha1.GSMSecret{
			ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{
					secretspizecomv1alpha1.AnnotationWIFAudience: "annotated-aud",
				},
			},
		},
	}

	got, err := m.getWIFAudience()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "annotated-aud" {
		t.Fatalf("expected annotated audience, got %q", got)
	}
}

func TestGetWIFAudienceMissing(t *testing.T) {
	t.Setenv("WIFAUDIENCE", "")
	m := &secretMaterializer{
		gsmSecret: &secretspizecomv1alpha1.GSMSecret{
			ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{},
			},
		},
	}

	if _, err := m.getWIFAudience(); err == nil {
		t.Fatalf("expected error when WIF audience is missing")
	}
}

func TestGetGSAFromAnnotation(t *testing.T) {
	m := &secretMaterializer{
		gsmSecret: &secretspizecomv1alpha1.GSMSecret{
			ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{
					secretspizecomv1alpha1.AnnotationGSA: "my-gsa@project.iam.gserviceaccount.com",
				},
			},
		},
	}

	if got := m.getGSA(); got != "my-gsa@project.iam.gserviceaccount.com" {
		t.Fatalf("expected GSA from annotation, got %q", got)
	}
}

func TestGetGSAEmptyWhenNoAnnotation(t *testing.T) {
	m := &secretMaterializer{
		gsmSecret: &secretspizecomv1alpha1.GSMSecret{
			ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{},
			},
		},
	}

	if got := m.getGSA(); got != "" {
		t.Fatalf("expected empty GSA when no annotation, got %q", got)
	}
}

func TestGetGSAEmptyWhenNilAnnotations(t *testing.T) {
	m := &secretMaterializer{
		gsmSecret: &secretspizecomv1alpha1.GSMSecret{
			ObjectMeta: metav1.ObjectMeta{
				Annotations: nil,
			},
		},
	}

	if got := m.getGSA(); got != "" {
		t.Fatalf("expected empty GSA when nil annotations, got %q", got)
	}
}

func TestGetGSATrimsWhitespace(t *testing.T) {
	m := &secretMaterializer{
		gsmSecret: &secretspizecomv1alpha1.GSMSecret{
			ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{
					secretspizecomv1alpha1.AnnotationGSA: "  my-gsa@project.iam.gserviceaccount.com  ",
				},
			},
		},
	}

	if got := m.getGSA(); got != "my-gsa@project.iam.gserviceaccount.com" {
		t.Fatalf("expected trimmed GSA, got %q", got)
	}
}

func TestGetGSAEmptyWhenWhitespaceOnly(t *testing.T) {
	m := &secretMaterializer{
		gsmSecret: &secretspizecomv1alpha1.GSMSecret{
			ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{
					secretspizecomv1alpha1.AnnotationGSA: "   ",
				},
			},
		},
	}

	if got := m.getGSA(); got != "" {
		t.Fatalf("expected empty GSA when whitespace only, got %q", got)
	}
}

func TestNewKeyedSecretPayload_Valid(t *testing.T) {
	value := []byte("hello")
	payload, err := newKeyedSecretPayload("VALID_KEY", value)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if payload.Key != "VALID_KEY" {
		t.Errorf("expected key VALID_KEY, got %q", payload.Key)
	}
	if !bytes.Equal(payload.Value, value) {
		t.Errorf("expected value %q, got %q", string(value), string(payload.Value))
	}
}

func TestNewKeyedSecretPayload_Invalid(t *testing.T) {
	_, err := newKeyedSecretPayload("bad key with spaces", []byte("nope"))
	if err == nil {
		t.Fatal("expected error for invalid key, got nil")
	}
}

func TestNewKeyedSecretPayload_Empty(t *testing.T) {
	_, err := newKeyedSecretPayload("", []byte("nope"))
	if err == nil {
		t.Fatal("expected error for empty key, got nil")
	}
}

// ==================== isTrustedSubsystem tests ====================

func TestIsTrustedSubsystem_EnabledWhenModeIsTrustedSubsystem(t *testing.T) {
	t.Setenv("MODE", "TRUSTED_SUBSYSTEM")
	m := &secretMaterializer{}

	if !m.isTrustedSubsystem() {
		t.Fatal("expected isTrustedSubsystem() to return true when MODE=TRUSTED_SUBSYSTEM")
	}
}

func TestIsTrustedSubsystem_DisabledWhenModeIsWIF(t *testing.T) {
	t.Setenv("MODE", "WIF")
	m := &secretMaterializer{}

	if m.isTrustedSubsystem() {
		t.Fatal("expected isTrustedSubsystem() to return false when MODE=WIF")
	}
}

func TestIsTrustedSubsystem_DisabledWhenModeIsEmpty(t *testing.T) {
	t.Setenv("MODE", "")
	m := &secretMaterializer{}

	if m.isTrustedSubsystem() {
		t.Fatal("expected isTrustedSubsystem() to return false when MODE is empty")
	}
}

func TestIsTrustedSubsystem_DisabledWhenModeNotSet(t *testing.T) {
	// Ensure the env var is not set (t.Setenv will restore it after the test)
	t.Setenv("MODE", "")
	m := &secretMaterializer{}

	if m.isTrustedSubsystem() {
		t.Fatal("expected isTrustedSubsystem() to return false when MODE is not set")
	}
}

func TestIsTrustedSubsystem_DisabledForOtherValues(t *testing.T) {
	t.Setenv("MODE", "other")
	m := &secretMaterializer{}

	if m.isTrustedSubsystem() {
		t.Fatal("expected isTrustedSubsystem() to return false for MODE=other")
	}
}

func TestIsTrustedSubsystem_CaseSensitive(t *testing.T) {
	// MODE check is case-sensitive; "trusted_subsystem" should NOT match
	t.Setenv("MODE", "trusted_subsystem")
	m := &secretMaterializer{}

	if m.isTrustedSubsystem() {
		t.Fatal("expected isTrustedSubsystem() to return false for lowercase 'trusted_subsystem'")
	}
}
