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
	api "kubedb.dev/apimachinery/apis/kubedb/v1alpha1"
)

func (c *Controller) manageUserSecretEvent(key string) error {
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
		//TODO: ensure that we have a placeholder secret that has kubedb as a user in the userlist
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
	if pgbouncer.Spec.UserListSecretRef == nil {
		return nil
	}
	pbSecretName := pgbouncer.Spec.UserListSecretRef.Name
	pbSecretNamespace := pgbouncer.GetNamespace()

	if pbSecretNamespace == secretInfo[namespaceKey] && pbSecretName == secretInfo[nameKey] {
		log.Infof("secret %s update found for PgBouncer: %s", secretInfo[nameKey], pgbouncer.Name)
		secret, err := c.Client.CoreV1().Secrets(secretInfo[namespaceKey]).Get(secretInfo[nameKey], v1.GetOptions{})
		if err != nil {
			return err
		}
		c.ensureUserlistHasDefaultAdmin(pgbouncer, secret)
	}

	return nil
}

func (c *Controller) ensureUserlistHasDefaultAdmin(pgbouncer *api.PgBouncer, secret *core.Secret) {
	for key, value := range secret.Data {
		if key != "" && value != nil {
			kubedbUserString := fmt.Sprintf(`"%s" "%s"`, pbAdminUser, pbAdminPassword)
			if !strings.Contains(string(value), kubedbUserString) {
				tmpData := fmt.Sprintln(string(value)) + kubedbUserString
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
			}
			break
		}
	}
}

func (c *Controller) getSecretKeyValuePair(pgbouncer *api.PgBouncer) (key, value string, err error) {
	if pgbouncer.Spec.UserListSecretRef != nil {
		return "", "", errors.New("no secret has been defined yet")
	}
	pbSecretName := pgbouncer.Spec.UserListSecretRef.Name
	pbSecretNamespace := pgbouncer.GetNamespace()

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
	return key, value, nil
}

func (c *Controller) getSecretKey(pgbouncer *api.PgBouncer) (key string, err error) {
	if pgbouncer.Spec.UserListSecretRef == nil {
		return "", errors.New("no secret has been defined yet")
	}
	pbSecretName := pgbouncer.Spec.UserListSecretRef.Name
	pbSecretNamespace := pgbouncer.GetNamespace()

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
