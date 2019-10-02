package admission

import (
	"fmt"
	"strings"
	"sync"

	"github.com/appscode/go/log"
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

func (pbValidator *PgBouncerValidator) Admit(req *admission.AdmissionRequest) *admission.AdmissionResponse {
	status := &admission.AdmissionResponse{}
	log.Info("Validator.go >>>  Admit ====")

	if (req.Operation != admission.Create && req.Operation != admission.Update && req.Operation != admission.Delete) ||
		len(req.SubResource) != 0 ||
		req.Kind.Group != api.SchemeGroupVersion.Group ||
		req.Kind.Kind != api.ResourceKindPgBouncer {
		status.Allowed = true
		return status
	}

	pbValidator.lock.RLock()
	defer pbValidator.lock.RUnlock()
	if !pbValidator.initialized {
		return hookapi.StatusUninitialized()
	}

	switch req.Operation {
	case admission.Delete:
		if req.Name != "" {
			// req.Object.Raw = nil, so read from kubernetes
			obj, err := pbValidator.extClient.KubedbV1alpha1().PgBouncers(req.Namespace).Get(req.Name, metav1.GetOptions{})
			if kerr.IsNotFound(err) {
				log.Infoln("obj ", obj.Name, " already deleted")
			}
			if err != nil && !kerr.IsNotFound(err) {
				return hookapi.StatusInternalServerError(err)
			}
		}
	default:
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

			if err := validateUpdate(pgbouncer, oldPgBouncer, req.Kind.Kind); err != nil {
				return hookapi.StatusBadRequest(fmt.Errorf("%v", err))
			}
		}
		// validate database specs
		if err = ValidatePgBouncer(pbValidator.client, pbValidator.extClient, obj.(*api.PgBouncer), true); err != nil {
			return hookapi.StatusForbidden(err)
		}
	}
	status.Allowed = true
	return status
}

// ValidatePgBouncer checks if the object satisfies all the requirements.
// It is not method of Interface, because it is referenced from controller package too.
func ValidatePgBouncer(client kubernetes.Interface, extClient cs.Interface, pgbouncer *api.PgBouncer, strictValidation bool) error {
	log.Info("Validator.go >>>  ValidatePgBouncer ====")
	if pgbouncer.Spec.Replicas == nil || *pgbouncer.Spec.Replicas < 1 {
		return fmt.Errorf(`spec.replicas "%v" invalid. Value must be greater than zero`, pgbouncer.Spec.Replicas)
	}
	if string(pgbouncer.Spec.Version) == "" {
		return fmt.Errorf(`spec.Version can't be empty`)
	}

	if strictValidation {
		// Check if usrlist is absent.
		if pgbouncer.Spec.UserListSecretRef != nil && pgbouncer.Spec.UserListSecretRef.Name != "" {
			if pgbouncer.Spec.ConnectionPool != nil && pgbouncer.Spec.ConnectionPool.AuthType != "any" {
				if _, err := client.CoreV1().Secrets(pgbouncer.GetNamespace()).Get(pgbouncer.Spec.UserListSecretRef.Name, metav1.GetOptions{}); err != nil {
					return err
				}
			}
		}

		// Check if pgbouncerVersion is absent or deprecated.
		// If deprecated, return error
		pgbouncerVersion, err := extClient.CatalogV1alpha1().PgBouncerVersions().Get(string(pgbouncer.Spec.Version), metav1.GetOptions{})
		if err != nil {
			return err
		}
		if pgbouncerVersion.Spec.Deprecated {
			return fmt.Errorf("pgbouncer %s/%s is using deprecated version %v. Skipped processing",
				pgbouncer.Namespace, pgbouncer.Name, pgbouncerVersion.Name)
		}
	}
	return nil
}

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
