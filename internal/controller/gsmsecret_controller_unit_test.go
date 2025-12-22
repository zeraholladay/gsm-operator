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

package controller

import (
	"context"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/event"

	secretspizecomv1alpha1 "github.com/zeraholladay/gsm-operator/api/v1alpha1"
)

func newTestScheme() *runtime.Scheme {
	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	_ = secretspizecomv1alpha1.AddToScheme(scheme)
	return scheme
}

func newTestReconciler(objs ...client.Object) *GSMSecretReconciler {
	scheme := newTestScheme()
	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(objs...).
		WithStatusSubresource(&secretspizecomv1alpha1.GSMSecret{}).
		Build()
	return &GSMSecretReconciler{
		Client: fakeClient,
		Scheme: scheme,
	}
}

// ==================== applySecret tests ====================

func TestApplySecret_CreateNew(t *testing.T) {
	owner := &secretspizecomv1alpha1.GSMSecret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-gsmsecret",
			Namespace: "default",
			UID:       types.UID("test-uid-123"),
		},
		Spec: secretspizecomv1alpha1.GSMSecretSpec{
			TargetSecret: secretspizecomv1alpha1.GSMSecretTargetSecret{Name: "my-secret"},
			Secrets:      []secretspizecomv1alpha1.GSMSecretEntry{{Key: "K", ProjectID: "p", SecretID: "s", Version: "1"}},
		},
	}
	r := newTestReconciler(owner)
	ctx := context.Background()

	desired := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-secret",
			Namespace: "default",
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			"KEY": []byte("value"),
		},
	}

	err := r.applySecret(ctx, owner, desired)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// Verify secret was created
	var created corev1.Secret
	err = r.Get(ctx, types.NamespacedName{Name: "my-secret", Namespace: "default"}, &created)
	if err != nil {
		t.Fatalf("expected secret to exist, got %v", err)
	}

	if string(created.Data["KEY"]) != "value" {
		t.Errorf("expected data['KEY']='value', got %q", string(created.Data["KEY"]))
	}
	if created.Type != corev1.SecretTypeOpaque {
		t.Errorf("expected Opaque type, got %v", created.Type)
	}

	// Verify owner reference was set
	if len(created.OwnerReferences) != 1 {
		t.Fatalf("expected 1 owner reference, got %d", len(created.OwnerReferences))
	}
	if created.OwnerReferences[0].UID != owner.UID {
		t.Errorf("expected owner UID %q, got %q", owner.UID, created.OwnerReferences[0].UID)
	}
}

func TestApplySecret_UpdateExisting(t *testing.T) {
	owner := &secretspizecomv1alpha1.GSMSecret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-gsmsecret",
			Namespace: "default",
			UID:       types.UID("test-uid-123"),
		},
		Spec: secretspizecomv1alpha1.GSMSecretSpec{
			TargetSecret: secretspizecomv1alpha1.GSMSecretTargetSecret{Name: "my-secret"},
			Secrets:      []secretspizecomv1alpha1.GSMSecretEntry{{Key: "K", ProjectID: "p", SecretID: "s", Version: "1"}},
		},
	}

	existingSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-secret",
			Namespace: "default",
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			"OLD_KEY": []byte("old-value"),
		},
	}

	r := newTestReconciler(owner, existingSecret)
	ctx := context.Background()

	desired := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-secret",
			Namespace: "default",
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			"NEW_KEY": []byte("new-value"),
		},
	}

	err := r.applySecret(ctx, owner, desired)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// Verify secret was updated
	var updated corev1.Secret
	err = r.Get(ctx, types.NamespacedName{Name: "my-secret", Namespace: "default"}, &updated)
	if err != nil {
		t.Fatalf("expected secret to exist, got %v", err)
	}

	if _, exists := updated.Data["OLD_KEY"]; exists {
		t.Error("expected OLD_KEY to be removed")
	}
	if string(updated.Data["NEW_KEY"]) != "new-value" {
		t.Errorf("expected NEW_KEY='new-value', got %q", string(updated.Data["NEW_KEY"]))
	}
}

