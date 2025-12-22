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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	secretspizecomv1alpha1 "github.com/zeraholladay/gsm-operator/api/v1alpha1"
)

var _ = Describe("GSMSecret Controller", func() {
	Context("When creating a GSMSecret resource", func() {
		const resourceName = "test-resource"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default",
		}

		AfterEach(func() {
			resource := &secretspizecomv1alpha1.GSMSecret{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			if err == nil {
				By("Cleanup the specific resource instance GSMSecret")
				Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
			}
		})

		It("should accept a valid GSMSecret spec", func() {
			By("creating the custom resource for the Kind GSMSecret")
			resource := &secretspizecomv1alpha1.GSMSecret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: "default",
					Annotations: map[string]string{
						secretspizecomv1alpha1.AnnotationWIFAudience: "//iam.googleapis.com/projects/123/locations/global/workloadIdentityPools/test-pool/providers/test-provider",
					},
				},
				Spec: secretspizecomv1alpha1.GSMSecretSpec{
					TargetSecret: secretspizecomv1alpha1.GSMSecretTargetSecret{
						Name: "test-target-secret",
					},
					Secrets: []secretspizecomv1alpha1.GSMSecretEntry{
						{
							Key:       "TEST_KEY",
							ProjectID: "test-project",
							SecretID:  "test-secret",
							Version:   "latest",
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, resource)).To(Succeed())

			By("verifying the resource was created")
			fetched := &secretspizecomv1alpha1.GSMSecret{}
			Expect(k8sClient.Get(ctx, typeNamespacedName, fetched)).To(Succeed())
			Expect(fetched.Spec.TargetSecret.Name).To(Equal("test-target-secret"))
			Expect(fetched.Spec.Secrets).To(HaveLen(1))
			Expect(fetched.Spec.Secrets[0].Key).To(Equal("TEST_KEY"))
		})

		It("should handle reconcile for missing resource gracefully", func() {
			By("Reconciling a non-existent resource")
			controllerReconciler := &GSMSecretReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			result, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      "non-existent-resource",
					Namespace: "default",
				},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(reconcile.Result{}))
		})
	})
})
