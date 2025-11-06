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

package v1alpha1

import (
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// EIPSpec defines the desired state of EIP
type EIPSpec struct {
	// AllocationID 指定已存在的EIP实例ID，如果指定则不会创建新的EIP
	// +optional
	AllocationID string `json:"allocationID,omitempty"`

	// Bandwidth EIP带宽，单位Mbps
	// +optional
	Bandwidth string `json:"bandwidth,omitempty"`

	// InternetChargeType 计费方式，支持PayByBandwidth和PayByTraffic
	// +kubebuilder:default:=PayByTraffic
	// +optional
	InternetChargeType string `json:"internetChargeType,omitempty"`

	// InstanceChargeType 实例计费方式，支持PrePaid和PostPaid
	// +optional
	InstanceChargeType string `json:"instanceChargeType,omitempty"`

	// ISP 线路类型
	// +optional
	ISP string `json:"isp,omitempty"`

	// PublicIPAddressPoolID 公网IP地址池ID
	// +optional
	PublicIPAddressPoolID string `json:"publicIPAddressPoolID,omitempty"`

	// ResourceGroupID 资源组ID
	// +optional
	ResourceGroupID string `json:"resourceGroupID,omitempty"`

	// Name EIP名称
	// +optional
	Name string `json:"name,omitempty"`

	// Description EIP描述
	// +optional
	Description string `json:"description,omitempty"`

	// SecurityProtectionTypes 安全防护类型
	// +optional
	SecurityProtectionTypes []string `json:"securityProtectionTypes,omitempty"`

	// Tags EIP标签
	// +optional
	Tags map[string]string `json:"tags,omitempty"`

	// BandwidthPackageID 带宽包ID
	// +optional
	BandwidthPackageID string `json:"bandwidthPackageID,omitempty"`

	// ReleaseStrategy EIP释放策略
	// +kubebuilder:validation:Enum=Never;OnDelete
	// +kubebuilder:default:=OnDelete
	ReleaseStrategy ReleaseStrategy `json:"releaseStrategy,omitempty"`
}

// ReleaseStrategy 定义EIP释放策略
// +kubebuilder:validation:Enum=Never;OnDelete
type ReleaseStrategy string

const (
	// ReleaseStrategyNever 永不释放EIP，即使删除CR也不释放
	ReleaseStrategyNever ReleaseStrategy = "Never"
	// ReleaseStrategyOnDelete 删除CR时释放EIP
	ReleaseStrategyOnDelete ReleaseStrategy = "OnDelete"
)

// EIPStatus defines the observed state of EIP
type EIPStatus struct {
	// AllocationID EIP实例ID
	AllocationID string `json:"allocationID,omitempty"`

	// EIPAddress EIP地址
	EIPAddress string `json:"eipAddress,omitempty"`

	// Status EIP状态
	Status string `json:"status,omitempty"`

	// ISP 线路类型
	ISP string `json:"isp,omitempty"`

	// InternetChargeType 计费方式
	InternetChargeType string `json:"internetChargeType,omitempty"`

	// InstanceChargeType 实例计费方式
	InstanceChargeType string `json:"instanceChargeType,omitempty"`

	// Bandwidth 带宽
	Bandwidth string `json:"bandwidth,omitempty"`

	// BandwidthPackageID 带宽包ID
	BandwidthPackageID string `json:"bandwidthPackageID,omitempty"`

	// ResourceGroupID 资源组ID
	ResourceGroupID string `json:"resourceGroupID,omitempty"`

	// Name EIP名称
	Name string `json:"name,omitempty"`

	// PublicIPAddressPoolID 公网IP地址池ID
	PublicIPAddressPoolID string `json:"publicIPAddressPoolID,omitempty"`

	// Description EIP描述
	Description string `json:"description,omitempty"`

	// Conditions EIP状态条件
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// LastSyncTime 最后同步时间
	LastSyncTime *metav1.Time `json:"lastSyncTime,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:resource:shortName=eip
//+kubebuilder:printcolumn:name="AllocationID",type=string,JSONPath=`.status.allocationID`
//+kubebuilder:printcolumn:name="EIP Address",type=string,JSONPath=`.status.eipAddress`
//+kubebuilder:printcolumn:name="Status",type=string,JSONPath=`.status.status`
//+kubebuilder:printcolumn:name="Bandwidth",type=string,JSONPath=`.status.bandwidth`
//+kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// EIP is the Schema for the eips API
type EIP struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   EIPSpec   `json:"spec,omitempty"`
	Status EIPStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// EIPList contains a list of EIP
type EIPList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []EIP `json:"items"`
}

func init() {
	SchemeBuilder.Register(&EIP{}, &EIPList{})
}

var eiplog = logf.Log.WithName("eip-resource")

// SetupWebhookWithManager sets up the webhook with the Manager.
func (r *EIP) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		Complete()
}

//+kubebuilder:webhook:path=/validate-eip-alibabacloud-com-v1alpha1-eip,mutating=false,failurePolicy=fail,sideEffects=None,groups=eip.alibabacloud.com,resources=eips,verbs=create;update,versions=v1alpha1,name=veip.kb.io,admissionReviewVersions=v1

var _ webhook.Validator = &EIP{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *EIP) ValidateCreate() (admission.Warnings, error) {
	eiplog.Info("validate create", "name", r.Name)
	return nil, r.validateEIP()
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *EIP) ValidateUpdate(old runtime.Object) (admission.Warnings, error) {
	eiplog.Info("validate update", "name", r.Name)
	return nil, r.validateEIP()
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *EIP) ValidateDelete() (admission.Warnings, error) {
	eiplog.Info("validate delete", "name", r.Name)
	return nil, nil
}

// validateEIP validates the EIP configuration
func (r *EIP) validateEIP() error {
	var allErrs field.ErrorList

	// 校验单线 EIP 必须使用按固定带宽付费
	if err := r.validateSingleLineISP(); err != nil {
		allErrs = append(allErrs, err)
	}

	// 校验实例计费类型与流量计费类型的关系
	if err := r.validateInstanceChargeType(); err != nil {
		allErrs = append(allErrs, err)
	}

	// 校验带宽值
	if err := r.validateBandwidth(); err != nil {
		allErrs = append(allErrs, err)
	}

	if len(allErrs) == 0 {
		return nil
	}

	return apierrors.NewInvalid(
		schema.GroupKind{Group: "eip.alibabacloud.com", Kind: "EIP"},
		r.Name,
		allErrs,
	)
}

// validateSingleLineISP 校验单线 ISP 不能使用包年包月，只能使用按带宽付费
func (r *EIP) validateSingleLineISP() *field.Error {
	// 单线 ISP 类型列表
	singleLineISPs := map[string]bool{
		"ChinaTelecom": true,
		"ChinaUnicom":  true,
		"ChinaMobile":  true,
	}

	// 如果使用单线 ISP
	if r.Spec.ISP != "" && singleLineISPs[r.Spec.ISP] {
		// 检查实例计费类型
		instanceChargeType := r.Spec.InstanceChargeType
		if instanceChargeType == "" {
			instanceChargeType = "PostPaid" // 默认值
		}

		// 单线 ISP 不能使用包年包月（PrePaid）
		if instanceChargeType == "PrePaid" {
			return field.Invalid(
				field.NewPath("spec").Child("instanceChargeType"),
				instanceChargeType,
				fmt.Sprintf("单线 ISP (%s) 不支持包年包月 (PrePaid)，只能使用后付费 (PostPaid)", r.Spec.ISP),
			)
		}

		// 检查流量计费方式
		internetChargeType := r.Spec.InternetChargeType
		if internetChargeType == "" {
			internetChargeType = "PayByTraffic" // 默认值
		}

		// 单线 ISP 只能使用按带宽付费
		if internetChargeType != "PayByBandwidth" {
			return field.Invalid(
				field.NewPath("spec").Child("internetChargeType"),
				internetChargeType,
				fmt.Sprintf("单线 ISP (%s) 只支持按固定带宽付费 (PayByBandwidth)，不支持按流量付费 (PayByTraffic)", r.Spec.ISP),
			)
		}
	}

	return nil
}

// validateInstanceChargeType 校验实例计费类型与流量计费类型的关系
func (r *EIP) validateInstanceChargeType() *field.Error {
	// 获取实例计费类型，默认为 PostPaid
	instanceChargeType := r.Spec.InstanceChargeType
	if instanceChargeType == "" {
		instanceChargeType = "PostPaid"
	}

	// 获取流量计费类型，默认为 PayByTraffic
	internetChargeType := r.Spec.InternetChargeType
	if internetChargeType == "" {
		internetChargeType = "PayByTraffic"
	}

	// 当实例计费类型为 PrePaid（预付费）时，流量计费类型必须为 PayByBandwidth
	if instanceChargeType == "PrePaid" && internetChargeType != "PayByBandwidth" {
		return field.Invalid(
			field.NewPath("spec").Child("internetChargeType"),
			internetChargeType,
			fmt.Sprintf("当 InstanceChargeType 为 PrePaid（预付费）时，InternetChargeType 必须为 PayByBandwidth，不能为 %s", internetChargeType),
		)
	}

	return nil
}

// validateBandwidth 校验带宽值
func (r *EIP) validateBandwidth() *field.Error {
	// 如果使用按带宽付费，必须指定带宽
	if r.Spec.InternetChargeType == "PayByBandwidth" && r.Spec.Bandwidth == "" {
		return field.Required(
			field.NewPath("spec").Child("bandwidth"),
			"使用按带宽付费 (PayByBandwidth) 时必须指定带宽值",
		)
	}

	return nil
}
