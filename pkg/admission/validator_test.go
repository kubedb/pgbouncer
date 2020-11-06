/*
Copyright AppsCode Inc. and Contributors

Licensed under the AppsCode Free Trial License 1.0.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    https://github.com/appscode/licenses/raw/1.0.0/AppsCode-Free-Trial-1.0.0.md

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package admission

//
//import (
//	"net/http"
//	"testing"
//
//	"gomodules.xyz/pointer"
//	api "kubedb.dev/apimachinery/apis/kubedb/v1alpha2"
//	"kubedb.dev/apimachinery/client/clientset/versioned/scheme"
//	admission "k8s.io/api/admission/v1beta1"
//	apps "k8s.io/api/apps/v1"
//	authenticationV1 "k8s.io/api/authentication/v1"
//	core "k8s.io/api/core/v1"
//	storageV1beta1 "k8s.io/api/storage/v1beta1"
//	metaV1 "k8s.io/apimachinery/pkg/apis/meta/v1"
//	"k8s.io/apimachinery/pkg/runtime"
//	"k8s.io/client-go/kubernetes/fake"
//	clientSetScheme "k8s.io/client-go/kubernetes/scheme"
//	"kmodules.xyz/client-go/meta"
//	mona "kmodules.xyz/monitoring-agent-api/api/v1"
//)
//
//func init() {
//	scheme.AddToScheme(clientSetScheme.Scheme)
//}
//
//var requestKind = metaV1.GroupVersionKind{
//	Group:   api.SchemeGroupVersion.Group,
//	Version: api.SchemeGroupVersion.Version,
//	Kind:    api.ResourceKindPgBouncer,
//}
//
//func TestPgBouncerValidator_Admit(t *testing.T) {
//	for _, c := range cases {
//		t.Run(c.testName, func(t *testing.T) {
//			validator := PgBouncerValidator{}
//
//			validator.initialized = true
//
//			validator.client = fake.NewSimpleClientset(
//				&core.Secret{
//					ObjectMeta: metaV1.ObjectMeta{
//						Name:      "foo-auth",
//						Namespace: "default",
//					},
//				},
//				&storageV1beta1.StorageClass{
//					ObjectMeta: metaV1.ObjectMeta{
//						Name: "standard",
//					},
//				},
//			)
//
//			objJS, err := meta.MarshalToJson(&c.object, api.SchemeGroupVersion)
//			if err != nil {
//				panic(err)
//			}
//			oldObjJS, err := meta.MarshalToJson(&c.oldObject, api.SchemeGroupVersion)
//			if err != nil {
//				panic(err)
//			}
//
//			req := new(admission.AdmissionRequest)
//
//			req.Kind = c.kind
//			req.Name = c.objectName
//			req.Namespace = c.namespace
//			req.Operation = c.operation
//			req.UserInfo = authenticationV1.UserInfo{}
//			req.Object.Raw = objJS
//			req.OldObject.Raw = oldObjJS
//
//			if c.operation == admission.Delete {
//				req.Object = runtime.RawExtension{}
//			}
//			if c.operation != admission.Update {
//				req.OldObject = runtime.RawExtension{}
//			}
//
//			response := validator.Admit(req)
//			if c.result == true {
//				if response.Allowed != true {
//					t.Errorf("expected: 'Allowed=true'. but got response: %v", response)
//				}
//			} else if c.result == false {
//				if response.Allowed == true || response.Result.Code == http.StatusInternalServerError {
//					t.Errorf("expected: 'Allowed=false', but got response: %v", response)
//				}
//			}
//		})
//	}
//
//}
//
//var cases = []struct {
//	testName   string
//	kind       metaV1.GroupVersionKind
//	objectName string
//	namespace  string
//	operation  admission.Operation
//	object     api.PgBouncer
//	oldObject  api.PgBouncer
//	heatUp     bool
//	result     bool
//}{
//	{"Create Valid PgBouncer",
//		requestKind,
//		"foo",
//		"default",
//		admission.Create,
//		samplePgBouncer(),
//		api.PgBouncer{},
//		false,
//		true,
//	},
//	{"Create Invalid PgBouncer",
//		requestKind,
//		"foo",
//		"default",
//		admission.Create,
//		getAwkwardPgBouncer(),
//		api.PgBouncer{},
//		false,
//		false,
//	},
//	{"Edit Status",
//		requestKind,
//		"foo",
//		"default",
//		admission.Update,
//		editStatus(samplePgBouncer()),
//		samplePgBouncer(),
//		false,
//		true,
//	},
//	{"Edit Spec.Monitor",
//		requestKind,
//		"foo",
//		"default",
//		admission.Update,
//		editSpecMonitor(samplePgBouncer()),
//		samplePgBouncer(),
//		false,
//		true,
//	},
//	{"Edit Invalid Spec.Monitor",
//		requestKind,
//		"foo",
//		"default",
//		admission.Update,
//		editSpecInvalidMonitor(samplePgBouncer()),
//		samplePgBouncer(),
//		false,
//		false,
//	},
//	{"Edit Spec.TerminationPolicy",
//		requestKind,
//		"foo",
//		"default",
//		admission.Update,
//		pauseDatabase(samplePgBouncer()),
//		samplePgBouncer(),
//		false,
//		true,
//	},
//	{"Delete PgBouncer when Spec.TerminationPolicy=DoNotTerminate",
//		requestKind,
//		"foo",
//		"default",
//		admission.Delete,
//		samplePgBouncer(),
//		api.PgBouncer{},
//		true,
//		false,
//	},
//	{"Delete PgBouncer when Spec.TerminationPolicy=Pause",
//		requestKind,
//		"foo",
//		"default",
//		admission.Delete,
//		pauseDatabase(samplePgBouncer()),
//		api.PgBouncer{},
//		true,
//		true,
//	},
//	{"Delete Non Existing PgBouncer",
//		requestKind,
//		"foo",
//		"default",
//		admission.Delete,
//		api.PgBouncer{},
//		api.PgBouncer{},
//		false,
//		true,
//	},
//}
//
//func samplePgBouncer() api.PgBouncer {
//	return api.PgBouncer{
//		TypeMeta: metaV1.TypeMeta{
//			Kind:       api.ResourceKindPgBouncer,
//			APIVersion: api.SchemeGroupVersion.String(),
//		},
//		ObjectMeta: metaV1.ObjectMeta{
//			Name:      "foo",
//			Namespace: "default",
//			Labels: map[string]string{
//				api.LabelDatabaseKind: api.ResourceKindPgBouncer,
//			},
//		},
//		Spec: api.PgBouncerSpec{
//			Version:  "9.6",
//			Replicas: pointer.Int32P(1),
//			UpdateStrategy: apps.StatefulSetUpdateStrategy{
//				Type: apps.RollingUpdateStatefulSetStrategyType,
//			},
//			TerminationPolicy: api.TerminationPolicyDoNotTerminate,
//		},
//	}
//}
//
//func getAwkwardPgBouncer() api.PgBouncer {
//	pgbouncer := samplePgBouncer()
//	pgbouncer.Spec.Version = "3.0"
//	return pgbouncer
//}
//
//func editStatus(old api.PgBouncer) api.PgBouncer {
//	old.Status = api.PgBouncerStatus{
//		Phase: api.DatabasePhaseCreating,
//	}
//	return old
//}
//
//func editSpecMonitor(old api.PgBouncer) api.PgBouncer {
//	old.Spec.Monitor = &mona.AgentSpec{
//		Agent: mona.AgentPrometheusBuiltin,
//		Prometheus: &mona.PrometheusSpec{
//			Port: 5670,
//		},
//	}
//	return old
//}
//
//// should be failed because more fields required for COreOS Monitoring
//func editSpecInvalidMonitor(old api.PgBouncer) api.PgBouncer {
//	old.Spec.Monitor = &mona.AgentSpec{
//		Agent: mona.AgentCoreOSPrometheus,
//	}
//	return old
//}
//
//func pauseDatabase(old api.PgBouncer) api.PgBouncer {
//	old.Spec.TerminationPolicy = api.TerminationPolicyPause
//	return old
//}
