package application

import (
	"context"
	"fmt"
	"time"

	nackapi "github.com/nats-io/nack/pkg/jetstream/apis/jetstream/v1beta2"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/kubernetes/scheme"
	typedv1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/record"
	"k8s.io/klog/v2"

	"github.com/nats-tower/nats-tower-operator/config"
	"github.com/nats-tower/nats-tower-operator/interfaces/k8s"
	"github.com/nats-tower/nats-tower-operator/interfaces/natstower"
)

type NATSTowerOperator struct {
	secretController      *k8s.Controller[corev1.Secret]
	podController         *k8s.Controller[corev1.Pod]
	nackAccountController *k8s.Controller[nackapi.Account]
	informersFactory      dynamicinformer.DynamicSharedInformerFactory
	towerOperatorConfig   *config.Config
	k8sClient             *k8s.Client
	eventRecorder         record.EventRecorder
	natsTowerClient       *natstower.NATSTowerClient
}

const (
	groupVersionResourcePod         = "v1/pods"
	groupVersionResourceSecrets     = "v1/secrets"
	groupVersionResourceNackAccount = "jetstream.nats.io/v1beta2/accounts"
	natsTowerSecretLabelKey         = "nats-tower.com/nats-tower-secret"
	natsTowerInstallationLabelKey   = "nats-tower.com/nats-tower-installation"
	natsTowerAccountLabelKey        = "nats-tower.com/nats-tower-account"
	natsTowerAccountTierLabelKey    = "nats-tower.com/nats-tower-account-tier"
	natsTowerCredentialTypeLabelKey = "nats-tower.com/nats-tower-credential-type"
	secretCredentialsKey            = "nats.creds"
)

func getPodUserDescription(clusterID string, pod *corev1.Pod) string {
	if pod == nil {
		return "nil"
	}

	name := pod.ObjectMeta.Name
	if appName, ok := pod.ObjectMeta.Labels["app.kubernetes.io/name"]; ok {
		name += " (" + appName + ")"
	}

	description := fmt.Sprintf("Generated User for pod '%s' in namespace '%s' on cluster '%s'",
		name, pod.ObjectMeta.Namespace, clusterID)
	return description
}

func getNACKAccountUserDescription(clusterID string, acc *nackapi.Account) string {
	if acc == nil {
		return "nil"
	}

	description := fmt.Sprintf("Generated User for NACK account in namespace '%s' on cluster '%s'",
		acc.ObjectMeta.Namespace, clusterID)
	return description
}

func CreateNATSTowerOperator(towerOperatorConfig *config.Config,
	k8sClient *k8s.Client,
	natsTowerClient *natstower.NATSTowerClient) (*NATSTowerOperator, error) {

	if towerOperatorConfig.ClusterID == "" {
		return nil, fmt.Errorf("clusterID is required")
	}

	resync := time.Minute * time.Duration(towerOperatorConfig.ResyncInterval)
	informersFactory := dynamicinformer.NewDynamicSharedInformerFactory(k8sClient.DynamicClient, resync)

	broadcaster := record.NewBroadcaster()
	broadcaster.StartLogging(klog.Infof)
	broadcaster.StartRecordingToSink(&typedv1.EventSinkImpl{Interface: k8sClient.ClientSet.CoreV1().Events("")})

	eventRecorder := broadcaster.NewRecorder(scheme.Scheme, corev1.EventSource{Component: "nats-tower-operator"})

	natsTowerOperator := &NATSTowerOperator{
		k8sClient:           k8sClient,
		informersFactory:    informersFactory,
		towerOperatorConfig: towerOperatorConfig,
		eventRecorder:       eventRecorder,
		natsTowerClient:     natsTowerClient,
	}

	// --------------- HANDLING PODS -------------------
	{
		gvr, err := k8s.GetGVRFromResource(k8sClient.DiscoveryMapper, groupVersionResourcePod)
		if err != nil {
			klog.Errorf("Error getting GVR, skip handling for resource '%s': %s.", groupVersionResourcePod, err.Error())
			return nil, err
		}

		towerOperatorConfig.PodConfig.Kind = groupVersionResourcePod
		natsTowerOperator.podController = k8s.NewController(towerOperatorConfig.PodConfig,
			getPodHandler(natsTowerOperator),
			informersFactory.ForResource(gvr))
	}
	// --------------- HANDLING SECRETS -------------------
	{
		gvr, err := k8s.GetGVRFromResource(k8sClient.DiscoveryMapper, groupVersionResourceSecrets)
		if err != nil {
			klog.Errorf("Error getting GVR, skip handling for resource '%s': %s.", groupVersionResourceSecrets, err.Error())
			return nil, err
		}

		towerOperatorConfig.SecretConfig.Kind = groupVersionResourceSecrets
		natsTowerOperator.secretController = k8s.NewController(towerOperatorConfig.SecretConfig,
			getSecretHandler(natsTowerOperator),
			informersFactory.ForResource(gvr))

	}
	// --------------- HANDLING NACK ACCOUNTS -------------------
	{
		gvr, err := k8s.GetGVRFromResource(k8sClient.DiscoveryMapper, groupVersionResourceNackAccount)
		if err != nil {
			klog.Errorf("Error getting GVR, skip handling for resource '%s': %s.", groupVersionResourceNackAccount, err.Error())
			return nil, err
		}

		towerOperatorConfig.NACKAccountConfig.Kind = groupVersionResourceNackAccount
		natsTowerOperator.nackAccountController = k8s.NewController(towerOperatorConfig.NACKAccountConfig,
			getNACKAccountHandler(natsTowerOperator),
			informersFactory.ForResource(gvr))
	}

	return natsTowerOperator, nil
}

