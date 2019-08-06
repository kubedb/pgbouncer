package framework

import (
	"fmt"
	"os"
	"time"

	"github.com/appscode/go/crypto/rand"
	"github.com/appscode/go/log"
	. "github.com/onsi/gomega"
	core "k8s.io/api/core/v1"
	kerr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	v1 "kmodules.xyz/client-go/core/v1"
	meta_util "kmodules.xyz/client-go/meta"
	store "kmodules.xyz/objectstore-api/api/v1"
	api "kubedb.dev/apimachinery/apis/kubedb/v1alpha1"
	"kubedb.dev/pgbouncer/pkg/controller"
	"stash.appscode.dev/stash/pkg/restic"
)

var (
	CustomSecretSuffix = "custom-secret"
	CustomUsername     = "username1234567890"
	CustomPassword     = "password0987654321"
)

func (i *Invocation) SecretForLocalBackend() *core.Secret {
	return &core.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      rand.WithUniqSuffix(i.app + "-local"),
			Namespace: i.namespace,
		},
		Data: map[string][]byte{},
	}
}

func (i *Invocation) SecretForS3Backend() *core.Secret {
	if os.Getenv(store.AWS_ACCESS_KEY_ID) == "" ||
		os.Getenv(store.AWS_SECRET_ACCESS_KEY) == "" {
		return &core.Secret{}
	}

	return &core.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      rand.WithUniqSuffix(i.app + "-s3"),
			Namespace: i.namespace,
		},
		Data: map[string][]byte{
			store.AWS_ACCESS_KEY_ID:     []byte(os.Getenv(store.AWS_ACCESS_KEY_ID)),
			store.AWS_SECRET_ACCESS_KEY: []byte(os.Getenv(store.AWS_SECRET_ACCESS_KEY)),
		},
	}
}

func (i *Invocation) SecretForMinioBackend() *core.Secret {
	if os.Getenv(store.AWS_ACCESS_KEY_ID) == "" ||
		os.Getenv(store.AWS_SECRET_ACCESS_KEY) == "" {
		return &core.Secret{}
	}

	return &core.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      rand.WithUniqSuffix(i.app + "-s3"),
			Namespace: i.namespace,
		},
		Data: map[string][]byte{
			store.AWS_ACCESS_KEY_ID:     []byte(os.Getenv(store.AWS_ACCESS_KEY_ID)),
			store.AWS_SECRET_ACCESS_KEY: []byte(os.Getenv(store.AWS_SECRET_ACCESS_KEY)),
			store.CA_CERT_DATA:          i.CertStore.CACertBytes(),
		},
	}
}

func (i *Invocation) SecretForDatabaseAuthentication(meta metav1.ObjectMeta) *core.Secret {
	return &core.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%v-%v", meta.Name, CustomSecretSuffix),
			Namespace: meta.Namespace,
		},
		StringData: map[string]string{
			controller.PostgresUser:     CustomUsername,
			controller.PostgresPassword: CustomPassword,
		},
	}
}

func (i *Invocation) SecretForDatabaseAuthenticationWithLabel(meta metav1.ObjectMeta) *core.Secret {
	//this Label mimics a secret created and manged by kubedb and not user.
	// It should get deleted during wipeout
	return &core.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("kubedb-%v-%v", meta.Name, CustomSecretSuffix),
			Namespace: meta.Namespace,
			Labels: map[string]string{
				meta_util.ManagedByLabelKey: api.GenericKey,
			},
		},
		StringData: map[string]string{
			controller.PostgresUser:     CustomUsername,
			controller.PostgresPassword: CustomPassword,
		},
	}
}

func (i *Invocation) PatchSecretForRestic(secret *core.Secret) *core.Secret {
	if secret == nil {
		return secret
	}

	secret.StringData = v1.UpsertMap(secret.StringData, map[string]string{
		restic.RESTIC_PASSWORD: "RESTIC_PASSWORD",
	})
	return secret
}

// TODO: Add more methods for Swift, Backblaze B2, Rest server backend.

func (f *Framework) CreateSecret(obj *core.Secret) error {
	_, err := f.kubeClient.CoreV1().Secrets(obj.Namespace).Create(obj)
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

func (f *Framework) GetPostgresCredentials() (string, string, error) {
	scrt, err := f.kubeClient.CoreV1().Secrets(f.namespace).Get(PostgresName+"-auth", metav1.GetOptions{})
	if err != nil {
		return "", "", err
	}
	username := string(scrt.Data[controller.PostgresUser])
	password := string(scrt.Data[controller.PostgresPassword])
	return username, password, nil
}