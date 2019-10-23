package controller

import (
	"bytes"
	"fmt"
	"path/filepath"
	"text/template"
	"time"

	api "kubedb.dev/apimachinery/apis/kubedb/v1alpha1"

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
)

const (
	PostgresPassword = "POSTGRES_PASSWORD"
	PostgresUser     = "POSTGRES_USER"
	PbRetryInterval  = time.Second * 5
	DefaultHostPort  = 5432
	ignoredParmeter  = "extra_float_digits"
	pbConfigFile     = "pgbouncer.ini"
)

var (
	cfgtpl = template.Must(template.New("cfg").Parse(`listen_port = {{ .Port }}
listen_addr = *
pool_mode = {{ .PoolMode }}
ignore_startup_parameters = extra_float_digits{{ if .IgnoreStartupParameters }}, {{.IgnoreStartupParameters}}{{ end }}
{{- if .MaxClientConnections  }}
max_client_conn = {{ .MaxClientConnections }}
{{- end }}
{{- if .MaxDBConnections  }}
max_db_connections = {{ .MaxDBConnections }}
{{- end }}
{{- if .MaxUserConnections  }}
max_user_connections = {{ .MaxUserConnections }}
{{- end }}
{{- if .MinPoolSize  }}
min_pool_size = {{ .MinPoolSize }}
{{- end }}
{{- if .DefaultPoolSize  }}
default_pool_size = {{ .DefaultPoolSize }}
{{- end }}
{{- if .ReservePoolSize  }}
reserve_pool_size = {{ .ReservePoolSize }}
{{- end }}
{{- if .ReservePoolTimeoutSeconds  }}
reserve_pool_timeout = {{ .ReservePoolTimeoutSeconds }}
{{- end }}
{{- if .StatsPeriodSeconds  }}
stats_period = {{ .StatsPeriodSeconds }}
{{- end }}
{{- if .AuthType }}
auth_type = {{ .AuthType }}
{{- end }}
{{- if .AuthUser }}
auth_user = {{ .AuthUser }}
{{- end }}
admin_users = kubedb{{range .AdminUsers }},{{.}}{{end}}
`))
)

func (c *Controller) generateConfig(pgbouncer *api.PgBouncer) (string, error) {
	var buf bytes.Buffer
	buf.WriteString("[databases]\n")
	if pgbouncer.Spec.Databases != nil {
		for _, db := range pgbouncer.Spec.Databases {
			var hostPort = int32(DefaultHostPort)
			name := db.DatabaseRef.Name
			namespace := pgbouncer.GetNamespace()

			appBinding, err := c.AppCatalogClient.AppcatalogV1alpha1().AppBindings(namespace).Get(name, metav1.GetOptions{})
			if err != nil {
				if kerr.IsNotFound(err) {
					log.Warning(err)
				} else {
					log.Error(err)
				}
				continue //Dont add pgbouncer databse base for this non existent appbinding
			}
			//if appBinding.Spec.ClientConfig.Service != nil {
			//	name = appBinding.Spec.ClientConfig.Service.Name
			//	namespace = appBinding.Namespace
			//	hostPort = appBinding.Spec.ClientConfig.Service.Port
			//}
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
					buf.WriteString(fmt.Sprint(db.Alias, "= host=", hostname, " port=", hostPort, " dbname=", db.DatabaseName))
					//dbinfo = dbinfo +fmt.Sprintln(db.Alias +"x = host=" + hostname +" port="+strconv.Itoa(int(hostPort))+" dbname="+db.DbName )
				}
			} else {
				//Reminder URL should contain host=localhost port=5432
				buf.WriteString(fmt.Sprint(db.Alias + " = " + *(appBinding.Spec.ClientConfig.URL) + " dbname=" + db.DatabaseName))
			}
			if db.UserName != "" {
				buf.WriteString(fmt.Sprint(" user=", db.UserName))
			}
			if db.Password != "" {
				buf.WriteString(fmt.Sprint(" password=", db.Password))
			}
			buf.WriteRune('\n')
		}
	}

	if pgbouncer.Spec.UserListSecretRef == nil {
		log.Infoln("PgBouncer doesn't have a userlist")
	}

	buf.WriteString("\n[pgbouncer]\n")
	buf.WriteString("logfile = /tmp/pgbouncer.log\n") // TODO: send log to stdout ?
	buf.WriteString("pidfile = /tmp/pgbouncer.pid\n")

	secretFileName, err := c.getUserListFileName(pgbouncer)
	if err != nil {
		return "", err
	}
	if secretFileName == "" {
		secretFileName = pbAdminData
	}
	println("AuthFileName = ", secretFileName)

	if pgbouncer.Spec.ConnectionPool == nil || (pgbouncer.Spec.ConnectionPool != nil && pgbouncer.Spec.ConnectionPool.AuthType != "any") {
		buf.WriteString(fmt.Sprintln("auth_file = ", filepath.Join(userListMountPath, secretFileName)))
	}
	if pgbouncer.Spec.ConnectionPool != nil {
		err := cfgtpl.Execute(&buf, pgbouncer.Spec.ConnectionPool)
		if err != nil {
			return "", err
		}
	}
	return buf.String(), nil
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

	cfg, err := c.generateConfig(pgbouncer)
	if err != nil {
		return kutil.VerbUnchanged, rerr
	}

	_, vt, err := core_util.CreateOrPatchConfigMap(c.Client, configMapMeta, func(in *core.ConfigMap) *core.ConfigMap {
		in.Labels = pgbouncer.OffshootLabels()
		core_util.EnsureOwnerReference(&in.ObjectMeta, ref)
		in.Data = map[string]string{
			pbConfigFile: cfg,
		}
		return in
	})
	if vt == kutil.VerbPatched {
		println("Configmap patched")
		//err = c.waitUntilPatchedConfigMapReady(pgbouncer, cfgMap)
		//if err != nil {
		//	return vt, err
		//}
		//err = c.reloadPgBouncer(pgbouncer)
		//if err != nil {
		//	//error is non blocking
		//	log.Infoln(err)
		//} else {
		//	log.Infoln("PgBouncer reloaded successfully")
		//}
		//err = c.AnnotateService(pgbouncer, "patched")

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
	log.Infoln("Waiting for updated configurations to synchronize")
	return wait.PollImmediate(PbRetryInterval, kutil.ReadinessTimeout, func() (bool, error) {
		if pgbouncerConfig, err := c.echoPgBouncerConfig(pgbouncer); err == nil {
			if newCfgMap.Data[pbConfigFile] == pgbouncerConfig {
				log.Infoln("configMap synchronized")
				return true, nil
			}
		}
		return false, nil
	})
}

