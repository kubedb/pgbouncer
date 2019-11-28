/*
Copyright The KubeDB Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package framework

import (
	"crypto/tls"
	"fmt"
	"net/http"

	api "kubedb.dev/apimachinery/apis/kubedb/v1alpha1"

	"github.com/appscode/go/sets"
	"github.com/aws/aws-sdk-go/aws"
	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/prom2json"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/runtime"
	v1 "kmodules.xyz/monitoring-agent-api/api/v1"
)

const (
	//metricsURL          = "http://127.0.0.1:56790/metrics"
	pbReservePoolMetric  = "pgbouncer_config_reserve_pool_size"
	ReservePoolSize      = 7
	pbMaxClientMetric    = "pgbouncer_config_max_client_conn"
	MaxClientConnections = 20
	metricsCount         = 2
	exporterPort         = 56790
)

func (f *Framework) AddMonitor(obj *api.PgBouncer) {
	obj.Spec.Monitor = &v1.AgentSpec{
		Agent: v1.AgentPrometheusBuiltin,
	}
}

//VerifyExporter uses metrics from given URL
//and check against known key and value
//to verify the connection is functioning as intended
func (f *Framework) VerifyExporter(meta metav1.ObjectMeta) error {
	tunnel, err := f.ForwardPort(meta, aws.Int(exporterPort))
	if err != nil {
		return err
	}
	defer tunnel.Close()

	metricsURL := fmt.Sprintf("http://127.0.0.1:%d/metrics", tunnel.Local)

	mfChan := make(chan *dto.MetricFamily, 1024)
	transport := makeTransport()
	go runtime.Must(func() error {
		return prom2json.FetchMetricFamilies(metricsURL, mfChan, transport)
	}())
	expectedMetricNames := sets.NewString(pbReservePoolMetric, pbMaxClientMetric)
	var expectedMetrics = map[string]int{
		pbReservePoolMetric: ReservePoolSize,
		pbMaxClientMetric:   MaxClientConnections,
	}

	var count = 0
	for mf := range mfChan {
		if expectedMetricNames.Has(*mf.Name) && expectedMetrics[*mf.Name] == int(*mf.Metric[0].Gauge.Value) {
			count++
		}
	}

	if count != metricsCount {
		return fmt.Errorf("could not find %d metrics", metricsCount-count)
	}

	return nil
}

func makeTransport() *http.Transport {
	return &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
}
