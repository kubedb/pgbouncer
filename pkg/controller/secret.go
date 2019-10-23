package controller

import (
	"errors"
	"fmt"
	"strings"

	"github.com/appscode/go/crypto/rand"
	"github.com/appscode/go/log"
	core "k8s.io/api/core/v1"
	kerr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	kutil "kmodules.xyz/client-go"
	api "kubedb.dev/apimachinery/apis/kubedb/v1alpha1"
)

func (c *Controller) manageUserSecretEvent(key string) error {
	//wait for pgbouncer to be ready
	log.Debugln("started processing secret, key:", key)
	_, exists, err := c.secretInformer.GetIndexer().GetByKey(key)
	if err != nil {
		log.Errorf("Fetching secret with key %s from store failed with %v", key, err)
		return err
	}
	splitKey := strings.Split(key, "/")

	if len(splitKey) != 2 || splitKey[0] == "" || splitKey[1] == "" {
		return errors.New("received unknown key")
	}
	//Now we are interested in this particular secret
	secretInfo := make(map[string]string)
	secretInfo[namespaceKey] = splitKey[0]
	secretInfo[nameKey] = splitKey[1]
	if secretInfo[namespaceKey] == systemNamespace || secretInfo[namespaceKey] == publicNamespace {
		log.Infoln("Secret updates in kube-system and kube-public are ignored by PgBouncer")
		return nil
	}

	pgBouncerList, err := c.ExtClient.KubedbV1alpha1().PgBouncers(core.NamespaceAll).List(v1.ListOptions{})
	if err != nil {
		return err
	}
	for _, pgbouncer := range pgBouncerList.Items {
		err := c.checkForPgBouncerSecret(&pgbouncer, secretInfo, exists)
		if err != nil {
			log.Warning(err)
		}
	}
	return nil
}

func (c *Controller) checkForPgBouncerSecret(pgbouncer *api.PgBouncer, secretInfo map[string]string, secretExists bool) error {
	if pgbouncer == nil {
		return errors.New("cant sync secret with pgbouncer == nil")
	}
	if pgbouncer.GetNamespace() != secretInfo[namespaceKey] {
		return nil
	}
	//three possibilties:
	//1. it may or may not be fallback secret
	//2a. There might be no secret associated with this pg bouncer
	//2b. this might be a user provided secret

	if fSecretSpec := c.GetDefaultSecretSpec(pgbouncer); fSecretSpec.Name == secretInfo[nameKey] {
		//its an event for fallback secret, which must always stay in kubedb provided form
		println("====> Update for admin Secret")
		if _, err := c.CreateOrPatchDefaultSecret(pgbouncer); err != nil {
			return err
		}
	} else if pgbouncer.Spec.UserListSecretRef == nil || pgbouncer.Spec.UserListSecretRef.Name == "" {
		return nil
	} else if pgbouncer.Spec.UserListSecretRef.Name == secretInfo[nameKey] {
		//ensure that default admin credentials are set
		// in case there is an update of the user provided secret
		//ensure that the default secret is updated as well
		println("====> Update for user Secret")
		if _, err := c.CreateOrPatchDefaultSecret(pgbouncer); err != nil {
			return err
		}
	}
	//need to ensure statefulset to mount volume containing the list, and configmap to load userlist from that path
	//if err := c.manageStatefulSet(pgbouncer); err != nil {
	//	return err
	//}
	if err := c.manageConfigMap(pgbouncer); err != nil {
		return err
	}
	println("Secret update managed")

	return nil
}

//func (c *Controller) patchUserListWithDefaultAdmin(pgbouncer *api.PgBouncer, secret *core.Secret) {
//	for key, value := range secret.Data {
//		if key != "" && value != nil {
//			adminSecretSpec := c.GetDefaultSecretSpec(pgbouncer)
//			err := c.waitUntilAdminSecretReady(pgbouncer, adminSecretSpec)
//			if err != nil {
//				log.Fatal(err)
//			}
//			adminSecret, err := c.Client.CoreV1().Secrets(adminSecretSpec.Namespace).Get(adminSecretSpec.Name, v1.GetOptions{})
//			if err != nil {
//				log.Fatal(err)
//			}
//			adminData := string(adminSecret.Data[pbAdminData])
//
//			println("==========adminSecret data  = ", string(adminSecret.Data[pbAdminData]))
//			kubedbUserString := fmt.Sprintf(`%s`, adminData)
//			if !strings.Contains(string(value), kubedbUserString) {
//				tmpData := fmt.Sprintln(string(value)) + kubedbUserString
//				secret.Data[key] = []byte(tmpData)
//				_, vt, err := core_util.CreateOrPatchSecret(c.Client, secret.ObjectMeta, func(in *core.Secret) *core.Secret {
//					in = secret
//					return in
//				}, false)
//				if err != nil {
//					log.Infoln("error patching secret with modified file, err = ", err)
//				}
//				if vt == kutil.VerbPatched {
//					log.Infoln("secret patched with kubedb as an admin")
//				}
//			}
//			break
//		}
//	}
//}

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

