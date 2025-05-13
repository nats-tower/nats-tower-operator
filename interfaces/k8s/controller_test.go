package k8s

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/nats-tower/nats-tower-operator/config"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	k8sfake "k8s.io/client-go/dynamic/fake"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/tools/cache"
)

var (
	noResyncPeriodFunc = func() time.Duration { return 0 }
)

type fixture struct {
	t          *testing.T
	controller *Controller[corev1.Pod]
}

func newFixture(t *testing.T, resource config.Resource, objects []runtime.Object) *fixture {
	kubeclient := k8sfake.NewSimpleDynamicClient(runtime.NewScheme())

	return &fixture{
		t:          t,
		controller: newController(resource, objects, kubeclient),
	}
}

func newResource(selectorQuery string) config.Resource {
	return config.Resource{
		Kind: "core/v1/pods",
		Selector: config.Selector{
			Query: selectorQuery,
		},
	}
}

func newPod() *corev1.Pod {
	labels := map[string]string{
		"nats.tower": "true",
	}
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "app1",
			Namespace: "app-ns",
			Labels:    labels,
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  "app1",
					Image: "app1:latest",
				},
			},
		},
	}
}

func newUnstructured(obj interface{}) *unstructured.Unstructured {
	res, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
	if err != nil {
		panic(err)
	}
	return &unstructured.Unstructured{Object: res}
}

func newController(resource config.Resource,
	objects []runtime.Object,
	kubeclient *k8sfake.FakeDynamicClient) *Controller[corev1.Pod] {
	k8sI := dynamicinformer.NewDynamicSharedInformerFactory(kubeclient, noResyncPeriodFunc())
	s := strings.SplitN(resource.Kind, "/", 3)
	gvr := schema.GroupVersionResource{Group: s[0], Version: s[1], Resource: s[2]}
	informer := k8sI.ForResource(gvr)
	c := NewController[corev1.Pod](resource, func(ctx context.Context,
		informer cache.SharedIndexInformer,
		ev EventItem,
		obj corev1.Pod) error {
		return nil
	}, informer)

	for _, d := range objects {
		_ = informer.Informer().GetIndexer().Add(d)
	}

	return c
}

func (f *fixture) runControllerSyncHandler(item EventItem, expectError bool) {
	err := f.controller.syncHandler(item)
	if !expectError && err != nil {
		f.t.Errorf("error syncing item: %v", err)
	} else if expectError && err == nil {
		f.t.Error("expected error syncing item, got nil")
	}

}

func getKey(pod *corev1.Pod, t *testing.T) string {
	key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(pod)
	if err != nil {
		t.Errorf("Unexpected error getting key for pod %v: %v", pod.Name, err)
		return ""
	}
	return key
}

func TestCreateDeployment(t *testing.T) {
	d := newPod()
	objects := []runtime.Object{newUnstructured(d)}
	resource := newResource("")
	item := EventItem{Key: getKey(d, t), ActionType: CreateAction}

	f := newFixture(t, resource, objects)
	f.runControllerSyncHandler(item, false)
}

func TestUpdateDeployment(t *testing.T) {
	d := newPod()
	objects := []runtime.Object{newUnstructured(d)}
	resource := newResource("")
	item := EventItem{Key: getKey(d, t), ActionType: UpdateAction}

	f := newFixture(t, resource, objects)
	f.runControllerSyncHandler(item, false)
}

func TestDeleteDeployment(t *testing.T) {
	d := newPod()
	objects := []runtime.Object{newUnstructured(d)}
	resource := newResource("")
	item := EventItem{Key: getKey(d, t), ActionType: DeleteAction}

	f := newFixture(t, resource, objects)
	f.runControllerSyncHandler(item, false)
}

func TestSelectorQueryFilterDeployment(t *testing.T) {
	d := newPod()
	objects := []runtime.Object{newUnstructured(d)}
	resource := newResource(".metadata.name != \"port-k8s-exporter\"")
	item := EventItem{Key: getKey(d, t), ActionType: DeleteAction}

	f := newFixture(t, resource, objects)
	f.runControllerSyncHandler(item, false)
}
