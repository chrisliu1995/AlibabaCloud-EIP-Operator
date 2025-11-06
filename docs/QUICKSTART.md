# AlibabaCloud-EIP-Operator 快速开始指南

本指南帮助你快速部署和使用 AlibabaCloud-EIP-Operator。

## 前置条件

1. **Kubernetes 集群** (版本 1.19+)
2. **kubectl** 已配置并连接到集群
3. **阿里云账号** 及 AccessKey/SecretKey
4. **Go 环境** (版本 1.20+，仅开发时需要)

## 快速部署

### 步骤 1: 准备配置文件

创建控制器配置文件 `ctrl-config.yaml`:

```yaml
regionID: cn-hangzhou  # 替换为你的区域
vpcID: vpc-xxxxx       # 替换为你的VPC ID
controllers:
  - "*"
kubeClientQPS: 50
kubeClientBurst: 100
```

创建凭证配置文件 `ctrl-secret.yaml`:

```yaml
accessKeyID: "YOUR_ACCESS_KEY"      # 替换为实际的AK
accessKeySecret: "YOUR_SECRET_KEY"  # 替换为实际的SK
```

### 步骤 2: 创建 ConfigMap 和 Secret

```bash
kubectl create namespace alibabacloud-eip-operator-system

kubectl create configmap ctrl-config \
  --from-file=ctrl-config.yaml=ctrl-config.yaml \
  -n alibabacloud-eip-operator-system

kubectl create secret generic ctrl-secret \
  --from-file=ctrl-secret.yaml=ctrl-secret.yaml \
  -n alibabacloud-eip-operator-system
```

### 步骤 3: 安装 CRD

```bash
cd alibabacloud-eip-operator
make install
```

验证 CRD 已安装:

```bash
kubectl get crd eips.eip.alibabacloud.com
```

### 步骤 4: 部署控制器

#### 方式一: 本地运行（开发测试）

```bash
make run
```

#### 方式二: 部署到集群

1. 构建镜像:
```bash
make docker-build IMG=your-registry/alibabacloud-eip-operator:v1.0
make docker-push IMG=your-registry/alibabacloud-eip-operator:v1.0
```

2. 创建部署文件 `deploy.yaml`:

```yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: alibabacloud-eip-operator
  namespace: alibabacloud-eip-operator-system
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: alibabacloud-eip-operator-role
rules:
- apiGroups: ["eip.alibabacloud.com"]
  resources: ["eips"]
  verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
- apiGroups: ["eip.alibabacloud.com"]
  resources: ["eips/status"]
  verbs: ["get", "update", "patch"]
- apiGroups: ["eip.alibabacloud.com"]
  resources: ["eips/finalizers"]
  verbs: ["update"]
- apiGroups: [""]
  resources: ["events"]
  verbs: ["create", "patch"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: alibabacloud-eip-operator-rolebinding
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: alibabacloud-eip-operator-role
subjects:
- kind: ServiceAccount
  name: alibabacloud-eip-operator
  namespace: alibabacloud-eip-operator-system
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: alibabacloud-eip-operator
  namespace: alibabacloud-eip-operator-system
spec:
  replicas: 1
  selector:
    matchLabels:
      app: alibabacloud-eip-operator
  template:
    metadata:
      labels:
        app: alibabacloud-eip-operator
    spec:
      serviceAccountName: alibabacloud-eip-operator
      containers:
      - name: manager
        image: your-registry/alibabacloud-eip-operator:v1.0
        command:
        - /manager
        args:
        - --leader-elect
        - --config=/etc/config/ctrl-config.yaml
        - --credential=/etc/credential/ctrl-secret.yaml
        volumeMounts:
        - name: config
          mountPath: /etc/config
        - name: credential
          mountPath: /etc/credential
        resources:
          limits:
            cpu: 500m
            memory: 512Mi
          requests:
            cpu: 100m
            memory: 128Mi
      volumes:
      - name: config
        configMap:
          name: ctrl-config
      - name: credential
        secret:
          secretName: ctrl-secret
```

3. 部署:
```bash
kubectl apply -f deploy.yaml
```

4. 验证部署:
```bash
kubectl get pods -n alibabacloud-eip-operator-system
kubectl logs -f deployment/alibabacloud-eip-operator -n alibabacloud-eip-operator-system
```