func TestApplySecret_AdoptsExistingSecret(t *testing.T) {
	owner := &secretspizecomv1alpha1.GSMSecret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-gsmsecret",
			Namespace: "default",
			UID:       types.UID("test-uid-123"),
		},
		Spec: secretspizecomv1alpha1.GSMSecretSpec{
			TargetSecret: secretspizecomv1alpha1.GSMSecretTargetSecret{Name: "my-secret"},
			Secrets:      []secretspizecomv1alpha1.GSMSecretEntry{{Key: "K", ProjectID: "p", SecretID: "s", Version: "1"}},
		},
	}

	// Existing secret without owner reference (orphaned)
	existingSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-secret",
			Namespace: "default",
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			"KEY": []byte("value"),
		},
	}

	r := newTestReconciler(owner, existingSecret)
	ctx := context.Background()

	desired := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-secret",
			Namespace: "default",
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			"KEY": []byte("updated-value"),
		},
	}

	err := r.applySecret(ctx, owner, desired)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// Verify owner reference was added
	var updated corev1.Secret
	err = r.Get(ctx, types.NamespacedName{Name: "my-secret", Namespace: "default"}, &updated)
	if err != nil {
		t.Fatalf("expected secret to exist, got %v", err)
	}

	if len(updated.OwnerReferences) != 1 {
		t.Fatalf("expected 1 owner reference, got %d", len(updated.OwnerReferences))
	}
	if updated.OwnerReferences[0].UID != owner.UID {
		t.Errorf("expected owner UID %q, got %q", owner.UID, updated.OwnerReferences[0].UID)
	}
}

func TestApplySecret_PreservesExistingLabelsAndAnnotations(t *testing.T) {
	owner := &secretspizecomv1alpha1.GSMSecret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-gsmsecret",
			Namespace: "default",
			UID:       types.UID("test-uid-123"),
		},
		Spec: secretspizecomv1alpha1.GSMSecretSpec{
			TargetSecret: secretspizecomv1alpha1.GSMSecretTargetSecret{Name: "my-secret"},
			Secrets:      []secretspizecomv1alpha1.GSMSecretEntry{{Key: "K", ProjectID: "p", SecretID: "s", Version: "1"}},
		},
	}

	existingSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-secret",
			Namespace: "default",
			Labels: map[string]string{
				"custom-label": "should-be-preserved",
			},
			Annotations: map[string]string{
				"custom-annotation": "should-also-be-preserved",
			},
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			"KEY": []byte("value"),
		},
	}

	r := newTestReconciler(owner, existingSecret)
	ctx := context.Background()

	desired := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-secret",
			Namespace: "default",
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			"KEY": []byte("updated-value"),
		},
	}

	err := r.applySecret(ctx, owner, desired)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	var updated corev1.Secret
	err = r.Get(ctx, types.NamespacedName{Name: "my-secret", Namespace: "default"}, &updated)
	if err != nil {
		t.Fatalf("expected secret to exist, got %v", err)
	}

	// Labels and annotations should be preserved (not overwritten)
	if updated.Labels["custom-label"] != "should-be-preserved" {
		t.Errorf("expected custom-label to be preserved, got %q", updated.Labels["custom-label"])
	}
	if updated.Annotations["custom-annotation"] != "should-also-be-preserved" {
		t.Errorf("expected custom-annotation to be preserved, got %q", updated.Annotations["custom-annotation"])
	}
}

// ==================== setStatusCondition tests ====================

