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

	// Check if the EIP instance is marked to be deleted
	if !eip.ObjectMeta.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(eip, eipFinalizer) {
			// Run finalization logic
			if err := r.finalizeEIP(ctx, eip); err != nil {
				return ctrl.Result{}, err
			}

			// Remove finalizer
			controllerutil.RemoveFinalizer(eip, eipFinalizer)
			err := r.Update(ctx, eip)
			if err != nil {
				return ctrl.Result{}, err
			}
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

	now := metav1.Now()
	eip.Status.LastSyncTime = &now

	return r.updateStatus(ctx, eip)
}

// finalizeEIP handles cleanup when EIP is being deleted
func (r *EIPReconciler) finalizeEIP(ctx context.Context, eip *eipv1alpha1.EIP) error {
	l := log.FromContext(ctx)

	r.setCondition(eip, conditionTypeProgressing, metav1.ConditionTrue, reasonDeleting, "Deleting EIP")
	_ = r.updateStatus(ctx, eip)

	// Only release EIP if ReleaseStrategy is OnDelete and it was created by operator
	if eip.Spec.ReleaseStrategy == eipv1alpha1.ReleaseStrategyOnDelete && eip.Status.AllocationID != "" {
		l.Info("releasing EIP", "allocationID", eip.Status.AllocationID)

		// Remove from bandwidth package first if needed
		if eip.Status.BandwidthPackageID != "" {
			if err := r.Aliyun.RemoveCommonBandwidthPackageIP(ctx, eip.Status.AllocationID, eip.Status.BandwidthPackageID); err != nil {
				// 如果 EIP 不存在，忽略错误
				if !isEIPNotFoundError(err) {
					l.Error(err, "failed to remove EIP from bandwidth package")
				}
				// Continue anyway
			}
		}

		if err := r.Aliyun.ReleaseEIPAddress(ctx, eip.Status.AllocationID); err != nil {
			// 如果 EIP 已经不存在，认为释放成功
			if isEIPNotFoundError(err) {
				l.Info("EIP not found, assuming already released", "allocationID", eip.Status.AllocationID)
				r.Record.Eventf(eip, "Normal", "AlreadyReleased", "EIP not found (already released): %s", eip.Status.AllocationID)
			} else {
				l.Error(err, "failed to release EIP")
				r.Record.Eventf(eip, "Warning", "ReleaseFailed", "Failed to release EIP: %v", err)
				return err
			}
		} else {
			r.Record.Eventf(eip, "Normal", "Released", "Released EIP: %s", eip.Status.AllocationID)
			l.Info("EIP released", "allocationID", eip.Status.AllocationID)
		}
	} else {
		l.Info("skipping EIP release", "releaseStrategy", eip.Spec.ReleaseStrategy)
		r.Record.Event(eip, "Normal", "Skipped", "Skipped EIP release due to ReleaseStrategy")
	}

	r.setCondition(eip, conditionTypeProgressing, metav1.ConditionFalse, reasonDeleted, "EIP deleted")
	return nil
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
