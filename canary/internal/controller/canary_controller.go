/*
Copyright 2024.

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
	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	canaryv1alpha1 "github.com/k8shuginn/canary-operator/api/v1alpha1"
)

const (
	Command           = "canary.k8shuginn.io/command"
	CommandApply      = "apply"
	CommandStop       = "stop"
	CommandRollback   = "rollback"
	CommandCompletion = "completion"
)

const (
	StateRunning  = "running"  // 실행 중
	StateError    = "error"    // 에러 발생 시
	StateStop     = "stop"     // 대기중, 롤백
	StateComplete = "complete" // 완료
)

const (
	AnnotationLastUpdate = "canary.k8shuginn.io/last-update"
	CanaryFinalizer      = "canary.k8shuginn.io/finalizer"
)

// CanaryReconciler reconciles a Canary object
type CanaryReconciler struct {
	client.Client
	Scheme *runtime.Scheme

	Cr *Cron
}

//+kubebuilder:rbac:groups=canary.k8shuginn.io,resources=canaries,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=canary.k8shuginn.io,resources=canaries/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=canary.k8shuginn.io,resources=canaries/finalizers,verbs=update
//+kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=events,verbs=create;patch
//+kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Canary object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.16.3/pkg/reconcile
func (r *CanaryReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Canary 리소스를 가져옵니다.
	// Canary 리소스가 없으면 삭제된 것으로 판단하고 종료합니다.
	canary := &canaryv1alpha1.Canary{}
	err := r.Get(ctx, req.NamespacedName, canary)
	if err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("[Reconcile] Canary resource not found. Ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}

		logger.Error(err, "[Reconcile] Failed to get Canary")
		return ctrl.Result{}, err
	}

	// Canary에 finalizer가 없으면 추가합니다.
	if !controllerutil.ContainsFinalizer(canary, CanaryFinalizer) {
		logger.Info("[Reconcile] Adding finalizer to the Canary")
		if ok := controllerutil.AddFinalizer(canary, CanaryFinalizer); !ok {
			logger.Error(err, "[Reconcile] Failed to add finalizer to the Canary", "namespace", req.Namespace, "name", req.Name)
			return ctrl.Result{Requeue: true}, err
		}

		if err = r.Update(ctx, canary); err != nil {
			logger.Error(err, "[Reconcile] Failed to update Canary with finalizer", "namespace", req.Namespace, "name", req.Name)
		}
		return ctrl.Result{}, err
	}

	// oldDeployment, newDeployment 정보를 가져옵니다.
	// 만약 Owner Reference가 없으면 추가합니다.
	oldDeploy, newDeploy := &appsv1.Deployment{}, &appsv1.Deployment{}
	_ = r.Get(ctx, client.ObjectKey{Namespace: req.Namespace, Name: canary.Spec.OldDeployment}, oldDeploy)
	_ = r.Get(ctx, client.ObjectKey{Namespace: req.Namespace, Name: canary.Spec.NewDeployment}, newDeploy)

	// Canary 리소스 삭제 시 finalizer 제거
	// oldDeployment, newDeployment owner reference 제거
	isToBeDeleted := canary.GetDeletionTimestamp() != nil
	if isToBeDeleted {
		r.toBeDeleted(ctx, logger, canary, oldDeploy, newDeploy)
		return ctrl.Result{}, nil
	}

	// oldDeployment, newDeployment이 없으면 에러 처리
	if msg, ok := isNotExists(oldDeploy, newDeploy); ok {
		canary.Status.OldReplicas = 0
		canary.Status.NewReplicas = 0
		canary.Status.State = StateError
		canary.Status.Message = msg
		_ = r.Status().Update(ctx, canary)
		r.Cr.Delete(req.Namespace, req.Name)
		logger.Info("[Reconcile] Deployment is not found.", "namespace", req.Namespace, "name", req.Name)
		return ctrl.Result{}, nil
	}

	// Annotation에 Command가 있으면 Command 처리
	if ok := r.applyCommand(ctx, logger, canary); ok {
		return ctrl.Result{}, nil
	}

	// Deployment replicas 동기화
	if isUpdate := r.syncDeployments(ctx, logger, canary, oldDeploy, newDeploy); isUpdate {
		logger.Info("[Reconcile] Deployment replicas are updated", "namespace", req.Namespace, "name", req.Name)
	}

	// new deployment이 crash되었을 경우 rollback
	if canary.Spec.EnableRollback {
		if isRollback := r.isCrash(ctx, logger, canary, oldDeploy, newDeploy); isRollback {
			return ctrl.Result{Requeue: true}, nil
		}
	}

	// Status Update
	if isCronDelete := r.stateUpdate(ctx, logger, canary, oldDeploy, newDeploy); isCronDelete {
		r.Cr.Delete(req.Namespace, req.Name)
		logger.Info("[Reconcile] Cron is deleted", "namespace", req.Namespace, "name", req.Name)
	}

	return ctrl.Result{}, nil
}

// stateUpdate Canary 상태를 업데이트합니다.
func (r *CanaryReconciler) stateUpdate(
	ctx context.Context,
	logger logr.Logger,
	canary *canaryv1alpha1.Canary,
	oldDeploy, newDeploy *appsv1.Deployment,
) bool {
	cronDelete := false

	_ = r.Get(ctx, client.ObjectKey{Namespace: canary.Namespace, Name: canary.Name}, canary)
	canary.Status.OldReplicas = *oldDeploy.Spec.Replicas
	canary.Status.NewReplicas = *newDeploy.Spec.Replicas
	if canary.Status.NewReplicas == canary.Spec.TotalReplicas || canary.Status.State == StateComplete {
		canary.Status.Message = "Canary is complete"
		canary.Status.State = StateComplete
		cronDelete = true
	} else if canary.Status.State == StateStop {
		cronDelete = true
	} else if canary.Status.State == StateError {
	} else if canary.Status.State == StateRunning {
		canary.Status.Message = "Canary is running"
		_ = r.Cr.Apply(canary.Namespace, canary.Name, canary.Spec.CronSchedule, canary.Spec.OldDeployment, canary.Spec.NewDeployment)
	} else {
		canary.Status.State = StateStop
		canary.Status.Message = "Canary is Pending"
		cronDelete = true
	}

	if err := r.Status().Update(ctx, canary); err != nil {
		logger.Error(err, "[Reconcile] Failed to update Canary status")
	}

	return cronDelete
}

// isCrash new deployment이 crash되었을 경우 rollback합니다.
func (r *CanaryReconciler) isCrash(
	ctx context.Context,
	logger logr.Logger,
	canary *canaryv1alpha1.Canary,
	oldDeploy, newDeploy *appsv1.Deployment,
) bool {
	podList := corev1.PodList{}
	if err := r.List(ctx, &podList, client.InNamespace(newDeploy.Namespace), client.MatchingLabels(newDeploy.Spec.Selector.MatchLabels)); err != nil {
		logger.Error(err, "[Reconcile] Failed to list Pods", "namespace", newDeploy.Namespace, "name", newDeploy.Name)
		return false
	}

	isRollback := false
LOOP:
	for _, pod := range podList.Items {
		for _, container := range pod.Status.ContainerStatuses {
			if container.RestartCount > 0 {
				isRollback = true
				break LOOP
			}
		}
	}

	if isRollback {
		// Canary 상태 변경
		if err := r.Get(ctx, client.ObjectKey{Namespace: canary.Namespace, Name: canary.Name}, canary); err != nil {
			logger.Error(err, "[Reconcile] Failed to get Canary after rollback")
		}

		canary.Status.CurrentStep = 0
		canary.Status.State = StateStop
		canary.Status.Message = fmt.Sprintf("[%s] Canary is rollbacked.", time.Now().Format(time.RFC3339))
		_ = r.Status().Update(ctx, canary)
		r.Cr.Delete(canary.Namespace, canary.Name)
		logger.Info("[Reconcile] Canary is rollbacked", "namespace", canary.Namespace, "name", canary.Name)
		return true
	}

	return false
}

// syncDeployments Deployment replicas를 동기화합니다.
func (r *CanaryReconciler) syncDeployments(
	ctx context.Context,
	logger logr.Logger,
	canary *canaryv1alpha1.Canary,
	oldDeploy, newDeploy *appsv1.Deployment,
) bool {
	// Owner만 추가되는 경우 true, Owner가 추가되지 않는 경우 false
	isOldUpdate := r.appendOwnerIfNotExists(canary, oldDeploy)
	if *oldDeploy.Spec.Replicas != canary.Spec.TotalReplicas-(canary.Spec.StepReplicas*canary.Status.CurrentStep) {
		*oldDeploy.Spec.Replicas = canary.Spec.TotalReplicas - (canary.Spec.StepReplicas * canary.Status.CurrentStep)
		isOldUpdate = true
	}
	if isOldUpdate {
		if err := r.Update(ctx, oldDeploy); err != nil {
			logger.Error(err, "[Reconcile] Failed to sync update oldDeployment", "namespace", canary.Namespace, "name", canary.Spec.OldDeployment)
		}
	}

	isNewUpdate := r.appendOwnerIfNotExists(canary, newDeploy)
	if *newDeploy.Spec.Replicas != canary.Spec.StepReplicas*canary.Status.CurrentStep {
		*newDeploy.Spec.Replicas = canary.Spec.StepReplicas * canary.Status.CurrentStep
		isNewUpdate = true
	}
	if isNewUpdate {
		if err := r.Update(ctx, newDeploy); err != nil {
			logger.Error(err, "[Reconcile] Failed to sync update newDeployment", "namespace", canary.Namespace, "name", canary.Spec.NewDeployment)
		}
	}

	return isOldUpdate || isNewUpdate
}

// appendOwnerIfNotExists Owner Reference가 없으면 Owner Reference를 추가합니다.
func (r *CanaryReconciler) appendOwnerIfNotExists(canary *canaryv1alpha1.Canary, deploy *appsv1.Deployment) bool {
	isExists := false
	for _, owner := range deploy.OwnerReferences {
		if owner.Kind == "Canary" || owner.UID == canary.UID {
			isExists = true
			break
		}
	}
	if !isExists {
		_ = controllerutil.SetControllerReference(canary, deploy, r.Scheme)
		return true
	}

	return false
}

// applyCommand Annotation에 Command가 있으면 Command 처리합니다.
func (r *CanaryReconciler) applyCommand(
	ctx context.Context,
	logger logr.Logger,
	canary *canaryv1alpha1.Canary,
) bool {
	if cmd, ok := canary.Annotations[Command]; ok {
		switch strings.ToLower(cmd) {
		case CommandApply:
			canary.Status.State = StateRunning
		case CommandRollback:
			canary.Status.CurrentStep = 0
			fallthrough
		case CommandStop:
			canary.Status.State = StateStop
			r.Cr.Delete(canary.Namespace, canary.Name)
		case CommandCompletion:
			canary.Status.State = StateComplete
			canary.Status.CurrentStep = canary.Spec.TotalReplicas / canary.Spec.StepReplicas
			r.Cr.Delete(canary.Namespace, canary.Name)
		}

		_ = r.Status().Update(ctx, canary)
		_ = r.Get(ctx, client.ObjectKey{Namespace: canary.Namespace, Name: canary.Name}, canary)
		delete(canary.Annotations, Command)
		if err := r.Update(ctx, canary); err != nil {
			logger.Error(err, "[Reconcile] Failed to update Canary with command", "namespace", canary.Namespace, "name", canary.Name)
		}

		return true
	}

	return false
}

// toBeDeleted Canary 리소스 삭제 시 finalizer 제거
func (r *CanaryReconciler) toBeDeleted(
	ctx context.Context,
	logger logr.Logger,
	canary *canaryv1alpha1.Canary,
	oldDeploy, newDeploy *appsv1.Deployment,
) {
	if !controllerutil.ContainsFinalizer(canary, CanaryFinalizer) {
		return
	}

	// oldDeployment, newDeployment owner reference 제거
	logger.Info("[Reconcile] Performing Finalizer Operations for Canary before delete CR")
	if removeOwnerReference(oldDeploy, canary.UID) {
		if err := r.Update(ctx, oldDeploy); err != nil {
			logger.Error(err, "[Reconcile] Failed to update oldDeployment delete owner reference", "namespace", canary.Namespace, "name", canary.Spec.OldDeployment)
		}
	}
	if removeOwnerReference(newDeploy, canary.UID) {
		if err := r.Update(ctx, newDeploy); err != nil {
			logger.Error(err, "[Reconcile] Failed to update newDeployment delete owner reference", "namespace", canary.Namespace, "name", canary.Spec.NewDeployment)
		}
	}

	// Canary 리소스 finalizer 제거
	if !controllerutil.RemoveFinalizer(canary, CanaryFinalizer) {
		logger.Error(nil, "[Reconcile] Failed to remove finalizer from the Canary", "namespace", canary.Namespace, "name", canary.Name)
		return
	}
	if err := r.Update(ctx, canary); err != nil {
		logger.Error(err, "[Reconcile] Failed to update Canary without finalizer", "namespace", canary.Namespace, "name", canary.Name)
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *CanaryReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&canaryv1alpha1.Canary{}).
		Owns(&appsv1.Deployment{}).
		Complete(r)
}

// isNotExists oldDeployment, newDeployment이 존재하지 않을 경우 에러 상태로 변경합니다.
func isNotExists(oldDeploy, newDeploy *appsv1.Deployment) (string, bool) {
	var message string
	if oldDeploy == nil || oldDeploy.Name == "" {
		message += "Old deployment not found. "
	}
	if newDeploy == nil || newDeploy.Name == "" {
		message += "New deployment not found. "
	}

	return message, message != ""
}

// removeOwnerReference Owner Reference를 제거합니다.
func removeOwnerReference(deploy *appsv1.Deployment, uid types.UID) bool {
	if deploy.OwnerReferences == nil {
		return false
	}

	for i, ownRefer := range deploy.OwnerReferences {
		if ownRefer.Kind == "Canary" && ownRefer.UID == uid {
			deploy.OwnerReferences = append(deploy.OwnerReferences[:i], deploy.OwnerReferences[i+1:]...)
			return true
		}
	}

	return false
}
