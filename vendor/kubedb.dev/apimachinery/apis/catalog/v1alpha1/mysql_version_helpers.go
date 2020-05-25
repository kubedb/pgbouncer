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

package v1alpha1

import (
	"fmt"

	"kubedb.dev/apimachinery/api/crds"
	"kubedb.dev/apimachinery/apis"

	"kmodules.xyz/client-go/apiextensions"
)

func (_ MySQLVersion) CustomResourceDefinition() *apiextensions.CustomResourceDefinition {
	return crds.MustCustomResourceDefinition(SchemeGroupVersion.WithResource(ResourcePluralMySQLVersion))
}

var _ apis.ResourceInfo = &MySQLVersion{}

func (m MySQLVersion) ResourceShortCode() string {
	return ResourceCodeMySQLVersion
}

func (m MySQLVersion) ResourceKind() string {
	return ResourceKindMySQLVersion
}

func (m MySQLVersion) ResourceSingular() string {
	return ResourceSingularMySQLVersion
}

func (m MySQLVersion) ResourcePlural() string {
	return ResourcePluralMySQLVersion
}

func (m MySQLVersion) ValidateSpecs() error {
	if m.Spec.Version == "" ||
		m.Spec.DB.Image == "" ||
		m.Spec.Tools.Image == "" ||
		m.Spec.Exporter.Image == "" ||
		m.Spec.InitContainer.Image == "" {
		return fmt.Errorf(`atleast one of the following specs is not set for mysqlVersion "%v":
spec.version,
spec.db.image,
spec.tools.image,
spec.exporter.image,
spec.initContainer.image.`, m.Name)
	}
	return nil
}
