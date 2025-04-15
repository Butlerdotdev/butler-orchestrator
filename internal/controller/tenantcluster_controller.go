package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	corev1alpha1 "github.com/Butlerdotdev/butler-orchestrator/api/v1alpha1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

const tenantClusterFinalizer = "core.butler.sh/finalizer"

const (
	ConditionReady       = "Ready"
	ConditionProvisioned = "Provisioned"
	ConditionError       = "Error"
)

type TenantClusterReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

//+kubebuilder:rbac:groups=core.butler.sh,resources=tenantclusters,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core.butler.sh,resources=tenantclusters/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=core.butler.sh,resources=tenantclusters/finalizers,verbs=update
//+kubebuilder:rbac:groups=*,resources=*,verbs=get;list;watch;create

func (r *TenantClusterReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx).WithValues("tenantcluster", req.NamespacedName)

	var tenantCluster corev1alpha1.TenantCluster
	if err := r.Get(ctx, req.NamespacedName, &tenantCluster); err != nil {
		if errors.IsNotFound(err) {
			log.Info("TenantCluster not found. Ignoring since object must have been deleted.")
			return ctrl.Result{}, nil
		}
		log.Error(err, "Failed to get TenantCluster")
		return ctrl.Result{}, err
	}

	// Add finalizer
	if tenantCluster.ObjectMeta.DeletionTimestamp.IsZero() {
		if !controllerutil.ContainsFinalizer(&tenantCluster, tenantClusterFinalizer) {
			controllerutil.AddFinalizer(&tenantCluster, tenantClusterFinalizer)
			if err := r.Update(ctx, &tenantCluster); err != nil {
				log.Error(err, "Failed to add finalizer")
				return ctrl.Result{}, err
			}
			log.Info("Added finalizer")
		}
	} else {
		// Handle deletion
		log.Info("TenantCluster is being deleted")
		controllerutil.RemoveFinalizer(&tenantCluster, tenantClusterFinalizer)
		if err := r.Update(ctx, &tenantCluster); err != nil {
			log.Error(err, "Failed to remove finalizer")
			return ctrl.Result{}, err
		}
		log.Info("Finalizer removed, deletion complete")
		return ctrl.Result{}, nil
	}

	if tenantCluster.Status.Phase == "" {
		tenantCluster.Status.Phase = corev1alpha1.PhasePending
		if err := r.Status().Update(ctx, &tenantCluster); err != nil {
			log.Error(err, "Failed to initialize status.phase")
			return ctrl.Result{}, err
		}
		log.Info("Initialized phase to Pending")
	}

	// Resolve provider CR
	providerRef := tenantCluster.Spec.Provider.Ref
	provider := &unstructured.Unstructured{}
	provider.SetAPIVersion(providerRef.APIVersion)
	provider.SetKind(providerRef.Kind)

	namespacedName := types.NamespacedName{
		Name:      providerRef.Name,
		Namespace: tenantCluster.Namespace,
	}

	if err := r.Client.Get(ctx, namespacedName, provider); err != nil {
		if errors.IsNotFound(err) {
			log.Info("Provider CR not found, creating", "ref", providerRef)

			// Decode providerConfig to a map
			// Decode providerConfig to a map
			var providerSpec map[string]interface{}
			if len(tenantCluster.Spec.ProviderConfig.Raw) > 0 {
				if err := json.Unmarshal(tenantCluster.Spec.ProviderConfig.Raw, &providerSpec); err != nil {
					log.Error(err, "Failed to decode providerConfig JSON into map")
					r.Recorder.Event(&tenantCluster, "Warning", "InvalidProviderConfig", fmt.Sprintf("Unable to decode providerConfig: %v", err))
					tenantCluster.Status.Phase = corev1alpha1.PhaseError
					_ = r.Status().Update(ctx, &tenantCluster)
					return ctrl.Result{}, err
				}
			} else {
				providerSpec = map[string]interface{}{}
			}

			provider.Object = map[string]interface{}{
				"apiVersion": providerRef.APIVersion,
				"kind":       providerRef.Kind,
				"metadata": map[string]interface{}{
					"name":      providerRef.Name,
					"namespace": tenantCluster.Namespace,
				},
				"spec": providerSpec,
			}

			if err := r.Client.Create(ctx, provider); err != nil {
				log.Error(err, "Failed to create provider CR", "ref", providerRef)
				r.Recorder.Event(&tenantCluster, "Warning", "CreateFailed", fmt.Sprintf("Failed to create provider CR %s: %v", providerRef.Name, err))
				return ctrl.Result{}, err
			}

			log.Info("Created provider CR", "ref", providerRef)
			r.Recorder.Event(&tenantCluster, "Normal", "Created", fmt.Sprintf("Created provider CR %s", providerRef.Name))
			return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
		}
		log.Error(err, "Failed to dereference provider CR", "ref", providerRef)
		r.Recorder.Event(&tenantCluster, "Warning", "DereferenceFailed", fmt.Sprintf("Failed to get provider CR: %v", err))
		tenantCluster.Status.Phase = corev1alpha1.PhaseError
		_ = r.Status().Update(ctx, &tenantCluster)
		return ctrl.Result{}, fmt.Errorf("unable to fetch provider CR: %w", err)
	}

	log.Info("Successfully dereferenced provider CR", "ref", providerRef)

	// Read phase from provider.status.phase
	providerStatusPhase, found, err := unstructured.NestedString(provider.Object, "status", "phase")
	if err != nil || !found {
		log.Info("Provider status.phase not available yet", "ref", providerRef)
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}

	log.Info("Provider reported status.phase", "phase", providerStatusPhase)

	var conditionType string
	var conditionStatus metav1.ConditionStatus
	var reason string

	switch providerStatusPhase {
	case "Ready":
		tenantCluster.Status.Phase = corev1alpha1.PhaseReady
		conditionType = ConditionReady
		conditionStatus = metav1.ConditionTrue
		reason = "ProviderClusterReady"
	case "Provisioning":
		tenantCluster.Status.Phase = corev1alpha1.PhaseProvisioning
		conditionType = ConditionProvisioned
		conditionStatus = metav1.ConditionFalse
		reason = "ProvisioningInProgress"
	case "Error":
		tenantCluster.Status.Phase = corev1alpha1.PhaseError
		conditionType = ConditionError
		conditionStatus = metav1.ConditionTrue
		reason = "ProviderClusterErrored"
	default:
		tenantCluster.Status.Phase = corev1alpha1.PhasePending
		conditionType = "Unknown"
		conditionStatus = metav1.ConditionUnknown
		reason = "UnknownStatus"
	}

	replaceOrAppendCondition(&tenantCluster.Status.Conditions, metav1.Condition{
		Type:    conditionType,
		Status:  conditionStatus,
		Reason:  reason,
		Message: fmt.Sprintf("Provider reported phase: %s", providerStatusPhase),
	})

	if err := r.Status().Update(ctx, &tenantCluster); err != nil {
		log.Error(err, "Failed to update TenantCluster status.phase")
		return ctrl.Result{}, err
	}

	r.Recorder.Event(&tenantCluster, "Normal", "PhaseUpdated", fmt.Sprintf("TenantCluster marked as %s", tenantCluster.Status.Phase))
	return ctrl.Result{}, nil
}

func (r *TenantClusterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1alpha1.TenantCluster{}).
		Named("tenantcluster").
		Complete(r)
}

func replaceOrAppendCondition(conditions *[]metav1.Condition, newCond metav1.Condition) {
	now := metav1.Now()
	for i, cond := range *conditions {
		if cond.Type == newCond.Type {
			(*conditions)[i] = newCond
			(*conditions)[i].LastTransitionTime = now
			return
		}
	}
	newCond.LastTransitionTime = now
	*conditions = append(*conditions, newCond)
}
