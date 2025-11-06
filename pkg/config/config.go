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

package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v2"
)

// Config 控制器配置
type Config struct {
	RegionID        string   `yaml:"regionID"`
	VPCID           string   `yaml:"vpcID"`
	Controllers     []string `yaml:"controllers"`
	KubeClientQPS   float32  `yaml:"kubeClientQPS"`
	KubeClientBurst int      `yaml:"kubeClientBurst"`
	AccessKeyID     string   `yaml:"-"`
	AccessKeySecret string   `yaml:"-"`
}

// Credential 凭证配置
type Credential struct {
	AccessKeyID     string `yaml:"accessKeyID"`
	AccessKeySecret string `yaml:"accessKeySecret"`
}

var globalConfig *Config

// ParseAndValidate 解析并验证配置文件
func ParseAndValidate(configPath, credentialPath string) (*Config, error) {
	// 解析配置文件
	configData, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(configData, &cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	// 解析凭证文件
	credData, err := os.ReadFile(credentialPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read credential file: %w", err)
	}

	var cred Credential
	if err := yaml.Unmarshal(credData, &cred); err != nil {
		return nil, fmt.Errorf("failed to unmarshal credential: %w", err)
	}

	// 填充凭证信息
	cfg.AccessKeyID = cred.AccessKeyID
	cfg.AccessKeySecret = cred.AccessKeySecret

	// 验证必填字段
	if cfg.RegionID == "" {
		return nil, fmt.Errorf("regionID is required")
	}
	if cfg.AccessKeyID == "" {
		return nil, fmt.Errorf("accessKeyID is required")
	}
	if cfg.AccessKeySecret == "" {
		return nil, fmt.Errorf("accessKeySecret is required")
	}

	// 设置默认值
	if cfg.KubeClientQPS == 0 {
		cfg.KubeClientQPS = 50
	}
	if cfg.KubeClientBurst == 0 {
		cfg.KubeClientBurst = 100
	}

	globalConfig = &cfg
	return &cfg, nil
}

// GetConfig 获取全局配置
func GetConfig() *Config {
	return globalConfig
}
