package controller

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"

	"github.com/appscode/go/log"
	core "k8s.io/api/core/v1"
	kerr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	clientsetscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/reference"
	kutil "kmodules.xyz/client-go"
	core_util "kmodules.xyz/client-go/core/v1"
	api "kubedb.dev/apimachinery/apis/kubedb/v1alpha1"
	le "kubedb.dev/pgbouncer/pkg/leader_election"
)

const (
	POSTGRES_PASSWORD  = "POSTGRES_PASSWORD"
	POSTGRES_USER      = "POSTGRES_USER"
	pgbouncerAdminName = "pgbouncer"
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

	_, vt, err := core_util.CreateOrPatchConfigMap(c.Client, configMapMeta, func(in *core.ConfigMap) *core.ConfigMap {
		var dbinfo = `[databases]
`
		var pbinfo = `[pgbouncer]
auth_file = /etc/config/userlist.txt
logfile = /tmp/pgbouncer.log
pidfile = /tmp/pgbouncer.pid
`
		var admins string
		var userListData string
		var listenAddress = "*"
		var pool_mode = "session"

		in.Labels = pgbouncer.OffshootLabels()
		core_util.EnsureOwnerReference(&in.ObjectMeta, ref)
		if pgbouncer.Spec.Databases != nil {
			for _, db := range pgbouncer.Spec.Databases {
				var namespace = pgbouncer.Namespace
				var hostPort = int32(5432)
				if db.PgObjectNamespace != "" {
					namespace = db.PgObjectNamespace
				}
				serv, err := c.Client.CoreV1().Services(namespace).Get(db.PgObjectName, metav1.GetOptions{})
				if err != nil {
					log.Fatal(err)
				}
				hostname := serv.Name + "." + serv.Namespace + ".svc.cluster.local"

				for _, port := range serv.Spec.Ports {
					if port.Port != 0 {
						hostPort = port.Port
						break
					}
				}
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
						println("This a TODO for not found errors")
					} else {
						log.Error(err)
						return nil
					}
				}
				//List of users
				userListData = userListData + fmt.Sprintf(`"%s" "%s"
`, string(username), password)
			}
		}

		if pgbouncer.Spec.ConnectionPoolConfig != nil {
			admins = fmt.Sprintf(`%s`, pgbouncerAdminName)
			listenPort := *pgbouncer.Spec.ConnectionPoolConfig.ListenPort
			pbinfo = pbinfo + fmt.Sprintf(`listen_port = %d
`, listenPort)
			if pgbouncer.Spec.ConnectionPoolConfig.ListenAddress != "" {
				listenAddress = pgbouncer.Spec.ConnectionPoolConfig.ListenAddress
			}
			pbinfo = pbinfo + fmt.Sprintf(`listen_addr = %s
`, listenAddress)
			if pgbouncer.Spec.ConnectionPoolConfig.PoolMode != "" {
				pool_mode = pgbouncer.Spec.ConnectionPoolConfig.PoolMode
			}
			pbinfo = pbinfo + fmt.Sprintf(`pool_mode = %s
`, pool_mode)

			adminList := pgbouncer.Spec.ConnectionPoolConfig.AdminUsers
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
	err = c.WaitUntilConfigMapReady(c.Client, configMapMeta)
	if err != nil {
		return vt, err
	}
	return vt, err
}

func (c *Controller) getDbCredentials(secretListItem api.SecretList) (string, string, error) {
	scrt, err := c.Client.CoreV1().Secrets(secretListItem.SecretNamespace).Get(secretListItem.SecretName, metav1.GetOptions{})
	if err != nil {
		println("================>Secret not found.", err)
		return "", "", err
	}
	username := scrt.Data[POSTGRES_USER]
	password := scrt.Data[POSTGRES_PASSWORD]
	md5key := md5.Sum([]byte(string(password) + string(username)))
	pbPassword := fmt.Sprintf("md5%s", hex.EncodeToString(md5key[:]))

	return string(username), pbPassword, nil
}
func (c *Controller) WaitUntilConfigMapReady(kubeClient kubernetes.Interface, meta metav1.ObjectMeta) error {
	return wait.PollImmediate(kutil.RetryInterval, kutil.ReadinessTimeout, func() (bool, error) {
		if _, err := kubeClient.CoreV1().ConfigMaps(meta.Namespace).Get(meta.Name, metav1.GetOptions{}); err == nil {
			return true, nil
		}
		return false, nil
	})
}
