package controller

import (
	"fmt"

	"github.com/appscode/go/log"
	"github.com/kubedb/apimachinery/apis"
	api "github.com/kubedb/apimachinery/apis/kubedb/v1alpha1"
	"github.com/kubedb/apimachinery/client/clientset/versioned/typed/kubedb/v1alpha1/util"
	core_util "kmodules.xyz/client-go/core/v1"
	"kmodules.xyz/client-go/tools/queue"
)

func (c *Controller) initWatcher() {
	fmt.Println(":001: =========>Informer")
	c.pgInformer = c.KubedbInformerFactory.Kubedb().V1alpha1().PgBouncers().Informer()
	fmt.Println(":002: =========>PgQueue")
	c.pgQueue = queue.New("PgBouncer", c.MaxNumRequeues, c.NumThreads, c.runPgBouncer)
	fmt.Println(":003: =========>PgLister")
	c.pbLister = c.KubedbInformerFactory.Kubedb().V1alpha1().PgBouncers().Lister()
	fmt.Println(":004: =========>EventHandler")
	c.pgInformer.AddEventHandler(queue.NewObservableUpdateHandler(c.pgQueue.GetQueue(), apis.EnableStatusSubresource))
}

func (c *Controller) runPgBouncer(key string) error {
	fmt.Println(":002.1: =========>runPgBouncer")
	log.Debugln("started processing, key:", key)
	obj, exists, err := c.pgInformer.GetIndexer().GetByKey(key)
	if err != nil {
		log.Errorf("Fetching object with key %s from store failed with %v", key, err)
		return err
	}

	if !exists {
		log.Debugf("PgBouncer %s does not exist anymore", key)
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
				pgbouncer, _, err = util.PatchPgBouncer(c.ExtClient.KubedbV1alpha1(), pgbouncer, func(in *api.PgBouncer) *api.PgBouncer {
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
				// c.pushFailureEvent(pgbouncer, err.Error())
				return err
			}
		}
	}
	return nil
}