func TestSetStatusCondition_NewCondition(t *testing.T) {
	gsmSecret := &secretspizecomv1alpha1.GSMSecret{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-gsmsecret",
			Namespace:  "default",
			Generation: 1,
		},
		Spec: secretspizecomv1alpha1.GSMSecretSpec{
			TargetSecret: secretspizecomv1alpha1.GSMSecretTargetSecret{Name: "my-secret"},
			Secrets:      []secretspizecomv1alpha1.GSMSecretEntry{{Key: "K", ProjectID: "p", SecretID: "s", Version: "1"}},
		},
		Status: secretspizecomv1alpha1.GSMSecretStatus{
			Conditions: []metav1.Condition{},
		},
	}

	r := newTestReconciler(gsmSecret)
	ctx := context.Background()

	err := r.setStatusCondition(ctx, gsmSecret, metav1.ConditionTrue, "Synced", "Secret successfully synced")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// Fetch updated resource
	var updated secretspizecomv1alpha1.GSMSecret
	err = r.Get(ctx, types.NamespacedName{Name: "test-gsmsecret", Namespace: "default"}, &updated)
	if err != nil {
		t.Fatalf("expected resource to exist, got %v", err)
	}

	if len(updated.Status.Conditions) != 1 {
		t.Fatalf("expected 1 condition, got %d", len(updated.Status.Conditions))
	}

	cond := updated.Status.Conditions[0]
	if cond.Type != conditionTypeReady {
		t.Errorf("expected condition type %q, got %q", conditionTypeReady, cond.Type)
	}
	if cond.Status != metav1.ConditionTrue {
		t.Errorf("expected status True, got %v", cond.Status)
	}
	if cond.Reason != "Synced" {
		t.Errorf("expected reason 'Synced', got %q", cond.Reason)
	}
	if cond.Message != "Secret successfully synced" {
		t.Errorf("expected message 'Secret successfully synced', got %q", cond.Message)
	}
	if updated.Status.ObservedGeneration != 1 {
		t.Errorf("expected ObservedGeneration 1, got %d", updated.Status.ObservedGeneration)
	}
}

func TestSetStatusCondition_UpdateExisting_SameStatus(t *testing.T) {
	// Use a time with second precision to match metav1.Time serialization behavior
	originalTime := metav1.NewTime(time.Now().Add(-1 * time.Hour).Truncate(time.Second))
	gsmSecret := &secretspizecomv1alpha1.GSMSecret{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-gsmsecret",
			Namespace:  "default",
			Generation: 2,
		},
		Spec: secretspizecomv1alpha1.GSMSecretSpec{
			TargetSecret: secretspizecomv1alpha1.GSMSecretTargetSecret{Name: "my-secret"},
			Secrets:      []secretspizecomv1alpha1.GSMSecretEntry{{Key: "K", ProjectID: "p", SecretID: "s", Version: "1"}},
		},
	}

	r := newTestReconciler(gsmSecret)
	ctx := context.Background()

	// First, set up the initial condition on the resource in the cluster
	gsmSecret.Status = secretspizecomv1alpha1.GSMSecretStatus{
		ObservedGeneration: 1,
		Conditions: []metav1.Condition{
			{
				Type:               conditionTypeReady,
				Status:             metav1.ConditionTrue,
				Reason:             "Synced",
				Message:            "Previous sync",
				LastTransitionTime: originalTime,
				ObservedGeneration: 1,
			},
		},
	}
	err := r.Status().Update(ctx, gsmSecret)
	if err != nil {
		t.Fatalf("failed to set initial status: %v", err)
	}

	// Refetch to get the updated resource
	var current secretspizecomv1alpha1.GSMSecret
	err = r.Get(ctx, types.NamespacedName{Name: "test-gsmsecret", Namespace: "default"}, &current)
	if err != nil {
		t.Fatalf("expected resource to exist, got %v", err)
	}

	// Record the transition time from the fetched resource
	if len(current.Status.Conditions) == 0 {
		t.Fatal("expected at least one condition after initial status update")
	}
	storedOriginalTime := current.Status.Conditions[0].LastTransitionTime

	// Now update with same status (True -> True)
	err = r.setStatusCondition(ctx, &current, metav1.ConditionTrue, "Synced", "New sync message")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	var updated secretspizecomv1alpha1.GSMSecret
	err = r.Get(ctx, types.NamespacedName{Name: "test-gsmsecret", Namespace: "default"}, &updated)
	if err != nil {
		t.Fatalf("expected resource to exist, got %v", err)
	}

	if len(updated.Status.Conditions) == 0 {
		t.Fatal("expected at least one condition")
	}
	cond := updated.Status.Conditions[0]
	// LastTransitionTime should NOT change when status stays the same
	if !cond.LastTransitionTime.Equal(&storedOriginalTime) {
		t.Errorf("expected LastTransitionTime to remain unchanged, got %v (original: %v)", cond.LastTransitionTime, storedOriginalTime)
	}
	// But message and observed generation should update
	if cond.Message != "New sync message" {
		t.Errorf("expected message 'New sync message', got %q", cond.Message)
	}
	if cond.ObservedGeneration != 2 {
		t.Errorf("expected ObservedGeneration 2, got %d", cond.ObservedGeneration)
	}
}

