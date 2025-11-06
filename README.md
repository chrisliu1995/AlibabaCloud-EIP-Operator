# AlibabaCloud-EIP-Operator

[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](https://opensource.org/licenses/Apache-2.0)
[![Go Report Card](https://goreportcard.com/badge/github.com/chrisliu1995/AlibabaCloud-EIP-Operator)](https://goreportcard.com/report/github.com/chrisliu1995/AlibabaCloud-EIP-Operator)

AlibabaCloud-EIP-Operator 是一个独立的 Kubernetes Operator，用于管理阿里云 EIP（弹性公网IP）的生命周期，不与 Pod 耦合。

## ✨ 功能特性

- 🚀 **独立 EIP 管理** - 通过 Kubernetes CRD 管理阿里云 EIP，不依赖 Pod
- ⚡ **自动创建 EIP** - 支持自动创建新的 EIP 实例
- 📦 **导入已有 EIP** - 支持导入和管理已存在的 EIP
- 📊 **带宽管理** - 支持动态调整 EIP 带宽
- 🔗 **带宽包集成** - 支持将 EIP 加入到共享带宽包
- 🔒 **灵活的释放策略** - 支持多种 EIP 释放策略（Never/OnDelete）
- 🏷️ **标签管理** - 支持为 EIP 添加自定义标签

## 📚 文档

- [快速开始指南](docs/QUICKSTART.md)
- [架构设计文档](docs/ARCHITECTURE.md)
- [项目详细说明](docs/PROJECT.md)
- [项目总结](docs/SUMMARY.md)

## 🚀 快速开始

### 前置条件

- Kubernetes 1.19+
- 阿里云账号及 AccessKey
- 配置好的 kubectl

### 安装

1. 配置阿里云凭证:
```bash
# 复制示例文件
cp config/default/credentials.yaml.example config/default/credentials.yaml

# 编辑文件，填入你的 AccessKey
vim config/default/credentials.yaml
```

2. 安装 CRD:
```bash
make install
```

3. 部署控制器:
```bash
make deploy
```

### 使用示例

#### 创建新的 EIP

```yaml
apiVersion: eip.alibabacloud.com/v1alpha1
kind: EIP
metadata:
  name: my-eip
spec:
  bandwidth: "5"
  internetChargeType: PayByTraffic
  name: my-eip-instance
  description: "My EIP created by operator"
  releaseStrategy: OnDelete
  tags:
    env: production
    app: myapp
```

#### 导入已有 EIP

```yaml
apiVersion: eip.alibabacloud.com/v1alpha1
kind: EIP
metadata:
  name: existing-eip
spec:
  allocationID: eip-bp1xxxxxxxxxxxxx
  releaseStrategy: Never  # 删除 CR 时不释放 EIP
```

#### 使用共享带宽包

```yaml
apiVersion: eip.alibabacloud.com/v1alpha1
kind: EIP
metadata:
  name: eip-with-cbwp
spec:
  bandwidth: "5"
  internetChargeType: PayByTraffic
  bandwidthPackageID: cbwp-bp1xxxxxxxxxxxxx
  releaseStrategy: OnDelete
```

## 📋 API 参考

### EIPSpec

| 字段 | 类型 | 描述 |
|------|------|------|
| allocationID | string | 已存在的 EIP 实例 ID，如果指定则不会创建新的 EIP |
| bandwidth | string | EIP 带宽，单位 Mbps |
| internetChargeType | string | 计费方式，支持 PayByBandwidth 和 PayByTraffic |
| bandwidthPackageID | string | 带宽包 ID |
| releaseStrategy | ReleaseStrategy | EIP 释放策略，支持 Never 和 OnDelete |
| name | string | EIP 名称 |
| description | string | EIP 描述 |
| tags | map[string]string | EIP 标签 |

更多字段请参考 [API 文档](api/v1alpha1/eip_types.go)。

### EIPStatus

| 字段 | 类型 | 描述 |
|------|------|------|
| allocationID | string | EIP 实例 ID |
| eipAddress | string | EIP 地址 |
| status | string | EIP 状态 |
| bandwidth | string | 当前带宽 |
| conditions | []Condition | 状态条件 |
| lastSyncTime | Time | 最后同步时间 |

## 🛠️ 开发

```bash
# 生成代码
make generate

# 生成 CRD
make manifests

# 本地运行
make run

# 构建镜像
make docker-build IMG=<your-registry>/alibabacloud-eip-operator:tag
```

## 🏛️ 架构

EIP Operator 基于 Kubebuilder 框架开发：

- **CRD (EIP)** - 定义 EIP 资源的期望状态和实际状态
- **Controller** - 监听 EIP 资源变化，调用阿里云 API
- **Aliyun Client** - 封装阿里云 VPC API 调用

更多架构细节请参考 [架构设计文档](docs/ARCHITECTURE.md)。

## ⚙️ 配置

控制器使用配置文件：

- `/etc/config/ctrl-config.yaml` - 控制器配置
- `/etc/credential/ctrl-secret.yaml` - 阿里云凭证配置

详细配置请参考 [快速开始指南](docs/QUICKSTART.md)。

## 🗑️ 卸载

### 快速卸载

使用 Makefile：
```bash
make undeploy
```

或使用卸载脚本：
```bash
./undeploy.sh
```

### 手动卸载

按以下顺序删除资源：

```bash
# 1. 删除所有 EIP 资源（可选）
kubectl delete eip --all -n default

# 2. 删除控制器
kubectl delete -f config/manager/

# 3. 删除 Webhook 配置
kubectl delete -f config/webhook/

# 4. 删除配置和凭证
kubectl delete -f config/default/configmap.yaml
kubectl delete -f config/default/credentials.yaml

# 5. 删除 RBAC
kubectl delete -f config/rbac/

# 6. 删除 CRD（会删除所有 EIP 资源）
kubectl delete -f config/crd/

# 7. 删除 Namespace
kubectl delete -f config/default/namespace.yaml
```

> ⚠️ **注意**：
> - 删除 CRD 会自动删除所有 EIP 自定义资源
> - 根据 `releaseStrategy` 设置，阿里云上的 EIP 可能会被释放
> - 建议在卸载前先检查并备份重要的 EIP 资源

## 📝 许可证

Apache License 2.0
