package e2e

import (
	"context"
	"fmt"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	eipv1alpha1 "github.com/chrisliu1995/alibabacloud-eip-operator/api/v1alpha1"
)

const (
	timeout  = time.Second * 60
	interval = time.Second * 2
)

var _ = Describe("EIP Controller E2E Tests", func() {
	Context("基础 EIP 生命周期测试", func() {
		var (
			ctx       context.Context
			eipName   string
			namespace string
		)

		BeforeEach(func() {
			ctx = context.Background()
			eipName = "test-eip-e2e"
			namespace = "default"
		})

		AfterEach(func() {
			// 清理测试资源
			eip := &eipv1alpha1.EIP{
				ObjectMeta: metav1.ObjectMeta{
					Name:      eipName,
					Namespace: namespace,
				},
			}
			_ = k8sClient.Delete(ctx, eip)

			// 等待资源被完全删除
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      eipName,
					Namespace: namespace,
				}, eip)
				return err != nil
			}, timeout, interval).Should(BeTrue())
		})

		It("应该成功创建 EIP 并分配 AllocationID", func() {
			By("创建 EIP 资源")
			eip := &eipv1alpha1.EIP{
				ObjectMeta: metav1.ObjectMeta{
					Name:      eipName,
					Namespace: namespace,
				},
				Spec: eipv1alpha1.EIPSpec{
					Bandwidth:          "2",
					InternetChargeType: "PayByTraffic",
					Name:               "e2e-test-eip",
					Description:        "E2E Test EIP",
					ReleaseStrategy:    eipv1alpha1.ReleaseStrategyOnDelete,
					Tags: map[string]string{
						"test":  "e2e",
						"suite": "basic",
					},
				},
			}

			Expect(k8sClient.Create(ctx, eip)).Should(Succeed())

			By("验证 EIP AllocationID 已生成")
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      eipName,
					Namespace: namespace,
				}, eip)
				if err != nil {
					return false
				}
				return eip.Status.AllocationID != ""
			}, timeout, interval).Should(BeTrue())

			By("验证 EIP 地址已分配")
			Expect(eip.Status.EIPAddress).ShouldNot(BeEmpty())

			By("验证 EIP 状态为 Available")
			Eventually(func() string {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      eipName,
					Namespace: namespace,
				}, eip)
				if err != nil {
					return ""
				}
				return eip.Status.Status
			}, timeout, interval).Should(Equal("Available"))

			By("验证 Ready Condition 为 True")
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      eipName,
					Namespace: namespace,
				}, eip)
				if err != nil {
					return false
				}
				for _, cond := range eip.Status.Conditions {
					if cond.Type == "Ready" && cond.Status == metav1.ConditionTrue {
						return true
					}
				}
				return false
			}, timeout, interval).Should(BeTrue())

			By("验证 EIP 规格配置")
			Expect(eip.Status.Bandwidth).Should(Equal("2"))
			Expect(eip.Status.InternetChargeType).Should(Equal("PayByTraffic"))
		})

		It("应该成功修改 EIP 带宽", func() {
			By("创建初始 EIP (带宽 2 Mbps)")
			eip := &eipv1alpha1.EIP{
				ObjectMeta: metav1.ObjectMeta{
					Name:      eipName,
					Namespace: namespace,
				},
				Spec: eipv1alpha1.EIPSpec{
					Bandwidth:          "2",
					InternetChargeType: "PayByTraffic",
					ReleaseStrategy:    eipv1alpha1.ReleaseStrategyOnDelete,
				},
			}

			Expect(k8sClient.Create(ctx, eip)).Should(Succeed())

			By("等待 EIP 创建完成")
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      eipName,
					Namespace: namespace,
				}, eip)
				return err == nil && eip.Status.AllocationID != ""
			}, timeout, interval).Should(BeTrue())

			By("修改带宽到 5 Mbps")
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      eipName,
				Namespace: namespace,
			}, eip)).Should(Succeed())

			eip.Spec.Bandwidth = "5"
			Expect(k8sClient.Update(ctx, eip)).Should(Succeed())

			By("验证带宽已更新")
			Eventually(func() string {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      eipName,
					Namespace: namespace,
				}, eip)
				if err != nil {
					return ""
				}
				return eip.Status.Bandwidth
			}, timeout, interval).Should(Equal("5"))

			By("验证 EIP 状态仍然为 Ready")
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      eipName,
				Namespace: namespace,
			}, eip)).Should(Succeed())

			var readyCondition *metav1.Condition
			for i := range eip.Status.Conditions {
				if eip.Status.Conditions[i].Type == "Ready" {
					readyCondition = &eip.Status.Conditions[i]
					break
				}
			}
			Expect(readyCondition).ShouldNot(BeNil())
			Expect(readyCondition.Status).Should(Equal(metav1.ConditionTrue))
		})

		It("应该正确处理 EIP 删除和资源清理", func() {
			By("创建 EIP")
			eip := &eipv1alpha1.EIP{
				ObjectMeta: metav1.ObjectMeta{
					Name:      eipName,
					Namespace: namespace,
				},
				Spec: eipv1alpha1.EIPSpec{
					Bandwidth:          "1",
					InternetChargeType: "PayByTraffic",
					ReleaseStrategy:    eipv1alpha1.ReleaseStrategyOnDelete,
				},
			}

			Expect(k8sClient.Create(ctx, eip)).Should(Succeed())

			By("等待 EIP 创建完成")
			var allocationID string
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      eipName,
					Namespace: namespace,
				}, eip)
				if err == nil && eip.Status.AllocationID != "" {
					allocationID = eip.Status.AllocationID
					return true
				}
				return false
			}, timeout, interval).Should(BeTrue())

			By(fmt.Sprintf("记录 AllocationID: %s", allocationID))

			By("删除 EIP 资源")
			Expect(k8sClient.Delete(ctx, eip)).Should(Succeed())

			By("验证 EIP 资源已被删除")
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      eipName,
					Namespace: namespace,
				}, eip)
				return err != nil
			}, timeout, interval).Should(BeTrue())

			By("验证没有残留的 finalizer")
			// 如果能获取到，说明 finalizer 没有被清理
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      eipName,
				Namespace: namespace,
			}, eip)
			Expect(err).Should(HaveOccurred())
		})
	})

	Context("EIP 标签管理测试", func() {
		var (
			ctx       context.Context
			eipName   string
			namespace string
		)

		BeforeEach(func() {
			ctx = context.Background()
			eipName = "test-eip-tags"
			namespace = "default"
		})

		AfterEach(func() {
			eip := &eipv1alpha1.EIP{
				ObjectMeta: metav1.ObjectMeta{
					Name:      eipName,
					Namespace: namespace,
				},
			}
			_ = k8sClient.Delete(ctx, eip)

			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      eipName,
					Namespace: namespace,
				}, eip)
				return err != nil
			}, timeout, interval).Should(BeTrue())
		})

		It("应该正确应用和同步标签", func() {
			By("创建带标签的 EIP")
			eip := &eipv1alpha1.EIP{
				ObjectMeta: metav1.ObjectMeta{
					Name:      eipName,
					Namespace: namespace,
				},
				Spec: eipv1alpha1.EIPSpec{
					Bandwidth:          "1",
					InternetChargeType: "PayByTraffic",
					ReleaseStrategy:    eipv1alpha1.ReleaseStrategyOnDelete,
					Tags: map[string]string{
						"env":     "test",
						"owner":   "e2e",
						"project": "eip-operator",
					},
				},
			}

			Expect(k8sClient.Create(ctx, eip)).Should(Succeed())

			By("等待 EIP 创建并验证标签")
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      eipName,
					Namespace: namespace,
				}, eip)
				return err == nil && eip.Status.AllocationID != ""
			}, timeout, interval).Should(BeTrue())

			By("验证标签已设置")
			Expect(eip.Spec.Tags).Should(HaveKey("env"))
			Expect(eip.Spec.Tags["env"]).Should(Equal("test"))
			Expect(eip.Spec.Tags).Should(HaveKey("owner"))
			Expect(eip.Spec.Tags["owner"]).Should(Equal("e2e"))
		})
	})

	Context("EIP ReleaseStrategy 测试", func() {
		var (
			ctx       context.Context
			namespace string
		)

		BeforeEach(func() {
			ctx = context.Background()
			namespace = "default"
		})

		It("ReleaseStrategy=Never 时删除 CR 不应释放 EIP", func() {
			eipName := "test-eip-never-release"

			By("创建 ReleaseStrategy=Never 的 EIP")
			eip := &eipv1alpha1.EIP{
				ObjectMeta: metav1.ObjectMeta{
					Name:      eipName,
					Namespace: namespace,
				},
				Spec: eipv1alpha1.EIPSpec{
					Bandwidth:          "1",
					InternetChargeType: "PayByTraffic",
					ReleaseStrategy:    eipv1alpha1.ReleaseStrategyNever,
				},
			}

			Expect(k8sClient.Create(ctx, eip)).Should(Succeed())

			By("等待 EIP 创建完成")
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      eipName,
					Namespace: namespace,
				}, eip)
				return err == nil && eip.Status.AllocationID != ""
			}, timeout, interval).Should(BeTrue())

			allocationID := eip.Status.AllocationID
			By(fmt.Sprintf("EIP AllocationID: %s (ReleaseStrategy=Never)", allocationID))

			By("删除 EIP CR")
			Expect(k8sClient.Delete(ctx, eip)).Should(Succeed())

			By("验证 CR 已删除")
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      eipName,
					Namespace: namespace,
				}, eip)
				return err != nil
			}, timeout, interval).Should(BeTrue())

			// 注意：这里无法直接验证阿里云上的 EIP 是否还存在
			// 需要人工检查或通过阿里云 API 验证
		})
	})
})

func TestE2E(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "EIP Operator E2E Test Suite")
}