func (c *Controller) GetDefaultSecretSpec(pgbouncer *api.PgBouncer) *core.Secret {
	return &core.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pgbouncer.GetName() + "-auth",
			Namespace: pgbouncer.Namespace,
			Labels:    pgbouncer.OffshootLabels(),
		},
	}
}

func (c *Controller) CreateOrPatchDefaultSecret(pgbouncer *api.PgBouncer) (kutil.VerbType, error) {
	var myPgBouncerPass = ""
	var adminSecretExists = true
	var vt kutil.VerbType
	vt = kutil.VerbCreated
	secretSpec := c.GetDefaultSecretSpec(pgbouncer)
	secret, err := c.Client.CoreV1().Secrets(secretSpec.Namespace).Get(secretSpec.Name, v1.GetOptions{})
	if err == nil {
		myPgBouncerPass = string(secret.Data[pbAdminPassword])
	}
	if kerr.IsNotFound(err) {
		myPgBouncerPass = rand.WithUniqSuffix(pbAdminUser)
		adminSecretExists = false
	}
	var myPgBouncerAdminData = fmt.Sprintf(`"%s" "%s"`, pbAdminUser, myPgBouncerPass)
	mySecretData := map[string]string{
		pbAdminData:     myPgBouncerAdminData,
		pbAdminPassword: myPgBouncerPass,
	}

	userSecretExists, userSecret, err := c.isUserSecretExists(pgbouncer)
	if err != nil {
		println("!!!!!!!!!!!!!!!!!!!!!!!!")
		return "", err
	}
	println("++++++userSecretExists= ", userSecretExists)
	if userSecretExists {
		userSecretData := c.getUserListSecretData(userSecret, "")
		myUserSecertData := fmt.Sprintln(myPgBouncerAdminData) + string(userSecretData)
		//i := map[string]string{pbUserData: myUserSecertData}
		//mySecretData = core_util.UpsertMap(mySecretData,i)
		//mySecretData = map[string]string{
		//	pbAdminData:     myPgBouncerAdminData,
		//	pbAdminPassword: myPgBouncerPass,
		//	pbUserData:      myUserSecertData,
		//}
		mySecretData[pbUserData] = myUserSecertData
	}

	log.Infoln("User Secret finally = ", mySecretData)
	secretSpec.StringData = mySecretData
	//_, vt, err := core_util.CreateOrPatchSecret(c.Client, secretSpec.ObjectMeta, func(in *core.Secret) *core.Secret {
	//	in.StringData = mySecretData
	//	return in
	//}, true)

	if adminSecretExists {
		var mismatch = false
		if len(secret.Data) != len(mySecretData) {
			mismatch = true
		} else {
			for key, value := range secret.Data {
				if string(value) != mySecretData[key] {
					mismatch = true
					break
				}
			}
		}

		if mismatch {
			err = c.Client.CoreV1().Secrets(pgbouncer.Namespace).Delete(secret.Name, &v1.DeleteOptions{})
			if err != nil {
				return "", err
			}
			_, err = c.Client.CoreV1().Secrets(pgbouncer.Namespace).Create(secretSpec)
			if err != nil {
				return "", err
			}
			vt = kutil.VerbPatched
		}
		vt = kutil.VerbUnchanged

	} else {
		_, err = c.Client.CoreV1().Secrets(pgbouncer.Namespace).Create(secretSpec)
		if err != nil {
			return "", err
		}
		vt = kutil.VerbCreated
	}
	return vt, err
}

func (c *Controller) waitUntilAdminSecretReady(pgbouncer *api.PgBouncer, secret *core.Secret) error {
	log.Infoln("Waiting for fallback Admin secret")
	return wait.PollImmediate(kutil.RetryInterval, kutil.ReadinessTimeout, func() (bool, error) {
		_, err := c.Client.CoreV1().Secrets(secret.Namespace).Get(secret.Name, v1.GetOptions{})
		if err == nil {
			return true, nil
		}
		if kerr.IsNotFound(err) {
			return false, nil
		}
		return false, err
	})
}

func (c *Controller) isUserSecretExists(pgbouncer *api.PgBouncer) (bool, *core.Secret, error) {
	if pgbouncer.Spec.UserListSecretRef == nil && pgbouncer.Spec.UserListSecretRef.Name == "" {
		return false, nil, nil
	}
	secret, err := c.Client.CoreV1().Secrets(pgbouncer.Namespace).Get(pgbouncer.Spec.UserListSecretRef.Name, v1.GetOptions{})
	if err == nil {
		return true, secret, nil
	} else if kerr.IsNotFound(err) {
		return false, nil, nil
	}
	return false, nil, err
}

func (c *Controller) getUserListSecretData(userSecret *core.Secret, key string) []byte {
	if key == "" { //no key provided, return any
		for _, value := range userSecret.Data {
			return value
		}
	}
	return userSecret.Data[key]
}

func (c *Controller) removeDefaultSecret(namespace string, name string) error {
	return c.Client.CoreV1().Secrets(namespace).Delete(name, &v1.DeleteOptions{})
}

//func (c *Controller) createOrPatchDefaultSecret(pgbouncer *api.PgBouncer, adminSecretExists bool)  error{
//
//}
