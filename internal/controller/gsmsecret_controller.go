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

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	secretswayfaircomv1alpha1 "github.com/wayfair-shared/gsm-operator/api/v1alpha1"
)

// GSMSecretReconciler reconciles a GSMSecret object
type GSMSecretReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=secrets.wayfair.com,resources=gsmsecrets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=secrets.wayfair.com,resources=gsmsecrets/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=secrets.wayfair.com,resources=gsmsecrets/finalizers,verbs=update

func (r *GSMSecretReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// 1. Load the GSMSecret instance.
	var gsm secretswayfaircomv1alpha1.GSMSecret
	if err := r.Get(ctx, req.NamespacedName, &gsm); err != nil {
		if apierrors.IsNotFound(err) {
			// Resource deleted; nothing to do.
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// 2. Fetch payloads from Google Secret Manager for each gsmSecrets entry.
	payloads, err := FetchGSMSecretPayloads(ctx, gsm.Spec.Secrets)
	if err != nil {
		log.Error(err, "failed to fetch GSM payloads")
		return ctrl.Result{}, err
	}

	// 3. Build the desired Kubernetes Secret from those payloads.
	secretName := gsm.Spec.TargetSecret.Name
	if secretName == "" {
		secretName = gsm.Name
	}

	desiredSecret, err := BuildOpaqueSecret(secretName, gsm.Namespace, payloads)
	if err != nil {
		log.Error(err, "failed to build Secret object")
		return ctrl.Result{}, err
	}

	// Ensure the Secret is owned by this GSMSecret for garbage collection.
	if err := ctrl.SetControllerReference(&gsm, desiredSecret, r.Scheme); err != nil {
		return ctrl.Result{}, err
	}

	// 4. Create or update the Secret in the cluster.
	var existing corev1.Secret
	key := types.NamespacedName{
		Name:      desiredSecret.Name,
		Namespace: desiredSecret.Namespace,
	}

	// Try to get such a secret:
	if err := r.Get(ctx, key, &existing); err != nil {
		// Return error if anything other than not found?
		if !apierrors.IsNotFound(err) {
			return ctrl.Result{}, err
		}

		// The secret does not exists, so create it.
		if err := r.Create(ctx, desiredSecret); err != nil {
			log.Error(err, "failed to create Secret", "secret", key)
			return ctrl.Result{}, err
		}
	} else {
		// Update data if it changed.
		existing.Data = desiredSecret.Data
		existing.Type = desiredSecret.Type

		if err := r.Update(ctx, &existing); err != nil {
			log.Error(err, "failed to update Secret", "secret", key)
			return ctrl.Result{}, err
		}
	}

	// No requeue: static materialization as described in the README.
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *GSMSecretReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&secretswayfaircomv1alpha1.GSMSecret{}).
		Named("gsmsecret").
		Complete(r)
}
