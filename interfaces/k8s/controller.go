package k8s

import (
	"context"
	"fmt"
	"time"

	nackapi "github.com/nats-io/nack/pkg/jetstream/apis/jetstream/v1beta2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"

	"github.com/nats-tower/nats-tower-operator/config"
	"github.com/nats-tower/nats-tower-operator/utils/jq"
)

type EventActionType string

const (
	CreateAction   EventActionType = "create"
	UpdateAction   EventActionType = "update"
	DeleteAction   EventActionType = "delete"
	MaxNumRequeues int             = 4
)

type EventItem struct {
	Key        string
	ActionType EventActionType
}

type K8sAPIObject interface {
	corev1.Pod | corev1.Secret | nackapi.Account
}

type Controller[T K8sAPIObject] struct {
	resource  config.Resource
	informer  cache.SharedIndexInformer
	lister    cache.GenericLister
	workqueue workqueue.RateLimitingInterface
	cb        func(ctx context.Context, informer cache.SharedIndexInformer, ev EventItem, obj T) error
}

func NewController[T K8sAPIObject](resource config.Resource,
	cb func(ctx context.Context, informer cache.SharedIndexInformer, ev EventItem, obj T) error,
	informer informers.GenericInformer) *Controller[T] {
	controller := &Controller[T]{
		resource:  resource,
		informer:  informer.Informer(),
		lister:    informer.Lister(),
		workqueue: workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter()),
		cb:        cb,
	}

	controller.informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			var err error
			var item EventItem
			item.ActionType = CreateAction
			item.Key, err = cache.MetaNamespaceKeyFunc(obj)
			if err == nil {
				controller.workqueue.Add(item)
			}
		},
		UpdateFunc: func(old interface{}, new interface{}) {
			var err error
			var item EventItem
			item.ActionType = UpdateAction
			item.Key, err = cache.MetaNamespaceKeyFunc(new)
			if err == nil {
				controller.workqueue.Add(item)
			}
		},
		DeleteFunc: func(obj interface{}) {
			var err error
			var item EventItem
			item.ActionType = DeleteAction
			item.Key, err = cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
			if err == nil {
				err = controller.objectHandler(obj, item)
				if err != nil {
					klog.Errorf("Error deleting item '%s' of resource '%s': %s", item.Key, resource.Kind, err.Error())
				}
			}
		},
	})

	return controller
}

func (c *Controller[T]) Shutdown() {
	klog.Infof("Shutting down controller for resource '%s'", c.resource.Kind)
	c.workqueue.ShutDown()
	klog.Infof("Closed controller for resource '%s'", c.resource.Kind)
}

func (c *Controller[T]) WaitForCacheSync(stopCh <-chan struct{}) error {
	klog.Infof("Waiting for informer cache to sync for resource '%s'", c.resource.Kind)
	if ok := cache.WaitForCacheSync(stopCh, c.informer.HasSynced); !ok {
		return fmt.Errorf("failed to wait for caches to sync")
	}

	return nil
}

func (c *Controller[T]) Run(workers int, stopCh <-chan struct{}) {
	defer utilruntime.HandleCrash()
	klog.Infof("Starting workers for resource '%s'", c.resource.Kind)
	for i := 0; i < workers; i++ {
		go wait.Until(c.runWorker, time.Second, stopCh)
	}
	klog.Infof("Started workers for resource '%s'", c.resource.Kind)
}

func (c *Controller[T]) runWorker() {
	for c.processNextWorkItem() {
	}
}

func (c *Controller[T]) processNextWorkItem() bool {
	obj, shutdown := c.workqueue.Get()

	if shutdown {
		return false
	}

	err := func(obj interface{}) error {
		defer c.workqueue.Done(obj)

		item, ok := obj.(EventItem)

		if !ok {
			c.workqueue.Forget(obj)
			utilruntime.HandleError(fmt.Errorf("expected event item of resource '%s' in workqueue but got %#v", c.resource.Kind, obj))
			return nil
		}

		if err := c.syncHandler(item); err != nil {
			if c.workqueue.NumRequeues(obj) >= MaxNumRequeues {
				utilruntime.HandleError(fmt.Errorf("error syncing '%s' of resource '%s': %s, give up after %d requeues", item.Key, c.resource.Kind, err.Error(), MaxNumRequeues))
				return nil
			}

			c.workqueue.AddRateLimited(obj)
			return fmt.Errorf("error syncing '%s' of resource '%s': %s, requeuing", item.Key, c.resource.Kind, err.Error())
		}

		c.workqueue.Forget(obj)
		return nil
	}(obj)

	if err != nil {
		utilruntime.HandleError(err)
		return true
	}

	return true
}

func (c *Controller[T]) syncHandler(item EventItem) error {
	obj, exists, err := c.informer.GetIndexer().GetByKey(item.Key)
	if err != nil {
		return fmt.Errorf("error fetching object with key '%s' from informer cache: %v", item.Key, err)
	}
	if !exists {
		utilruntime.HandleError(fmt.Errorf("'%s' in work queue no longer exists", item.Key))
		return nil
	}

	err = c.objectHandler(obj, item)
	if err != nil {
		return fmt.Errorf("error handling object with key '%s': %v", item.Key, err)
	}

	return nil
}

func (c *Controller[T]) objectHandler(obj interface{}, item EventItem) error {

	unstructuredObj, ok := obj.(*unstructured.Unstructured)
	if !ok {
		return fmt.Errorf("error casting to unstructured")
	}
	var structuredObj T
	err := runtime.DefaultUnstructuredConverter.FromUnstructured(unstructuredObj.Object, &structuredObj)
	if err != nil {
		return fmt.Errorf("error converting from unstructured: %v", err)
	}

	var selectorResult = true
	if c.resource.Selector.Query != "" {
		selectorResult, err = jq.ParseBool(c.resource.Selector.Query, unstructuredObj.Object)
		if err != nil {
			return fmt.Errorf("invalid selector query '%s': %v", c.resource.Selector.Query, err)
		}
	}
	if !selectorResult {
		return nil
	}

	err = c.cb(context.Background(), c.informer, item, structuredObj)
	if err != nil {
		return fmt.Errorf("error handling object with key '%s': %v", item.Key, err)
	}

	return nil
}
