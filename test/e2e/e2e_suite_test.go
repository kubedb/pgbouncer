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
package e2e_test

import (
	"flag"
	"os"
	"path/filepath"
	"testing"
	"time"

	cs "kubedb.dev/apimachinery/client/clientset/versioned"
	"kubedb.dev/apimachinery/client/clientset/versioned/scheme"
	"kubedb.dev/pgbouncer/test/e2e/framework"

	. "github.com/onsi/ginkgo"
	"github.com/onsi/ginkgo/reporters"
	. "github.com/onsi/gomega"
	kext_cs "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/typed/apiextensions/v1beta1"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes"
	clientSetScheme "k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"k8s.io/client-go/util/homedir"
	ka "k8s.io/kube-aggregator/pkg/client/clientset_generated/clientset"
	"kmodules.xyz/client-go/logs"
	"kmodules.xyz/client-go/tools/clientcmd"
	appcat_cs "kmodules.xyz/custom-resources/client/clientset/versioned/typed/appcatalog/v1alpha1"
)

// To Run E2E tests:
// 1. have kubedb operator (or postgres+pgbouncer operators) running
//  - make install-postgres-operator
//  - make install
// 2. make e2e-tests

var (
	storageClass   = "standard"
	kubeconfigPath = func() string {
		kubecfg := os.Getenv("KUBECONFIG")
		if kubecfg != "" {
			return kubecfg
		}
		return filepath.Join(homedir.HomeDir(), ".kube", "config")
	}()
	kubeContext = ""
)

func init() {
	runtime.Must(scheme.AddToScheme(clientSetScheme.Scheme))

	flag.StringVar(&storageClass, "storageclass", storageClass, "Kubernetes StorageClass name")
	flag.StringVar(&framework.DockerRegistry, "docker-registry", framework.DockerRegistry, "User provided docker repository")
	flag.StringVar(&framework.DBCatalogName, "db-catalog", framework.DBCatalogName, "PgBouncer version")
}

const (
	TIMEOUT = 20 * time.Minute
)

var (
	root *framework.Framework
)

func TestE2e(t *testing.T) {
	logs.InitLogs()
	RegisterFailHandler(Fail)
	SetDefaultEventuallyTimeout(TIMEOUT)

	junitReporter := reporters.NewJUnitReporter("junit.xml")
	RunSpecsWithDefaultAndCustomReporters(t, "e2e Suite", []Reporter{junitReporter})
}

var _ = BeforeSuite(func() {

	By("Using kubeconfig from " + kubeconfigPath)
	config, err := clientcmd.BuildConfigFromContext(kubeconfigPath, kubeContext)
	Expect(err).NotTo(HaveOccurred())
	// raise throttling time. ref: https://github.com/appscode/voyager/issues/640
	config.Burst = 100
	config.QPS = 100

	// Clients
	kubeClient := kubernetes.NewForConfigOrDie(config)
	dbClient := cs.NewForConfigOrDie(config)
	kaClient := ka.NewForConfigOrDie(config)
	appCatalogClient := appcat_cs.NewForConfigOrDie(config)
	aPIExtKubeClient := kext_cs.NewForConfigOrDie(config)

	// Framework
	root = framework.New(config, kubeClient, aPIExtKubeClient, dbClient, kaClient, appCatalogClient, storageClass)

	// Create namespace
	By("Using namespace " + root.Namespace())
	err = root.CreateNamespace()
	Expect(err).NotTo(HaveOccurred())

	By("Setup postgres resources")
	root.SetupPostgresResources(kubeconfigPath)

	root.EventuallyCRD().Should(Succeed())
})

var _ = AfterSuite(func() {
	By("Delete left over PgBouncer objects")
	root.CleanPgBouncer()
	By("Delete left over Postgres objects")
	root.CleanPostgres()
	By("Delete left over workloads if exists any")
	root.CleanWorkloadLeftOvers()
	By("Delete Namespace")
	err := root.DeleteNamespace()
	Expect(err).NotTo(HaveOccurred())
})
