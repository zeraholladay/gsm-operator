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
	"bytes"
	"context"
	"fmt"
	"os"
	"strconv"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	secretspizecomv1alpha1 "github.com/zeraholladay/gsm-operator/api/v1alpha1"
)

const (
	// defaultResyncInterval is how often to re-reconcile even if no events occur,
	// ensuring GSM secret changes are eventually reflected.
	// Can be overridden via RESYNC_INTERVAL_SECONDS environment variable.
	defaultResyncInterval = 5 * time.Minute

	// Condition types for GSMSecret status.
	conditionTypeReady = "Ready"
)

// getResyncInterval returns the resync interval from RESYNC_INTERVAL_SECONDS env var,
// or the default of 5 minutes if not set or invalid.
func getResyncInterval() time.Duration {
	if v := os.Getenv("RESYNC_INTERVAL_SECONDS"); v != "" {
		if seconds, err := strconv.Atoi(v); err == nil && seconds > 0 {
			return time.Duration(seconds) * time.Second
		}
	}
	return defaultResyncInterval
}

// GSMSecretReconciler reconciles a GSMSecret object.
type GSMSecretReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=secrets.pize.com,resources=gsmsecrets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=secrets.pize.com,resources=gsmsecrets/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=secrets.pize.com,resources=gsmsecrets/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;patch;delete
func (r *GSMSecretReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// 1. FETCH: Load the GSMSecret instance.
	var gsmSecret secretspizecomv1alpha1.GSMSecret
	if err := r.Get(ctx, req.NamespacedName, &gsmSecret); err != nil {
		if apierrors.IsNotFound(err) {
			// Resource deleted; nothing to do.
			log.V(1).Info("GSMSecret resource not found; assuming it was deleted", "name", req.Name, "namespace", req.Namespace)
			return ctrl.Result{}, nil
		}
		log.Error(err, "failed to fetch GSMSecret from API server", "name", req.Name, "namespace", req.Namespace)
		return ctrl.Result{}, err
	}

	log.Info("starting reconciliation",
		"name", gsmSecret.Name,
		"namespace", gsmSecret.Namespace,
		"specTargetSecret", gsmSecret.Spec.TargetSecret.Name,
	)

	// 2. MATERIALIZE: Initialize the helper with one clean call.
	m := r.newSecretMaterializer(&gsmSecret)

	// Delegate the heavy lifting to the materializer.
	if err := m.resolvePayloads(ctx); err != nil {
		log.Error(err, "failed to fetch GSM payloads")
		if statusErr := r.setStatusCondition(ctx, &gsmSecret, metav1.ConditionFalse, "FetchFailed", err.Error()); statusErr != nil {
			log.Error(statusErr, "failed to update status after fetch error")
		}
		return ctrl.Result{}, err
	}
	log.Info("fetched GSM payloads for GSMSecret",
		"name", gsmSecret.Name,
		"namespace", gsmSecret.Namespace,
		"payloadCount", len(m.payloads),
	)

	// Build the desired Kubernetes Secret from those payloads.
	desiredSecret, err := m.buildOpaqueSecret(ctx)
	if err != nil {
		log.Error(err, "failed to build Secret object")
		if statusErr := r.setStatusCondition(ctx, &gsmSecret, metav1.ConditionFalse, "BuildFailed", err.Error()); statusErr != nil {
			log.Error(statusErr, "failed to update status after build error")
		}
		return ctrl.Result{}, err
	}

	// 3. APPLY: Ensure the cluster state matches our desired state.
	if err := r.applySecret(ctx, &gsmSecret, desiredSecret); err != nil {
		log.Error(err, "failed to apply Kubernetes Secret")
		if statusErr := r.setStatusCondition(ctx, &gsmSecret, metav1.ConditionFalse, "ApplyFailed", err.Error()); statusErr != nil {
			log.Error(statusErr, "failed to update status after apply error")
		}
		return ctrl.Result{}, err
	}

	// 4. STATUS: Mark reconciliation as successful.
	if err := r.setStatusCondition(ctx, &gsmSecret, metav1.ConditionTrue, "Synced", "Secret successfully synced from GSM"); err != nil {
		log.Error(err, "failed to update status after successful reconciliation")
		return ctrl.Result{}, err
	}

	log.Info("reconciliation complete")
	// Requeue after interval to pick up GSM secret changes.
	return ctrl.Result{RequeueAfter: getResyncInterval()}, nil
}

// newSecretMaterializer acts as a factory/constructor.
// It hides the wiring of the client, scheme, and raw data from the main loop.
func (r *GSMSecretReconciler) newSecretMaterializer(gsm *secretspizecomv1alpha1.GSMSecret) *secretMaterializer {
	return &secretMaterializer{
		gsmSecret:    gsm,
		kubeClientFn: getInClusterKubeClient,
		// payloads slice is implicitly nil/empty, which is correct for a new instance
	}
}

