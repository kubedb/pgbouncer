package admission

import (
	"fmt"
	"strings"
	"sync"

	admission "k8s.io/api/admission/v1beta1"
	kerr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/mergepatch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	meta_util "kmodules.xyz/client-go/meta"
	hookapi "kmodules.xyz/webhook-runtime/admission/v1beta1"
	api "kubedb.dev/apimachinery/apis/kubedb/v1alpha1"
	cs "kubedb.dev/apimachinery/client/clientset/versioned"
)

type PgBouncerValidator struct {
	client      kubernetes.Interface
	extClient   cs.Interface
	lock        sync.RWMutex
	initialized bool
}

var _ hookapi.AdmissionHook = &PgBouncerValidator{}

var forbiddenEnvVars = []string{
	"POSTGRES_PASSWORD",
	"POSTGRES_USER",
}

func (a *PgBouncerValidator) Resource() (plural schema.GroupVersionResource, singular string) {
	return schema.GroupVersionResource{
			Group:    "validators.kubedb.com",
			Version:  "v1alpha1",
			Resource: "pgbouncervalidators",
		},
		"pgbouncervalidator"
}

func (a *PgBouncerValidator) Initialize(config *rest.Config, stopCh <-chan struct{}) error {
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

func (a *PgBouncerValidator) Admit(req *admission.AdmissionRequest) *admission.AdmissionResponse {
	status := &admission.AdmissionResponse{}
	println("Validator.go:=========>Admit starts")

	if (req.Operation != admission.Create && req.Operation != admission.Update && req.Operation != admission.Delete) ||
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

	switch req.Operation {
	case admission.Delete:
		println("Validator.go:=========>Case: Delete")
		if req.Name != "" {
			// req.Object.Raw = nil, so read from kubernetes
			obj, err := a.extClient.KubedbV1alpha1().PgBouncers(req.Namespace).Get(req.Name, metav1.GetOptions{})
			if err != nil && !kerr.IsNotFound(err) {
				return hookapi.StatusInternalServerError(err)
			} else if err == nil && obj.Spec.TerminationPolicy == api.TerminationPolicyDoNotTerminate {
				return hookapi.StatusBadRequest(fmt.Errorf(`pgbouncer "%v/%v" can't be paused. To delete, change spec.terminationPolicy`, req.Namespace, req.Name))
			}
		}
	default:
		println("Validator.go:=========>Case: Default")
		obj, err := meta_util.UnmarshalFromJSON(req.Object.Raw, api.SchemeGroupVersion)
		if err != nil {
			return hookapi.StatusBadRequest(err)
		}
		if req.Operation == admission.Update {
			// validate changes made by user
			oldObject, err := meta_util.UnmarshalFromJSON(req.OldObject.Raw, api.SchemeGroupVersion)
			if err != nil {
				return hookapi.StatusBadRequest(err)
			}

			pgbouncer := obj.(*api.PgBouncer).DeepCopy()
			oldPgBouncer := oldObject.(*api.PgBouncer).DeepCopy()
			oldPgBouncer.SetDefaults()
			// Allow changing Database Secret only if there was no secret have set up yet.

			if err := validateUpdate(pgbouncer, oldPgBouncer, req.Kind.Kind); err != nil {
				return hookapi.StatusBadRequest(fmt.Errorf("%v", err))
			}
		}
		// validate database specs
		if err = ValidatePgBouncer(a.client, a.extClient, obj.(*api.PgBouncer), false); err != nil {
			return hookapi.StatusForbidden(err)
		}
	}
	status.Allowed = true
	return status
}

// ValidatePgBouncer checks if the object satisfies all the requirements.
// It is not method of Interface, because it is referenced from controller package too.
func ValidatePgBouncer(client kubernetes.Interface, extClient cs.Interface, pgbouncer *api.PgBouncer, strictValidation bool) error {
	if pgbouncer.Spec.Replicas == nil || *pgbouncer.Spec.Replicas < 1 {
		return fmt.Errorf(`spec.replicas "%v" invalid. Value must be greater than zero`, pgbouncer.Spec.Replicas)
	}
	if pgbouncer.Spec.UpdateStrategy.Type == "" {
		return fmt.Errorf(`'spec.updateStrategy.type' is missing`)
	}

	if pgbouncer.Spec.TerminationPolicy == "" {
		return fmt.Errorf(`'spec.terminationPolicy' is missing`)
	}

	//if err := matchWithDormantDatabase(extClient, pgbouncer); err != nil {
	//	return err
	//}
	return nil
}

//func matchWithDormantDatabase(extClient cs.Interface, pgbouncer *api.PgBouncer) error {
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
//	drmnOriginSpec := dormantDb.Spec.Origin.Spec.PgBouncer
//	drmnOriginSpec.SetDefaults()
//	originalSpec := pgbouncer.Spec
//
//	// Skip checking UpdateStrategy
//	drmnOriginSpec.UpdateStrategy = originalSpec.UpdateStrategy
//
//	// Skip checking ServiceAccountName
//	drmnOriginSpec.PodTemplate.Spec.ServiceAccountName = originalSpec.PodTemplate.Spec.ServiceAccountName
//
//	// Skip checking TerminationPolicy
//	drmnOriginSpec.TerminationPolicy = originalSpec.TerminationPolicy
//
//	// Skip checking Monitoring
//	drmnOriginSpec.Monitor = originalSpec.Monitor
//
//	// Skip Checking Backup Scheduler
//	drmnOriginSpec.BackupSchedule = originalSpec.BackupSchedule
//
//	// Skip Checking LeaderElectionConfigs
//	drmnOriginSpec.LeaderElection = originalSpec.LeaderElection
//
//	if !meta_util.Equal(drmnOriginSpec, &originalSpec) {
//		diff := meta_util.Diff(drmnOriginSpec, &originalSpec)
//		log.Errorf("pgbouncer spec mismatches with OriginSpec in DormantDatabases. Diff: %v", diff)
//		return errors.New(fmt.Sprintf("pgbouncer spec mismatches with OriginSpec in DormantDatabases. Diff: %v", diff))
//	}
//
//	return nil
//}

func validateUpdate(obj, oldObj runtime.Object, kind string) error {
	preconditions := getPreconditionFunc()
	_, err := meta_util.CreateStrategicPatch(oldObj, obj, preconditions...)
	if err != nil {
		if mergepatch.IsPreconditionFailed(err) {
			return fmt.Errorf("%v.%v", err, preconditionFailedError(kind))
		}
		return err
	}
	return nil
}

func getPreconditionFunc() []mergepatch.PreconditionFunc {
	preconditions := []mergepatch.PreconditionFunc{
		mergepatch.RequireKeyUnchanged("apiVersion"),
		mergepatch.RequireKeyUnchanged("kind"),
		mergepatch.RequireMetadataKeyUnchanged("name"),
		mergepatch.RequireMetadataKeyUnchanged("namespace"),
	}

	for _, field := range preconditionSpecFields {
		preconditions = append(preconditions,
			meta_util.RequireChainKeyUnchanged(field),
		)
	}
	return preconditions
}

var preconditionSpecFields = []string{
	"spec.standby",
	"spec.streaming",
	"spec.archiver",
	"spec.databaseSecret",
	"spec.storageType",
	"spec.storage",
	"spec.init",
	"spec.podTemplate.spec.nodeSelector",
}

func preconditionFailedError(kind string) error {
	str := preconditionSpecFields
	strList := strings.Join(str, "\n\t")
	return fmt.Errorf(strings.Join([]string{`At least one of the following was changed:
	apiVersion
	kind
	name
	namespace`, strList}, "\n\t"))
}
