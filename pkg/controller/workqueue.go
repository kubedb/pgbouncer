package controller

import (
	"errors"
	"strings"

	"github.com/appscode/go/log"
	core "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	core_util "kmodules.xyz/client-go/core/v1"
	"kmodules.xyz/client-go/tools/queue"
	"kubedb.dev/apimachinery/apis"
	api "kubedb.dev/apimachinery/apis/kubedb/v1alpha1"
	"kubedb.dev/apimachinery/client/clientset/versioned/typed/kubedb/v1alpha1/util"
)

const (
	systemNamespace = "kube-system"
	publicNamespace = "kube-public"
)

func (c *Controller) initWatcher() {
	c.pgInformer = c.KubedbInformerFactory.Kubedb().V1alpha1().PgBouncers().Informer()
	c.pgQueue = queue.New("PgBouncer", c.MaxNumRequeues, c.NumThreads, c.runPgBouncer)
	c.pbLister = c.KubedbInformerFactory.Kubedb().V1alpha1().PgBouncers().Lister()
	c.pgInformer.AddEventHandler(queue.NewObservableUpdateHandler(c.pgQueue.GetQueue(), apis.EnableStatusSubresource))
}

func (c *Controller) initSecretWatcher() {
	c.secretInformer = c.KubeInformerFactory.Core().V1().Secrets().Informer()
	c.secretQueue = queue.New("Secret", c.MaxNumRequeues, c.NumThreads, c.runPgSecret)
	c.secretLister = c.KubeInformerFactory.Core().V1().Secrets().Lister()
	c.secretInformer.AddEventHandler(queue.DefaultEventHandler(c.secretQueue.GetQueue()))
}

func (c *Controller) runPgBouncer(key string) error {
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

func (c *Controller) runPgSecret(key string) error {
	println(":003.1: =========>runPgSecret:::::::::::: Key = ", key)
	//log.Debugln("started processing, key:", key)
	_, exists, err := c.secretInformer.GetIndexer().GetByKey(key)
	if err != nil {
		log.Errorf("Fetching secret with key %s from store failed with %v", key, err)
		log.Infoln("Fetching secret with key %s from store failed with %v", key, err)
		return err
	}


	splitKey := strings.Split(key, "/")
	for i, s := range splitKey {
		println(i, " = ", s)
	}

	if len(splitKey) != 2 || splitKey[0] == "" || splitKey[1] == "" {
		println("len(splitKey) != 2")
		return nil
	}
	//Now we are interested in this particular secret
	secretInfo := make(map[string]string)
	secretInfo["Namespace"] = splitKey[0]
	secretInfo["Name"] = splitKey[1]
	if secretInfo["Namespace"] == systemNamespace || secretInfo["Namespace"] == publicNamespace {
		return nil
	}
	pgBouncerList, err := c.ExtClient.KubedbV1alpha1().PgBouncers(core.NamespaceAll).List(v1.ListOptions{})
	if err != nil {
		return err
	}

	if !exists {
		log.Debugf("PgSecret %s does not exist anymore", key)
		log.Infof("==========Delete event for secret %s ==========", key)

	} else {
		// Note that you also have to check the uid if you have a local controlled resource, which
		// is dependent on the actual instance, to detect that a PgBouncer was recreated with the same name
		//deepSecret := secretObj.(*core.Secret).DeepCopy()
		//if deepSecret.Namespace == systemNamespace{
		//	return nil
		//}
		//pgBouncerList, err := c.ExtClient.KubedbV1alpha1().PgBouncers(core.NamespaceAll).List(v1.ListOptions{})
		//if err != nil{
		//	if kerr.IsNotFound(err){
		//
		//	}
		//	return err
		//}
		//println("::::::: Look for ",secret.Namespace, "==>", secret.Name, "in all PgBouncers")
		//for _, pgbouncer := range pgBouncerList.Items {
		//	println(pgbouncer.Name)
		//	err := c.syncSecretWithPgBouncer(secret, &pgbouncer)
		//	if err != nil {
		//		log.Warning(err)
		//	}
		//}
		log.Infof("==========Create/Update event for secret %s ==========", key)
	}
	for _, pgbouncer := range pgBouncerList.Items {
		println(pgbouncer.Name)
		err := c.syncSecretWithPgBouncer(secretInfo, &pgbouncer)
		if err != nil {
			log.Warning(err)
		}
	}
	println(":003.1: =========>runPgSecret Done")
	return nil
}

func (c *Controller) syncSecretWithPgBouncer(secretInfo map[string]string, pgbouncer *api.PgBouncer) error {
	if pgbouncer == nil {
		println("pgbouncer == nil")
		return errors.New("pgbouncer == nil")
	}
	secretList := pgbouncer.Spec.SecretList
	for _, singleSecret := range secretList {
		if singleSecret.SecretNamespace == secretInfo["Namespace"] && singleSecret.SecretName == secretInfo["Name"] {
			println(singleSecret.SecretNamespace, " ", singleSecret.SecretName, " == ", secretInfo["Namespace"], " ", secretInfo["Name"])
			println("Ensure config:")
			vt, err := c.ensureConfigMapFromCRD(pgbouncer)
			if err != nil {
				println("ensureConfigMapFromCRD err= ", err)
				return err
			}
			println("Verbtype = ", vt+" :::for secret.Name = ", secretInfo["Name"], "pb name = ", pgbouncer.Name)
			break
		} else {
			println(singleSecret.SecretNamespace, " ", singleSecret.SecretName, " != ", secretInfo["Namespace"], " ", secretInfo["Name"])
		}
	}
	return nil
}

/*
log.Debugln("started processing, key:", key)
	secretObj, exists, err := c.secretInformer.GetIndexer().GetByKey(key)
	if err != nil {
		log.Errorf("Fetching secret with key %s from store failed with %v", key, err)
		return err
	}

	if !exists {
		log.Debugf("PgSecret %s does not exist anymore", key)
	} else {
		// Note that you also have to check the uid if you have a local controlled resource, which
		// is dependent on the actual instance, to detect that a PgBouncer was recreated with the same name
		deepSecret := secretObj.(*core.Secret).DeepCopy()
		if deepSecret.DeletionTimestamp != nil {
			if core_util.HasFinalizer(deepSecret.ObjectMeta, api.GenericKey) {
				//secret is getting deleted.
				log.Infoln("====================")
				log.Infoln("==========Delete event for secrets==========")
				log.Infoln("====================")
			}
		} else {
			// it is a create or update event
			log.Infoln("====================")
			log.Infoln("==========Create or update event for secrets==========")
			log.Infoln("====================")
		}
	}
*/
