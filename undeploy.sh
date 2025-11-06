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

warn "⚠️  此操作将删除 AlibabaCloud-EIP-Operator 及其所有资源"
read -p "是否继续? (y/n) " -n 1 -r
echo
if [[ ! $REPLY =~ ^[Yy]$ ]]; then
    info "卸载已取消"
    exit 0
fi

info "开始卸载 AlibabaCloud-EIP-Operator..."

# 0. 检查是否有 EIP 资源
EIP_COUNT=$(kubectl get eip --all-namespaces --no-headers 2>/dev/null | wc -l | tr -d ' ')
if [ "$EIP_COUNT" -gt 0 ]; then
    warn "发现 $EIP_COUNT 个 EIP 资源"
    kubectl get eip --all-namespaces
    echo
    warn "删除 EIP 资源可能会根据 releaseStrategy 释放阿里云 EIP！"
    read -p "是否删除所有 EIP 资源? (y/n) " -n 1 -r
    echo
    if [[ $REPLY =~ ^[Yy]$ ]]; then
        info "0. 删除 EIP 资源..."
        kubectl delete eip --all --all-namespaces --timeout=60s
    fi
fi

# 1. 删除 Manager
info "1. 删除 Controller Manager..."
kubectl delete -f config/manager/manager.yaml --ignore-not-found=true

# 2. 删除 Webhook 配置
info "2. 删除 Webhook 配置..."
kubectl delete -f config/webhook/manifests.yaml --ignore-not-found=true
kubectl delete -f config/webhook/service.yaml --ignore-not-found=true
kubectl delete secret webhook-server-cert -n alibabacloud-eip-operator-system --ignore-not-found=true

# 3. 删除配置
info "3. 删除配置..."
kubectl delete -f config/default/credentials.yaml --ignore-not-found=true
kubectl delete -f config/default/configmap.yaml --ignore-not-found=true

# 4. 删除 RBAC 资源
info "4. 删除 RBAC 资源..."
kubectl delete -f config/rbac/role_binding.yaml --ignore-not-found=true
kubectl delete -f config/rbac/role.yaml --ignore-not-found=true
kubectl delete -f config/rbac/service_account.yaml --ignore-not-found=true

# 4. 删除 CRD
warn "是否删除 CRD? 这将删除所有 EIP 自定义资源！"
read -p "删除 CRD? (y/n) " -n 1 -r
echo
if [[ $REPLY =~ ^[Yy]$ ]]; then
    info "5. 删除 CRD..."
    kubectl delete -f config/crd/eip.alibabacloud.com_eips.yaml --ignore-not-found=true
fi

# 5. 删除 Namespace
warn "是否删除 Namespace?"
read -p "删除 Namespace? (y/n) " -n 1 -r
echo
if [[ $REPLY =~ ^[Yy]$ ]]; then
    info "6. 删除 Namespace..."
    kubectl delete -f config/default/namespace.yaml --ignore-not-found=true
fi

info "✅ AlibabaCloud-EIP-Operator 卸载完成！"
echo
info "验证卸载："
echo "  kubectl get namespace alibabacloud-eip-operator-system"
echo "  kubectl get crd eips.eip.alibabacloud.com"
echo "  kubectl get eip --all-namespaces"
