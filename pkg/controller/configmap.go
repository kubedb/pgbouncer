package controller

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/appscode/go/log"
	"github.com/pkg/errors"
	core "k8s.io/api/core/v1"
	kerr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/wait"
	clientsetscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/reference"
	kutil "kmodules.xyz/client-go"
	core_util "kmodules.xyz/client-go/core/v1"
	"kmodules.xyz/client-go/tools/exec"
	api "kubedb.dev/apimachinery/apis/kubedb/v1alpha1"
	le "kubedb.dev/pgbouncer/pkg/leader_election"
)

const (
	PostgresPassword   = "POSTGRES_PASSWORD"
	PostgresUser       = "POSTGRES_USER"
	pgbouncerAdminName = "pgbouncer"
	PbRetryInterval    = time.Second * 5
	DefaultHostPort    = 5432
)

func (c *Controller) deleteLeaderLockConfigMap(meta metav1.ObjectMeta) error {
	if err := c.Client.CoreV1().ConfigMaps(meta.Namespace).Delete(le.GetLeaderLockName(meta.Name), nil); !kerr.IsNotFound(err) {
		return err
	}
	return nil
}

func (c *Controller) ensureConfigMapFromCRD(pgbouncer *api.PgBouncer) (kutil.VerbType, error) {
	configMapMeta := metav1.ObjectMeta{
		Name:      pgbouncer.OffshootName(),
		Namespace: pgbouncer.Namespace,
	}
	ref, rerr := reference.GetReference(clientsetscheme.Scheme, pgbouncer)
	if rerr != nil {
		return kutil.VerbUnchanged, rerr
	}

	cfgMap, vt, err := core_util.CreateOrPatchConfigMap(c.Client, configMapMeta, func(in *core.ConfigMap) *core.ConfigMap {
		var dbinfo = `[databases]
`
		var pbinfo = `[pgbouncer]
auth_file = /etc/config/userlist.txt
logfile = /tmp/pgbouncer.log
pidfile = /tmp/pgbouncer.pid
`
		var admins string
		var userListData string

		in.ObjectMeta.Annotations = map[string]string{
				"podConfigMap":"ready",
		}

		in.Labels = pgbouncer.OffshootLabels()
		core_util.EnsureOwnerReference(&in.ObjectMeta, ref)
		if pgbouncer.Spec.Databases != nil {
			for _, db := range pgbouncer.Spec.Databases {

				var hostPort = int32(DefaultHostPort)
				namespace := db.AppBindingNamespace
				name := db.AppBindingName
				appBinding, err := c.AppCatalogClient.AppBindings(namespace).Get(name, metav1.GetOptions{})
				if err != nil {
					if kerr.IsNotFound(err) {
						println("TODO: expect appbinding ", name, " to be ready later (currently just skipping)")
						log.Warning(err)
					} else {
						log.Error(err)
					}
					continue //Dont add pgbouncer databse base for this non existent appbinding
				}
				if appBinding.Spec.ClientConfig.Service != nil {
					name = appBinding.Spec.ClientConfig.Service.Name
					namespace = appBinding.Namespace
					hostPort = appBinding.Spec.ClientConfig.Service.Port
				}

				hostname := name + "." + namespace + ".svc.cluster.local"

				//dbinfo
				dbinfo = dbinfo + fmt.Sprintf(`%s = host=%s port=%d dbname=%s
`, db.Alias, hostname, hostPort, db.DbName)
			}
		}

		if pgbouncer.Spec.SecretList != nil {
			for _, secretListItem := range pgbouncer.Spec.SecretList {
				username, password, err := c.getDbCredentials(secretListItem)
				if err != nil {
					if kerr.IsNotFound(err) {
						log.Warningf("Secret %s not found in namespace %s.", secretListItem.SecretName, secretListItem.SecretNamespace)
						log.Warningln("ConfigMap will be updated with this secret when its available")
					} else {
						log.Warningln("Error extracting database credentials from secret ", secretListItem.SecretNamespace, "/", secretListItem.SecretName, ". ", err)
						return nil
					}
					continue
				}
				//List of users
				userListData = userListData + fmt.Sprintf(`"%s" "%s"
`, string(username), password)
			}
		}

		if pgbouncer.Spec.ConnectionPool != nil {
			admins = fmt.Sprintf(`%s`, pgbouncerAdminName)
			pbinfo = pbinfo + fmt.Sprintf(`listen_port = %d
`, *pgbouncer.Spec.ConnectionPool.ListenPort)
			pbinfo = pbinfo + fmt.Sprintf(`listen_addr = %s
`, pgbouncer.Spec.ConnectionPool.ListenAddress)
			pbinfo = pbinfo + fmt.Sprintf(`pool_mode = %s
`, pgbouncer.Spec.ConnectionPool.PoolMode)

			adminList := pgbouncer.Spec.ConnectionPool.AdminUsers
			for _, adminListItem := range adminList {
				admins = fmt.Sprintf(`%s,%s`, admins, adminListItem)
			}
			pbinfo = pbinfo + fmt.Sprintf(`admin_users = %s
`, admins)
		}

		pgbouncerData := fmt.Sprintf(`%s
%s`, dbinfo, pbinfo)
		//println(pgbouncerData)
		userListData = userListData + fmt.Sprintf(`"%s" "%s"
`, pgbouncerAdminName, "md59c7cb15d3dbd78fcbdfd1e46bcc6105e")
		//println(userListData)

		in.Data = map[string]string{
			"pgbouncer.ini": pgbouncerData,
			"userlist.txt":  userListData,
		}
		return in
	})
	if vt == kutil.VerbPatched {
		err = c.waitUntilPatchedConfigMapReady(pgbouncer, cfgMap)
		if err != nil {
			return vt, err
		}
		err = c.reloadPgBouncer(pgbouncer)
		if err != nil {
			//error is non blocking
			log.Infoln(err)
		} else {
			log.Infoln("PgBouncer reloaded successfully")
		}
		_, _, err = core_util.CreateOrPatchConfigMap(c.Client, configMapMeta, func(in *core.ConfigMap) *core.ConfigMap {
			in.ObjectMeta.Annotations = map[string]string{
				"podConfigMap":"patched",
			}
			return in
		})
		//revert to ready state so that it doesnt create a patched-ready loop
		_, _, err = core_util.CreateOrPatchConfigMap(c.Client, configMapMeta, func(in *core.ConfigMap) *core.ConfigMap {
			in.ObjectMeta.Annotations = map[string]string{
				"podConfigMap":"ready",
			}
			return in
		})
	}
	return vt, err
}

