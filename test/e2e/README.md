# E2E 测试说明

## 概述

本目录包含 AlibabaCloud-EIP-Operator 的端到端（E2E）测试用例，使用 Ginkgo 和 Gomega 框架编写。

## 测试用例

### 1. 基础 EIP 生命周期测试
- **创建 EIP**: 验证 EIP 资源创建和 AllocationID 分配
- **修改带宽**: 验证 EIP 带宽动态调整功能
- **删除 EIP**: 验证 EIP 删除和资源清理

### 2. 标签管理测试
- **标签应用**: 验证 EIP 标签的正确应用和同步

### 3. ReleaseStrategy 测试
- **Never 策略**: 验证删除 CR 时不释放阿里云 EIP

## 运行测试

### 前置条件

1. 已部署 EIP Operator 到 Kubernetes 集群
2. 配置了有效的阿里云凭证
3. 安装了 Ginkgo CLI（可选）

```bash
go install github.com/onsi/ginkgo/v2/ginkgo@latest
```

### 运行所有 E2E 测试

使用 Makefile:
```bash
make test-e2e
```

或直接使用 go test:
```bash
go test -v ./test/e2e/... -ginkgo.v -ginkgo.progress
```

### 快速运行（跳过编译）

```bash
make test-e2e-quick
```

### 运行特定测试

```bash
# 只运行基础生命周期测试
go test -v ./test/e2e/... -ginkgo.focus="基础 EIP 生命周期"

# 只运行标签管理测试
go test -v ./test/e2e/... -ginkgo.focus="标签管理"
```

### 使用 Ginkgo CLI

```bash
cd test/e2e
ginkgo -v --progress
```

## 测试覆盖范围

| 功能模块 | 测试用例 | 状态 |
|---------|---------|------|
| EIP 创建 | 验证 AllocationID 生成 | ✅ |
| EIP 创建 | 验证 IP 地址分配 | ✅ |
| EIP 创建 | 验证状态同步 | ✅ |
| EIP 创建 | 验证 Conditions | ✅ |
| 带宽调整 | 修改带宽值 | ✅ |
| 带宽调整 | 验证状态更新 | ✅ |
| EIP 删除 | Finalizer 处理 | ✅ |
| EIP 删除 | 资源清理 | ✅ |
| 标签管理 | 标签应用 | ✅ |
| 标签管理 | 标签同步 | ✅ |
| ReleaseStrategy | Never 策略 | ✅ |

## 测试输出示例

```
Running Suite: EIP Operator E2E Test Suite
===========================================

• [SLOW TEST: 15.234 seconds]
基础 EIP 生命周期测试
  应该成功创建 EIP 并分配 AllocationID
  
  创建 EIP 资源
  验证 EIP AllocationID 已生成
  验证 EIP 地址已分配
  验证 EIP 状态为 Available
  验证 Ready Condition 为 True
  验证 EIP 规格配置
  
• [SLOW TEST: 18.567 seconds]
基础 EIP 生命周期测试
  应该成功修改 EIP 带宽
  
  创建初始 EIP (带宽 2 Mbps)
  等待 EIP 创建完成
  修改带宽到 5 Mbps
  验证带宽已更新
  验证 EIP 状态仍然为 Ready

Ran 5 of 5 Specs in 45.123 seconds
SUCCESS! -- 5 Passed | 0 Failed | 0 Pending | 0 Skipped
```

## 故障排查

### 测试超时

如果测试超时，增加超时时间：
```bash
go test -v ./test/e2e/... -ginkgo.v -timeout 60m
```

### 查看详细日志

```bash
go test -v ./test/e2e/... -ginkgo.v -ginkgo.trace
```

### 清理测试资源

测试会在 AfterEach 中自动清理资源，如果需要手动清理：
```bash
kubectl delete eip --all -n default
```

## 添加新测试

1. 在 `eip_test.go` 中添加新的 `Describe` 或 `It` 块
2. 使用 `By` 函数描述测试步骤
3. 使用 Gomega 的 `Eventually` 处理异步操作
4. 在 `AfterEach` 中确保资源清理

示例：
```go
It("应该正确处理新功能", func() {
    By("步骤1: 准备测试数据")
    // 测试代码
    
    By("步骤2: 执行操作")
    // 测试代码
    
    By("步骤3: 验证结果")
    Eventually(func() bool {
        // 验证逻辑
        return true
    }, timeout, interval).Should(BeTrue())
})
```

## CI/CD 集成

在 CI 流程中运行 E2E 测试：

```yaml
# .github/workflows/e2e.yml
- name: Run E2E Tests
  run: |
    make deploy
    sleep 30  # 等待 operator 就绪
    make test-e2e
```

## 参考文档

- [Ginkgo 文档](https://onsi.github.io/ginkgo/)
- [Gomega 文档](https://onsi.github.io/gomega/)
- [Kubernetes E2E 框架](https://github.com/kubernetes-sigs/e2e-framework)
