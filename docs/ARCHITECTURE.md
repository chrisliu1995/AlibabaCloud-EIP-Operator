# AlibabaCloud-EIP-Operator 架构设计

## 整体架构

```
┌─────────────────────────────────────────────────────────────┐
│                        Kubernetes API                        │
│                                                              │
│  ┌────────────┐  ┌────────────┐  ┌────────────┐            │
│  │  EIP CR 1  │  │  EIP CR 2  │  │  EIP CR 3  │  ...       │
│  └────────────┘  └────────────┘  └────────────┘            │
└─────────────────────────────────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────────┐
│             AlibabaCloud-EIP-Operator Controller             │
│                                                              │
│  ┌────────────────────────────────────────────────────┐    │
│  │              Controller Runtime                     │    │
│  │                                                     │    │
│  │  ┌──────────────────────────────────────────────┐ │    │
│  │  │         EIP Reconciler                       │ │    │
│  │  │                                              │ │    │
│  │  │  • Watch EIP Resources                      │ │    │
│  │  │  • Create/Update/Delete EIP                 │ │    │
│  │  │  • Sync Status                              │ │    │
│  │  │  • Handle Finalizers                        │ │    │
│  │  └──────────────────────────────────────────────┘ │    │
│  └────────────────────────────────────────────────────┘    │
└─────────────────────────────────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────────┐
│                    Aliyun Client                            │
│                                                              │
│  • AllocateEipAddress                                       │
│  • DescribeEipAddresses                                     │
│  • ReleaseEIPAddress                                        │
│  • ModifyEipAddressAttribute                                │
│  • AddCommonBandwidthPackageIP                              │
│  • RemoveCommonBandwidthPackageIP                           │
│  • TagResources                                             │
└─────────────────────────────────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────────┐
│                   Alibaba Cloud VPC API                      │
│                                                              │
│  ┌─────────┐  ┌─────────┐  ┌─────────┐  ┌─────────┐       │
│  │  EIP 1  │  │  EIP 2  │  │  EIP 3  │  │  CBWP   │  ...  │
│  └─────────┘  └─────────┘  └─────────┘  └─────────┘       │
└─────────────────────────────────────────────────────────────┘
```

## 核心组件

### 1. CRD 定义 (Custom Resource Definition)

**位置**: `api/v1alpha1/eip_types.go`

**作用**:
- 定义 EIP 资源的 API Schema
- 包括 Spec（期望状态）和 Status（实际状态）
- 提供验证规则和默认值

**主要结构**:
```go
type EIP struct {
    metav1.TypeMeta
    metav1.ObjectMeta
    Spec   EIPSpec
    Status EIPStatus
}

type EIPSpec struct {
    AllocationID       string
    Bandwidth          string
    InternetChargeType string
    ReleaseStrategy    ReleaseStrategy
    // ... 其他字段
}

type EIPStatus struct {
    AllocationID  string
    EIPAddress    string
    Status        string
    Conditions    []metav1.Condition
    // ... 其他字段
}
```

### 2. 控制器 (Controller)

**位置**: `internal/controller/eip_controller.go`

**作用**:
- 监听 EIP 资源的变化
- 实现协调循环（Reconcile Loop）
- 调用阿里云 API 进行实际操作
- 更新资源状态

**核心方法**:

#### Reconcile
```go
func (r *EIPReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error)
```
- 入口方法，处理所有 EIP 资源事件
- 添加/移除 Finalizer
- 调用 reconcileEIP 或 finalizeEIP

#### reconcileEIP
```go
func (r *EIPReconciler) reconcileEIP(ctx context.Context, eip *eipv1alpha1.EIP) (ctrl.Result, error)
```
- 创建 EIP（如果需要）
- 同步状态
- 更新带宽
- 管理带宽包
- 设置 Condition

#### finalizeEIP
```go
func (r *EIPReconciler) finalizeEIP(ctx context.Context, eip *eipv1alpha1.EIP) error
```
- 根据 ReleaseStrategy 决定是否释放 EIP
- 清理资源
- 移除 Finalizer

### 3. 阿里云客户端 (Aliyun Client)

**位置**: `pkg/aliyun/interface.go` (接口定义)

**实现**: 复用 `ack-extend-network-controller/pkg/aliyun/client`

**作用**:
- 封装阿里云 VPC API 调用
- 提供统一的接口
- 处理错误和重试

**主要接口**:
```go
type API interface {
    AllocateEipAddress(ctx, opts) (*EIPAddress, error)
    DescribeEipAddresses(ctx, id, ...) ([]EIPAddress, error)
    ReleaseEIPAddress(ctx, id) error
    ModifyEipAddressAttribute(ctx, id, bandwidth) error
    AddCommonBandwidthPackageIP(ctx, eipID, pkgID) error
    RemoveCommonBandwidthPackageIP(ctx, eipID, pkgID) error
    TagResources(ctx, type, ids, tags) error
}
```

## 工作流程

### 创建 EIP 流程

```
1. 用户创建 EIP CR
       ↓
2. Controller 监听到 Create 事件
       ↓
3. 添加 Finalizer
       ↓
4. 检查 Spec.AllocationID
       ├─ 已设置 → 跳到步骤 7
       └─ 未设置 → 继续
       ↓
5. 调用 AllocateEipAddress 创建 EIP
       ↓
6. 更新 Spec.AllocationID
       ↓
7. 调用 DescribeEipAddresses 获取状态
       ↓
8. 更新 Status
       ↓
9. 应用标签（如果有）
       ↓
10. 设置 Ready Condition
       ↓
11. 返回，等待下次同步（5分钟后）
```

