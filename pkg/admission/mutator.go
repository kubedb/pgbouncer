package admission

import (
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sync"

	"github.com/appscode/go/log"

	"github.com/appscode/go/types"
	admission "k8s.io/api/admission/v1beta1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	meta_util "kmodules.xyz/client-go/meta"
	mona "kmodules.xyz/monitoring-agent-api/api/v1"
	hookapi "kmodules.xyz/webhook-runtime/admission/v1beta1"
	api "kubedb.dev/apimachinery/apis/kubedb/v1alpha1"
	cs "kubedb.dev/apimachinery/client/clientset/versioned"
)

const (
	defaultListenPort     = int32(5432)
	defaultListenAddress  = "*"
	defaultPoolMode       = "session"
	defaultPgBouncerImage = "rezoan/pb:latest"
)

type PgBouncerMutator struct {
	client      kubernetes.Interface
	extClient   cs.Interface
	lock        sync.RWMutex
	initialized bool
}

var _ hookapi.AdmissionHook = &PgBouncerMutator{}

func (a *PgBouncerMutator) Resource() (plural schema.GroupVersionResource, singular string) {
	return schema.GroupVersionResource{
			Group:    "mutators.kubedb.com",
			Version:  "v1alpha1",
			Resource: "pgbouncermutators",
		},
		"pgbouncermutator"
}

func (a *PgBouncerMutator) Initialize(config *rest.Config, stopCh <-chan struct{}) error {
	log.Infoln("Mutator.go >>>  Initialize ====")
	a.lock.Lock()
	defer a.lock.Unlock()

	a.initialized = true

	var err error
	if a.client, err = kubernetes.NewForConfig(config); err != nil {
		return err
	}
	if a.extClient, err = cs.NewForConfig(config); err != nil {
		return err
	}
	return err
}

func (a *PgBouncerMutator) Admit(req *admission.AdmissionRequest) *admission.AdmissionResponse {
	log.Info("Mutator.go >>>  Admit ====")
	status := &admission.AdmissionResponse{}

	// N.B.: No Mutating for delete
	if (req.Operation != admission.Create && req.Operation != admission.Update) ||
		len(req.SubResource) != 0 ||
		req.Kind.Group != api.SchemeGroupVersion.Group ||
		req.Kind.Kind != api.ResourceKindPgBouncer {
		status.Allowed = true
		return status
	}

	a.lock.RLock()
	defer a.lock.RUnlock()
	if !a.initialized {
		return hookapi.StatusUninitialized()
	}
	obj, err := meta_util.UnmarshalFromJSON(req.Object.Raw, api.SchemeGroupVersion)
	if err != nil {
		return hookapi.StatusBadRequest(err)
	}
	if err := a.CheckSecretAvailable(obj.(*api.PgBouncer).DeepCopy());err != nil {
		return hookapi.StatusForbidden(err)
	}

	dbMod, err := setDefaultValues(a.client, a.extClient, obj.(*api.PgBouncer).DeepCopy())
	if err != nil {
		return hookapi.StatusForbidden(err)
	} else if dbMod != nil {
		patch, err := meta_util.CreateJSONPatch(req.Object.Raw, dbMod)
		if err != nil {
			return hookapi.StatusInternalServerError(err)
		}
		status.Patch = patch
		patchType := admission.PatchTypeJSONPatch
		status.PatchType = &patchType
	}

	status.Allowed = true
	return status
}

// setDefaultValues provides the defaulting that is performed in mutating stage of creating/updating a PgBouncer database
func setDefaultValues(client kubernetes.Interface, extClient cs.Interface, pgbouncer *api.PgBouncer) (runtime.Object, error) {
	log.Infoln("Mutator.go >>>  setDefaultValues ====")

	if pgbouncer.Spec.Replicas == nil {
		pgbouncer.Spec.Replicas = types.Int32P(1)
	}

	//TODO: Make sure image an image path is set

	if pgbouncer.Spec.ConnectionPool != nil {
		if pgbouncer.Spec.ConnectionPool.ListenPort == nil {
			pgbouncer.Spec.ConnectionPool.ListenPort = types.Int32P(defaultListenPort)
		}
		if pgbouncer.Spec.ConnectionPool.ListenAddress == "" {
			pgbouncer.Spec.ConnectionPool.ListenAddress = defaultListenAddress
		}
		if pgbouncer.Spec.ConnectionPool.PoolMode == "" {
			pgbouncer.Spec.ConnectionPool.PoolMode = defaultPoolMode
		}
	}
	//Set default namespace for unspecified namespaces
	if pgbouncer.Spec.Databases != nil {
		for i, db := range pgbouncer.Spec.Databases {
			if db.AppBindingNamespace == "" {
				pgbouncer.Spec.Databases[i].AppBindingNamespace = pgbouncer.Namespace
			}
		}
	}
	if pgbouncer.Spec.UserList.SecretNamespace == "" {
		pgbouncer.Spec.UserList.SecretNamespace = pgbouncer.Namespace
	}
	//TODO: refuse pgbouncer without secret
	pgbouncer.SetDefaults()


	// If monitoring spec is given without port,
	// set default Listening port
	setMonitoringPort(pgbouncer)

	return pgbouncer, nil
}

// Assign Default Monitoring Port if MonitoringSpec Exists
// and the AgentVendor is Prometheus.
func setMonitoringPort(pgbouncer *api.PgBouncer) {
	if pgbouncer.Spec.Monitor != nil &&
		pgbouncer.GetMonitoringVendor() == mona.VendorPrometheus {
		if pgbouncer.Spec.Monitor.Prometheus == nil {
			pgbouncer.Spec.Monitor.Prometheus = &mona.PrometheusSpec{}
		}
		if pgbouncer.Spec.Monitor.Prometheus.Port == 0 {
			pgbouncer.Spec.Monitor.Prometheus.Port = api.PrometheusExporterPortNumber
		}
	}
}

func (a *PgBouncerMutator) CheckSecretAvailable(bouncer *api.PgBouncer) error {
	_, err := a.client.CoreV1().Secrets(bouncer.Spec.UserList.SecretNamespace).Get(bouncer.Spec.UserList.SecretName, v1.GetOptions{})
	return err
}