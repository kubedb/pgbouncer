package controller

import (
	"strings"

	"github.com/appscode/go/log"
	core "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientsetscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/reference"
	kutil "kmodules.xyz/client-go"
	core_util "kmodules.xyz/client-go/core/v1"
	appcat "kmodules.xyz/custom-resources/apis/appcatalog/v1alpha1"
	appcat_util "kmodules.xyz/custom-resources/client/clientset/versioned/typed/appcatalog/v1alpha1/util"
	api "kubedb.dev/apimachinery/apis/kubedb/v1alpha1"
	"kubedb.dev/apimachinery/pkg/eventer"
)

func (c *Controller) ensureAppBinding(db *api.PgBouncer) (kutil.VerbType, error) {
	appmeta := db.AppBindingMeta()

	meta := metav1.ObjectMeta{
		Name:      appmeta.Name(),
		Namespace: db.Namespace,
	}

	ref, err := reference.GetReference(clientsetscheme.Scheme, db)
	if err != nil {
		return kutil.VerbUnchanged, err
	}

	_, vt, err := appcat_util.CreateOrPatchAppBinding(c.AppCatalogClient.AppcatalogV1alpha1(), meta, func(in *appcat.AppBinding) *appcat.AppBinding {
		core_util.EnsureOwnerReference(&in.ObjectMeta, ref)
		in.Labels = db.OffshootLabels()
		in.Annotations = db.Spec.ServiceTemplate.Annotations

		in.Spec.Type = appmeta.Type()
		in.Spec.ClientConfig.InsecureSkipTLSVerify = false
		in.Spec.SecretTransforms = []appcat.SecretTransform{
			{
				RenameKey: &appcat.RenameKeyTransform{
					From: PostgresUser,
					To:   appcat.KeyUsername,
				},
			},
			{
				RenameKey: &appcat.RenameKeyTransform{
					From: PostgresPassword,
					To:   appcat.KeyPassword,
				},
			},
		}
		return in
	})

	if err != nil {
		return kutil.VerbUnchanged, err
	} else if vt != kutil.VerbUnchanged {
		c.recorder.Eventf(
			db,
			core.EventTypeNormal,
			eventer.EventReasonSuccessful,
			"Successfully %s appbinding",
			vt,
		)
	}
	return vt, nil
}

func (c *Controller) manageAppBindingEvent(key string) error {
	//wait for pgboncer to ber ready
	log.Debugln("started processing appBindings, key:", key)
	_, exists, err := c.appBindingInformer.GetIndexer().GetByKey(key)
	if err != nil {
		log.Errorf("Fetching appBinding with key %s from store failed with %v", key, err)
		return err
	}
	splitKey := strings.Split(key, "/")

	if len(splitKey) != 2 || splitKey[0] == "" || splitKey[1] == "" {
		return nil
	}
	//Now we are interested in this particular appBinding
	appBindingInfo := make(map[string]string)
	appBindingInfo[namespaceKey] = splitKey[0]
	appBindingInfo[nameKey] = splitKey[1]
	if appBindingInfo[namespaceKey] == systemNamespace || appBindingInfo[namespaceKey] == publicNamespace {
		return nil
	}
	if !exists {
		log.Debugf("PgBouncer Secret %s deleted.", key)

	} else {
		log.Debugf("Updates for PgBouncer Secret %s received.", key)
	}
	pgBouncerList, err := c.ExtClient.KubedbV1alpha1().PgBouncers(core.NamespaceAll).List(metav1.ListOptions{})
	if err != nil {
		return err
	}
	for _, pgBouncer := range pgBouncerList.Items {
		if pgBouncer.GetNamespace() == appBindingInfo[namespaceKey] {
			err := c.checkAppBindingsInPgBouncerDatabases(appBindingInfo, &pgBouncer)
			if err != nil {
				log.Warning(err)
			}
		}
	}
	return nil
}
func (c *Controller) checkAppBindingsInPgBouncerDatabases(appBindingInfo map[string]string, pgbouncer *api.PgBouncer) error {
	if pgbouncer.Spec.Databases != nil && len(pgbouncer.Spec.Databases) > 0 {
		for _, db := range pgbouncer.Spec.Databases {
			if db.DatabaseRef.Name == appBindingInfo[nameKey] {
				log.Infoln("A matching appBinding is found.")
				err := c.manageService(pgbouncer)
				if err != nil {
					return err
				}
				err = c.manageConfigMap(pgbouncer)
				if err != nil {
					return err
				}
			}
		}
	}
	return nil
}
