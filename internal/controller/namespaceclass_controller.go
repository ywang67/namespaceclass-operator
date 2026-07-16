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
	"encoding/json"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	namespaceclassv1alpha1 "github.com/ywang67/namespaceclass-operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

// NamespaceClassReconciler reconciles a NamespaceClass object
type NamespaceClassReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=namespaceclass.akuity.io,resources=namespaceclasses,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=namespaceclass.akuity.io,resources=namespaceclasses/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=namespaceclass.akuity.io,resources=namespaceclasses/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=serviceaccounts,verbs=get;create;update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the NamespaceClass object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.24.1/pkg/reconcile
func (r *NamespaceClassReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = logf.FromContext(ctx)

	// TODO(user): your logic here
	namespace := &corev1.Namespace{}
	if err := r.Get(ctx, req.NamespacedName, namespace); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	className, ok := namespace.Labels["namespaceclass.akuity.io/name"]
	if !ok || className == "" {
		return ctrl.Result{}, nil
	}

	namespaceClass := &namespaceclassv1alpha1.NamespaceClass{}
	if err := r.Get(ctx, client.ObjectKey{Name: className}, namespaceClass); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	for _, resource := range namespaceClass.Spec.Resources {
		obj := &unstructured.Unstructured{}
		if err := json.Unmarshal(resource.Raw, obj); err != nil {
			return ctrl.Result{}, err
		}

		obj.SetNamespace(namespace.Name)
		labels := obj.GetLabels()
		if labels == nil {
			labels = map[string]string{}
		}

		//  Add labels to indicate that the resource is managed by the namespaceclass-operator and belongs to the specific NamespaceClass.For save delection.
		labels["namespaceclass.akuity.io/managed-by"] = "namespaceclass-operator"
		labels["namespaceclass.akuity.io/class"] = className
		obj.SetLabels(labels)
		existing := &unstructured.Unstructured{}
		existing.SetGroupVersionKind(obj.GroupVersionKind())

		err := r.Get(ctx, client.ObjectKeyFromObject(obj), existing)
		if apierrors.IsNotFound(err) {
			if err := r.Create(ctx, obj); err != nil {
				return ctrl.Result{}, err
			}
			continue
		}
		if err != nil {
			return ctrl.Result{}, err
		}

		// Update the existing resource with the new spec, preserving the resource version to avoid conflicts.
		obj.SetResourceVersion(existing.GetResourceVersion())
		if err := r.Update(ctx, obj); err != nil {
			return ctrl.Result{}, err
		}

	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *NamespaceClassReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Namespace{}).
		Named("namespaceclass").
		Complete(r)
}
