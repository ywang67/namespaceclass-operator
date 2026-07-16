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
	"k8s.io/apimachinery/pkg/runtime/schema"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	namespaceclassv1alpha1 "github.com/ywang67/namespaceclass-operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
)

const managedResourcesAnnotation = "namespaceclass.akuity.io/managed-resources"

type resourceRef struct {
	Group   string `json:"group"`
	Version string `json:"version"`
	Kind    string `json:"kind"`
	Name    string `json:"name"`
}

// NamespaceClassReconciler reconciles a NamespaceClass object
type NamespaceClassReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=namespaceclass.akuity.io,resources=namespaceclasses,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=namespaceclass.akuity.io,resources=namespaceclasses/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=namespaceclass.akuity.io,resources=namespaceclasses/finalizers,verbs=update
// +kubebuilder:rbac:groups=*,resources=*,verbs=get;list;watch;create;update;patch;delete

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
	log := logf.FromContext(ctx)

	namespace := &corev1.Namespace{}
	if err := r.Get(ctx, req.NamespacedName, namespace); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	className, ok := namespace.Labels["namespaceclass.akuity.io/name"]
	if !ok || className == "" {
		return ctrl.Result{}, nil
	}

	log.Info("reconciling namespace", "namespace", namespace.Name, "class", className)

	namespaceClass := &namespaceclassv1alpha1.NamespaceClass{}
	if err := r.Get(ctx, client.ObjectKey{Name: className}, namespaceClass); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	var oldRefs []resourceRef
	if data := namespace.Annotations[managedResourcesAnnotation]; data != "" {
		_ = json.Unmarshal([]byte(data), &oldRefs)
	}
	var newRefs []resourceRef
	for _, resource := range namespaceClass.Spec.Resources {
		obj := &unstructured.Unstructured{}
		if err := json.Unmarshal(resource.Raw, obj); err != nil {
			return ctrl.Result{}, err
		}

		// We only support namespaced resources.
		mapping, err := r.RESTMapper().RESTMapping(obj.GroupVersionKind().GroupKind(), obj.GroupVersionKind().Version)
		if err != nil {
			log.Error(err, "failed to resolve resource scope, skipping", "kind", obj.GetKind(), "name", obj.GetName())
			continue
		}
		if mapping.Scope.Name() != meta.RESTScopeNameNamespace {
			log.Info("skipping cluster-scoped resource; NamespaceClass only supports namespaced resources",
				"kind", obj.GetKind(), "name", obj.GetName())
			continue
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

		err = r.Get(ctx, client.ObjectKeyFromObject(obj), existing)
		if apierrors.IsNotFound(err) {
			if err := r.Create(ctx, obj); err != nil {
				return ctrl.Result{}, err
			}
		} else if err != nil {
			return ctrl.Result{}, err
		} else {
			// Update the existing resource, preserving the resource version to avoid conflicts.
			obj.SetResourceVersion(existing.GetResourceVersion())
			if err := r.Update(ctx, obj); err != nil {
				return ctrl.Result{}, err
			}
		}

		gvk := obj.GroupVersionKind()
		newRefs = append(newRefs, resourceRef{
			Group: gvk.Group, Version: gvk.Version, Kind: gvk.Kind, Name: obj.GetName(),
		})
	}

	newSet := map[resourceRef]bool{}
	for _, ref := range newRefs {
		newSet[ref] = true
	}
	for _, ref := range oldRefs {
		if newSet[ref] {
			continue
		}
		obj := &unstructured.Unstructured{}
		obj.SetGroupVersionKind(schema.GroupVersionKind{Group: ref.Group, Version: ref.Version, Kind: ref.Kind})
		obj.SetName(ref.Name)
		obj.SetNamespace(namespace.Name)
		if err := r.Delete(ctx, obj); err != nil && !apierrors.IsNotFound(err) {
			log.Error(err, "failed to delete stale resource", "namespace", namespace.Name, "kind", ref.Kind, "name", ref.Name)
			return ctrl.Result{}, err
		}
		log.Info("deleted stale resource", "namespace", namespace.Name, "kind", ref.Kind, "name", ref.Name)
	}

	data, err := json.Marshal(newRefs)
	if err != nil {
		return ctrl.Result{}, err
	}
	if namespace.Annotations == nil {
		namespace.Annotations = map[string]string{}
	}
	namespace.Annotations[managedResourcesAnnotation] = string(data)
	if err := r.Update(ctx, namespace); err != nil {
		log.Error(err, "failed to update ledger annotation", "namespace", namespace.Name)
		return ctrl.Result{}, err
	}
	log.Info("reconcile complete", "namespace", namespace.Name, "class", className, "ledger", string(data))
	return ctrl.Result{}, nil
}

// make sure ns that are labeled with a specific NamespaceClass are reconciled when the NamespaceClass changes.
func (r *NamespaceClassReconciler) namespacesForClass(
	ctx context.Context, obj client.Object,
) []reconcile.Request {
	className := obj.GetName()

	var nsList corev1.NamespaceList
	if err := r.List(ctx, &nsList); err != nil {
		return nil
	}

	var requests []reconcile.Request
	for _, ns := range nsList.Items {
		if ns.Labels["namespaceclass.akuity.io/name"] == className {
			requests = append(requests, reconcile.Request{
				NamespacedName: client.ObjectKey{Name: ns.Name},
			})
		}
	}
	return requests
}

// SetupWithManager sets up the controller with the Manager.
func (r *NamespaceClassReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Namespace{}).
		Watches(
			&namespaceclassv1alpha1.NamespaceClass{},
			handler.EnqueueRequestsFromMapFunc(r.namespacesForClass),
		).
		Named("namespaceclass").
		Complete(r)
}
