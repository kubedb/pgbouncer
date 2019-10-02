package controller

import (
	"github.com/appscode/go/encoding/json/types"
	"github.com/appscode/go/log"
	pcm "github.com/coreos/prometheus-operator/pkg/client/versioned/typed/monitoring/v1"
	core "k8s.io/api/core/v1"
	crd_api "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	crd_cs "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/typed/apiextensions/v1beta1"
	"k8s.io/apimachinery/pkg/labels"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	corev1_listers "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	reg_util "kmodules.xyz/client-go/admissionregistration/v1beta1"
	apiext_util "kmodules.xyz/client-go/apiextensions/v1beta1"
	meta_util "kmodules.xyz/client-go/meta"
	"kmodules.xyz/client-go/tools/queue"
	appcat_util "kmodules.xyz/custom-resources/apis/appcatalog/v1alpha1"
	appcat_cs "kmodules.xyz/custom-resources/client/clientset/versioned"
	appcat_listers "kmodules.xyz/custom-resources/client/listers/appcatalog/v1alpha1"
	"kubedb.dev/apimachinery/apis"
	catalog "kubedb.dev/apimachinery/apis/catalog/v1alpha1"
	api "kubedb.dev/apimachinery/apis/kubedb/v1alpha1"
	cs "kubedb.dev/apimachinery/client/clientset/versioned"
	kutildb "kubedb.dev/apimachinery/client/clientset/versioned/typed/kubedb/v1alpha1/util"
	api_listers "kubedb.dev/apimachinery/client/listers/kubedb/v1alpha1"
	amc "kubedb.dev/apimachinery/pkg/controller"
	"kubedb.dev/apimachinery/pkg/eventer"
)

type Controller struct {
	amc.Config
	*amc.Controller

	// Prometheus client
	promClient pcm.MonitoringV1Interface
	// Event Recorder
	recorder record.EventRecorder
	// labelselector for event-handler of Snapshot, Dormant and Job
	selector labels.Selector

	// PgBouncer
	pgQueue    *queue.Worker
	pgInformer cache.SharedIndexInformer
	pbLister   api_listers.PgBouncerLister
	// Secret
	secretQueue    *queue.Worker
	secretInformer cache.SharedIndexInformer
	secretLister   corev1_listers.SecretLister

	appBindingQueue    *queue.Worker
	appBindingInformer cache.SharedIndexInformer
	appBindingLister   appcat_listers.AppBindingLister
}

func (c *Controller) WaitUntilPaused(*api.DormantDatabase) error {
	panic("implement me")
}

func (c *Controller) WipeOutDatabase(*api.DormantDatabase) error {
	panic("implement me")
}

var _ amc.Deleter = &Controller{}

func New(
	clientConfig *rest.Config,
	client kubernetes.Interface,
	apiExtKubeClient crd_cs.ApiextensionsV1beta1Interface,
	extClient cs.Interface,
	dc dynamic.Interface,
	appCatalogClient appcat_cs.Interface,
	promClient pcm.MonitoringV1Interface,
	opt amc.Config,
	recorder record.EventRecorder,
) *Controller {
	return &Controller{
		Controller: &amc.Controller{
			ClientConfig:     clientConfig,
			Client:           client,
			ExtClient:        extClient,
			ApiExtKubeClient: apiExtKubeClient,
			DynamicClient:    dc,
			AppCatalogClient: appCatalogClient,
		},
		Config:     opt,
		promClient: promClient,
		recorder:   recorder,
		selector: labels.SelectorFromSet(map[string]string{
			api.LabelDatabaseKind: api.ResourceKindPgBouncer,
		}),
	}
}

// Ensuring Custom Resource Definitions
func (c *Controller) EnsureCustomResourceDefinitions() error {
	log.Infoln("Ensuring CustomResourceDefinition...")
	crds := []*crd_api.CustomResourceDefinition{
		api.PgBouncer{}.CustomResourceDefinition(),
		catalog.PgBouncerVersion{}.CustomResourceDefinition(),
		appcat_util.AppBinding{}.CustomResourceDefinition(),
	}
	log.Infoln("Ensured CustomResourceDefinitions")
	err := apiext_util.RegisterCRDs(c.ApiExtKubeClient, crds)
	println(" err  = ", err)
	return err
}

// InitInformer initializes PgBouncer, DormantDB amd Snapshot watcher
func (c *Controller) Init() error {
	c.initWatcher()
	c.initSecretWatcher()
	c.initAppBindingWatcher()
	return nil
}

// RunControllers runs queue.worker
func (c *Controller) RunControllers(stopCh <-chan struct{}) {
	c.pgQueue.Run(stopCh)
	c.secretQueue.Run(stopCh)
	c.appBindingQueue.Run(stopCh)
}

// Blocks caller. Intended to be called as a Go routine.
func (c *Controller) Run(stopCh <-chan struct{}) {
	go c.StartAndRunControllers(stopCh)

	if c.EnableMutatingWebhook {
		cancel1, _ := reg_util.SyncMutatingWebhookCABundle(c.ClientConfig, mutatingWebhookConfig)
		defer cancel1()
	}
	if c.EnableValidatingWebhook {
		cancel2, _ := reg_util.SyncValidatingWebhookCABundle(c.ClientConfig, validatingWebhookConfig)
		defer cancel2()
	}

	<-stopCh
}

// StartAndRunControllers starts InformetFactory and runs queue.worker
func (c *Controller) StartAndRunControllers(stopCh <-chan struct{}) {
	defer utilruntime.HandleCrash()

	log.Infoln("Starting KubeDB controller")
	c.KubeInformerFactory.Start(stopCh)
	c.KubedbInformerFactory.Start(stopCh)
	c.AppCatInformerFactory.Start(stopCh)

	// Wait for all involved caches to be synced, before processing items from the queue is started
	for t, v := range c.KubeInformerFactory.WaitForCacheSync(stopCh) {
		if !v {
			log.Fatalf("%v timed out waiting for caches to sync", t)
			return
		}
	}
	for t, v := range c.KubedbInformerFactory.WaitForCacheSync(stopCh) {
		if !v {
			log.Fatalf("%v timed out waiting for caches to sync", t)
			return
		}
	}
	for t, v := range c.AppCatInformerFactory.WaitForCacheSync(stopCh) {
		if !v {
			log.Fatalf("%v timed out waiting for appCatalog caches to sync", t)
			return
		}
	}

	c.RunControllers(stopCh)

	<-stopCh
	log.Infoln("Stopping KubeDB controller")
}

func (c *Controller) pushFailureEvent(pgbouncer *api.PgBouncer, reason string) {
	c.recorder.Eventf(
		pgbouncer,
		core.EventTypeWarning,
		eventer.EventReasonFailedToStart,
		`Fail to be ready PgBouncer: "%v". Reason: %v`,
		pgbouncer.Name,
		reason,
	)

	pg, err := kutildb.UpdatePgBouncerStatus(c.ExtClient.KubedbV1alpha1(), pgbouncer, func(in *api.PgBouncerStatus) *api.PgBouncerStatus {
		in.Phase = api.DatabasePhaseFailed
		in.Reason = reason
		in.ObservedGeneration = types.NewIntHash(pgbouncer.Generation, meta_util.GenerationHash(pgbouncer))
		return in
	}, apis.EnableStatusSubresource)
	if err != nil {
		c.recorder.Eventf(
			pgbouncer,
			core.EventTypeWarning,
			eventer.EventReasonFailedToUpdate,
			err.Error(),
		)
	}
	pgbouncer.Status = pg.Status
}
