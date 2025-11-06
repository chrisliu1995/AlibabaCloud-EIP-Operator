#!/bin/bash

set -e

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# 打印信息
info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# 检查 kubectl
if ! command -v kubectl &> /dev/null; then
    error "kubectl 未安装，请先安装 kubectl"
    exit 1
fi

# 检查集群连接
if ! kubectl cluster-info &> /dev/null; then
    error "无法连接到 Kubernetes 集群"
    exit 1
fi

info "开始部署 AlibabaCloud-EIP-Operator..."

# 1. 创建 Namespace
info "1. 创建 Namespace..."
kubectl apply -f config/default/namespace.yaml

# 2. 安装 CRD
info "2. 安装 CRD..."
kubectl apply -f config/crd/eip.alibabacloud.com_eips.yaml

# 3. 创建 RBAC 资源
info "3. 创建 RBAC 资源..."
kubectl apply -f config/rbac/service_account.yaml
kubectl apply -f config/rbac/role.yaml
kubectl apply -f config/rbac/role_binding.yaml

# 4. 创建配置
info "4. 创建配置..."
kubectl apply -f config/default/configmap.yaml

# 5. 创建凭证 (需要用户先修改)
warn "请确保已在 config/default/credentials.yaml 中配置了正确的阿里云凭证！"
read -p "是否继续部署? (y/n) " -n 1 -r
echo
if [[ ! $REPLY =~ ^[Yy]$ ]]; then
    error "部署已取消"
    exit 1
fi

kubectl apply -f config/default/credentials.yaml

# 6. 部署 Webhook 资源
info "6. 部署 Webhook 资源..."
kubectl apply -f config/webhook/service.yaml
kubectl apply -f config/webhook/manifests.yaml

# 7. 生成 Webhook 证书
info "7. 生成 Webhook 证书..."
sleep 2  # 等待 Webhook 配置创建完成
bash hack/generate-webhook-cert.sh

# 8. 部署 Manager
info "8. 部署 Controller Manager..."
kubectl apply -f config/manager/manager.yaml

# 等待部署就绪
info "等待 Operator 就绪..."
kubectl wait --for=condition=available --timeout=300s \
    deployment/alibabacloud-eip-operator-controller-manager \
    -n alibabacloud-eip-operator-system

info "✅ AlibabaCloud-EIP-Operator 部署成功！"
info "查看 Pod 状态:"
kubectl get pods -n alibabacloud-eip-operator-system

info "查看日志:"
echo "kubectl logs -f deployment/alibabacloud-eip-operator-controller-manager -n alibabacloud-eip-operator-system"
