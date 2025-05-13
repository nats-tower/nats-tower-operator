package main

import (
	"context"
	"runtime/debug"

	"k8s.io/klog/v2"

	"github.com/nats-tower/nats-tower-operator/application"
	"github.com/nats-tower/nats-tower-operator/config"
	"github.com/nats-tower/nats-tower-operator/interfaces/k8s"
	"github.com/nats-tower/nats-tower-operator/interfaces/natstower"
	"github.com/nats-tower/nats-tower-operator/utils"
)

func main() {
	klog.InitFlags(nil)
	buildInfo, _ := debug.ReadBuildInfo()

	klog.Infof("Start - go_version:%s - build-settings: %+v",
		buildInfo.GoVersion, buildInfo.Settings)

	// Load configuration from environment
	cfg, err := config.NewConfigFromEnv()
	if err != nil {
		klog.Fatalf("Error loading configuration: %s", err.Error())
	}

	stopCh := utils.SetupSignalHandler()

	k8sConfig := k8s.NewKubeConfig()

	clientConfig, err := k8sConfig.ClientConfig()
	if err != nil {
		klog.Fatalf("Error getting K8s client config: %s", err.Error())
	}

	k8sClient, err := k8s.NewClient(clientConfig)
	if err != nil {
		klog.Fatalf("Error building K8s client: %s", err.Error())
	}

	natsTowerClient, err := natstower.CreateNATSTowerClient(context.Background(),
		natstower.NATSTowerClientConfig{
			NATSTowerURL:    cfg.TowerURL,
			NATSTowerAPIKey: cfg.TowerAPIToken,
		})
	if err != nil {
		klog.Fatalf("Error creating NATSTowerClient: %s", err.Error())
	}

	klog.Infof("Valid NATS installations: %+v", cfg.ValidInstallations)
	klog.Info("Starting NATS Tower Operator")

	operator, err := application.CreateNATSTowerOperator(cfg, k8sClient, natsTowerClient)
	if err != nil {
		klog.Fatalf("Error CreateNATSTowerOperator: %s", err.Error())
	}
	operator.Handle(stopCh)
	klog.Info("Started NATS Tower Operator")
}