func TestSetStatusCondition_StatusTransition(t *testing.T) {
	originalTime := metav1.NewTime(time.Now().Add(-1 * time.Hour))
	gsmSecret := &secretspizecomv1alpha1.GSMSecret{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-gsmsecret",
			Namespace:  "default",
			Generation: 2,
		},
		Spec: secretspizecomv1alpha1.GSMSecretSpec{
			TargetSecret: secretspizecomv1alpha1.GSMSecretTargetSecret{Name: "my-secret"},
			Secrets:      []secretspizecomv1alpha1.GSMSecretEntry{{Key: "K", ProjectID: "p", SecretID: "s", Version: "1"}},
		},
		Status: secretspizecomv1alpha1.GSMSecretStatus{
			ObservedGeneration: 1,
			Conditions: []metav1.Condition{
				{
					Type:               conditionTypeReady,
					Status:             metav1.ConditionTrue,
					Reason:             "Synced",
					Message:            "Previous sync",
					LastTransitionTime: originalTime,
					ObservedGeneration: 1,
				},
			},
		},
	}

	r := newTestReconciler(gsmSecret)
	ctx := context.Background()

	// Transition from True -> False
	err := r.setStatusCondition(ctx, gsmSecret, metav1.ConditionFalse, "FetchFailed", "GSM fetch failed")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	var updated secretspizecomv1alpha1.GSMSecret
	err = r.Get(ctx, types.NamespacedName{Name: "test-gsmsecret", Namespace: "default"}, &updated)
	if err != nil {
		t.Fatalf("expected resource to exist, got %v", err)
	}

	cond := updated.Status.Conditions[0]
	if cond.Status != metav1.ConditionFalse {
		t.Errorf("expected status False, got %v", cond.Status)
	}
	// LastTransitionTime SHOULD change when status changes
	if cond.LastTransitionTime.Equal(&originalTime) {
		t.Error("expected LastTransitionTime to be updated on status change")
	}
	if cond.Reason != "FetchFailed" {
		t.Errorf("expected reason 'FetchFailed', got %q", cond.Reason)
	}
}

func TestSetStatusCondition_MultipleConditionReasons(t *testing.T) {
	tests := []struct {
		name    string
		status  metav1.ConditionStatus
		reason  string
		message string
	}{
		{"FetchFailed", metav1.ConditionFalse, "FetchFailed", "Failed to fetch GSM secret"},
		{"BuildFailed", metav1.ConditionFalse, "BuildFailed", "Failed to build K8s secret"},
		{"ApplyFailed", metav1.ConditionFalse, "ApplyFailed", "Failed to apply secret"},
		{"Synced", metav1.ConditionTrue, "Synced", "Successfully synced"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gsmSecret := &secretspizecomv1alpha1.GSMSecret{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "test-gsmsecret",
					Namespace:  "default",
					Generation: 1,
				},
				Spec: secretspizecomv1alpha1.GSMSecretSpec{
					TargetSecret: secretspizecomv1alpha1.GSMSecretTargetSecret{Name: "my-secret"},
					Secrets:      []secretspizecomv1alpha1.GSMSecretEntry{{Key: "K", ProjectID: "p", SecretID: "s", Version: "1"}},
				},
			}

			r := newTestReconciler(gsmSecret)
			ctx := context.Background()

			err := r.setStatusCondition(ctx, gsmSecret, tt.status, tt.reason, tt.message)
			if err != nil {
				t.Fatalf("expected no error, got %v", err)
			}

			var updated secretspizecomv1alpha1.GSMSecret
			err = r.Get(ctx, types.NamespacedName{Name: "test-gsmsecret", Namespace: "default"}, &updated)
			if err != nil {
				t.Fatalf("expected resource to exist, got %v", err)
			}

			if len(updated.Status.Conditions) != 1 {
				t.Fatalf("expected 1 condition, got %d", len(updated.Status.Conditions))
			}

			cond := updated.Status.Conditions[0]
			if cond.Status != tt.status {
				t.Errorf("expected status %v, got %v", tt.status, cond.Status)
			}
			if cond.Reason != tt.reason {
				t.Errorf("expected reason %q, got %q", tt.reason, cond.Reason)
			}
			if cond.Message != tt.message {
				t.Errorf("expected message %q, got %q", tt.message, cond.Message)
			}
		})
	}
}

