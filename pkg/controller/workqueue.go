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
	kutil "kmodules.xyz/client-go"
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
				return err
			}
		}
	}
	return nil
}

func (c *Controller) runPgSecret(key string) error {
	//wait for pgboncer to ber ready
	log.Debugln("started processing, key:", key)
	_, exists, err := c.secretInformer.GetIndexer().GetByKey(key)
	if err != nil {
		log.Errorf("Fetching secret with key %s from store failed with %v", key, err)
		return err
	}
	splitKey := strings.Split(key, "/")

	if len(splitKey) != 2 || splitKey[0] == "" || splitKey[1] == "" {
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
		log.Debugf("PgBouncer Secret %s deleted.", key)

	} else {
		log.Debugf("Updates for PgBouncer Secret %s received.", key)
	}
	for _, pgbouncer := range pgBouncerList.Items {
		err := c.syncSecretWithPgBouncer(secretInfo, &pgbouncer)
		if err != nil {
			log.Warning(err)
		}
	}
	return nil
}

func (c *Controller) syncSecretWithPgBouncer(secretInfo map[string]string, pgbouncer *api.PgBouncer) error {
	if pgbouncer == nil {
		return errors.New("cant sync secret with pgbouncer == nil")
	}
	secretList := pgbouncer.Spec.SecretList
	for _, singleSecret := range secretList {
		if singleSecret.SecretNamespace == secretInfo["Namespace"] && singleSecret.SecretName == secretInfo["Name"] {
			log.Infof("Secret %s update found for PgBouncer: %s",secretInfo["Name"], pgbouncer.Name)
			//log.Infof(singleSecret.SecretNamespace, " ", singleSecret.SecretName, " == ", secretInfo["Namespace"], " ", secretInfo["Name"])
			vt, err := c.ensureConfigMapFromCRD(pgbouncer)
			if err != nil {
				return err
			}
			if vt != kutil.VerbUnchanged{
				log.Infof("%s configMap for PgBouncer = m%s and PgBouncer Secret = %s' ", vt, pgbouncer.Name, secretInfo["Name"])
			}
			break
		}
	}
	return nil
}
