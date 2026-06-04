/*
Copyright 2025.

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
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	eipv1alpha1 "github.com/chrisliu1995/alibabacloud-eip-operator/api/v1alpha1"
	aliyunclient "github.com/chrisliu1995/alibabacloud-eip-operator/pkg/aliyun"
)

const (
	eipFinalizer = "eip.alibabacloud.com/finalizer"

	// Condition types
	conditionTypeReady       = "Ready"
	conditionTypeSynced      = "Synced"
	conditionTypeProgressing = "Progressing"

	// Reasons
	reasonCreating   = "Creating"
	reasonCreated    = "Created"
	reasonUpdating   = "Updating"
	reasonUpdated    = "Updated"
	reasonDeleting   = "Deleting"
	reasonDeleted    = "Deleted"
	reasonSyncFailed = "SyncFailed"
	reasonThrottled  = "Throttled"
)

const (
	eipCtrlRequeueAfter         = 30 * time.Second
	eipCtrlRequeueAfterThrottle = 2 * time.Minute // 流控时使用更长的重试间隔
)

// isThrottlingError 检查是否为流控错误
func isThrottlingError(err error) bool {
	if err == nil {
		return false
	}
	errMsg := err.Error()
	return strings.Contains(errMsg, "Throttling.User") ||
		strings.Contains(errMsg, "Throttling.Api") ||
		strings.Contains(errMsg, "RequestLimitExceeded")
}

// isEIPNotFoundError 检查是否为 EIP 不存在错误
func isEIPNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	errMsg := err.Error()
	return strings.Contains(errMsg, "InvalidAllocationId.NotFound") ||
		strings.Contains(errMsg, "InvalidAllocationID.NotFound") ||
		strings.Contains(errMsg, "Specified allocation ID is not found")
}

// EIPReconciler reconciles a EIP object
type EIPReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	Record record.EventRecorder
	Aliyun aliyunclient.API
}

//+kubebuilder:rbac:groups=eip.alibabacloud.com,resources=eips,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=eip.alibabacloud.com,resources=eips/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=eip.alibabacloud.com,resources=eips/finalizers,verbs=update
//+kubebuilder:rbac:groups="",resources=events,verbs=create;patch

// Reconcile is part of the main kubernetes reconciliation loop
func (r *EIPReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	l := log.FromContext(ctx)

	// Fetch the EIP instance
	eip := &eipv1alpha1.EIP{}
	err := r.Get(ctx, req.NamespacedName, eip)
	if err != nil {
		if errors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return ctrl.Result{}, err
	}

	// Check if the EIP instance is marked to be deleted.
	// finalizeEIP 内部根据云端状态决定何时移除 finalizer：
	// 仅当云端 EIP 已不存在或已成功释放后才移除，InUse 时只 Requeue 等待。
	if !eip.ObjectMeta.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(eip, eipFinalizer) {
			return r.finalizeEIP(ctx, eip)
		}
		return ctrl.Result{}, nil
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(eip, eipFinalizer) {
		controllerutil.AddFinalizer(eip, eipFinalizer)
		err = r.Update(ctx, eip)
		if err != nil {
			return ctrl.Result{}, err
		}
	}

	// Reconcile EIP
	result, err := r.reconcileEIP(ctx, eip)
	if err != nil {
		l.Error(err, "failed to reconcile EIP")
		r.Record.Eventf(eip, "Warning", "ReconcileFailed", "Failed to reconcile EIP: %v", err)
		return result, err
	}

	return result, nil
}

// reconcileEIP handles the main reconciliation logic
func (r *EIPReconciler) reconcileEIP(ctx context.Context, eip *eipv1alpha1.EIP) (ctrl.Result, error) {
	l := log.FromContext(ctx)

	// If AllocationID is not set, create a new EIP
	if eip.Spec.AllocationID == "" {
		// Check if we already have an allocation ID in status
		if eip.Status.AllocationID != "" {
			eip.Spec.AllocationID = eip.Status.AllocationID
			if err := r.Update(ctx, eip); err != nil {
				return ctrl.Result{}, err
			}
		} else {
			// Create new EIP
			l.Info("creating new EIP")
			r.setCondition(eip, conditionTypeProgressing, metav1.ConditionTrue, reasonCreating, "Creating new EIP")
			if err := r.updateStatus(ctx, eip); err != nil {
				return ctrl.Result{}, err
			}

			allocationID, err := r.createEIP(ctx, eip)
			if err != nil {
				// 检查是否为流控错误
				if isThrottlingError(err) {
					l.Info("API throttled, will retry later")
					r.setCondition(eip, conditionTypeReady, metav1.ConditionFalse, reasonThrottled, "API throttled, retrying later")
					r.Record.Eventf(eip, "Warning", "Throttled", "API request throttled, will retry in %v", eipCtrlRequeueAfterThrottle)
					_ = r.updateStatus(ctx, eip)
					return ctrl.Result{RequeueAfter: eipCtrlRequeueAfterThrottle}, nil
				}
				r.setCondition(eip, conditionTypeReady, metav1.ConditionFalse, reasonSyncFailed, fmt.Sprintf("Failed to create EIP: %v", err))
				_ = r.updateStatus(ctx, eip)
				return ctrl.Result{RequeueAfter: eipCtrlRequeueAfter}, err
			}

			eip.Spec.AllocationID = allocationID
			eip.Status.AllocationID = allocationID
			if err := r.Update(ctx, eip); err != nil {
				return ctrl.Result{}, err
			}

			r.Record.Eventf(eip, "Normal", "Created", "Created EIP with AllocationID: %s", allocationID)
			r.setCondition(eip, conditionTypeProgressing, metav1.ConditionFalse, reasonCreated, "EIP created successfully")
		}
	}

	// Sync EIP status from Aliyun
	if err := r.syncEIPStatus(ctx, eip); err != nil {
		// 检查是否为流控错误
		if isThrottlingError(err) {
			l.Info("API throttled during status sync, will retry later")
			r.Record.Eventf(eip, "Warning", "Throttled", "API request throttled during sync, will retry in %v", eipCtrlRequeueAfterThrottle)
			return ctrl.Result{RequeueAfter: eipCtrlRequeueAfterThrottle}, nil
		}
		r.setCondition(eip, conditionTypeReady, metav1.ConditionFalse, reasonSyncFailed, fmt.Sprintf("Failed to sync EIP status: %v", err))
		_ = r.updateStatus(ctx, eip)
		return ctrl.Result{RequeueAfter: eipCtrlRequeueAfter}, err
	}

	// Update bandwidth if needed
	if eip.Spec.Bandwidth != "" && eip.Status.Bandwidth != eip.Spec.Bandwidth && eip.Status.BandwidthPackageID == "" {
		l.Info("updating EIP bandwidth", "from", eip.Status.Bandwidth, "to", eip.Spec.Bandwidth)
		r.setCondition(eip, conditionTypeProgressing, metav1.ConditionTrue, reasonUpdating, "Updating EIP bandwidth")
		if err := r.updateStatus(ctx, eip); err != nil {
			return ctrl.Result{}, err
		}

		if err := r.Aliyun.ModifyEipAddressAttribute(ctx, eip.Spec.AllocationID, eip.Spec.Bandwidth); err != nil {
			r.setCondition(eip, conditionTypeReady, metav1.ConditionFalse, reasonSyncFailed, fmt.Sprintf("Failed to update bandwidth: %v", err))
			_ = r.updateStatus(ctx, eip)
			return ctrl.Result{RequeueAfter: eipCtrlRequeueAfter}, err
		}

		r.Record.Eventf(eip, "Normal", "Updated", "Updated EIP bandwidth to %s", eip.Spec.Bandwidth)
		r.setCondition(eip, conditionTypeProgressing, metav1.ConditionFalse, reasonUpdated, "EIP bandwidth updated")
	}

	// Handle bandwidth package
	if eip.Spec.BandwidthPackageID != "" {
		if eip.Status.BandwidthPackageID != eip.Spec.BandwidthPackageID {
			// Remove from old package if exists
			if eip.Status.BandwidthPackageID != "" {
				l.Info("removing EIP from bandwidth package", "packageID", eip.Status.BandwidthPackageID)
				if err := r.Aliyun.RemoveCommonBandwidthPackageIP(ctx, eip.Spec.AllocationID, eip.Status.BandwidthPackageID); err != nil {
					l.Error(err, "failed to remove EIP from bandwidth package")
				}
			}

			// Add to new package
			l.Info("adding EIP to bandwidth package", "packageID", eip.Spec.BandwidthPackageID)
			if err := r.Aliyun.AddCommonBandwidthPackageIP(ctx, eip.Spec.AllocationID, eip.Spec.BandwidthPackageID); err != nil {
				r.setCondition(eip, conditionTypeReady, metav1.ConditionFalse, reasonSyncFailed, fmt.Sprintf("Failed to add to bandwidth package: %v", err))
				_ = r.updateStatus(ctx, eip)
				return ctrl.Result{RequeueAfter: eipCtrlRequeueAfter}, err
			}

			r.Record.Eventf(eip, "Normal", "Updated", "Added EIP to bandwidth package: %s", eip.Spec.BandwidthPackageID)
		}
	} else if eip.Status.BandwidthPackageID != "" {
		// Remove from bandwidth package
		l.Info("removing EIP from bandwidth package", "packageID", eip.Status.BandwidthPackageID)
		if err := r.Aliyun.RemoveCommonBandwidthPackageIP(ctx, eip.Spec.AllocationID, eip.Status.BandwidthPackageID); err != nil {
			l.Error(err, "failed to remove EIP from bandwidth package")
		}
	}

	// Re-sync status
	if err := r.syncEIPStatus(ctx, eip); err != nil {
		return ctrl.Result{RequeueAfter: eipCtrlRequeueAfter}, err
	}

	// Set Ready condition
	r.setCondition(eip, conditionTypeReady, metav1.ConditionTrue, "Available", "EIP is ready")
	r.setCondition(eip, conditionTypeSynced, metav1.ConditionTrue, "Synced", "EIP synced successfully")
	if err := r.updateStatus(ctx, eip); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{RequeueAfter: 5 * time.Minute}, nil
}

// createEIP creates a new EIP instance
func (r *EIPReconciler) createEIP(ctx context.Context, eip *eipv1alpha1.EIP) (string, error) {
	l := log.FromContext(ctx)

	opts := &aliyunclient.EIPOptions{
		InternetChargeType:      eip.Spec.InternetChargeType,
		Bandwidth:               eip.Spec.Bandwidth,
		ISP:                     eip.Spec.ISP,
		InstanceChargeType:      eip.Spec.InstanceChargeType,
		PublicIPAddressPoolID:   eip.Spec.PublicIPAddressPoolID,
		ResourceGroupID:         eip.Spec.ResourceGroupID,
		Name:                    eip.Spec.Name,
		Description:             eip.Spec.Description,
		SecurityProtectionTypes: eip.Spec.SecurityProtectionTypes,
	}

	if opts.InternetChargeType == "" {
		opts.InternetChargeType = "PayByTraffic"
	}
	if opts.Description == "" {
		opts.Description = "created by alibabacloud-eip-operator"
	}

	eipAddr, err := r.Aliyun.AllocateEipAddress(ctx, opts)
	if err != nil {
		l.Error(err, "failed to allocate EIP")
		return "", err
	}

	l.Info("EIP created", "allocationID", eipAddr.AllocationID)

	// Tag the EIP if tags are specified
	if len(eip.Spec.Tags) > 0 {
		if err := r.Aliyun.TagResources(ctx, "EIP", []string{eipAddr.AllocationID}, eip.Spec.Tags); err != nil {
			l.Error(err, "failed to tag EIP", "allocationID", eipAddr.AllocationID)
			// Don't fail the reconciliation for tagging errors
		}
	}

	return eipAddr.AllocationID, nil
}

// syncEIPStatus syncs the EIP status from Aliyun
func (r *EIPReconciler) syncEIPStatus(ctx context.Context, eip *eipv1alpha1.EIP) error {
	l := log.FromContext(ctx)

	if eip.Spec.AllocationID == "" {
		return nil
	}

	eips, err := r.Aliyun.DescribeEipAddresses(ctx, eip.Spec.AllocationID, "", "", "")
	if err != nil {
		l.Error(err, "failed to describe EIP")
		return err
	}

	if len(eips) != 1 {
		return fmt.Errorf("expected 1 EIP, got %d", len(eips))
	}

	eipInfo := eips[0]

	// Update status
	eip.Status.AllocationID = eipInfo.AllocationID
	eip.Status.EIPAddress = eipInfo.IPAddress
	eip.Status.Status = eipInfo.Status
	eip.Status.ISP = eipInfo.ISP
	eip.Status.InternetChargeType = eipInfo.InternetChargeType
	eip.Status.InstanceChargeType = eipInfo.ChargeType
	eip.Status.Bandwidth = eipInfo.Bandwidth
	eip.Status.BandwidthPackageID = eipInfo.BandwidthPackageID
	eip.Status.ResourceGroupID = eipInfo.ResourceGroupID
	eip.Status.Name = eipInfo.Name
	eip.Status.PublicIPAddressPoolID = eipInfo.PublicIPAddressPoolID
	eip.Status.Description = eipInfo.Description
	eip.Status.SecurityProtectionTypes = eipInfo.SecurityProtectionTypes

	now := metav1.Now()
	eip.Status.LastSyncTime = &now

	return r.updateStatus(ctx, eip)
}

// finalizeEIP handles cleanup when EIP is being deleted.
// 关键约束：通过 NLB ZoneMappings 绑定的 EIP 无法通过 UnassociateEipAddress 强行解绑
// （会报 IncorrectEipStatus），只有删除 NLB 后 EIP 才会自动变 Available。
// 因此 InUse 时只 Requeue 等待上游资源释放，绝不强行 Unassociate。
//
// 流程：
//  1. 状态置 Deleting；
//  2. ReleaseStrategy=Never 或从未分配 -> 直接移除 finalizer；
//  3. DescribeEipAddresses 查询云端：NotFound 直接移除 finalizer；
//  4. 根据云端 EIP 状态：
//     - InUse     -> Requeue 等待，不调 Unassociate；
//     - Available -> 释放 EIP 后移除 finalizer；
//     - 其它      -> Requeue 等待稳定。
func (r *EIPReconciler) finalizeEIP(ctx context.Context, eip *eipv1alpha1.EIP) (ctrl.Result, error) {
	l := log.FromContext(ctx)

	r.setCondition(eip, conditionTypeProgressing, metav1.ConditionTrue, reasonDeleting, "Deleting EIP")
	_ = r.updateStatus(ctx, eip)

	// 1. 保留 ReleaseStrategy=Never 的语义：不释放云端 EIP，仅移除 finalizer
	if eip.Spec.ReleaseStrategy != eipv1alpha1.ReleaseStrategyOnDelete {
		l.Info("skipping EIP release", "releaseStrategy", eip.Spec.ReleaseStrategy)
		r.Record.Event(eip, "Normal", "Skipped", "Skipped EIP release due to ReleaseStrategy")
		return r.removeEIPFinalizer(ctx, eip)
	}

	// 2. 从未分配成功，直接放行
	if eip.Status.AllocationID == "" {
		return r.removeEIPFinalizer(ctx, eip)
	}

	// 3. 查询云端当前状态
	eips, err := r.Aliyun.DescribeEipAddresses(ctx, eip.Status.AllocationID, "", "", "")
	if err != nil {
		if isEIPNotFoundError(err) {
			l.Info("EIP not found in cloud, removing finalizer", "allocationID", eip.Status.AllocationID)
			r.Record.Eventf(eip, "Normal", "AlreadyReleased", "EIP not found (already released): %s", eip.Status.AllocationID)
			return r.removeEIPFinalizer(ctx, eip)
		}
		if isThrottlingError(err) {
			l.Info("API throttled while describing EIP during finalize, will retry later")
			r.Record.Eventf(eip, "Warning", "Throttled", "API throttled during finalize, retry in %v", eipCtrlRequeueAfterThrottle)
			return ctrl.Result{RequeueAfter: eipCtrlRequeueAfterThrottle}, nil
		}
		l.Error(err, "failed to describe EIP during finalize")
		return ctrl.Result{RequeueAfter: eipCtrlRequeueAfter}, err
	}
	if len(eips) == 0 {
		l.Info("EIP not in cloud response, treat as released", "allocationID", eip.Status.AllocationID)
		return r.removeEIPFinalizer(ctx, eip)
	}

	cloudEIP := eips[0]
	switch cloudEIP.Status {
	case "InUse":
		// 不强行 Unassociate：通过 NLB ZoneMappings 绑定的 EIP 必须等 NLB 删除后才会变 Available
		l.Info("EIP is InUse, waiting for associated resource to be deleted",
			"allocationID", eip.Status.AllocationID, "instanceId", cloudEIP.InstanceID, "instanceType", cloudEIP.InstanceType)
		r.Record.Eventf(eip, "Normal", "WaitingForRelease",
			"EIP is InUse (instance=%s/%s), waiting for associated resource to be deleted",
			cloudEIP.InstanceType, cloudEIP.InstanceID)
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil

	case "Available":
		l.Info("releasing EIP", "allocationID", eip.Status.AllocationID)
		// 先从共享带宽包移除（若有）
		if eip.Status.BandwidthPackageID != "" {
			if err := r.Aliyun.RemoveCommonBandwidthPackageIP(ctx, eip.Status.AllocationID, eip.Status.BandwidthPackageID); err != nil {
				if !isEIPNotFoundError(err) {
					l.Error(err, "failed to remove EIP from bandwidth package")
				}
			}
		}

		if err := r.Aliyun.ReleaseEIPAddress(ctx, eip.Status.AllocationID); err != nil {
			if isEIPNotFoundError(err) {
				l.Info("EIP not found during release, assuming already released", "allocationID", eip.Status.AllocationID)
				r.Record.Eventf(eip, "Normal", "AlreadyReleased", "EIP not found (already released): %s", eip.Status.AllocationID)
				return r.removeEIPFinalizer(ctx, eip)
			}
			if isThrottlingError(err) {
				l.Info("API throttled while releasing EIP, will retry later")
				r.Record.Eventf(eip, "Warning", "Throttled", "API throttled while releasing, retry in %v", eipCtrlRequeueAfterThrottle)
				return ctrl.Result{RequeueAfter: eipCtrlRequeueAfterThrottle}, nil
			}
			l.Error(err, "failed to release EIP")
			r.Record.Eventf(eip, "Warning", "ReleaseFailed", "Failed to release EIP: %v", err)
			return ctrl.Result{RequeueAfter: eipCtrlRequeueAfter}, err
		}
		r.Record.Eventf(eip, "Normal", "Released", "Released EIP: %s", eip.Status.AllocationID)
		l.Info("EIP released", "allocationID", eip.Status.AllocationID)
		return r.removeEIPFinalizer(ctx, eip)

	default:
		// Associating / Unassociating / Releasing 等中间态，等待稳定
		l.Info("EIP in transient state, waiting", "allocationID", eip.Status.AllocationID, "status", cloudEIP.Status)
		r.Record.Eventf(eip, "Normal", "WaitingForStable",
			"EIP status is %q, waiting before release", cloudEIP.Status)
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}
}

// removeEIPFinalizer 标记 progressing=false 并移除 finalizer，提交 CR 更新。
func (r *EIPReconciler) removeEIPFinalizer(ctx context.Context, eip *eipv1alpha1.EIP) (ctrl.Result, error) {
	r.setCondition(eip, conditionTypeProgressing, metav1.ConditionFalse, reasonDeleted, "EIP deleted")
	_ = r.updateStatus(ctx, eip)

	controllerutil.RemoveFinalizer(eip, eipFinalizer)
	if err := r.Update(ctx, eip); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

// setCondition sets a condition on the EIP
func (r *EIPReconciler) setCondition(eip *eipv1alpha1.EIP, conditionType string, status metav1.ConditionStatus, reason, message string) {
	condition := metav1.Condition{
		Type:               conditionType,
		Status:             status,
		ObservedGeneration: eip.Generation,
		LastTransitionTime: metav1.Now(),
		Reason:             reason,
		Message:            message,
	}

	apimeta.SetStatusCondition(&eip.Status.Conditions, condition)
}

// updateStatus updates the EIP status
func (r *EIPReconciler) updateStatus(ctx context.Context, eip *eipv1alpha1.EIP) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		return r.Status().Update(ctx, eip)
	})
}

// SetupWithManager sets up the controller with the Manager.
func (r *EIPReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&eipv1alpha1.EIP{}).
		Complete(r)
}