// ==================== Predicate tests ====================

func TestSecretDataEqual(t *testing.T) {
	tests := []struct {
		name     string
		a        map[string][]byte
		b        map[string][]byte
		expected bool
	}{
		{
			name:     "both nil",
			a:        nil,
			b:        nil,
			expected: true,
		},
		{
			name:     "both empty",
			a:        map[string][]byte{},
			b:        map[string][]byte{},
			expected: true,
		},
		{
			name:     "nil and empty are equal",
			a:        nil,
			b:        map[string][]byte{},
			expected: true,
		},
		{
			name:     "same single key",
			a:        map[string][]byte{"key": []byte("value")},
			b:        map[string][]byte{"key": []byte("value")},
			expected: true,
		},
		{
			name:     "different values",
			a:        map[string][]byte{"key": []byte("value1")},
			b:        map[string][]byte{"key": []byte("value2")},
			expected: false,
		},
		{
			name:     "different keys",
			a:        map[string][]byte{"key1": []byte("value")},
			b:        map[string][]byte{"key2": []byte("value")},
			expected: false,
		},
		{
			name:     "different lengths",
			a:        map[string][]byte{"key1": []byte("value")},
			b:        map[string][]byte{"key1": []byte("value"), "key2": []byte("value")},
			expected: false,
		},
		{
			name:     "multiple keys same",
			a:        map[string][]byte{"key1": []byte("v1"), "key2": []byte("v2")},
			b:        map[string][]byte{"key1": []byte("v1"), "key2": []byte("v2")},
			expected: true,
		},
		{
			name:     "binary data same",
			a:        map[string][]byte{"bin": {0x00, 0x01, 0x02}},
			b:        map[string][]byte{"bin": {0x00, 0x01, 0x02}},
			expected: true,
		},
		{
			name:     "binary data different",
			a:        map[string][]byte{"bin": {0x00, 0x01, 0x02}},
			b:        map[string][]byte{"bin": {0x00, 0x01, 0x03}},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := secretDataEqual(tt.a, tt.b)
			if result != tt.expected {
				t.Errorf("secretDataEqual() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestSecretDataChangedPredicate_Update(t *testing.T) {
	pred := secretDataChangedPredicate{}

	tests := []struct {
		name      string
		oldSecret *corev1.Secret
		newSecret *corev1.Secret
		expected  bool
	}{
		{
			name: "data unchanged - should skip",
			oldSecret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default", ResourceVersion: "1"},
				Type:       corev1.SecretTypeOpaque,
				Data:       map[string][]byte{"key": []byte("value")},
			},
			newSecret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default", ResourceVersion: "2"},
				Type:       corev1.SecretTypeOpaque,
				Data:       map[string][]byte{"key": []byte("value")},
			},
			expected: false,
		},
		{
			name: "data value changed - should trigger",
			oldSecret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
				Type:       corev1.SecretTypeOpaque,
				Data:       map[string][]byte{"key": []byte("old-value")},
			},
			newSecret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
				Type:       corev1.SecretTypeOpaque,
				Data:       map[string][]byte{"key": []byte("new-value")},
			},
			expected: true,
		},
		{
			name: "new key added - should trigger",
			oldSecret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
				Type:       corev1.SecretTypeOpaque,
				Data:       map[string][]byte{"key1": []byte("value1")},
			},
			newSecret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
				Type:       corev1.SecretTypeOpaque,
				Data:       map[string][]byte{"key1": []byte("value1"), "key2": []byte("value2")},
			},
			expected: true,
		},
		{
			name: "key removed - should trigger",
			oldSecret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
				Type:       corev1.SecretTypeOpaque,
				Data:       map[string][]byte{"key1": []byte("value1"), "key2": []byte("value2")},
			},
			newSecret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
				Type:       corev1.SecretTypeOpaque,
				Data:       map[string][]byte{"key1": []byte("value1")},
			},
			expected: true,
		},
		{
			name: "type changed - should trigger",
			oldSecret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
				Type:       corev1.SecretTypeOpaque,
				Data:       map[string][]byte{"key": []byte("value")},
			},
			newSecret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
				Type:       corev1.SecretTypeTLS,
				Data:       map[string][]byte{"key": []byte("value")},
			},
			expected: true,
		},
		{
			name: "only labels changed - should skip",
			oldSecret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "default",
					Labels:    map[string]string{"old": "label"},
				},
				Type: corev1.SecretTypeOpaque,
				Data: map[string][]byte{"key": []byte("value")},
			},
			newSecret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "default",
					Labels:    map[string]string{"new": "label"},
				},
				Type: corev1.SecretTypeOpaque,
				Data: map[string][]byte{"key": []byte("value")},
			},
			expected: false,
		},
		{
			name: "only annotations changed - should skip",
			oldSecret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "test",
					Namespace:   "default",
					Annotations: map[string]string{"old": "annotation"},
				},
				Type: corev1.SecretTypeOpaque,
				Data: map[string][]byte{"key": []byte("value")},
			},
			newSecret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "test",
					Namespace:   "default",
					Annotations: map[string]string{"new": "annotation"},
				},
				Type: corev1.SecretTypeOpaque,
				Data: map[string][]byte{"key": []byte("value")},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := event.UpdateEvent{
				ObjectOld: tt.oldSecret,
				ObjectNew: tt.newSecret,
			}
			result := pred.Update(e)
			if result != tt.expected {
				t.Errorf("secretDataChangedPredicate.Update() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestSecretDataChangedPredicate_NonSecretObjects(t *testing.T) {
	pred := secretDataChangedPredicate{}

	// Test with non-Secret objects - should return true to allow the event
	gsmSecret := &secretspizecomv1alpha1.GSMSecret{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
	}

	e := event.UpdateEvent{
		ObjectOld: gsmSecret,
		ObjectNew: gsmSecret,
	}

	if !pred.Update(e) {
		t.Error("expected true for non-Secret objects")
	}
}

func TestSecretDataChangedPredicate_DefaultFuncs(t *testing.T) {
	pred := secretDataChangedPredicate{}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
		Type:       corev1.SecretTypeOpaque,
		Data:       map[string][]byte{"key": []byte("value")},
	}

	// Create and Delete events should pass through (default behavior from predicate.Funcs)
	createEvent := event.CreateEvent{Object: secret}
	if !pred.Create(createEvent) {
		t.Error("expected Create to return true by default")
	}

	deleteEvent := event.DeleteEvent{Object: secret}
	if !pred.Delete(deleteEvent) {
		t.Error("expected Delete to return true by default")
	}

	genericEvent := event.GenericEvent{Object: secret}
	if !pred.Generic(genericEvent) {
		t.Error("expected Generic to return true by default")
	}
}