func (c *Controller) reloadPgBouncer(pgbouncer *api.PgBouncer) error {
	localPort := *pgbouncer.Spec.ConnectionPool.Port

	pod, err := c.getPgBouncerPod(pgbouncer)
	if err != nil {
		return err
	}
	options := []func(options *exec.Options){
		exec.Command(c.reloadCmd(pgbouncer, localPort)...),
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

func (c *Controller) reloadCmd(pgbouncer *api.PgBouncer, localPort int32) []string {
	adminSecretSpec := c.GetDefaultSecretSpec(pgbouncer)
	adminSecret, err := c.Client.CoreV1().Secrets(adminSecretSpec.Namespace).Get(adminSecretSpec.Name, metav1.GetOptions{})
	if err != nil {
		log.Infoln(err)
	}
	adminPassword := string(adminSecret.Data[pbAdminPassword])
	return []string{"env", fmt.Sprintf("PGPASSWORD=%s", adminPassword), "psql", "--host=127.0.0.1", fmt.Sprintf("--port=%d", localPort), fmt.Sprintf("--username=%s", pbAdminUser), "pgbouncer", "--command=RELOAD"}
}

func (c *Controller) getPgBouncerConfigCmd() []string {
	return []string{"cat", fmt.Sprintf("%s/pgbouncer.ini", configMountPath)}
}
func (c *Controller) getUserListCmd(bouncer *api.PgBouncer) ([]string, error) {
	secretFileName, err := c.getSecretKey(bouncer)
	if err != nil {
		return nil, err
	}
	return []string{"cat", fmt.Sprintf("%s/%s", userListMountPath, secretFileName)}, nil
}

func (c *Controller) getUserListFileName(pgbouncer *api.PgBouncer) (string, error) {
	defaultSecretSpec := c.GetDefaultSecretSpec(pgbouncer)
	defaultSecret, err := c.Client.CoreV1().Secrets(pgbouncer.Namespace).Get(defaultSecretSpec.Name, metav1.GetOptions{})
	if err != nil {
		return "", err
	}
	if _, exists := defaultSecret.Data[pbUserData]; exists {
		return pbUserData, nil
	}
	return pbAdminData, nil
}
