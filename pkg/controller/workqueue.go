package controller

import (
	"kmodules.xyz/client-go/tools/queue"
)

const (
	systemNamespace = "kube-system"
	publicNamespace = "kube-public"
	namespaceKey    = "namespace"
	nameKey         = "name"
	pbAdminUser     = "kubedb"
	pbAdminDatabase = "pgbouncer"
	pbAdminPassword = "pb-password"
	pbAdminData     = "pb-admin"
	pbUserData      = "pb-user"
)

func (c *Controller) initWatcher() {
	c.pgInformer = c.KubedbInformerFactory.Kubedb().V1alpha1().PgBouncers().Informer()
	c.pgQueue = queue.New("PgBouncer", c.MaxNumRequeues, c.NumThreads, c.managePgBouncerEvent)
	c.pbLister = c.KubedbInformerFactory.Kubedb().V1alpha1().PgBouncers().Lister()
	c.pgInformer.AddEventHandler(queue.NewObservableUpdateHandler(c.pgQueue.GetQueue(), true))
}

func (c *Controller) initSecretWatcher() {
	c.secretInformer = c.KubeInformerFactory.Core().V1().Secrets().Informer()
	c.secretQueue = queue.New("Secret", c.MaxNumRequeues, c.NumThreads, c.manageUserSecretEvent)
	c.secretLister = c.KubeInformerFactory.Core().V1().Secrets().Lister()
	c.secretInformer.AddEventHandler(queue.DefaultEventHandler(c.secretQueue.GetQueue()))
}

func (c *Controller) initAppBindingWatcher() {
	c.appBindingInformer = c.AppCatInformerFactory.Appcatalog().V1alpha1().AppBindings().Informer()
	c.appBindingQueue = queue.New("AppBinding", c.MaxNumRequeues, c.NumThreads, c.manageAppBindingEvent)
	c.appBindingLister = c.AppCatInformerFactory.Appcatalog().V1alpha1().AppBindings().Lister()
	c.appBindingInformer.AddEventHandler(queue.DefaultEventHandler(c.appBindingQueue.GetQueue()))
}
