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
	"fmt"
	"time"

	api "kubedb.dev/apimachinery/apis/kubedb/v1alpha1"
	"kubedb.dev/pgbouncer/pkg/controller"

	"github.com/appscode/go/log"
	. "github.com/onsi/gomega"
	core "k8s.io/api/core/v1"
	kerr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	core_util "kmodules.xyz/client-go/core/v1"
)

const (
	PgBouncerUserListSecret = "pb-userlist-secret"
)

func (f *Framework) CreateSecret(obj *core.Secret) error {
	_, err := f.kubeClient.CoreV1().Secrets(obj.Namespace).Create(obj)
	return err
}

func (f *Framework) CreateUserListSecret() error {
	username, password, err := f.GetPostgresCredentials()
	if err != nil {
		return err
	}
	useListSecretSpec := &core.Secret{
		StringData: map[string]string{
			"userlist.txt": fmt.Sprintf(`"%s" "%s"
"%s" "%s"`, username, password, testUser, testPass),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      PgBouncerUserListSecret,
			Namespace: f.namespace,
		},
		//TypeMeta: metav1.TypeMeta{},
	}
	_, err = f.kubeClient.CoreV1().Secrets(f.namespace).Create(useListSecretSpec)
	return err
}

func (f *Framework) AddUserToUserListSecret(username, password string) error {
	sec, err := f.kubeClient.CoreV1().Secrets(f.namespace).Get(PgBouncerUserListSecret, metav1.GetOptions{})
	if err != nil {
		return err
	}
	for key, data := range sec.StringData {
		//add new data to existing data
		newData := string(data) + fmt.Sprintf(`
"%s" "%s"`, username, password)
		sec.StringData[key] = newData
	}
	_, _, err = core_util.CreateOrPatchSecret(f.kubeClient, sec.ObjectMeta, func(in *core.Secret) *core.Secret {
		return in
	}, false)
	return err
}

func (f *Framework) UpdateSecret(meta metav1.ObjectMeta, transformer func(core.Secret) core.Secret) error {
	attempt := 0
	for ; attempt < maxAttempts; attempt = attempt + 1 {
		cur, err := f.kubeClient.CoreV1().Secrets(meta.Namespace).Get(meta.Name, metav1.GetOptions{})
		if kerr.IsNotFound(err) {
			return nil
		} else if err == nil {
			modified := transformer(*cur)
			_, err = f.kubeClient.CoreV1().Secrets(cur.Namespace).Update(&modified)
			if err == nil {
				return nil
			}
		}
		log.Errorf("Attempt %d failed to update Secret %s@%s due to %s.", attempt, cur.Name, cur.Namespace, err)
		time.Sleep(updateRetryInterval)
	}
	return fmt.Errorf("failed to update Secret %s@%s after %d attempts", meta.Name, meta.Namespace, attempt)
}

func (f *Framework) DeleteSecret(meta metav1.ObjectMeta) error {
	err := f.kubeClient.CoreV1().Secrets(meta.Namespace).Delete(meta.Name, deleteInForeground())
	if !kerr.IsNotFound(err) {
		return err
	}
	return nil
}

func (f *Framework) EventuallyDBSecretCount(meta metav1.ObjectMeta) GomegaAsyncAssertion {
	labelMap := map[string]string{
		api.LabelDatabaseKind: api.ResourceKindPgBouncer,
		api.LabelDatabaseName: meta.Name,
	}
	labelSelector := labels.SelectorFromSet(labelMap)
	return Eventually(
		func() int {
			secretList, err := f.kubeClient.CoreV1().Secrets(meta.Namespace).List(
				metav1.ListOptions{
					LabelSelector: labelSelector.String(),
				},
			)
			Expect(err).NotTo(HaveOccurred())
			return len(secretList.Items)
		},
		time.Minute*5,
		time.Second*5,
	)
}

func (f *Framework) CheckSecret() error {
	_, err := f.kubeClient.CoreV1().Secrets(f.namespace).Get(PostgresName+"-auth", metav1.GetOptions{})
	return err
}

func (f *Framework) CheckUserListSecret() error {
	_, err := f.kubeClient.CoreV1().Secrets(f.namespace).Get(PgBouncerUserListSecret, metav1.GetOptions{})
	return err
}

func (f *Framework) DeleteUserListSecret() error {
	err := f.kubeClient.CoreV1().Secrets(f.namespace).Delete(PgBouncerUserListSecret, &metav1.DeleteOptions{})
	return err
}

func (f *Framework) GetPostgresCredentials() (string, string, error) {
	scrt, err := f.kubeClient.CoreV1().Secrets(f.namespace).Get(PostgresName+"-auth", metav1.GetOptions{})
	if err != nil {
		return "", "", err
	}
	username := string(scrt.Data[controller.PostgresUser])
	password := string(scrt.Data[controller.PostgresPassword])
	return username, password, nil
}