// ==================== GSMSecret Predicate tests ====================

func TestGSMSecretChangedPredicate_Update(t *testing.T) {
	pred := gsmSecretChangedPredicate{}

	tests := []struct {
		name        string
		oldGSM      *secretspizecomv1alpha1.GSMSecret
		newGSM      *secretspizecomv1alpha1.GSMSecret
		expected    bool
		description string
	}{
		{
			name: "generation changed - should trigger",
			oldGSM: &secretspizecomv1alpha1.GSMSecret{
				ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default", Generation: 1},
			},
			newGSM: &secretspizecomv1alpha1.GSMSecret{
				ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default", Generation: 2},
			},
			expected:    true,
			description: "spec change should trigger reconcile",
		},
		{
			name: "KSA annotation added - should trigger",
			oldGSM: &secretspizecomv1alpha1.GSMSecret{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "test",
					Namespace:  "default",
					Generation: 1,
				},
			},
			newGSM: &secretspizecomv1alpha1.GSMSecret{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "test",
					Namespace:  "default",
					Generation: 1,
					Annotations: map[string]string{
						secretspizecomv1alpha1.AnnotationKSA: "my-sa",
					},
				},
			},
			expected:    true,
			description: "adding KSA annotation should trigger reconcile",
		},
		{
			name: "GSA annotation changed - should trigger",
			oldGSM: &secretspizecomv1alpha1.GSMSecret{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "test",
					Namespace:  "default",
					Generation: 1,
					Annotations: map[string]string{
						secretspizecomv1alpha1.AnnotationGSA: "old-gsa@project.iam",
					},
				},
			},
			newGSM: &secretspizecomv1alpha1.GSMSecret{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "test",
					Namespace:  "default",
					Generation: 1,
					Annotations: map[string]string{
						secretspizecomv1alpha1.AnnotationGSA: "new-gsa@project.iam",
					},
				},
			},
			expected:    true,
			description: "changing GSA annotation should trigger reconcile",
		},
		{
			name: "WIF audience annotation removed - should trigger",
			oldGSM: &secretspizecomv1alpha1.GSMSecret{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "test",
					Namespace:  "default",
					Generation: 1,
					Annotations: map[string]string{
						secretspizecomv1alpha1.AnnotationWIFAudience: "aud",
					},
				},
			},
			newGSM: &secretspizecomv1alpha1.GSMSecret{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "test",
					Namespace:  "default",
					Generation: 1,
				},
			},
			expected:    true,
			description: "removing WIF audience annotation should trigger reconcile",
		},
		{
			name: "status only change - should skip",
			oldGSM: &secretspizecomv1alpha1.GSMSecret{
				ObjectMeta: metav1.ObjectMeta{
					Name:            "test",
					Namespace:       "default",
					Generation:      1,
					ResourceVersion: "100",
				},
				Status: secretspizecomv1alpha1.GSMSecretStatus{
					ObservedGeneration: 0,
				},
			},
			newGSM: &secretspizecomv1alpha1.GSMSecret{
				ObjectMeta: metav1.ObjectMeta{
					Name:            "test",
					Namespace:       "default",
					Generation:      1,
					ResourceVersion: "101",
				},
				Status: secretspizecomv1alpha1.GSMSecretStatus{
					ObservedGeneration: 1,
					Conditions: []metav1.Condition{
						{Type: "Ready", Status: metav1.ConditionTrue},
					},
				},
			},
			expected:    false,
			description: "status-only update should NOT trigger reconcile",
		},
		{
			name: "labels changed but not annotations - should skip",
			oldGSM: &secretspizecomv1alpha1.GSMSecret{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "test",
					Namespace:  "default",
					Generation: 1,
					Labels:     map[string]string{"old": "label"},
				},
			},
			newGSM: &secretspizecomv1alpha1.GSMSecret{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "test",
					Namespace:  "default",
					Generation: 1,
					Labels:     map[string]string{"new": "label"},
				},
			},
			expected:    false,
			description: "label-only changes should NOT trigger reconcile",
		},
		{
			name: "unrelated annotation changed - should skip",
			oldGSM: &secretspizecomv1alpha1.GSMSecret{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "test",
					Namespace:   "default",
					Generation:  1,
					Annotations: map[string]string{"kubectl.kubernetes.io/last-applied-configuration": "old"},
				},
			},
			newGSM: &secretspizecomv1alpha1.GSMSecret{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "test",
					Namespace:   "default",
					Generation:  1,
					Annotations: map[string]string{"kubectl.kubernetes.io/last-applied-configuration": "new"},
				},
			},
			expected:    false,
			description: "unrelated annotation changes should NOT trigger reconcile",
		},
		{
			name: "relevant annotation unchanged with unrelated annotation changed - should skip",
			oldGSM: &secretspizecomv1alpha1.GSMSecret{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "test",
					Namespace:  "default",
					Generation: 1,
					Annotations: map[string]string{
						secretspizecomv1alpha1.AnnotationKSA: "my-sa",
						"some-other-annotation":              "old-value",
					},
				},
			},
			newGSM: &secretspizecomv1alpha1.GSMSecret{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "test",
					Namespace:  "default",
					Generation: 1,
					Annotations: map[string]string{
						secretspizecomv1alpha1.AnnotationKSA: "my-sa",
						"some-other-annotation":              "new-value",
					},
				},
			},
			expected:    false,
			description: "changes to unrelated annotations should NOT trigger when relevant ones unchanged",
		},
		{
			name: "no change at all - should skip",
			oldGSM: &secretspizecomv1alpha1.GSMSecret{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "test",
					Namespace:  "default",
					Generation: 1,
					Annotations: map[string]string{
						secretspizecomv1alpha1.AnnotationKSA: "my-sa",
					},
				},
			},
			newGSM: &secretspizecomv1alpha1.GSMSecret{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "test",
					Namespace:  "default",
					Generation: 1,
					Annotations: map[string]string{
						secretspizecomv1alpha1.AnnotationKSA: "my-sa",
					},
				},
			},
			expected:    false,
			description: "no meaningful change should NOT trigger reconcile",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := event.UpdateEvent{
				ObjectOld: tt.oldGSM,
				ObjectNew: tt.newGSM,
			}
			result := pred.Update(e)
			if result != tt.expected {
				t.Errorf("gsmSecretChangedPredicate.Update() = %v, want %v (%s)", result, tt.expected, tt.description)
			}
		})
	}
}

