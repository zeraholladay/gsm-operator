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
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	secretspizecomv1alpha1 "github.com/zeraholladay/gsm-operator/api/v1alpha1"
)

// GSMSecretReconciler reconciles a GSMSecret object.
type GSMSecretReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=secrets.pize.com,resources=gsmsecrets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=secrets.pize.com,resources=gsmsecrets/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=secrets.pize.com,resources=gsmsecrets/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch

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
		return ctrl.Result{}, err
	}

	// 3. APPLY: Ensure the cluster state matches our desired state.
	if err := r.applySecret(ctx, &gsmSecret, desiredSecret); err != nil {
		log.Error(err, "failed to apply Kubernetes Secret")
		return ctrl.Result{}, err
	}

	log.Info("reconciliation complete")
	return ctrl.Result{}, nil
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
	// Note: You might want to compare existing.Data vs desired.Data here to avoid
	// unnecessary API calls, but a blind Update is safe for correctness.
	existing.Data = desired.Data
	existing.Type = desired.Type

	log.Info("updating existing Kubernetes Secret", "secret", key)
	return r.Update(ctx, &existing)
}

// SetupWithManager sets up the controller with the Manager.
func (r *GSMSecretReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&secretspizecomv1alpha1.GSMSecret{}).
		Named("gsmsecret").
		Complete(r)
}
