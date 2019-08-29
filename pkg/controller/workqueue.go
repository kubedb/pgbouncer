package controller

import (
	"errors"
	"fmt"
	"strings"

	"github.com/appscode/go/log"
	core "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kutil "kmodules.xyz/client-go"
	core_util "kmodules.xyz/client-go/core/v1"
	"kmodules.xyz/client-go/tools/queue"
	"kubedb.dev/apimachinery/apis"
	api "kubedb.dev/apimachinery/apis/kubedb/v1alpha1"
	"kubedb.dev/apimachinery/client/clientset/versioned/typed/kubedb/v1alpha1/util"
)

const (
	systemNamespace   = "kube-system"
	publicNamespace   = "kube-public"
	namespaceKey      = "namespace"
	nameKey           = "name"
	pbAdminUser = "pgbouncer"
	pbAdminPassword = "kubedb"
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
	log.Debugln("started processing secret, key:", key)
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
	secretInfo[namespaceKey] = splitKey[0]
	secretInfo[nameKey] = splitKey[1]
	if secretInfo[namespaceKey] == systemNamespace || secretInfo[namespaceKey] == publicNamespace {
		return nil
	}
	if !exists {
		log.Debugf("PgBouncer Secret %s deleted.", key)

	} else {
		log.Debugf("Updates for PgBouncer Secret %s received.", key)
	}
	pgBouncerList, err := c.ExtClient.KubedbV1alpha1().PgBouncers(core.NamespaceAll).List(v1.ListOptions{})
	if err != nil {
		return err
	}
	for _, pgbouncer := range pgBouncerList.Items {
		err := c.ensureUserListInSecret(secretInfo, &pgbouncer)
		if err != nil {
			log.Warning(err)
		}
	}
	return nil
}

func (c *Controller) ensureUserListInSecret(secretInfo map[string]string, pgbouncer *api.PgBouncer) error {
	if pgbouncer == nil {
		return errors.New("cant sync secret with pgbouncer == nil")
	}
	pbSecretName := pgbouncer.Spec.UserList.SecretName
	pbSecretNamespace := pgbouncer.Spec.UserList.SecretNamespace

	if pbSecretNamespace == secretInfo[namespaceKey] && pbSecretName == secretInfo[nameKey] {
		log.Infof("secret %s update found for PgBouncer: %s", secretInfo[nameKey], pgbouncer.Name)
		secret, err := c.Client.CoreV1().Secrets(secretInfo[namespaceKey]).Get(secretInfo[nameKey], v1.GetOptions{})
		if err != nil {
			return err
		}
		//TODO: ensure that changes to secrets result in secret being updated in pod (No need to RELOAD)
		//TODO: ensure that we have kubedb as a user in the userlist
		c.ensureUserlistHasDefaultAdmin(pgbouncer, secret)
	}

	return nil
}

func (c *Controller) ensureUserlistHasDefaultAdmin(pgbouncer *api.PgBouncer, secret *core.Secret) {
	for key, value := range secret.Data {
		if key != "" && value != nil {
			kubedbUserString := fmt.Sprintf(`"%s" "%s"`, pbAdminUser, pbAdminPassword)
			if !strings.Contains(string(value), kubedbUserString) {
				tmpData := string(value) + fmt.Sprintf(`
%s`, kubedbUserString)
				secret.Data[key] = []byte(tmpData)
				_, vt, err := core_util.CreateOrPatchSecret(c.Client, secret.ObjectMeta, func(in *core.Secret) *core.Secret {
					in = secret
					return in
				}, false)
				if err != nil {
					log.Infoln("error patching secret with modified file, err = ", err)
				}
				if vt == kutil.VerbPatched {
					log.Infoln("secret patched with kubedb as an admin")
				}

				//Annotate secret to mark that secret is already patched
			}
			break
		}
	}
}

func (c *Controller) getSecretKeyValuePair(pgbouncer *api.PgBouncer) (key, value string, err error) {
	pbSecretName := pgbouncer.Spec.UserList.SecretName
	pbSecretNamespace := pgbouncer.Spec.UserList.SecretNamespace
	if pbSecretName == "" && pbSecretNamespace == "" {
		return "", "", errors.New("no secret has been defined yet")
	}
	sec, err := c.Client.CoreV1().Secrets(pbSecretNamespace).Get(pbSecretName, metav1.GetOptions{})
	if err != nil {
			//secret has not been created yet, which is fine. We have watcher to take action when its created
			return "", "", err
	}
	for k, v := range sec.Data {
		if k != "" && v != nil {
			key = k
			value = string(v)
			break
		}
	}
	return key,value, nil
}

func (c *Controller) getSecretKey(pgbouncer *api.PgBouncer) (key string, err error) {
	pbSecretName := pgbouncer.Spec.UserList.SecretName
	pbSecretNamespace := pgbouncer.Spec.UserList.SecretNamespace
	if pbSecretName == "" && pbSecretNamespace == "" {
		return "", errors.New("no secret has been defined yet")
	}
	sec, err := c.Client.CoreV1().Secrets(pbSecretNamespace).Get(pbSecretName, metav1.GetOptions{})
	if err != nil {
		//secret has not been created yet, which is fine. We have watcher to take action when its created
		return "", err
	}
	for k, v := range sec.Data {
		if k != "" && v != nil {
			key = k
			break
		}
	}
	return key, nil
}