func (c *NATSTowerOperator) Handle(stopCh <-chan struct{}) {
	klog.Info("Starting informers")
	c.informersFactory.Start(stopCh)
	klog.Info("Waiting for informers cache sync")

	if err := c.podController.WaitForCacheSync(stopCh); err != nil {
		klog.Fatalf("Error while waiting for informer cache sync: %s", err.Error())
	}
	if err := c.secretController.WaitForCacheSync(stopCh); err != nil {
		klog.Fatalf("Error while waiting for informer cache sync: %s", err.Error())
	}
	if err := c.nackAccountController.WaitForCacheSync(stopCh); err != nil {
		klog.Fatalf("Error while waiting for informer cache sync: %s", err.Error())
	}

	klog.Info("Starting controllers")

	c.podController.Run(1, stopCh)

	c.secretController.Run(1, stopCh)

	c.nackAccountController.Run(1, stopCh)

	<-stopCh
	klog.Info("Shutting down controllers")

	c.podController.Shutdown()

	c.secretController.Shutdown()

	c.nackAccountController.Shutdown()

	klog.Info("Exporter exiting")
}

func (c *NATSTowerOperator) UpsertSecret(ctx context.Context,
	source runtime.Object,
	namespace, name, credType string,
	creds *natstower.ConnectionInfo,
	lastRevision *corev1.Secret) error {
	// Check if is an update
	if lastRevision != nil {
		if lastRevision.Data == nil {
			lastRevision.Data = map[string][]byte{}
		}
		if lastRevision.Labels == nil {
			lastRevision.Labels = map[string]string{}
		}
		lastRevision.Data[secretCredentialsKey] = []byte(creds.Creds)
		lastRevision.Data["URLS"] = []byte(creds.URLs)
		lastRevision.Data["ACCOUNT_NAME"] = []byte(creds.AccountName)
		lastRevision.Labels[natsTowerSecretLabelKey] = "true"
		lastRevision.Labels[natsTowerCredentialTypeLabelKey] = credType
		_, err := c.k8sClient.ClientSet.CoreV1().Secrets(namespace).Update(ctx, lastRevision, v1.UpdateOptions{})
		if err != nil {

			c.eventRecorder.Eventf(source,
				corev1.EventTypeWarning,
				"ErrorUpsertingSecret",
				"Could not update secret %s/%s: %v",
				namespace, name, err)

			return err
		}

		c.eventRecorder.Eventf(source,
			corev1.EventTypeNormal,
			"Updated",
			"Updated secret %s/%s", namespace, name)

		return nil
	}
	_, err := c.k8sClient.ClientSet.CoreV1().Secrets(namespace).Create(ctx, &corev1.Secret{
		ObjectMeta: v1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				natsTowerSecretLabelKey:         "true",
				natsTowerCredentialTypeLabelKey: credType,
			},
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			secretCredentialsKey: []byte(creds.Creds),
			"URLS":               []byte(creds.URLs),
			"ACCOUNT_NAME":       []byte(creds.AccountName),
		},
	}, v1.CreateOptions{})
	if err != nil {

		c.eventRecorder.Eventf(source,
			corev1.EventTypeWarning,
			"ErrorUpsertingSecret",
			"Could not create secret %s/%s: %v",
			namespace, name, err)

		return err
	}

	c.eventRecorder.Eventf(source,
		corev1.EventTypeNormal,
		"Created",
		"Created secret %s/%s", namespace, name)
	return nil
}
