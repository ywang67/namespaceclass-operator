/*
Copyright 2026.

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
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	namespaceclassv1alpha1 "github.com/ywang67/namespaceclass-operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
)

var _ = Describe("NamespaceClass Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-resource"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name: resourceName,
		}
		namespace := &corev1.Namespace{}

		// created temp ns and nsclass instances in beforeEach
		BeforeEach(func() {
			By("creating the custom resource for the Kind NamespaceClass")
			err := k8sClient.Get(ctx, typeNamespacedName, namespace)
			if err != nil && errors.IsNotFound(err) {
				resource := &corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name:   resourceName,
						Labels: map[string]string{"namespaceclass.akuity.io/name": "test-class"},
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}

			namespaceClass := &namespaceclassv1alpha1.NamespaceClass{}
			namespaceClassKey := types.NamespacedName{Name: "test-class"}

			err = k8sClient.Get(ctx, namespaceClassKey, namespaceClass)
			if errors.IsNotFound(err) {
				namespaceClass = &namespaceclassv1alpha1.NamespaceClass{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-class",
					},
					Spec: namespaceclassv1alpha1.NamespaceClassSpec{
						Resources: []runtime.RawExtension{
							{
								Raw: []byte(`{
						"apiVersion":"v1",
						"kind":"ServiceAccount",
						"metadata":{"name":"application"}
					}`),
							},
						},
					},
				}

				Expect(k8sClient.Create(ctx, namespaceClass)).To(Succeed())
			}
		})

		AfterEach(func() {
			resource := &corev1.Namespace{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the specific resource instance Namespace")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
		})
		It("should successfully reconcile the resource", func() {
			By("Reconciling the created resource")
			controllerReconciler := &NamespaceClassReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			serviceAccount := &corev1.ServiceAccount{}

			err = k8sClient.Get(
				ctx,
				types.NamespacedName{
					Name:      "application",
					Namespace: resourceName,
				},
				serviceAccount,
			)

			Expect(err).NotTo(HaveOccurred())
			Expect(serviceAccount.Name).To(Equal("application"))
			Expect(serviceAccount.Namespace).To(Equal(resourceName))
		})
	})
})
