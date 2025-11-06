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

package controller

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	eipv1alpha1 "github.com/chrisliu1995/alibabacloud-eip-operator/api/v1alpha1"
)

var cfg *rest.Config
var k8sClient client.Client
var testEnv *envtest.Environment

func TestControllers(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Controller Suite")
}

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	By("bootstrapping test environment")
	testEnv = &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "config", "crd")},
		ErrorIfCRDPathMissing: false,
	}

	var err error
	cfg, err = testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	err = eipv1alpha1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient).NotTo(BeNil())
})

var _ = AfterSuite(func() {
	By("tearing down the test environment")
	err := testEnv.Stop()
	Expect(err).NotTo(HaveOccurred())
})

var _ = Describe("EIP Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-eip"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default",
		}

		It("should successfully create and delete EIP", func() {
			By("Creating a new EIP")
			eip := &eipv1alpha1.EIP{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: "default",
				},
				Spec: eipv1alpha1.EIPSpec{
					Bandwidth:          "5",
					InternetChargeType: "PayByTraffic",
					ReleaseStrategy:    eipv1alpha1.ReleaseStrategyOnDelete,
				},
			}
			Expect(k8sClient.Create(ctx, eip)).Should(Succeed())

			By("Checking if EIP was created")
			createdEIP := &eipv1alpha1.EIP{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, typeNamespacedName, createdEIP)
				return err == nil
			}, time.Second*10, time.Millisecond*250).Should(BeTrue())

			By("Checking EIP finalizer")
			Expect(createdEIP.Finalizers).Should(ContainElement(eipFinalizer))

			By("Deleting the EIP")
			Expect(k8sClient.Delete(ctx, eip)).Should(Succeed())

			By("Checking if EIP was deleted")
			Eventually(func() bool {
				err := k8sClient.Get(ctx, typeNamespacedName, createdEIP)
				return err != nil
			}, time.Second*10, time.Millisecond*250).Should(BeTrue())
		})
	})
})
