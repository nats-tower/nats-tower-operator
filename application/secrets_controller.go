package application

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"

	"github.com/nats-tower/nats-tower-operator/interfaces/k8s"
)

func getSecretHandler(natsTowerOperator *NATSTowerOperator) func(ctx context.Context, informer cache.SharedIndexInformer, ev k8s.EventItem, obj corev1.Secret) error {
	return func(ctx context.Context, informer cache.SharedIndexInformer, ev k8s.EventItem, obj corev1.Secret) error {
		klog.Infof("secret[%s]: %s - %s", ev.ActionType, obj.Name, ev.Key)

		if obj.Labels == nil {
			return nil
		}

		if obj.Labels[natsTowerSecretLabelKey] == "" {
			return nil
		}

		if ev.ActionType != k8s.DeleteAction {
			return nil
		}

		// Delete User Auth at NATS Tower

		return nil
	}
}
