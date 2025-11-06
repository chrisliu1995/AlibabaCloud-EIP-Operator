#!/bin/bash

# 生成 Webhook 自签名证书
# 用于本地测试环境

set -e

NAMESPACE="alibabacloud-eip-operator-system"
SERVICE_NAME="alibabacloud-eip-operator-webhook-service"
SECRET_NAME="webhook-server-cert"

echo "生成 Webhook 证书..."

# 创建临时目录
TMP_DIR=$(mktemp -d)
cd "$TMP_DIR"

# 生成 CA 私钥和证书
openssl genrsa -out ca.key 2048
openssl req -x509 -new -nodes -key ca.key -subj "/CN=webhook-ca" -days 3650 -out ca.crt

# 生成服务器私钥
openssl genrsa -out tls.key 2048

# 创建证书签名请求
cat >csr.conf <<EOF
[req]
req_extensions = v3_req
distinguished_name = req_distinguished_name
[req_distinguished_name]
[ v3_req ]
basicConstraints = CA:FALSE
keyUsage = nonRepudiation, digitalSignature, keyEncipherment
extendedKeyUsage = serverAuth
subjectAltName = @alt_names
[alt_names]
DNS.1 = ${SERVICE_NAME}
DNS.2 = ${SERVICE_NAME}.${NAMESPACE}
DNS.3 = ${SERVICE_NAME}.${NAMESPACE}.svc
DNS.4 = ${SERVICE_NAME}.${NAMESPACE}.svc.cluster.local
EOF

# 生成 CSR
openssl req -new -key tls.key -subj "/CN=webhook-server" -out tls.csr -config csr.conf

# 签署证书
openssl x509 -req -in tls.csr -CA ca.crt -CAkey ca.key -CAcreateserial -out tls.crt -days 3650 -extensions v3_req -extfile csr.conf

# 创建或更新 Secret
echo "创建 Kubernetes Secret..."
kubectl create secret generic ${SECRET_NAME} \
  --from-file=tls.key=tls.key \
  --from-file=tls.crt=tls.crt \
  --namespace=${NAMESPACE} \
  --dry-run=client -o yaml | kubectl apply -f -

# 更新 ValidatingWebhookConfiguration 的 caBundle
echo "更新 ValidatingWebhookConfiguration..."
CA_BUNDLE=$(cat ca.crt | base64 | tr -d '\n')

kubectl patch validatingwebhookconfiguration alibabacloud-eip-operator-validating-webhook-configuration \
  --type='json' -p="[{\"op\": \"add\", \"path\": \"/webhooks/0/clientConfig/caBundle\", \"value\":\"${CA_BUNDLE}\"}]"

# 清理
cd -
rm -rf "$TMP_DIR"

echo "✅ Webhook 证书生成并配置成功！"
