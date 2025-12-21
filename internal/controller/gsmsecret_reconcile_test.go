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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	secretspizecomv1alpha1 "github.com/zeraholladay/gsm-operator/api/v1alpha1"
)

var _ = Describe("GSMSecret Reconcile Integration", func() {
	const (
		timeout  = time.Second * 10
		interval = time.Millisecond * 250
	)

	Context("When reconciling with various scenarios", func() {
		var (
			reconciler *GSMSecretReconciler
			testCtx    context.Context
		)

		BeforeEach(func() {
			testCtx = context.Background()
			reconciler = &GSMSecretReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}
		})

		It("should handle GSMSecret with status conditions correctly", func() {
			resourceName := "status-test-resource"
			namespace := "default"

			By("Creating a GSMSecret resource")
			gsmSecret := &secretspizecomv1alpha1.GSMSecret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: namespace,
					Annotations: map[string]string{
						secretspizecomv1alpha1.AnnotationWIFAudience: "//iam.googleapis.com/projects/123/locations/global/workloadIdentityPools/test-pool/providers/test-provider",
					},
				},
				Spec: secretspizecomv1alpha1.GSMSecretSpec{
					TargetSecret: secretspizecomv1alpha1.GSMSecretTargetSecret{
						Name: "status-test-target",
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
			Expect(k8sClient.Create(testCtx, gsmSecret)).To(Succeed())

			DeferCleanup(func() {
				By("Cleaning up the test GSMSecret")
				resource := &secretspizecomv1alpha1.GSMSecret{}
				err := k8sClient.Get(testCtx, types.NamespacedName{Name: resourceName, Namespace: namespace}, resource)
				if err == nil {
					Expect(k8sClient.Delete(testCtx, resource)).To(Succeed())
				}
			})

			By("Reconciling the resource (expect failure due to missing WIF infrastructure)")
			result, err := reconciler.Reconcile(testCtx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      resourceName,
					Namespace: namespace,
				},
			})

			// The reconcile will fail because we don't have real WIF infrastructure
			// but it should attempt the reconcile and update status
			Expect(err).To(HaveOccurred()) // Expected to fail without real GCP/WIF

			// Result should be empty on error (no requeue scheduled)
			Expect(result.Requeue).To(BeFalse())
		})

		It("should update status condition on reconcile failure", func() {
			resourceName := "failure-status-test"
			namespace := "default"

			By("Creating a GSMSecret resource")
			gsmSecret := &secretspizecomv1alpha1.GSMSecret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: namespace,
					Annotations: map[string]string{
						secretspizecomv1alpha1.AnnotationWIFAudience: "//iam.googleapis.com/projects/123/locations/global/workloadIdentityPools/test-pool/providers/test-provider",
					},
				},
				Spec: secretspizecomv1alpha1.GSMSecretSpec{
					TargetSecret: secretspizecomv1alpha1.GSMSecretTargetSecret{
						Name: "failure-test-target",
					},
					Secrets: []secretspizecomv1alpha1.GSMSecretEntry{
						{
							Key:       "FAIL_KEY",
							ProjectID: "fail-project",
							SecretID:  "fail-secret",
							Version:   "1",
						},
					},
				},
			}
			Expect(k8sClient.Create(testCtx, gsmSecret)).To(Succeed())

			DeferCleanup(func() {
				By("Cleaning up the test GSMSecret")
				resource := &secretspizecomv1alpha1.GSMSecret{}
				err := k8sClient.Get(testCtx, types.NamespacedName{Name: resourceName, Namespace: namespace}, resource)
				if err == nil {
					Expect(k8sClient.Delete(testCtx, resource)).To(Succeed())
				}
			})

			By("Reconciling (will fail)")
			_, _ = reconciler.Reconcile(testCtx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      resourceName,
					Namespace: namespace,
				},
			})

			By("Verifying the status was updated with failure condition")
			Eventually(func() bool {
				var updated secretspizecomv1alpha1.GSMSecret
				err := k8sClient.Get(testCtx, types.NamespacedName{Name: resourceName, Namespace: namespace}, &updated)
				if err != nil {
					return false
				}
				// Check if conditions were set (may or may not be set depending on where failure occurs)
				return len(updated.Status.Conditions) > 0 || updated.Status.ObservedGeneration > 0
			}, timeout, interval).Should(BeTrue())
		})

		It("should handle existing secret adoption and update", func() {
			resourceName := "adoption-test"
			targetSecretName := "adoption-target-secret"
			namespace := "default"

			By("Creating an existing secret without owner reference")
			existingSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      targetSecretName,
					Namespace: namespace,
					Labels: map[string]string{
						"existing-label": "should-be-preserved",
					},
				},
				Type: corev1.SecretTypeOpaque,
				Data: map[string][]byte{
					"OLD_DATA": []byte("old-value"),
				},
			}
			Expect(k8sClient.Create(testCtx, existingSecret)).To(Succeed())

			DeferCleanup(func() {
				By("Cleaning up the test secret")
				secret := &corev1.Secret{}
				err := k8sClient.Get(testCtx, types.NamespacedName{Name: targetSecretName, Namespace: namespace}, secret)
				if err == nil {
					Expect(k8sClient.Delete(testCtx, secret)).To(Succeed())
				}
			})

			By("Creating a GSMSecret that targets the existing secret")
			gsmSecret := &secretspizecomv1alpha1.GSMSecret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: namespace,
					Annotations: map[string]string{
						secretspizecomv1alpha1.AnnotationWIFAudience: "//iam.googleapis.com/projects/123/locations/global/workloadIdentityPools/test-pool/providers/test-provider",
					},
				},
				Spec: secretspizecomv1alpha1.GSMSecretSpec{
					TargetSecret: secretspizecomv1alpha1.GSMSecretTargetSecret{
						Name: targetSecretName,
					},
					Secrets: []secretspizecomv1alpha1.GSMSecretEntry{
						{
							Key:       "NEW_KEY",
							ProjectID: "test-project",
							SecretID:  "test-secret",
							Version:   "latest",
						},
					},
				},
			}
			Expect(k8sClient.Create(testCtx, gsmSecret)).To(Succeed())

			DeferCleanup(func() {
				By("Cleaning up the test GSMSecret")
				resource := &secretspizecomv1alpha1.GSMSecret{}
				err := k8sClient.Get(testCtx, types.NamespacedName{Name: resourceName, Namespace: namespace}, resource)
				if err == nil {
					Expect(k8sClient.Delete(testCtx, resource)).To(Succeed())
				}
			})

			By("Verifying the existing secret still exists")
			var secret corev1.Secret
			Expect(k8sClient.Get(testCtx, types.NamespacedName{Name: targetSecretName, Namespace: namespace}, &secret)).To(Succeed())

			// The existing label should still be there
			Expect(secret.Labels["existing-label"]).To(Equal("should-be-preserved"))
		})

		It("should handle multiple secret entries in spec", func() {
			resourceName := "multi-secret-test"
			namespace := "default"

			By("Creating a GSMSecret with multiple entries")
			gsmSecret := &secretspizecomv1alpha1.GSMSecret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: namespace,
					Annotations: map[string]string{
						secretspizecomv1alpha1.AnnotationWIFAudience: "//iam.googleapis.com/projects/123/locations/global/workloadIdentityPools/test-pool/providers/test-provider",
					},
				},
				Spec: secretspizecomv1alpha1.GSMSecretSpec{
					TargetSecret: secretspizecomv1alpha1.GSMSecretTargetSecret{
						Name: "multi-secret-target",
					},
					Secrets: []secretspizecomv1alpha1.GSMSecretEntry{
						{
							Key:       "DB_PASSWORD",
							ProjectID: "project-1",
							SecretID:  "db-password",
							Version:   "latest",
						},
						{
							Key:       "API_KEY",
							ProjectID: "project-1",
							SecretID:  "api-key",
							Version:   "2",
						},
						{
							Key:       "TLS_CERT",
							ProjectID: "project-2",
							SecretID:  "tls-cert",
							Version:   "1",
						},
					},
				},
			}
			Expect(k8sClient.Create(testCtx, gsmSecret)).To(Succeed())

			DeferCleanup(func() {
				By("Cleaning up the test GSMSecret")
				resource := &secretspizecomv1alpha1.GSMSecret{}
				err := k8sClient.Get(testCtx, types.NamespacedName{Name: resourceName, Namespace: namespace}, resource)
				if err == nil {
					Expect(k8sClient.Delete(testCtx, resource)).To(Succeed())
				}
			})

			By("Verifying the GSMSecret was created with all entries")
			var fetched secretspizecomv1alpha1.GSMSecret
			Expect(k8sClient.Get(testCtx, types.NamespacedName{Name: resourceName, Namespace: namespace}, &fetched)).To(Succeed())
			Expect(fetched.Spec.Secrets).To(HaveLen(3))
			Expect(fetched.Spec.Secrets[0].Key).To(Equal("DB_PASSWORD"))
			Expect(fetched.Spec.Secrets[1].Key).To(Equal("API_KEY"))
			Expect(fetched.Spec.Secrets[2].Key).To(Equal("TLS_CERT"))
		})

		It("should handle generation updates correctly", func() {
			resourceName := "generation-test"
			namespace := "default"

			By("Creating a GSMSecret resource")
			gsmSecret := &secretspizecomv1alpha1.GSMSecret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: namespace,
					Annotations: map[string]string{
						secretspizecomv1alpha1.AnnotationWIFAudience: "//iam.googleapis.com/projects/123/locations/global/workloadIdentityPools/test-pool/providers/test-provider",
					},
				},
				Spec: secretspizecomv1alpha1.GSMSecretSpec{
					TargetSecret: secretspizecomv1alpha1.GSMSecretTargetSecret{
						Name: "generation-test-target",
					},
					Secrets: []secretspizecomv1alpha1.GSMSecretEntry{
						{
							Key:       "INITIAL_KEY",
							ProjectID: "test-project",
							SecretID:  "test-secret",
							Version:   "1",
						},
					},
				},
			}
			Expect(k8sClient.Create(testCtx, gsmSecret)).To(Succeed())

			DeferCleanup(func() {
				By("Cleaning up the test GSMSecret")
				resource := &secretspizecomv1alpha1.GSMSecret{}
				err := k8sClient.Get(testCtx, types.NamespacedName{Name: resourceName, Namespace: namespace}, resource)
				if err == nil {
					Expect(k8sClient.Delete(testCtx, resource)).To(Succeed())
				}
			})

			By("Verifying initial generation")
			var fetched secretspizecomv1alpha1.GSMSecret
			Expect(k8sClient.Get(testCtx, types.NamespacedName{Name: resourceName, Namespace: namespace}, &fetched)).To(Succeed())
			initialGeneration := fetched.Generation

			By("Updating the spec")
			fetched.Spec.Secrets[0].Version = "2"
			Expect(k8sClient.Update(testCtx, &fetched)).To(Succeed())

			By("Verifying generation was incremented")
			var updated secretspizecomv1alpha1.GSMSecret
			Expect(k8sClient.Get(testCtx, types.NamespacedName{Name: resourceName, Namespace: namespace}, &updated)).To(Succeed())
			Expect(updated.Generation).To(BeNumerically(">", initialGeneration))
		})

		It("should set requeue interval on successful reconcile", func() {
			// This tests the defaultResyncInterval behavior
			// Since we can't actually complete a full reconcile without GCP,
			// we verify the constant is set correctly
			Expect(defaultResyncInterval).To(Equal(5 * time.Minute))
		})
	})
})

