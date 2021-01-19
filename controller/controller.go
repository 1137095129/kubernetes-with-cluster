package controller

import (
	"fmt"
	"github.com/sirupsen/logrus"
	"github.com/wang1137095129/kubernetes-with-cluster/event"
	"github.com/wang1137095129/kubernetes-with-cluster/handler"
	"github.com/wang1137095129/kubernetes-with-cluster/util"
	core_v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

var (
	serverStartTime time.Time
)

const (
	maxRetries = 5
)

type Event struct {
	key          string
	eventType    string
	nameSpace    string
	resourceType string
}

type Controller struct {
	logger       *logrus.Entry
	clientSet    kubernetes.Interface
	queue        workqueue.RateLimitingInterface
	informer     cache.SharedIndexInformer
	eventHandler handler.Handler
}

func Start(handler handler.Handler, nameSpace string) {
	config, err := rest.InClusterConfig()
	if err != nil {
		panic(err)
	}
	kubeClient, err := kubernetes.NewForConfig(config)
	if err != nil {
		panic(err)
	}

	notReadyNodeInformer := cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(options v1.ListOptions) (runtime.Object, error) {
				options.FieldSelector = "involvedObject.kind=Node,type=Normal,reason=NotReady"
				return kubeClient.CoreV1().Events(nameSpace).List(options)
			},
			WatchFunc: func(options v1.ListOptions) (watch.Interface, error) {
				options.FieldSelector = "involvedObject.kind=Node,type=Normal,reason=NotReady"
				return kubeClient.CoreV1().Events(nameSpace).Watch(options)
			},
		},
		&core_v1.Event{},
		0,
		cache.Indexers{},
	)
	nodeNotReadyController := newResourceController(kubeClient, handler, notReadyNodeInformer, "NodeNotReady")

	nodeNotReadyStopCh := make(chan struct{})
	defer close(nodeNotReadyStopCh)
	go nodeNotReadyController.Run(nodeNotReadyStopCh)

	nodeRebootInformer := cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(options v1.ListOptions) (runtime.Object, error) {
				options.FieldSelector = "involvedObject.kind=Node,type=Warning,reason=Rebooted"
				return kubeClient.CoreV1().Events(nameSpace).List(options)
			},
			WatchFunc: func(options v1.ListOptions) (watch.Interface, error) {
				options.FieldSelector = "involvedObject.kind=Node,type=Warning,reason=Rebooted"
				return kubeClient.CoreV1().Events(nameSpace).Watch(options)
			},
		},
		&core_v1.Event{},
		0,
		cache.Indexers{},
	)
	nodeRebootedController := newResourceController(kubeClient, handler, nodeRebootInformer, "NodeRebooted")
	nodeRebootStopCh := make(chan struct{})
	defer close(nodeRebootStopCh)
	go nodeRebootedController.Run(nodeRebootStopCh)

	podNormalInformer := cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(options v1.ListOptions) (runtime.Object, error) {
				return kubeClient.CoreV1().Pods(nameSpace).List(options)
			},
			WatchFunc: func(options v1.ListOptions) (watch.Interface, error) {
				return kubeClient.CoreV1().Pods(nameSpace).Watch(options)
			},
		},
		&core_v1.Pod{},
		0,
		cache.Indexers{},
	)

	podNormalController := newResourceController(kubeClient, handler, podNormalInformer, "pod")

	podNormalChan := make(chan struct{})
	defer close(podNormalChan)

	go podNormalController.Run(podNormalChan)

	backOffPodInformer := cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(options v1.ListOptions) (runtime.Object, error) {
				options.FieldSelector = "involvedObject.kind=Pod,type=Warning,reason=Backoff"
				return kubeClient.CoreV1().Events(nameSpace).List(options)
			},
			WatchFunc: func(options v1.ListOptions) (watch.Interface, error) {
				options.FieldSelector = "involvedObject.kind=Pod,type=Warning,reason=Backoff"
				return kubeClient.CoreV1().Events(nameSpace).Watch(options)
			},
		},
		&core_v1.Event{},
		0,
		cache.Indexers{},
	)

	backOffPodController := newResourceController(kubeClient, handler, backOffPodInformer, "Backoff")

	backOffPodChan := make(chan struct{})
	defer close(backOffPodChan)

	go backOffPodController.Run(backOffPodChan)

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGTERM)
	signal.Notify(signals, syscall.SIGINT)
	<-signals
}