## 使用示例

### 示例 1: 创建新的 EIP

创建文件 `eip-new.yaml`:

```yaml
apiVersion: eip.alibabacloud.com/v1alpha1
kind: EIP
metadata:
  name: my-new-eip
spec:
  bandwidth: "5"
  internetChargeType: PayByTraffic
  name: my-eip-instance
  description: "My first EIP"
  releaseStrategy: OnDelete
  tags:
    env: production
    team: platform
```

应用:
```bash
kubectl apply -f eip-new.yaml
```

查看状态:
```bash
# 列表查看
kubectl get eip

# 详细信息
kubectl describe eip my-new-eip

# 查看状态
kubectl get eip my-new-eip -o jsonpath='{.status.eipAddress}'
```

### 示例 2: 导入已有 EIP

创建文件 `eip-import.yaml`:

```yaml
apiVersion: eip.alibabacloud.com/v1alpha1
kind: EIP
metadata:
  name: imported-eip
spec:
  allocationID: "eip-bp1xxxxxxxxxx"  # 替换为实际的EIP ID
  releaseStrategy: Never  # 删除CR时不释放EIP
```

应用:
```bash
kubectl apply -f eip-import.yaml
```

### 示例 3: 修改 EIP 带宽

```bash
kubectl patch eip my-new-eip --type merge -p '{"spec":{"bandwidth":"10"}}'
```

查看变更:
```bash
kubectl get eip my-new-eip -o jsonpath='{.status.bandwidth}'
```

### 示例 4: 加入共享带宽包

```bash
kubectl patch eip my-new-eip --type merge -p '{"spec":{"bandwidthPackageID":"cbwp-bp1xxxxxxxxxx"}}'
```

### 示例 5: 删除 EIP

```bash
# 删除 CR，同时释放 EIP（releaseStrategy=OnDelete）
kubectl delete eip my-new-eip

# 仅删除 CR，保留 EIP（releaseStrategy=Never）
kubectl delete eip imported-eip
```

## 监控和故障排查

### 查看控制器日志

```bash
kubectl logs -f deployment/alibabacloud-eip-operator -n alibabacloud-eip-operator-system
```

### 查看事件

```bash
kubectl get events --field-selector involvedObject.kind=EIP
kubectl describe eip <eip-name>
```

### 查看 EIP 状态条件

```bash
kubectl get eip <eip-name> -o jsonpath='{.status.conditions}' | jq
```

### 常见问题

#### 1. EIP 创建失败

检查:
- 阿里云凭证是否正确
- 账户余额是否充足
- 是否超过 EIP 配额
- 查看控制器日志

```bash
kubectl logs deployment/eip-operator -n eip-operator-system | grep ERROR
```

#### 2. 状态不同步

手动触发同步:
```bash
kubectl annotate eip <eip-name> kubectl.kubernetes.io/restartedAt="$(date +%Y-%m-%dT%H:%M:%S)"
```

#### 3. EIP 无法删除

检查 finalizer:
```bash
kubectl get eip <eip-name> -o jsonpath='{.metadata.finalizers}'
```

强制删除（慎用）:
```bash
kubectl patch eip <eip-name> -p '{"metadata":{"finalizers":[]}}' --type=merge
```

## 卸载

### 方式一：使用 Makefile（推荐）

```bash
make undeploy
```

这会自动按正确的顺序删除所有组件。

### 方式二：使用卸载脚本

```bash
./undeploy.sh
```

脚本会交互式地询问是否删除 CRD 和 Namespace，避免误删除数据。

### 方式三：手动卸载

按以下顺序手动删除各个组件：

#### 1. 删除所有 EIP 资源（可选）

```bash
# 查看所有 EIP 资源
kubectl get eip --all-namespaces

# 删除所有 EIP（注意：根据 releaseStrategy，可能会释放阿里云 EIP）
kubectl delete eip --all --all-namespaces

# 或删除指定的 EIP
kubectl delete eip <eip-name>
```

⚠️ **重要提示**：
- 如果 EIP 的 `releaseStrategy` 设置为 `OnDelete`，删除 CR 会同时释放阿里云上的 EIP
- 如果设置为 `Never`，只删除 CR，保留阿里云 EIP

#### 2. 删除控制器

