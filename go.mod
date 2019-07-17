module github.com/kubedb/pgbouncer

go 1.12

require (
	github.com/appscode/go v0.0.0-20190523031839-1468ee3a76e8
	github.com/aws/aws-sdk-go v1.14.33
	github.com/coreos/bbolt v1.3.3 // indirect
	github.com/coreos/prometheus-operator v0.29.0
	github.com/dnaeon/go-vcr v1.0.1 // indirect
	github.com/emicklei/go-restful v2.9.6+incompatible // indirect
	github.com/go-openapi/spec v0.19.2 // indirect
	github.com/go-openapi/swag v0.19.4 // indirect
	github.com/graymeta/stow v0.1.0 // indirect
	github.com/kubedb/apimachinery v0.0.0-20190716064116-ef4c5ae5dfbb
	github.com/pkg/errors v0.8.1
	github.com/prometheus/client_golang v0.9.4 // indirect
	github.com/prometheus/procfs v0.0.3 // indirect
	github.com/spf13/cobra v0.0.5
	github.com/spf13/pflag v1.0.3
	go.etcd.io/bbolt v1.3.3 // indirect
	k8s.io/api v0.0.0-20190503110853-61630f889b3c
	k8s.io/apiextensions-apiserver v0.0.0-20190508224317-421cff06bf05
	k8s.io/apimachinery v0.0.0-20190508063446-a3da69d3723c
	k8s.io/apiserver v0.0.0-20190508223931-4756b09d7af2
	k8s.io/cli-runtime v0.0.0-20190508184404-b26560c459bd // indirect
	k8s.io/client-go v11.0.0+incompatible
	k8s.io/component-base v0.0.0-20190508223741-40efa6d42997 // indirect
	k8s.io/klog v0.3.3 // indirect
	k8s.io/kube-aggregator v0.0.0-20190508224022-f9852b6d3a84 // indirect
	k8s.io/kubernetes v1.14.4 // indirect
	kmodules.xyz/client-go v0.0.0-20190527113919-eb165836b702
	kmodules.xyz/custom-resources v0.0.0-20190508103408-464e8324c3ec
	kmodules.xyz/monitoring-agent-api v0.0.0-20190508125842-489150794b9b
	kmodules.xyz/offshoot-api v0.0.0-20190527060812-295f97bb8061
	kmodules.xyz/webhook-runtime v0.0.0-20190508094945-962d01212c5b
	stash.appscode.dev/stash v0.0.0-20190529085714-0a45b44ab960
)

replace (
	github.com/graymeta/stow => github.com/appscode/stow v0.0.0-20190506085026-ca5baa008ea3
	gopkg.in/robfig/cron.v2 => github.com/appscode/cron v0.0.0-20170717094345-ca60c6d796d4
	k8s.io/api => k8s.io/api v0.0.0-20190313235455-40a48860b5ab
	k8s.io/apiextensions-apiserver => k8s.io/apiextensions-apiserver v0.0.0-20190315093550-53c4693659ed
	k8s.io/apimachinery => github.com/kmodules/apimachinery v0.0.0-20190508045248-a52a97a7a2bf
	k8s.io/apiserver => github.com/kmodules/apiserver v0.0.0-20190508082252-8397d761d4b5
	k8s.io/cli-runtime => k8s.io/cli-runtime v0.0.0-20190314001948-2899ed30580f
	k8s.io/cloud-provider => k8s.io/cloud-provider v0.0.0-20190314002645-c892ea32361a
	k8s.io/component-base => k8s.io/component-base v0.0.0-20190314000054-4a91899592f4
	k8s.io/klog => k8s.io/klog v0.3.0
	k8s.io/kube-aggregator => k8s.io/kube-aggregator v0.0.0-20190314000639-da8327669ac5
	k8s.io/kube-openapi => k8s.io/kube-openapi v0.0.0-20190228160746-b3a7cee44a30
	k8s.io/metrics => k8s.io/metrics v0.0.0-20190314001731-1bd6a4002213
	k8s.io/utils => k8s.io/utils v0.0.0-20190221042446-c2654d5206da
)