func (c *Controller) getDbCredentials(secretListItem api.SecretList) (string, string, error) {
	scrt, err := c.Client.CoreV1().Secrets(secretListItem.SecretNamespace).Get(secretListItem.SecretName, metav1.GetOptions{})
	if err != nil {
		return "", "", err
	}
	username := scrt.Data[PostgresUser]
	password := scrt.Data[PostgresPassword]
	md5key := md5.Sum([]byte(string(password) + string(username)))
	pbPassword := fmt.Sprintf("md5%s", hex.EncodeToString(md5key[:]))

	return string(username), pbPassword, nil
}

func (c *Controller) waitUntilPatchedConfigMapReady(pgbouncer *api.PgBouncer, newCfgMap *core.ConfigMap) error {
	if _, err := c.getPgBouncerPod(pgbouncer); err != nil {
		log.Warning("Pods not ready")
		return nil
	}
	return wait.PollImmediate(PbRetryInterval, kutil.ReadinessTimeout, func() (bool, error) {
		if pgbouncerConfig, _, err := c.echoPgBouncerConfig(pgbouncer); err == nil {
			println("::::::::::::>Comparing existing map with new map")
			if newCfgMap.Data["pgbouncer.ini"] == pgbouncerConfig {
				return true, nil
			}
		}
		return false, nil
	})
}

func (c *Controller) reloadPgBouncer(bouncer *api.PgBouncer) error {
	localPort := *bouncer.Spec.ConnectionPool.ListenPort

	pod, err := c.getPgBouncerPod(bouncer)
	if err != nil {
		return err
	}
	options := []func(options *exec.Options){
		exec.Command(c.reloadCmd(localPort)...),
	}

	if _, err := exec.ExecIntoPod(c.ClientConfig, &pod, options...); err != nil {
		return errors.Wrapf(err, "Failed to execute RELOAD command")
	}
	return nil
}

func (c *Controller) echoPgBouncerConfig(bouncer *api.PgBouncer) (string, string, error) {
	pod, err := c.getPgBouncerPod(bouncer)
	if err != nil {
		return "", "", err
	}
	options := []func(options *exec.Options){
		exec.Command(c.getPgBouncerConfigCmd()...),
	}

	pgbouncerconfig, err := exec.ExecIntoPod(c.ClientConfig, &pod, options...)
	if err != nil {
		return "", "", errors.Wrapf(err, "Failed to execute get config command")
	}
	options = []func(options *exec.Options){
		exec.Command(c.getUserListCmd()...),
	}

	userlist, err := exec.ExecIntoPod(c.ClientConfig, &pod, options...)
	if err != nil {
		return "", "", errors.Wrapf(err, "Failed to execute RELOAD command")
	}
	return pgbouncerconfig, userlist, nil
}

func (c *Controller) getPgBouncerPod(bouncer *api.PgBouncer) (core.Pod, error) {
	pbPodLabels := labels.FormatLabels(bouncer.OffshootSelectors())
	var pod core.Pod

	podlist, err := c.Client.CoreV1().Pods(bouncer.Namespace).List(metav1.ListOptions{LabelSelector: pbPodLabels})
	if err != nil {
		return pod, err
	}
	if len(podlist.Items) == 0 {
		return pod, errors.New("Pods not found")
	}
	pod = podlist.Items[0]
	return pod, nil
}

func (c *Controller) reloadCmd(localPort int32) []string {
	return []string{"env", "PGPASSWORD=kubedb123", "psql", "--host=127.0.0.1", fmt.Sprintf("--port=%d", localPort), "--username=pgbouncer", "pgbouncer", "--command=RELOAD"}
}

func (c *Controller) getPgBouncerConfigCmd() []string {
	return []string{"cat", "/etc/config/pgbouncer.ini"}
}
func (c *Controller) getUserListCmd() []string {
	return []string{"cat", "/etc/config/userlist.txt"}
}