// applySecret handles the generic K8s "Create or Update" logic.
// This removes the boilerplate from Reconcile, making the flow linear and readable.
func (r *GSMSecretReconciler) applySecret(ctx context.Context, owner *secretspizecomv1alpha1.GSMSecret, desired *corev1.Secret) error {
	log := logf.FromContext(ctx)

	// 1. Set OwnerReference so deleting the GSMSecret deletes the generated Secret.
	if err := ctrl.SetControllerReference(owner, desired, r.Scheme); err != nil {
		return fmt.Errorf("failed to set controller reference: %w", err)
	}

	// 2. Check if the secret already exists.
	var existing corev1.Secret
	key := types.NamespacedName{
		Name:      desired.Name,
		Namespace: desired.Namespace,
	}

	err := r.Get(ctx, key, &existing)
	if err != nil && !apierrors.IsNotFound(err) {
		return err // Actual API error.
	}

	// 3. Create if not found.
	if apierrors.IsNotFound(err) {
		log.Info("creating new Kubernetes Secret", "secret", key)
		return r.Create(ctx, desired)
	}

	// 4. Update if found.
	// Set controller reference on the existing secret to ensure ownership is
	// established even for pre-existing secrets (handles adoption scenario).
	if err := ctrl.SetControllerReference(owner, &existing, r.Scheme); err != nil {
		return fmt.Errorf("failed to set controller reference on existing secret: %w", err)
	}

	existing.Data = desired.Data
	existing.Type = desired.Type

	log.Info("updating existing Kubernetes Secret", "secret", key)
	return r.Update(ctx, &existing)
}

// setStatusCondition updates the GSMSecret's status with a Ready condition.
func (r *GSMSecretReconciler) setStatusCondition(
	ctx context.Context,
	gsmSecret *secretspizecomv1alpha1.GSMSecret,
	status metav1.ConditionStatus,
	reason, message string,
) error {
	// Update observed generation to indicate we've processed this spec version.
	gsmSecret.Status.ObservedGeneration = gsmSecret.Generation

	// Build the new condition.
	newCondition := metav1.Condition{
		Type:               conditionTypeReady,
		Status:             status,
		ObservedGeneration: gsmSecret.Generation,
		LastTransitionTime: metav1.Now(),
		Reason:             reason,
		Message:            message,
	}

	// Find and update existing condition or append new one.
	found := false
	for i, c := range gsmSecret.Status.Conditions {
		if c.Type == conditionTypeReady {
			// Only update LastTransitionTime if status actually changed.
			if c.Status == status {
				newCondition.LastTransitionTime = c.LastTransitionTime
			}
			gsmSecret.Status.Conditions[i] = newCondition
			found = true
			break
		}
	}
	if !found {
		gsmSecret.Status.Conditions = append(gsmSecret.Status.Conditions, newCondition)
	}

	return r.Status().Update(ctx, gsmSecret)
}

// gsmSecretChangedPredicate triggers reconciliation when the GSMSecret's spec or
// relevant annotations change. This ignores status-only updates (which don't increment
// generation) while still reacting to annotation changes that affect behavior.
type gsmSecretChangedPredicate struct {
	predicate.Funcs
}

// relevantAnnotations are the annotation keys that affect controller behavior.
// Changes to these annotations should trigger reconciliation.
var relevantAnnotations = []string{
	secretspizecomv1alpha1.AnnotationKSA,
	secretspizecomv1alpha1.AnnotationGSA,
	secretspizecomv1alpha1.AnnotationWIFAudience,
	secretspizecomv1alpha1.AnnotationRelease,
}

// Update returns true if the GSMSecret's generation or relevant annotations have changed.
func (gsmSecretChangedPredicate) Update(e event.UpdateEvent) bool {
	// Always reconcile if generation changed (spec change)
	if e.ObjectOld.GetGeneration() != e.ObjectNew.GetGeneration() {
		return true
	}

	// Check if any relevant annotations changed
	oldAnnotations := e.ObjectOld.GetAnnotations()
	newAnnotations := e.ObjectNew.GetAnnotations()
	for _, key := range relevantAnnotations {
		if oldAnnotations[key] != newAnnotations[key] {
			return true
		}
	}

	// Status-only update or irrelevant annotation change, skip reconciliation
	return false
}

// secretDataChangedPredicate triggers reconciliation only when Secret data actually changes.
// This avoids unnecessary reconciles when only metadata (like resourceVersion) changes.
type secretDataChangedPredicate struct {
	predicate.Funcs
}

// Update returns true only if the Secret's Data or Type has changed.
func (secretDataChangedPredicate) Update(e event.UpdateEvent) bool {
	oldSecret, ok := e.ObjectOld.(*corev1.Secret)
	if !ok {
		return true // Not a Secret, allow the event
	}
	newSecret, ok := e.ObjectNew.(*corev1.Secret)
	if !ok {
		return true // Not a Secret, allow the event
	}

	// Check if Type changed
	if oldSecret.Type != newSecret.Type {
		return true
	}

	// Check if Data changed (deep comparison)
	if !secretDataEqual(oldSecret.Data, newSecret.Data) {
		return true
	}

	// No meaningful change, skip reconciliation
	return false
}

// secretDataEqual compares two secret data maps for equality.
func secretDataEqual(a, b map[string][]byte) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if bv, ok := b[k]; !ok || !bytes.Equal(v, bv) {
			return false
		}
	}
	return true
}

// SetupWithManager sets up the controller with the Manager.
func (r *GSMSecretReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		// Watch GSMSecret with custom predicate to ignore status-only updates.
		// Reconcile when: spec changes (generation bump) OR annotations change.
		// Skip when: only status changes (e.g., after we update conditions).
		For(&secretspizecomv1alpha1.GSMSecret{},
			builder.WithPredicates(gsmSecretChangedPredicate{})).
		// Watch owned Secrets, but only trigger reconcile when data actually changes.
		// This prevents double reconciles when we update a Secret (which triggers an
		// update event) but the data hasn't meaningfully changed.
		Owns(&corev1.Secret{},
			builder.WithPredicates(secretDataChangedPredicate{})).
		Named("gsmsecret").
		Complete(r)
}
