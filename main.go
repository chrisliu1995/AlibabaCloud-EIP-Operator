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

package main

import (
	"flag"
	"os"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/klog/v2/klogr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	eipv1alpha1 "github.com/chrisliu1995/alibabacloud-eip-operator/api/v1alpha1"
	"github.com/chrisliu1995/alibabacloud-eip-operator/internal/controller"
	aliyunclient "github.com/chrisliu1995/alibabacloud-eip-operator/pkg/aliyun"
	"github.com/chrisliu1995/alibabacloud-eip-operator/pkg/config"
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(eipv1alpha1.AddToScheme(scheme))
}

func main() {
	var metricsAddr string
	var enableLeaderElection bool
	var probeAddr string
	var configFilePath string
	var credentialFilePath string

	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.StringVar(&configFilePath, "config", "/etc/config/ctrl-config.yaml", "config file for controlplane")
	flag.StringVar(&credentialFilePath, "credential", "/etc/credential/ctrl-secret.yaml", "secret file for controlplane")
	flag.Parse()

	ctrl.SetLogger(klogr.New())
	setupLog.Info("Starting AlibabaCloud-EIP-Operator")

	// 解析配置
	cfg, err := config.ParseAndValidate(configFilePath, credentialFilePath)
	if err != nil {
		setupLog.Error(err, "unable to load config")
		os.Exit(1)
	}
	setupLog.Info("loaded config", "config", cfg)

	// 创建阿里云客户端
	aliyun, err := aliyunclient.NewClient(
		cfg.AccessKeyID,
		cfg.AccessKeySecret,
		cfg.RegionID,
	)
	if err != nil {
		setupLog.Error(err, "unable to create aliyun client")
		os.Exit(1)
	}

	restCfg := ctrl.GetConfigOrDie()
	restCfg.QPS = cfg.KubeClientQPS
	restCfg.Burst = cfg.KubeClientBurst

	mgr, err := ctrl.NewManager(restCfg, ctrl.Options{
		Scheme: scheme,
		Metrics: metricsserver.Options{
			BindAddress: metricsAddr,
		},
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "alibabacloud-eip-operator.alibabacloud.com",
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	if err = (&controller.EIPReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
		Record: mgr.GetEventRecorderFor("eip-controller"),
		Aliyun: aliyun,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "EIP")
		os.Exit(1)
	}

	// 设置 Webhook
	if err = (&eipv1alpha1.EIP{}).SetupWebhookWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create webhook", "webhook", "EIP")
		os.Exit(1)
	}

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
