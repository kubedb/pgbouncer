module kubedb.dev/pgbouncer

go 1.12

require (
	github.com/aws/aws-sdk-go v1.31.13
	github.com/codeskyblue/go-sh v0.0.0-20200712050446-30169cf553fe
	github.com/go-xorm/xorm v0.7.9
	github.com/jetstack/cert-manager v0.16.0
	github.com/lib/pq v1.5.2
	github.com/onsi/ginkgo v1.11.0
	github.com/onsi/gomega v1.8.1
	github.com/prometheus-operator/prometheus-operator v0.42.0
	github.com/prometheus/client_model v0.2.0
	github.com/prometheus/prom2json v1.3.0
	github.com/spf13/cobra v1.1.1
	github.com/spf13/pflag v1.0.5
	go.bytebuilders.dev/license-verifier v0.5.1
	go.bytebuilders.dev/license-verifier/kubernetes v0.5.1
	gomodules.xyz/blobfs v0.1.5
	gomodules.xyz/cert v1.2.0
	gomodules.xyz/password-generator v0.2.6
	gomodules.xyz/pointer v0.0.0-20201105071923-daf60fa55209
	gomodules.xyz/runtime v0.0.0-20201104200926-d838b09dda8b
	gomodules.xyz/x v0.0.0-20201105065653-91c568df6331
	k8s.io/api v0.18.9
	k8s.io/apiextensions-apiserver v0.18.9
	k8s.io/apimachinery v0.18.9
	k8s.io/apiserver v0.18.9
	k8s.io/client-go v0.18.9
	k8s.io/kube-aggregator v0.18.9
	kmodules.xyz/client-go v0.0.0-20201229202653-b029d8e7bcbb
	kmodules.xyz/custom-resources v0.0.0-20201124062543-bd8d35c21b0c
	kmodules.xyz/monitoring-agent-api v0.0.0-20201105074044-be7a1044891a
	kmodules.xyz/offshoot-api v0.0.0-20201105074700-8675f5f686f2
	kmodules.xyz/webhook-runtime v0.0.0-20201105073856-2dc7382b88c6
	kubedb.dev/apimachinery v0.15.3-0.20201229203022-a36050ca55a5
)

replace bitbucket.org/ww/goautoneg => gomodules.xyz/goautoneg v0.0.0-20120707110453-a547fc61f48d

replace cloud.google.com/go => cloud.google.com/go v0.49.0

replace git.apache.org/thrift.git => github.com/apache/thrift v0.13.0

replace github.com/Azure/azure-sdk-for-go => github.com/Azure/azure-sdk-for-go v35.0.0+incompatible

replace github.com/Azure/go-ansiterm => github.com/Azure/go-ansiterm v0.0.0-20170929234023-d6e3b3328b78

replace github.com/Azure/go-autorest => github.com/Azure/go-autorest v13.0.0+incompatible

replace github.com/Azure/go-autorest/autorest => github.com/Azure/go-autorest/autorest v0.9.0

replace github.com/Azure/go-autorest/autorest/adal => github.com/Azure/go-autorest/autorest/adal v0.5.0

replace github.com/Azure/go-autorest/autorest/azure/auth => github.com/Azure/go-autorest/autorest/azure/auth v0.2.0

replace github.com/Azure/go-autorest/autorest/date => github.com/Azure/go-autorest/autorest/date v0.1.0

replace github.com/Azure/go-autorest/autorest/mocks => github.com/Azure/go-autorest/autorest/mocks v0.2.0

replace github.com/Azure/go-autorest/autorest/to => github.com/Azure/go-autorest/autorest/to v0.2.0

replace github.com/Azure/go-autorest/autorest/validation => github.com/Azure/go-autorest/autorest/validation v0.1.0

replace github.com/Azure/go-autorest/logger => github.com/Azure/go-autorest/logger v0.1.0

replace github.com/Azure/go-autorest/tracing => github.com/Azure/go-autorest/tracing v0.5.0

replace github.com/go-openapi/analysis => github.com/go-openapi/analysis v0.19.5

replace github.com/go-openapi/errors => github.com/go-openapi/errors v0.19.2

replace github.com/go-openapi/jsonpointer => github.com/go-openapi/jsonpointer v0.19.3

replace github.com/go-openapi/jsonreference => github.com/go-openapi/jsonreference v0.19.3

replace github.com/go-openapi/loads => github.com/go-openapi/loads v0.19.4

replace github.com/go-openapi/runtime => github.com/go-openapi/runtime v0.19.4

replace github.com/go-openapi/spec => github.com/go-openapi/spec v0.19.3

replace github.com/go-openapi/strfmt => github.com/go-openapi/strfmt v0.19.3

replace github.com/go-openapi/swag => github.com/go-openapi/swag v0.19.5

replace github.com/go-openapi/validate => github.com/go-openapi/validate v0.19.5

replace github.com/gogo/protobuf => github.com/gogo/protobuf v1.3.1

replace github.com/golang/protobuf => github.com/golang/protobuf v1.3.2

replace github.com/googleapis/gnostic => github.com/googleapis/gnostic v0.3.1

replace github.com/imdario/mergo => github.com/imdario/mergo v0.3.5

replace github.com/prometheus-operator/prometheus-operator => github.com/prometheus-operator/prometheus-operator v0.42.0

replace github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring => github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring v0.42.0

replace github.com/prometheus/client_golang => github.com/prometheus/client_golang v1.7.1

replace go.etcd.io/etcd => go.etcd.io/etcd v0.0.0-20191023171146-3cf2f69b5738

replace google.golang.org/api => google.golang.org/api v0.14.0

replace google.golang.org/genproto => google.golang.org/genproto v0.0.0-20191115194625-c23dd37a84c9

replace google.golang.org/grpc => google.golang.org/grpc v1.26.0

replace k8s.io/api => github.com/kmodules/api v0.18.10-0.20200922195318-d60fe725dea0

replace k8s.io/apimachinery => github.com/kmodules/apimachinery v0.19.0-alpha.0.0.20200922195535-0c9a1b86beec

replace k8s.io/apiserver => github.com/kmodules/apiserver v0.18.10-0.20200922195747-1bd1cc8f00d1

replace k8s.io/cli-runtime => k8s.io/cli-runtime v0.18.9

replace k8s.io/client-go => github.com/kmodules/k8s-client-go v0.18.10-0.20200922201634-73fedf3d677e

replace k8s.io/component-base => k8s.io/component-base v0.18.9

replace k8s.io/kube-openapi => k8s.io/kube-openapi v0.0.0-20200410145947-61e04a5be9a6

replace k8s.io/kubernetes => github.com/kmodules/kubernetes v1.19.0-alpha.0.0.20200922200158-8b13196d8dc4

replace k8s.io/utils => k8s.io/utils v0.0.0-20200324210504-a9aa75ae1b89