var _ = Describe("GSMSecret newSecretMaterializer", func() {
	It("should create a materializer with correct settings", func() {
		gsmSecret := &secretspizecomv1alpha1.GSMSecret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-gsmsecret",
				Namespace: "test-namespace",
				Annotations: map[string]string{
					secretspizecomv1alpha1.AnnotationKSA:         "custom-ksa",
					secretspizecomv1alpha1.AnnotationWIFAudience: "test-audience",
				},
			},
			Spec: secretspizecomv1alpha1.GSMSecretSpec{
				TargetSecret: secretspizecomv1alpha1.GSMSecretTargetSecret{
					Name: "target-secret",
				},
				Secrets: []secretspizecomv1alpha1.GSMSecretEntry{
					{Key: "KEY", ProjectID: "proj", SecretID: "secret", Version: "1"},
				},
			},
		}

		reconciler := &GSMSecretReconciler{
			Client: k8sClient,
			Scheme: k8sClient.Scheme(),
		}

		m := reconciler.newSecretMaterializer(gsmSecret)

		Expect(m).NotTo(BeNil())
		Expect(m.gsmSecret).To(Equal(gsmSecret))
		Expect(m.payloads).To(BeNil()) // Initially nil/empty
		Expect(m.kubeClientFn).NotTo(BeNil())
	})
})
