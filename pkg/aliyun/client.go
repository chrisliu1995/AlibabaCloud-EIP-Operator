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
	"fmt"

	"github.com/aliyun/alibaba-cloud-sdk-go/services/vpc"
)

// Client 阿里云客户端
type Client struct {
	vpcClient *vpc.Client
	regionID  string
}

// NewClient 创建阿里云客户端
func NewClient(accessKeyID, accessKeySecret, regionID string) (*Client, error) {
	vpcClient, err := vpc.NewClientWithAccessKey(regionID, accessKeyID, accessKeySecret)
	if err != nil {
		return nil, fmt.Errorf("failed to create vpc client: %w", err)
	}

	return &Client{
		vpcClient: vpcClient,
		regionID:  regionID,
	}, nil
}

// AllocateEipAddress 创建EIP
func (c *Client) AllocateEipAddress(ctx context.Context, opts *EIPOptions) (*EIPAddress, error) {
	req := vpc.CreateAllocateEipAddressRequest()
	req.Scheme = "https"
	req.RegionId = c.regionID

	if opts != nil {
		if opts.InternetChargeType != "" {
			req.InternetChargeType = opts.InternetChargeType
		}
		if opts.Bandwidth != "" {
			req.Bandwidth = opts.Bandwidth
		}
		if opts.ISP != "" {
			req.ISP = opts.ISP
		}
		if opts.InstanceChargeType != "" {
			req.InstanceChargeType = opts.InstanceChargeType
		}
		if opts.PublicIPAddressPoolID != "" {
			req.PublicIpAddressPoolId = opts.PublicIPAddressPoolID
		}
		if opts.ResourceGroupID != "" {
			req.ResourceGroupId = opts.ResourceGroupID
		}
		if opts.Name != "" {
			req.Name = opts.Name
		}
		if opts.Description != "" {
			req.Description = opts.Description
		}
		if len(opts.SecurityProtectionTypes) > 0 {
			req.SecurityProtectionTypes = &opts.SecurityProtectionTypes
		}
	}

	resp, err := c.vpcClient.AllocateEipAddress(req)
	if err != nil {
		return nil, fmt.Errorf("failed to allocate eip: %w", err)
	}

	return &EIPAddress{
		AllocationID: resp.AllocationId,
		IPAddress:    resp.EipAddress,
	}, nil
}

// DescribeEipAddresses 查询EIP
func (c *Client) DescribeEipAddresses(ctx context.Context, allocationID, eipAddress, associatedInstanceID, associatedInstanceType string) ([]EIPAddress, error) {
	req := vpc.CreateDescribeEipAddressesRequest()
	req.Scheme = "https"
	req.RegionId = c.regionID

	if allocationID != "" {
		req.AllocationId = allocationID
	}
	if eipAddress != "" {
		req.EipAddress = eipAddress
	}
	if associatedInstanceID != "" {
		req.AssociatedInstanceId = associatedInstanceID
	}
	if associatedInstanceType != "" {
		req.AssociatedInstanceType = associatedInstanceType
	}

	resp, err := c.vpcClient.DescribeEipAddresses(req)
	if err != nil {
		return nil, fmt.Errorf("failed to describe eip addresses: %w", err)
	}

	result := make([]EIPAddress, 0, len(resp.EipAddresses.EipAddress))
	for _, eip := range resp.EipAddresses.EipAddress {
		tags := make(map[string]string)
		for _, tag := range eip.Tags.Tag {
			tags[tag.Key] = tag.Value
		}

		result = append(result, EIPAddress{
			AllocationID:          eip.AllocationId,
			Status:                eip.Status,
			ChargeType:            eip.ChargeType,
			BandwidthPackageID:    eip.BandwidthPackageId,
			Bandwidth:             eip.Bandwidth,
			IPAddress:             eip.IpAddress,
			InstanceID:            eip.InstanceId,
			InstanceType:          eip.InstanceType,
			InternetChargeType:    eip.InternetChargeType,
			PublicIPAddressPoolID: eip.PublicIpAddressPoolId,
			ISP:                   eip.ISP,
			Name:                  eip.Name,
			ResourceGroupID:       eip.ResourceGroupId,
			PrivateIPAddress:      eip.PrivateIpAddress,
			Description:           eip.Descritpion,
			Tags:                  tags,
		})
	}

	return result, nil
}

// ReleaseEIPAddress 释放EIP
func (c *Client) ReleaseEIPAddress(ctx context.Context, eipID string) error {
	req := vpc.CreateReleaseEipAddressRequest()
	req.Scheme = "https"
	req.AllocationId = eipID

	_, err := c.vpcClient.ReleaseEipAddress(req)
	if err != nil {
		return fmt.Errorf("failed to release eip: %w", err)
	}

	return nil
}

// ModifyEipAddressAttribute 修改EIP属性
func (c *Client) ModifyEipAddressAttribute(ctx context.Context, allocationID string, bandwidth string) error {
	req := vpc.CreateModifyEipAddressAttributeRequest()
	req.Scheme = "https"
	req.AllocationId = allocationID
	req.Bandwidth = bandwidth

	_, err := c.vpcClient.ModifyEipAddressAttribute(req)
	if err != nil {
		return fmt.Errorf("failed to modify eip attribute: %w", err)
	}

	return nil
}

// AddCommonBandwidthPackageIP 添加EIP到带宽包
func (c *Client) AddCommonBandwidthPackageIP(ctx context.Context, eipID, packageID string) error {
	req := vpc.CreateAddCommonBandwidthPackageIpRequest()
	req.Scheme = "https"
	req.IpInstanceId = eipID
	req.BandwidthPackageId = packageID

	_, err := c.vpcClient.AddCommonBandwidthPackageIp(req)
	if err != nil {
		return fmt.Errorf("failed to add eip to bandwidth package: %w", err)
	}

	return nil
}

// RemoveCommonBandwidthPackageIP 从带宽包移除EIP
func (c *Client) RemoveCommonBandwidthPackageIP(ctx context.Context, eipID, packageID string) error {
	req := vpc.CreateRemoveCommonBandwidthPackageIpRequest()
	req.Scheme = "https"
	req.IpInstanceId = eipID
	req.BandwidthPackageId = packageID

	_, err := c.vpcClient.RemoveCommonBandwidthPackageIp(req)
	if err != nil {
		return fmt.Errorf("failed to remove eip from bandwidth package: %w", err)
	}

	return nil
}

// TagResources 为资源打标签
func (c *Client) TagResources(ctx context.Context, resourceType string, resourceIDs []string, tags map[string]string) error {
	if len(resourceIDs) == 0 || len(tags) == 0 {
		return nil
	}

	req := vpc.CreateTagResourcesRequest()
	req.Scheme = "https"
	req.RegionId = c.regionID
	req.ResourceType = resourceType
	req.ResourceId = &resourceIDs

	tagList := make([]vpc.TagResourcesTag, 0, len(tags))
	for k, v := range tags {
		tagList = append(tagList, vpc.TagResourcesTag{
			Key:   k,
			Value: v,
		})
	}
	req.Tag = &tagList

	_, err := c.vpcClient.TagResources(req)
	if err != nil {
		return fmt.Errorf("failed to tag resources: %w", err)
	}

	return nil
}
