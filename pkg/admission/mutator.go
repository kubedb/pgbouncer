package admission

import (
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
	defaultListenPort    = int32(5432)
	DefaultListenAddress = "*"
	defaultPoolMode      = "session"
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
		if pgbouncer.Spec.ConnectionPool.Port == nil {
			pgbouncer.Spec.ConnectionPool.Port = types.Int32P(defaultListenPort)
		}
		if pgbouncer.Spec.ConnectionPool.PoolMode == "" {
			pgbouncer.Spec.ConnectionPool.PoolMode = defaultPoolMode
		}
	}
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
