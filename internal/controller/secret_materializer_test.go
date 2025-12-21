package controller

import (
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

	if got := m.getKSA(); got != "gsm-reader" {
		t.Fatalf("expected default KSA gsm-reader, got %q", got)
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
