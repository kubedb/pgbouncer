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

package framework

import (
	"path/filepath"

	api "kubedb.dev/apimachinery/apis/kubedb/v1alpha1"
	cs "kubedb.dev/apimachinery/client/clientset/versioned"

	"github.com/appscode/go/crypto/rand"
	cm "github.com/jetstack/cert-manager/pkg/client/clientset/versioned"
	. "github.com/onsi/gomega"
	"github.com/spf13/afero"
	"gomodules.xyz/cert/certstore"
	crd_cs "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/typed/apiextensions/v1beta1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	ka "k8s.io/kube-aggregator/pkg/client/clientset_generated/clientset"
	appcat_cs "kmodules.xyz/custom-resources/client/clientset/versioned/typed/appcatalog/v1alpha1"
)

var (
	DockerRegistry = "kubedbci"
	DBCatalogName  = "10.2-v4"
)

type Framework struct {
	restConfig        *rest.Config
	kubeClient        kubernetes.Interface
	apiExtKubeClient  crd_cs.ApiextensionsV1beta1Interface
	dbClient          cs.Interface
	kaClient          ka.Interface
	appCatalogClient  appcat_cs.AppcatalogV1alpha1Interface
	namespace         string
	name              string
	StorageClass      string
	CertStore         *certstore.CertStore
	certManagerClient cm.Interface
}

func New(
	restConfig *rest.Config,
	kubeClient kubernetes.Interface,
	apiExtKubeClient crd_cs.ApiextensionsV1beta1Interface,
	dbClient cs.Interface,
	kaClient ka.Interface,
	appCatalogClient appcat_cs.AppcatalogV1alpha1Interface,
	certManagerClient cm.Interface,
	storageClass string,

) *Framework {
	store, err := certstore.NewCertStore(afero.NewMemMapFs(), filepath.Join("", "pki"))
	Expect(err).NotTo(HaveOccurred())

	err = store.InitCA()
	Expect(err).NotTo(HaveOccurred())
	return &Framework{
		restConfig:        restConfig,
		kubeClient:        kubeClient,
		apiExtKubeClient:  apiExtKubeClient,
		dbClient:          dbClient,
		kaClient:          kaClient,
		appCatalogClient:  appCatalogClient,
		certManagerClient: certManagerClient,
		name:              "pgbouncer-operator",
		namespace:         rand.WithUniqSuffix(api.ResourceSingularPgBouncer),
		StorageClass:      storageClass,
		CertStore:         store,
	}
}

func (f *Framework) Invoke() *Invocation {
	return &Invocation{
		Framework: f,
		app:       rand.WithUniqSuffix("pgbouncer-e2e"),
	}
}

func (fi *Invocation) App() string {
	return fi.app
}

func (fi *Invocation) ExtClient() cs.Interface {
	return fi.dbClient
}

type Invocation struct {
	*Framework
	app string
}
