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

package aliyun

import (
	"context"
)

// API 阿里云VPC API接口
type API interface {
	// EIP相关接口
	AllocateEipAddress(ctx context.Context, opts *EIPOptions) (*EIPAddress, error)
	DescribeEipAddresses(ctx context.Context, allocationID, eipAddress, associatedInstanceID, associatedInstanceType string) ([]EIPAddress, error)
	ReleaseEIPAddress(ctx context.Context, eipID string) error
	ModifyEipAddressAttribute(ctx context.Context, allocationID string, bandwidth string) error

	// 带宽包相关接口
	AddCommonBandwidthPackageIP(ctx context.Context, eipID, packageID string) error
	RemoveCommonBandwidthPackageIP(ctx context.Context, eipID, packageID string) error

	// 标签相关接口
	TagResources(ctx context.Context, resourceType string, resourceIDs []string, tags map[string]string) error
}

// EIPOptions EIP创建选项
type EIPOptions struct {
	InternetChargeType      string
	Bandwidth               string
	ISP                     string
	InstanceChargeType      string
	PublicIPAddressPoolID   string
	ResourceGroupID         string
	Name                    string
	Description             string
	SecurityProtectionTypes []string
}

// EIPAddress EIP地址信息
type EIPAddress struct {
	AllocationID          string
	Status                string
	ChargeType            string
	BandwidthPackageID    string
	Bandwidth             string
	IPAddress             string
	InstanceID            string
	InstanceType          string
	InternetChargeType    string
	PublicIPAddressPoolID string
	ISP                   string
	Name                  string
	ResourceGroupID       string
	PrivateIPAddress      string
	Description           string
	Tags                  map[string]string
}

const (
	// EIPStatusAvailable EIP可用状态
	EIPStatusAvailable = "Available"
	// EIPStatusInUse EIP使用中状态
	EIPStatusInUse = "InUse"
	// EIPStatusAssociating EIP绑定中状态
	EIPStatusAssociating = "Associating"
	// EIPStatusUnassociating EIP解绑中状态
	EIPStatusUnassociating = "Unassociating"
)

const (
	// EIPAssociatedInstanceTypeNetworkInterface 绑定到ENI
	EIPAssociatedInstanceTypeNetworkInterface = "NetworkInterface"
	// EIPInstanceTypeNetworkInterface 实例类型为ENI
	EIPInstanceTypeNetworkInterface = "NetworkInterface"
)
