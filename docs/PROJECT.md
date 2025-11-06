# AlibabaCloud-EIP-Operator 项目说明

## 项目概述

AlibabaCloud-EIP-Operator 是一个独立的 Kubernetes Operator，用于管理阿里云 EIP（弹性公网IP）的生命周期，完全不依赖 Pod，作为独立资源进行管理。

## 目录结构

```
alibabacloud-eip-operator/
├── api/
│   └── v1alpha1/
│       ├── groupversion_info.go    # API 组版本信息
│       └── eip_types.go            # EIP CRD 类型定义
├── internal/
│   └── controller/
│       ├── eip_controller.go       # EIP 控制器实现
│       └── eip_controller_test.go  # 控制器单元测试
├── pkg/
│   └── aliyun/
│       └── interface.go            # 阿里云 API 接口定义
├── config/
│   ├── crd/                        # CRD YAML（通过 make manifests 生成）
│   └── samples/
│       └── eip_v1alpha1_eip.yaml  # EIP 资源示例
├── hack/
│   └── boilerplate.go.txt         # 代码头部模板
├── main.go                        # 程序入口
├── go.mod                         # Go 模块定义
├── Makefile                       # 构建脚本
├── Dockerfile                     # 容器镜像构建
└── README.md                      # 项目文档
```

## 核心组件

### 1. API 定义 (api/v1alpha1/)

#### EIP CRD
- **GroupVersion**: `eip.alibabacloud.com/v1alpha1`
- **Kind**: `EIP`
- **ShortName**: `eip`

#### EIPSpec 主要字段
- `allocationID`: 已存在的 EIP ID（可选）
- `bandwidth`: EIP 带宽
- `internetChargeType`: 计费方式（默认 PayByTraffic）
- `instanceChargeType`: 实例计费方式
- `isp`: 线路类型
- `publicIPAddressPoolID`: 公网 IP 地址池 ID
- `resourceGroupID`: 资源组 ID
- `name`: EIP 名称
- `description`: EIP 描述
- `securityProtectionTypes`: 安全防护类型
- `tags`: EIP 标签
- `bandwidthPackageID`: 带宽包 ID
- `releaseStrategy`: 释放策略（Never/OnDelete，默认 OnDelete）

#### EIPStatus 主要字段
- `allocationID`: EIP 实例 ID
- `eipAddress`: EIP 地址
- `status`: EIP 状态（Available/InUse/Associating/Unassociating）
- `bandwidth`: 当前带宽
- `bandwidthPackageID`: 当前带宽包 ID
- `conditions`: 状态条件
- `lastSyncTime`: 最后同步时间

### 2. 控制器 (internal/controller/)

#### EIPReconciler
控制器负责：
- **创建 EIP**: 根据 Spec 自动创建新的 EIP
- **导入 EIP**: 管理已存在的 EIP
- **状态同步**: 定期同步 EIP 状态到 CR
- **带宽管理**: 动态调整 EIP 带宽
- **带宽包管理**: 加入/移出共享带宽包
- **标签管理**: 为 EIP 添加标签
- **释放管理**: 根据释放策略决定是否删除 EIP

#### 工作流程
1. 监听 EIP CR 创建/更新/删除事件
2. 添加 Finalizer 确保清理逻辑执行
3. 如果未指定 AllocationID，调用阿里云 API 创建 EIP
4. 同步 EIP 状态到 Status
5. 处理带宽、带宽包等配置变更
6. 周期性同步（5分钟）
7. CR 删除时根据 ReleaseStrategy 决定是否释放 EIP

### 3. 阿里云客户端 (pkg/aliyun/)

定义了必要的阿里云 API 接口：
- `AllocateEipAddress`: 创建 EIP
- `DescribeEipAddresses`: 查询 EIP
- `ReleaseEIPAddress`: 释放 EIP
- `ModifyEipAddressAttribute`: 修改 EIP 属性
- `AddCommonBandwidthPackageIP`: 加入带宽包
- `RemoveCommonBandwidthPackageIP`: 移出带宽包
- `TagResources`: 打标签

实际实现复用了父项目 `ack-extend-network-controller/pkg/aliyun/client` 的代码。

## 使用场景

### 场景 1: 自动创建和管理 EIP
适用于需要动态创建 EIP 的场景，删除 CR 时自动清理 EIP。

```yaml
apiVersion: eip.alibabacloud.com/v1alpha1
kind: EIP
metadata:
  name: auto-eip
spec:
  bandwidth: "10"
  internetChargeType: PayByTraffic
  releaseStrategy: OnDelete
```

### 场景 2: 导入已有 EIP 进行管理
适用于导入已存在的 EIP，仅进行状态同步和配置管理，不删除 EIP。

```yaml
apiVersion: eip.alibabacloud.com/v1alpha1
kind: EIP
metadata:
  name: imported-eip
spec:
  allocationID: "eip-bp1xxxxx"
  releaseStrategy: Never
```

### 场景 3: 使用共享带宽包
适用于多个 EIP 共享带宽的场景。

```yaml
apiVersion: eip.alibabacloud.com/v1alpha1
kind: EIP
metadata:
  name: cbwp-eip
spec:
  bandwidth: "100"
  bandwidthPackageID: "cbwp-bp1xxxxx"
  releaseStrategy: OnDelete
```

## 部署和使用

### 1. 生成 CRD
```bash
cd eip-operator
make manifests
```

### 2. 安装 CRD
```bash
make install
```

### 3. 配置凭证
确保配置文件存在：
- `/etc/config/ctrl-config.yaml`
- `/etc/credential/ctrl-secret.yaml`

### 4. 运行控制器
```bash
# 本地运行
make run

# 或构建并部署到集群
make docker-build IMG=<registry>/eip-operator:v1.0
make deploy
```

### 5. 创建 EIP 资源
```bash
kubectl apply -f config/samples/eip_v1alpha1_eip.yaml
```

### 6. 查看 EIP 状态
```bash
kubectl get eip
kubectl describe eip <name>
```

## 与原项目的关系

### 代码复用
- **阿里云客户端**: 直接引用父项目的 `pkg/aliyun/client` 包
- **配置管理**: 使用父项目的 `pkg/config` 包
- **构建工具**: 参考父项目的 Makefile 和 Dockerfile

### 独立性
- **独立的 CRD**: `eip.alibabacloud.com/v1alpha1` 不同于 `alibabacloud.com/v1beta1`
- **独立的控制器**: 不依赖 Pod，专注于 EIP 生命周期管理
- **独立的镜像**: 可以单独构建和部署

### 集成方式
作为独立项目，可以：
1. 与原项目并行部署，各自管理不同的资源
2. 单独部署，仅用于 EIP 管理
3. 集成到原项目中，作为新的控制器

## 技术栈

- **语言**: Go 1.20
- **框架**: Kubebuilder / controller-runtime
- **Kubernetes**: 1.19+
- **云服务**: 阿里云 VPC API

## 下一步工作

1. **生成 DeepCopy 代码**:
   ```bash
   make generate
   ```

2. **生成 CRD YAML**:
   ```bash
   make manifests
   ```

3. **编写测试**:
   - 单元测试
   - 集成测试
   - E2E 测试

4. **完善功能**:
   - Webhook 验证
   - 指标监控
   - 日志优化
   - 错误处理增强

5. **文档完善**:
   - API 文档
   - 操作手册
   - 故障排查指南

## 许可证

Apache License 2.0