func TestGSMSecretChangedPredicate_DefaultFuncs(t *testing.T) {
	pred := gsmSecretChangedPredicate{}

	gsmSecret := &secretspizecomv1alpha1.GSMSecret{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
	}

	// Create and Delete events should pass through (default behavior from predicate.Funcs)
	createEvent := event.CreateEvent{Object: gsmSecret}
	if !pred.Create(createEvent) {
		t.Error("expected Create to return true by default")
	}

	deleteEvent := event.DeleteEvent{Object: gsmSecret}
	if !pred.Delete(deleteEvent) {
		t.Error("expected Delete to return true by default")
	}

	genericEvent := event.GenericEvent{Object: gsmSecret}
	if !pred.Generic(genericEvent) {
		t.Error("expected Generic to return true by default")
	}
}

func TestGetResyncInterval_Default(t *testing.T) {
	t.Setenv("RESYNC_INTERVAL_SECONDS", "")

	interval := getResyncInterval()
	if interval != 5*time.Minute {
		t.Errorf("expected default 5 minutes, got %v", interval)
	}
}

func TestGetResyncInterval_CustomValue(t *testing.T) {
	t.Setenv("RESYNC_INTERVAL_SECONDS", "120")

	interval := getResyncInterval()
	if interval != 2*time.Minute {
		t.Errorf("expected 2 minutes, got %v", interval)
	}
}

func TestGetResyncInterval_InvalidValue(t *testing.T) {
	t.Setenv("RESYNC_INTERVAL_SECONDS", "not-a-number")

	interval := getResyncInterval()
	if interval != 5*time.Minute {
		t.Errorf("expected default 5 minutes for invalid value, got %v", interval)
	}
}

func TestGetResyncInterval_ZeroValue(t *testing.T) {
	t.Setenv("RESYNC_INTERVAL_SECONDS", "0")

	interval := getResyncInterval()
	if interval != 5*time.Minute {
		t.Errorf("expected default 5 minutes for zero value, got %v", interval)
	}
}

func TestGetResyncInterval_NegativeValue(t *testing.T) {
	t.Setenv("RESYNC_INTERVAL_SECONDS", "-60")

	interval := getResyncInterval()
	if interval != 5*time.Minute {
		t.Errorf("expected default 5 minutes for negative value, got %v", interval)
	}
}
