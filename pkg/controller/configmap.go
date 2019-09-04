package controller

import (
	"fmt"
	"path/filepath"
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
	PostgresPassword = "POSTGRES_PASSWORD"
	PostgresUser     = "POSTGRES_USER"
	PbRetryInterval  = time.Second * 5
	DefaultHostPort  = 5432
	ignoredParmeter  = "extra_float_digits"
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
		var dbinfo = fmt.Sprintln("[databases]")
		var pbinfo = fmt.Sprintln("[pgbouncer]")
		pbinfo = pbinfo + fmt.Sprintln("logfile = /tmp/pgbouncer.log")
		pbinfo = pbinfo + fmt.Sprintln("pidfile = /tmp/pgbouncer.pid")

		authFileLocation := filepath.Join(userListMountPath, c.getUserListFileName(pgbouncer))
		pbinfo = pbinfo + fmt.Sprintln("auth_file = ", authFileLocation)

		var admins string

		in.Labels = pgbouncer.OffshootLabels()
		core_util.EnsureOwnerReference(&in.ObjectMeta, ref)
		if pgbouncer.Spec.Databases != nil {
			for _, db := range pgbouncer.Spec.Databases {
				var hostPort = int32(DefaultHostPort)
				name := db.AppBindingName
				namespace := db.AppBindingNamespace

				appBinding, err := c.AppCatalogClient.AppcatalogV1alpha1().AppBindings(namespace).Get(name, metav1.GetOptions{})
				if err != nil {
					if kerr.IsNotFound(err) {
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
				var hostname string
				if appBinding.Spec.ClientConfig.URL == nil {
					if appBinding.Spec.ClientConfig.Service != nil {
						//urlString, err := appBinding.URL()
						//if err != nil {
						//	log.Errorln(err)
						//}
						//parsedURL, err := pq.ParseURL(urlString)
						//if err != nil {
						//	log.Errorln(err)
						//}
						//println(":::Parsed URL= ", parsedURL)
						//parsedURL = strings.ReplaceAll(parsedURL, " sslmode=disable","")
						//dbinfo = dbinfo +fmt.Sprintln(db.Alias +" = " + parsedURL +" dbname="+db.DbName )

						hostname = appBinding.Spec.ClientConfig.Service.Name + "." + namespace + ".svc"
						hostPort = appBinding.Spec.ClientConfig.Service.Port
						dbinfo = dbinfo +fmt.Sprintln(db.Alias +"= host=" + hostname +" port="+fmt.Sprintf("%d",hostPort)+" dbname="+db.DbName )
						//dbinfo = dbinfo +fmt.Sprintln(db.Alias +"x = host=" + hostname +" port="+strconv.Itoa(int(hostPort))+" dbname="+db.DbName )
					}
				} else {
					//Reminder URL should contain host=localhost port=5432
					dbinfo = dbinfo + fmt.Sprintln(db.Alias + " = " + *(appBinding.Spec.ClientConfig.URL)+ " dbname=" + db.DbName)
				}

			}
		}

		if pgbouncer.Spec.UserList.SecretName == "" {
			log.Infoln("PgBouncer userlist was not provided")
		}

		if pgbouncer.Spec.ConnectionPool != nil {
			pbinfo = pbinfo + fmt.Sprintln("listen_port =" + fmt.Sprintf("%d",*pgbouncer.Spec.ConnectionPool.ListenPort))
			pbConnectionPool := pgbouncer.Spec.ConnectionPool
			admins = fmt.Sprintf("%s", pbAdminUser)
			pbinfo = pbinfo + fmt.Sprintln("listen_addr = ", pbConnectionPool.ListenAddress)
			pbinfo = pbinfo + fmt.Sprintln("pool_mode = ", pbConnectionPool.PoolMode)
			//TODO: add max connection and pool size
			pbinfo = pbinfo + fmt.Sprintln("ignore_startup_parameters =", ignoredParmeter)
			if pbConnectionPool.MaxClientConn != nil {
				pbinfo = pbinfo + fmt.Sprintln("max_client_conn = ", pbConnectionPool.MaxClientConn)
			}
			if pbConnectionPool.MaxDbConnections != nil {
				pbinfo = pbinfo + fmt.Sprintln("max_db_connections = ", pbConnectionPool.MaxDbConnections)
			}
			if pbConnectionPool.MaxUserConnections != nil {
				pbinfo = pbinfo + fmt.Sprintln("max_user_connections = ", pbConnectionPool.MaxUserConnections)
			}
			if pbConnectionPool.MinPoolSize != nil {
				pbinfo = pbinfo + fmt.Sprintln("min_pool_size = ", pbConnectionPool.MinPoolSize)
			}
			if pbConnectionPool.DefaultPoolSize != nil {
				pbinfo = pbinfo + fmt.Sprintln("default_pool_size = ", pbConnectionPool.DefaultPoolSize)
			}
			if pbConnectionPool.ReservePoolSize != nil {
				pbinfo = pbinfo + fmt.Sprintln("reserve_pool_size = ", pbConnectionPool.ReservePoolSize)
			}
			if pbConnectionPool.ReservePoolTimeout != nil {
				pbinfo = pbinfo + fmt.Sprintln("reserve_pool_timeout = ", pbConnectionPool.ReservePoolTimeout)
			}

			adminList := pgbouncer.Spec.ConnectionPool.AdminUsers
			for _, adminListItem := range adminList {
				admins = fmt.Sprintf("%s,%s", admins, adminListItem)
			}
			pbinfo = pbinfo + fmt.Sprintln("admin_users = ", admins)
		}
		pgbouncerData := fmt.Sprintln(dbinfo)
		pgbouncerData = pgbouncerData + pbinfo

		in.Data = map[string]string{
			"pgbouncer.ini": pgbouncerData,
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
		err = c.AnnotateService(pgbouncer, "patched")

	} else {
		err = c.AnnotateService(pgbouncer, "ready")
	}
	return vt, err
}

//func (c *Controller) getDbCredentials(secretListItem api.SecretList) (string, string, error) {
//	scrt, err := c.Client.CoreV1().Secrets(secretListItem.SecretNamespace).Get(secretListItem.SecretName, metav1.GetOptions{})
//	if err != nil {
//		return "", "", err
//	}
//	username := scrt.Data[PostgresUser]
//	password := scrt.Data[PostgresPassword]
//	md5key := md5.Sum([]byte(string(password) + string(username)))
//	pbPassword := fmt.Sprintf("md5%s", hex.EncodeToString(md5key[:]))
//
//	return string(username), pbPassword, nil
//}

func (c *Controller) waitUntilPatchedConfigMapReady(pgbouncer *api.PgBouncer, newCfgMap *core.ConfigMap) error {
	if _, err := c.getPgBouncerPod(pgbouncer); err != nil {
		log.Warning("Pods not ready")
		return nil
	}
	println("Waiting for updated configurations to synchronize")
	return wait.PollImmediate(PbRetryInterval, kutil.ReadinessTimeout, func() (bool, error) {
		println("_")
		if pgbouncerConfig, err := c.echoPgBouncerConfig(pgbouncer); err == nil {
			if newCfgMap.Data["pgbouncer.ini"] == pgbouncerConfig {
				println("Done!")
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

func (c *Controller) echoPgBouncerConfig(bouncer *api.PgBouncer) (string, error) {
	pod, err := c.getPgBouncerPod(bouncer)
	if err != nil {
		return "", err
	}
	options := []func(options *exec.Options){
		exec.Command(c.getPgBouncerConfigCmd()...),
	}

	pgbouncerconfig, err := exec.ExecIntoPod(c.ClientConfig, &pod, options...)
	if err != nil {
		return "", errors.Wrapf(err, "Failed to execute get config command")
	}
	return pgbouncerconfig, nil
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

	return []string{"env", fmt.Sprintf("PGPASSWORD=%s", pbAdminPassword), "psql", "--host=127.0.0.1", fmt.Sprintf("--port=%d", localPort), fmt.Sprintf("--username=%s", pbAdminUser), "pgbouncer", "--command=RELOAD"}
}

func (c *Controller) getPgBouncerConfigCmd() []string {
	return []string{"cat", "/etc/config/pgbouncer.ini"}
}
func (c *Controller) getUserListCmd(bouncer *api.PgBouncer) ([]string, error) {
	//TODO: extract pgbouncer secrets's filename for userlist
	secretFileName, err := c.getSecretKey(bouncer)
	if err != nil {
		return nil, err
	}
	return []string{"cat", fmt.Sprintf("/var/run/pgbouncer/secrets/%s", secretFileName)}, nil
}

func (c *Controller) getUserListFileName(bouncer *api.PgBouncer) (filename string) {
	if bouncer.Spec.UserList.SecretName == "" {
		return ""
	}
	var ns string
	if bouncer.Spec.UserList.SecretNamespace != "" {
		ns = bouncer.Spec.UserList.SecretNamespace
	} else {
		ns = bouncer.Namespace

	}
	sec, err := c.Client.CoreV1().Secrets(ns).Get(bouncer.Spec.UserList.SecretName, metav1.GetOptions{})
	if err != nil {
		log.Infoln(err)
		return ""
	}
	secStData := sec.Data
	for key := range secStData {
		if key != "" {
			filename = key
			break
		}
	}
	return filename
}
