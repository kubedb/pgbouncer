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
	"context"
	"fmt"
	"time"

	api "kubedb.dev/apimachinery/apis/kubedb/v1alpha2"
	"kubedb.dev/pgbouncer/pkg/controller"

	. "github.com/onsi/gomega"
	"gomodules.xyz/x/crypto/rand"
	"gomodules.xyz/x/log"
	core "k8s.io/api/core/v1"
	kerr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	core_util "kmodules.xyz/client-go/core/v1"
	meta_util "kmodules.xyz/client-go/meta"
)

const (
	PgBouncerUserListSecret = "pb-userlist-secret"
	userListKey             = controller.UserListKey
)

func (f *Framework) CreateSecret(obj *core.Secret) error {
	_, err := f.kubeClient.CoreV1().Secrets(obj.Namespace).Create(context.TODO(), obj, metav1.CreateOptions{})
	return err
}

func (f *Framework) GetUserListSecret() *core.Secret {
	username, password, err := f.GetPostgresCredentials()
	if err != nil {
		log.Infoln(err)
	}
	userListSecretSpec := &core.Secret{
		StringData: map[string]string{
			userListKey: fmt.Sprintf(`"%s" "%s"
"%s" "%s"`, username, password, testUser, testPass),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      rand.WithUniqSuffix(PgBouncerUserListSecret),
			Namespace: f.namespace,
		},
	}
	return userListSecretSpec
}

func (f *Framework) AddUserToUserListSecret(meta metav1.ObjectMeta, username, password string) error {
	sec, err := f.kubeClient.CoreV1().Secrets(f.namespace).Get(context.TODO(), meta.Name, metav1.GetOptions{})
	if err != nil {
		return err
	}
	for key, data := range sec.StringData {
		//add new data to existing data
		newData := string(data) + fmt.Sprintf(`
"%s" "%s"`, username, password)
		sec.StringData[key] = newData
	}
	_, _, err = core_util.CreateOrPatchSecret(context.TODO(), f.kubeClient, sec.ObjectMeta, func(in *core.Secret) *core.Secret {
		return in
	}, metav1.PatchOptions{}, false)
	return err
}

func (f *Framework) UpdateSecret(meta metav1.ObjectMeta, transformer func(core.Secret) core.Secret) error {
	attempt := 0
	for ; attempt < maxAttempts; attempt = attempt + 1 {
		cur, err := f.kubeClient.CoreV1().Secrets(meta.Namespace).Get(context.TODO(), meta.Name, metav1.GetOptions{})
		if kerr.IsNotFound(err) {
			return nil
		} else if err == nil {
			modified := transformer(*cur)
			_, err = f.kubeClient.CoreV1().Secrets(cur.Namespace).Update(context.TODO(), &modified, metav1.UpdateOptions{})
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
	err := f.kubeClient.CoreV1().Secrets(meta.Namespace).Delete(context.TODO(), meta.Name, meta_util.DeleteInForeground())
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
				context.TODO(),
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
	_, err := f.kubeClient.CoreV1().Secrets(f.namespace).Get(context.TODO(), PostgresName+"-auth", metav1.GetOptions{})
	return err
}

func (f *Framework) CheckUserListSecret(meta metav1.ObjectMeta) error {
	_, err := f.kubeClient.CoreV1().Secrets(f.namespace).Get(context.TODO(), meta.Name, metav1.GetOptions{})
	return err
}

func (f *Framework) DeleteUserListSecret(meta metav1.ObjectMeta) error {
	return f.kubeClient.CoreV1().Secrets(f.namespace).Delete(context.TODO(), meta.Name, metav1.DeleteOptions{})
}

func (f *Framework) GetPostgresCredentials() (string, string, error) {
	scrt, err := f.kubeClient.CoreV1().Secrets(f.namespace).Get(context.TODO(), PostgresName+"-auth", metav1.GetOptions{})
	if err != nil {
		return "", "", err
	}
	username := string(scrt.Data[core.BasicAuthUsernameKey])
	password := string(scrt.Data[core.BasicAuthPasswordKey])
	return username, password, nil
}