//newResourceController create resource by options,
func newResourceController(client kubernetes.Interface, eventHandler handler.Handler, informer cache.SharedIndexInformer, resourceType string) *Controller {
	queue := workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())
	var newEvent Event
	var err error
	informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			newEvent.key, err = cache.MetaNamespaceKeyFunc(obj)
			newEvent.eventType = "create"
			newEvent.nameSpace = util.GetObjectMetadata(obj).Namespace
			newEvent.resourceType = resourceType
			logrus.WithField("pkg", "watch-"+resourceType).Infof("Processing add to %v:%s", resourceType, newEvent.key)
			if err == nil {
				queue.Add(newEvent)
			}
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			newEvent.key, err = cache.MetaNamespaceKeyFunc(oldObj)
			newEvent.eventType = "update"
			newEvent.nameSpace = util.GetObjectMetadata(oldObj).Namespace
			newEvent.resourceType = resourceType
			logrus.WithField("pkg", "watch-"+resourceType).Infof("Processing update to %v:%s", resourceType, newEvent.key)
			if err == nil {
				queue.Add(newEvent)
			}
		},
		DeleteFunc: func(obj interface{}) {
			newEvent.key, err = cache.MetaNamespaceKeyFunc(obj)
			newEvent.eventType = "delete"
			newEvent.nameSpace = util.GetObjectMetadata(obj).Namespace
			newEvent.resourceType = resourceType
			logrus.WithField("pkg", "watch-"+resourceType).Infof("Processing delete to %v:%s", resourceType, newEvent.key)
			if err == nil {
				queue.Add(newEvent)
			}
		},
	})
	return &Controller{
		logger:       logrus.WithField("pkg", "watch-"+resourceType),
		clientSet:    client,
		queue:        queue,
		informer:     informer,
		eventHandler: eventHandler,
	}
}

func (c *Controller) Run(stopCh <-chan struct{}) {
	defer utilruntime.HandleCrash()
	defer c.queue.ShutDown()

	c.logger.Info("Starting watch controller")
	serverStartTime = time.Now().Local()

	go c.informer.Run(stopCh)

	if !cache.WaitForCacheSync(stopCh, c.HasSynced) {
		utilruntime.HandleError(fmt.Errorf("Time out waiting for cache sync"))
		return
	}

	c.logger.Info("Watch controller synced and ready")

	wait.Until(c.runWorker, time.Second, stopCh)
}

func (c *Controller) HasSynced() bool {
	return c.informer.HasSynced()
}

func (c *Controller) LastSyncResourceVersion() string {
	return c.informer.LastSyncResourceVersion()
}

func (c *Controller) runWorker() {
	for c.processNextItem() {

	}
}

func (c *Controller) processNextItem() bool {
	newEvent, quit := c.queue.Get()
	if quit {
		return false
	}
	defer c.queue.Done(newEvent)
	err := c.processItem(newEvent.(Event))
	if err == nil {
		c.queue.Forget(newEvent)
	} else if c.queue.NumRequeues(newEvent) < maxRetries {
		c.logger.Errorf("Error processing %s (will retry):%v", newEvent.(Event).key, err)
		c.queue.AddRateLimited(newEvent)
	} else {
		c.logger.Errorf("Error prcessing %s (will give up):%v", newEvent.(Event).key, err)
		c.queue.Forget(newEvent)
		utilruntime.HandleError(err)
	}
	return true
}

func (c *Controller) processItem(newEvent Event) error {
	obj, _, err := c.informer.GetIndexer().GetByKey(newEvent.key)
	if err != nil {
		return fmt.Errorf("Error fetching object with key %s from store: %v", newEvent.key, err)
	}
	metadata := util.GetObjectMetadata(obj)

	var status string

	if newEvent.nameSpace == "" && strings.Contains(newEvent.key, "/") {
		split := strings.Split(newEvent.key, "/")
		newEvent.nameSpace = split[0]
		newEvent.key = split[1]
	}

	switch newEvent.eventType {
	case "create":
		if metadata.CreationTimestamp.Sub(serverStartTime) > 0 {
			switch newEvent.resourceType {
			case "NodeNotReady":
				status = "Danger"
			case "NodeReady":
				status = "Normal"
			case "NodeReboot":
				status = "Danger"
			case "Backoff":
				status = "Danger"
			default:
				status = "Normal"
			}
			kbEvent := event.Event{
				Namespace: newEvent.nameSpace,
				Kind:      newEvent.resourceType,
				Status:    status,
				Name:      metadata.Name,
				Reason:    "Created",
			}
			c.eventHandler.Handle(kbEvent)
			return nil
		}
	case "update":
		switch newEvent.resourceType {
		case "BackOff":
			status = "Danger"
		default:
			status = "Warning"
		}
		kbEvent := event.Event{
			Namespace: newEvent.nameSpace,
			Kind:      newEvent.resourceType,
			Status:    status,
			Name:      metadata.Name,
			Reason:    "Updated",
		}
		c.eventHandler.Handle(kbEvent)
		return nil
	case "delete":
		kbEvent := event.Event{
			Namespace: newEvent.nameSpace,
			Kind:      newEvent.resourceType,
			Status:    "Danger",
			Name:      metadata.Name,
			Reason:    "Deleted",
		}
		c.eventHandler.Handle(kbEvent)
		return nil
	}

	return nil
}