```bash
kubectl delete -f config/manager/manager.yaml
```

#### 3. 删除 Webhook 配置

```bash
# 删除 Webhook Service
kubectl delete -f config/webhook/service.yaml

# 删除 ValidatingWebhookConfiguration
kubectl delete -f config/webhook/manifests.yaml

# 删除 Webhook 证书 Secret
kubectl delete secret webhook-server-cert -n alibabacloud-eip-operator-system
```

#### 4. 删除配置和凭证

```bash
# 删除 ConfigMap
kubectl delete -f config/default/configmap.yaml

# 删除凭证 Secret
kubectl delete -f config/default/credentials.yaml
```

#### 5. 删除 RBAC 资源

```bash
kubectl delete -f config/rbac/role_binding.yaml
kubectl delete -f config/rbac/role.yaml
kubectl delete -f config/rbac/service_account.yaml
```

#### 6. 卸载 CRD

⚠️ **警告**：删除 CRD 会自动删除所有 EIP 自定义资源！

```bash
# 首先确认没有重要的 EIP 资源
kubectl get eip --all-namespaces

# 删除 CRD
kubectl delete -f config/crd/eip.alibabacloud.com_eips.yaml

# 或使用 make 命令
make uninstall
```

#### 7. 删除 Namespace

```bash
kubectl delete -f config/default/namespace.yaml

# 或直接删除
kubectl delete namespace alibabacloud-eip-operator-system
```

### 验证卸载

确认所有资源已删除：

```bash
# 检查 Namespace
kubectl get namespace alibabacloud-eip-operator-system

# 检查 CRD
kubectl get crd eips.eip.alibabacloud.com

# 检查 EIP 资源
kubectl get eip --all-namespaces

# 检查 ValidatingWebhookConfiguration
kubectl get validatingwebhookconfiguration alibabacloud-eip-operator-validating-webhook-configuration
```

如果资源不存在，说明卸载成功。

### 卸载后清理

#### 清理本地构建产物

```bash
# 清理二进制文件
rm -rf bin/

# 清理 vendor 目录（可选）
rm -rf vendor/
```

#### 清理 Docker 镜像（可选）

```bash
# 查看本地镜像
docker images | grep alibabacloud-eip-operator

# 删除镜像
docker rmi <image-id>
```

### 常见问题

#### 1. Namespace 一直处于 Terminating 状态

可能是因为有资源的 finalizer 未清理：

```bash
# 查看 Namespace 详情
kubectl get namespace alibabacloud-eip-operator-system -o yaml

# 强制删除 Namespace（慎用）
kubectl patch namespace alibabacloud-eip-operator-system -p '{"metadata":{"finalizers":[]}}' --type=merge
```

#### 2. EIP 资源无法删除

检查并清理 finalizer：

```bash
# 查看 EIP 的 finalizer
kubectl get eip <eip-name> -o jsonpath='{.metadata.finalizers}'

# 移除 finalizer（会跳过清理逻辑，直接删除）
kubectl patch eip <eip-name> -p '{"metadata":{"finalizers":[]}}' --type=merge

# 然后删除资源
kubectl delete eip <eip-name>
```

⚠️ **注意**：强制删除 finalizer 会跳过控制器的清理逻辑，可能导致阿里云资源未释放。

#### 3. ValidatingWebhookConfiguration 阻止删除

如果 Webhook 配置导致无法删除资源：

```bash
# 先删除 ValidatingWebhookConfiguration
kubectl delete validatingwebhookconfiguration alibabacloud-eip-operator-validating-webhook-configuration

# 然后再删除其他资源
```

### 重新安装

卸载后如需重新安装：

```bash
# 确保旧资源已完全清理
kubectl get all -n alibabacloud-eip-operator-system

# 重新部署
make deploy

# 或使用脚本
./deploy.sh
```

## 下一步

- 阅读 [README.md](README.md) 了解更多功能
- 查看 [PROJECT.md](PROJECT.md) 了解架构设计
- 浏览 [config/samples/](config/samples/) 查看更多示例
- 开发自定义功能，参考 [内部文档](internal/controller/eip_controller.go)

## 获取帮助

如有问题，请：
1. 查看控制器日志
2. 检查 EIP 资源的 Events
3. 确认阿里云凭证和配置
4. 查看项目文档
