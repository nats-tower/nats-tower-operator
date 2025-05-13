package application

import (
	"context"
	"fmt"

	nackapi "github.com/nats-io/nack/pkg/jetstream/apis/jetstream/v1beta2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"

	"github.com/nats-tower/nats-tower-operator/interfaces/k8s"
	"github.com/nats-tower/nats-tower-operator/interfaces/natstower"
)

func getNACKAccountHandler(natsTowerOperator *NATSTowerOperator) func(ctx context.Context, informer cache.SharedIndexInformer, ev k8s.EventItem, obj nackapi.Account) error {
	return func(ctx context.Context, informer cache.SharedIndexInformer, ev k8s.EventItem, obj nackapi.Account) error {
		klog.Infof("NACK Account[%s]: %s - %s", ev.ActionType, obj.Name, ev.Key)

		// 1. check if annotated with nats.tower/secret
		// 2. check if a secret is defined in the annotation nats.tower/secret
		if obj.Labels == nil {
			return nil
		}
		if obj.Labels[natsTowerSecretLabelKey] == "" {
			return nil
		}

		installationPublicKey := obj.Labels[natsTowerInstallationLabelKey]

		if installationPublicKey == "" && natsTowerOperator.towerOperatorConfig.DefaultInstallation == "" {
			// Record event as we require the label
			natsTowerOperator.eventRecorder.Eventf(&obj,
				corev1.EventTypeWarning,
				"MissingInstallationLabel",
				"Require label %s to generate secret", natsTowerInstallationLabelKey)
			return nil
		}

		if installationPublicKey == "" {
			installationPublicKey = natsTowerOperator.towerOperatorConfig.DefaultInstallation
			natsTowerOperator.eventRecorder.Eventf(&obj,
				corev1.EventTypeNormal,
				"DefaultInstallation",
				"Will use default installation %s to generate secret",
				natsTowerOperator.towerOperatorConfig.DefaultInstallation)
		} else {
			_, ok := natsTowerOperator.towerOperatorConfig.ValidInstallations[installationPublicKey]

			if !ok {
				// Record event as we require valid label value
				natsTowerOperator.eventRecorder.Eventf(&obj,
					corev1.EventTypeWarning,
					"InvalidInstallationLabel",
					"Require label %s to be one of %+v to generate secret",
					natsTowerInstallationLabelKey,
					natsTowerOperator.towerOperatorConfig.ValidInstallations)
				return nil
			}
		}

		if ev.ActionType == k8s.DeleteAction {
			// Do nothing on account deletes
			return nil
		}

		// 3. check which type of credentials is required
		credentialType := "user"
		if obj.Labels[natsTowerCredentialTypeLabelKey] != "" {
			credentialType = obj.Labels[natsTowerCredentialTypeLabelKey]
		}

		switch credentialType {
		case "user":
		default:
			credentialType = "user"
		}

		var creds *natstower.ConnectionInfo

		// 4. check if secret is defined in the same namespace as the pod
		secret, err := natsTowerOperator.k8sClient.ClientSet.CoreV1().Secrets(obj.Namespace).Get(ctx,
			obj.Labels[natsTowerSecretLabelKey], v1.GetOptions{})
		if err != nil {
			if errors.IsNotFound(err) {
				klog.Infof("Secret[%s] not found in namespace[%s]",
					obj.Labels[natsTowerSecretLabelKey], obj.Namespace)

				switch credentialType {
				case "user":
					creds, err = natsTowerOperator.natsTowerClient.CreateOrGetUserAuth(ctx,
						obj.Namespace,
						installationPublicKey,
						obj.Name, // account name is the same as the NACK account name
						obj.Labels[natsTowerSecretLabelKey],
						getNACKAccountUserDescription(natsTowerOperator.towerOperatorConfig.ClusterID, &obj))

					if err == natstower.ErrK8sAccessNotAllowed {

						natsTowerOperator.eventRecorder.Eventf(&obj,
							corev1.EventTypeWarning,
							"ErrorK8sAccessNotAllowed",
							"Please add the namespace '%s' & cluster '%s' for account '%s' to the k8s access list on NATS Tower",
							obj.Namespace, natsTowerOperator.towerOperatorConfig.ClusterID, obj.Name)

						return err
					}

					if err != nil {

						natsTowerOperator.eventRecorder.Eventf(&obj,
							corev1.EventTypeWarning,
							"ErrorCreatingUserAuth",
							"Could not CreateOrGetUserAuth to create new secret:%v", err)

						return err
					}
				default:
					return fmt.Errorf("invalid credential type: %s - must be 'user'", credentialType)
				}

				return natsTowerOperator.UpsertSecret(ctx,
					&obj,
					obj.Namespace,
					obj.Labels[natsTowerSecretLabelKey],
					credentialType,
					creds,
					nil)
			}
			klog.Errorf("Secret[%s] not found in namespace[%s]:%T - %v",
				obj.Labels[natsTowerSecretLabelKey], obj.Namespace, err, err)
			return err
		}

		switch credentialType {
		case "user":
			// 4a. check if secret has a key named {secretCredentialsKey}
			if secret.Data[secretCredentialsKey] != nil && string(secret.Data[secretCredentialsKey]) != "" {
				return nil
			}
			creds, err = natsTowerOperator.natsTowerClient.CreateOrGetUserAuth(ctx,
				obj.Namespace,
				installationPublicKey,
				obj.Name, // account name is the same as the NACK account name
				obj.Labels[natsTowerSecretLabelKey],
				getNACKAccountUserDescription(natsTowerOperator.towerOperatorConfig.ClusterID, &obj))

			if err == natstower.ErrK8sAccessNotAllowed {

				natsTowerOperator.eventRecorder.Eventf(&obj,
					corev1.EventTypeWarning,
					"ErrorK8sAccessNotAllowed",
					"Please add the namespace '%s' & cluster '%s' for account '%s' to the k8s access list on NATS Tower",
					obj.Namespace, natsTowerOperator.towerOperatorConfig.ClusterID, obj.Name)

				return err
			}

			if err != nil {

				natsTowerOperator.eventRecorder.Eventf(&obj,
					corev1.EventTypeWarning,
					"ErrorCreatingUserAuth",
					"Could not CreateOrGetUserAuth to update secret:%v", err)

				return err
			}
		default:
			return fmt.Errorf("invalid credential type: %s - must be 'user'", credentialType)
		}

		// TODO check URLS and restart pod?

		return natsTowerOperator.UpsertSecret(ctx,
			&obj,
			obj.Namespace,
			obj.Labels[natsTowerSecretLabelKey],
			credentialType,
			creds,
			secret)
	}
}
