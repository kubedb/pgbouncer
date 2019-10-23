package controller

import (
	"errors"
	"strings"

	api "kubedb.dev/apimachinery/apis/kubedb/v1alpha1"
	"kubedb.dev/apimachinery/client/clientset/versioned/typed/kubedb/v1alpha1/util"

	"github.com/appscode/go/log"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	core_util "kmodules.xyz/client-go/core/v1"
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

func (c *Controller) managePgBouncerEvent(key string) error {
	log.Debugln("started processing, key:", key)
	obj, exists, err := c.pgInformer.GetIndexer().GetByKey(key)
	if err != nil {
		log.Errorf("Fetching object with key %s from store failed with %v", key, err)
		return err
	}
	if !exists {
		log.Debugf("PgBouncer %s does not exist anymore", key)
		splitKey := strings.Split(key, "/")

		if len(splitKey) != 2 || splitKey[0] == "" || splitKey[1] == "" {
			return errors.New("received unknown key")
		}
		//Now we are interested in this particular secret
		pgbouncerNamespace := splitKey[0]
		pgbouncerName := splitKey[1]
		_, err := c.Client.CoreV1().Secrets(pgbouncerNamespace).Get(pgbouncerName+"-auth", v1.GetOptions{})
		if err == nil {
			return c.removeDefaultSecret(pgbouncerNamespace, pgbouncerName+"-auth")
		}
		log.Infoln("pgbouncer default secret not found")

	} else {
		// Note that you also have to check the uid if you have a local controlled resource, which
		// is dependent on the actual instance, to detect that a PgBouncer was recreated with the same name
		pgbouncer := obj.(*api.PgBouncer).DeepCopy()
		if pgbouncer.DeletionTimestamp != nil {
			if core_util.HasFinalizer(pgbouncer.ObjectMeta, api.GenericKey) {
				if err := c.terminate(pgbouncer); err != nil {
					log.Errorln(err)
					return err
				}
				_, _, err = util.PatchPgBouncer(c.ExtClient.KubedbV1alpha1(), pgbouncer, func(in *api.PgBouncer) *api.PgBouncer {
					in.ObjectMeta = core_util.RemoveFinalizer(in.ObjectMeta, api.GenericKey)
					return in
				})
				return err
			}
		} else {
			pgbouncer, _, err = util.PatchPgBouncer(c.ExtClient.KubedbV1alpha1(), pgbouncer, func(in *api.PgBouncer) *api.PgBouncer {
				in.ObjectMeta = core_util.AddFinalizer(in.ObjectMeta, api.GenericKey)
				return in
			})
			if err != nil {
				return err
			}
			if err := c.create(pgbouncer); err != nil {
				log.Errorln(err)
				c.pushFailureEvent(pgbouncer, err.Error())
				return err
			}
		}
	}
	return nil
}