### 更新带宽流程

```
1. 用户修改 Spec.Bandwidth
       ↓
2. Controller 监听到 Update 事件
       ↓
3. 比较 Spec.Bandwidth 和 Status.Bandwidth
       ├─ 相同 → 跳过
       └─ 不同 → 继续
       ↓
4. 检查是否在带宽包中
       ├─ 是 → 跳过（带宽包模式不能单独修改）
       └─ 否 → 继续
       ↓
5. 调用 ModifyEipAddressAttribute
       ↓
6. 重新同步状态
       ↓
7. 更新 Status.Bandwidth
       ↓
8. 发送 Event
```

### 删除 EIP 流程

```
1. 用户删除 EIP CR
       ↓
2. Controller 监听到 Delete 事件
       ↓
3. 检查 Finalizer
       ├─ 无 → 直接删除
       └─ 有 → 继续
       ↓
4. 执行 finalizeEIP
       ↓
5. 检查 ReleaseStrategy
       ├─ Never → 跳到步骤 9
       └─ OnDelete → 继续
       ↓
6. 检查是否在带宽包中
       ├─ 是 → 调用 RemoveCommonBandwidthPackageIP
       └─ 否 → 继续
       ↓
7. 调用 ReleaseEIPAddress
       ↓
8. 发送 Event
       ↓
9. 移除 Finalizer
       ↓
10. CR 被删除
```

## 状态管理

### Condition Types

1. **Ready**: EIP 是否就绪可用
2. **Synced**: 状态是否已同步
3. **Progressing**: 是否正在处理中

### Condition Reasons

- `Creating`: 正在创建 EIP
- `Created`: EIP 创建成功
- `Updating`: 正在更新 EIP
- `Updated`: EIP 更新成功
- `Deleting`: 正在删除 EIP
- `Deleted`: EIP 删除成功
- `SyncFailed`: 同步失败
- `InvalidConfig`: 配置无效

### 状态转换

```
Initial → Creating → Created → Ready
                              ↓
                         Updating → Updated → Ready
                              ↓
                         Deleting → Deleted → Removed
```

## 并发控制

### Finalizer 机制

- **作用**: 确保资源被正确清理
- **添加时机**: 首次 Reconcile
- **移除时机**: 清理完成后
- **好处**: 防止资源泄露

### Leader Election

- **作用**: 多副本部署时只有一个 Leader 工作
- **实现**: 基于 controller-runtime
- **配置**: `--leader-elect` 参数

### 重试机制

- **场景**: API 调用失败
- **策略**: 指数退避
- **实现**: `retry.RetryOnConflict`

## 错误处理

### API 错误处理

```go
if err := r.Aliyun.AllocateEipAddress(ctx, opts); err != nil {
    // 记录日志
    l.Error(err, "failed to allocate EIP")
    
    // 发送 Event
    r.Record.Eventf(eip, "Warning", "CreateFailed", "...")
    
    // 更新 Condition
    r.setCondition(eip, "Ready", "False", "SyncFailed", "...")
    
    // 返回错误，触发重试
    return ctrl.Result{RequeueAfter: 30*time.Second}, err
}
```

### 状态冲突处理

```go
func (r *EIPReconciler) updateStatus(ctx, eip) error {
    return retry.RetryOnConflict(retry.DefaultRetry, func() error {
        return r.Status().Update(ctx, eip)
    })
}
```

## 监控与可观测性

### 日志

- **级别**: Info, Error
- **框架**: logr (controller-runtime)
- **字段**: 结构化日志，包含上下文信息

### 事件 (Events)

- **类型**: Normal, Warning
- **作用**: 记录关键操作和错误
- **查看**: `kubectl describe eip <name>`

### 指标 (Metrics)

- **暴露**: `/metrics` 端点（默认 :8080）
- **格式**: Prometheus 格式
- **内容**: 
  - 控制器运行时指标
  - API 调用延迟
  - 错误计数

### 健康检查

- **Healthz**: `/healthz` (默认 :8081)
- **Readyz**: `/readyz` (默认 :8081)

## 扩展性

### 添加新功能

1. 在 `EIPSpec` 中添加新字段
2. 在 `reconcileEIP` 中处理新字段
3. 更新状态同步逻辑
4. 运行 `make generate` 和 `make manifests`

### 添加新的 API 调用

1. 在 `pkg/aliyun/interface.go` 中定义接口
2. 确保实现在 `ack-extend-network-controller/pkg/aliyun/client` 中存在
3. 在控制器中调用

## 性能考虑

### 同步周期

- **默认**: 5 分钟
- **可配置**: 修改 `reconcileEIP` 返回的 `RequeueAfter`

### 并发限制

- **默认**: controller-runtime 默认配置
- **可配置**: 通过 `MaxConcurrentReconciles` 设置

### 资源限制

```yaml
resources:
  limits:
    cpu: 500m
    memory: 512Mi
  requests:
    cpu: 100m
    memory: 128Mi
```

## 安全考虑

### RBAC

- **最小权限**: 只授予必要的权限
- **作用域**: 集群级别（EIP 是集群资源）

### 凭证管理

- **存储**: Kubernetes Secret
- **访问**: 通过 Volume 挂载
- **轮换**: 支持动态更新（需重启）

### 审计

- **Kubernetes 审计**: 记录所有 API 操作
- **阿里云审计**: 通过 ActionTrail 记录
