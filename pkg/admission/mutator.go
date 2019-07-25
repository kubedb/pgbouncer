package admission

import (
	"github.com/appscode/go/log"
	"sync"

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
	defaultListenPort = int32(5432)
	defaultListenAddress = "*"
	defaultPoolMode = "session"

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
	log.Infoln("Mutator.go :::::::::::::::::::  Initialize ====")
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
	log.Infoln("Mutator.go :::::::::::::::::::  Admit ====")
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
	log.Infoln("Mutator.go :::::::::::::::::::  setDefaultValues ====")

	if pgbouncer.Spec.Replicas == nil {
		pgbouncer.Spec.Replicas = types.Int32P(1)
	}
	if pgbouncer.Spec.ConnectionPoolConfig != nil{
		if pgbouncer.Spec.ConnectionPoolConfig.ListenPort == nil{
			pgbouncer.Spec.ConnectionPoolConfig.ListenPort = types.Int32P(defaultListenPort)
		}
		if pgbouncer.Spec.ConnectionPoolConfig.ListenAddress == ""{
			pgbouncer.Spec.ConnectionPoolConfig.ListenAddress = defaultListenAddress
		}
		if pgbouncer.Spec.ConnectionPoolConfig.PoolMode == ""{
			pgbouncer.Spec.ConnectionPoolConfig.PoolMode = defaultPoolMode
		}
	}
	pgbouncer.SetDefaults()

	//if err := setDefaultsFromDormantDB(extClient, pgbouncer); err != nil {
	//	return nil, err
	//}

	// If monitoring spec is given without port,
	// set default Listening port

	//setMonitoringPort(pgbouncer)

	return pgbouncer, nil
}

// setDefaultsFromDormantDB takes values from Similar Dormant Database
//func setDefaultsFromDormantDB(extClient cs.Interface, pgbouncer *api.PgBouncer) error {
//	// Check if DormantDatabase exists or not
//	dormantDb, err := extClient.KubedbV1alpha1().DormantDatabases(pgbouncer.Namespace).Get(pgbouncer.Name, metav1.GetOptions{})
//	if err != nil {
//		if !kerr.IsNotFound(err) {
//			return err
//		}
//		return nil
//	}
//
//	// Check DatabaseKind
//	if value, _ := meta_util.GetStringValue(dormantDb.Labels, api.LabelDatabaseKind); value != api.ResourceKindPgBouncer {
//		return errors.New(fmt.Sprintf(`invalid PgBouncer: "%v/%v". Exists DormantDatabase "%v/%v" of different Kind`, pgbouncer.Namespace, pgbouncer.Name, dormantDb.Namespace, dormantDb.Name))
//	}
//
//	// Check Origin Spec
//
//	// If DatabaseSecret of new object is not given,
//	// Take dormantDatabaseSecretName
//	if pgbouncer.Spec.DatabaseSecret == nil {
//		pgbouncer.Spec.DatabaseSecret = ddbOriginSpec.DatabaseSecret
//	}
//
//	// If Monitoring Spec of new object is not given,
//	// Take Monitoring Settings from Dormant
//	if pgbouncer.Spec.Monitor == nil {
//		pgbouncer.Spec.Monitor = ddbOriginSpec.Monitor
//	} else {
//		ddbOriginSpec.Monitor = pgbouncer.Spec.Monitor
//	}
//
//	// If Backup Scheduler of new object is not given,
//	// Take Backup Scheduler Settings from Dormant
//	if pgbouncer.Spec.BackupSchedule == nil {
//		pgbouncer.Spec.BackupSchedule = ddbOriginSpec.BackupSchedule
//	} else {
//		ddbOriginSpec.BackupSchedule = pgbouncer.Spec.BackupSchedule
//	}
//
//	// If LeaderElectionConfig of new object is not given,
//	// Take configs from Dormant
//	if pgbouncer.Spec.LeaderElection == nil {
//		pgbouncer.Spec.LeaderElection = ddbOriginSpec.LeaderElection
//	} else {
//		ddbOriginSpec.LeaderElection = pgbouncer.Spec.LeaderElection
//	}
//
//	// Skip checking UpdateStrategy
//	ddbOriginSpec.UpdateStrategy = pgbouncer.Spec.UpdateStrategy
//
//	// Skip checking ServiceAccountName
//	ddbOriginSpec.PodTemplate.Spec.ServiceAccountName = pgbouncer.Spec.PodTemplate.Spec.ServiceAccountName
//
//	// Skip checking TerminationPolicy
//	ddbOriginSpec.TerminationPolicy = pgbouncer.Spec.TerminationPolicy
//
//	if !meta_util.Equal(ddbOriginSpec, &pgbouncer.Spec) {
//		diff := meta_util.Diff(ddbOriginSpec, &pgbouncer.Spec)
//		log.Errorf("pgbouncer spec mismatches with OriginSpec in DormantDatabases. Diff: %v", diff)
//		return errors.New(fmt.Sprintf("pgbouncer spec mismatches with OriginSpec in DormantDatabases. Diff: %v", diff))
//	}
//
//	if _, err := meta_util.GetString(pgbouncer.Annotations, api.AnnotationInitialized); err == kutil.ErrNotFound &&
//		pgbouncer.Spec.Init != nil &&
//		(pgbouncer.Spec.Init.SnapshotSource != nil || pgbouncer.Spec.Init.StashRestoreSession != nil) {
//		pgbouncer.Annotations = core_util.UpsertMap(pgbouncer.Annotations, map[string]string{
//			api.AnnotationInitialized: "",
//		})
//	}
//
//	// Delete  Matching dormantDatabase in Controller
//
//	return nil
//}

// Assign Default Monitoring Port if MonitoringSpec Exists
// and the AgentVendor is Prometheus.
func setMonitoringPort(pgbouncer *api.PgBouncer) {
	log.Infoln("Mutator.go :::::::::::::::::::  setMonitoringPort ====")
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
