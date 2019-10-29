package framework

import (
	"fmt"
	"time"

	api "kubedb.dev/apimachinery/apis/kubedb/v1alpha1"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	kerr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	kutil "kmodules.xyz/client-go"
)

const (
	operatorGetRetryInterval = time.Second * 5
)

func (f *Framework) InstallKubeDBOperators(kubeconfigPath string) {
	//sh := shell.NewSession()

	//By("Installing Posgtres Operator")
	//sh.SetDir("../../../postgres")
	//err := sh.Command("env", "REGISTRY=rezoan", "make", "install").Run()
	//Expect(err).ShouldNot(HaveOccurred())

	By("Setup postgres")
	postgres := f.Invoke().Postgres()
	err := f.CreatePostgres(postgres)
	Expect(err).ShouldNot(HaveOccurred())
	By("Waiting for running Postgres")
	err = f.WaitUntilPostgresReady(postgres.Name)
	Expect(err).ShouldNot(HaveOccurred())
}
func (f *Framework) WaitUntilPostgresReady(name string) error {
	return wait.PollImmediate(operatorGetRetryInterval, kutil.ReadinessTimeout, func() (bool, error) {
		if pg, err := f.dbClient.KubedbV1alpha1().Postgreses(f.Namespace()).Get(name, metav1.GetOptions{}); err == nil {
			if pg.Status.Phase == api.DatabasePhaseRunning {
				return true, nil
			}
		}
		return false, nil
	})
}

func (f *Framework) CleanAdmissionConfigs() {
	// delete validating Webhook
	if err := f.kubeClient.AdmissionregistrationV1beta1().ValidatingWebhookConfigurations().DeleteCollection(deleteInForeground(), metav1.ListOptions{
		LabelSelector: "app=kubedb",
	}); err != nil && !kerr.IsNotFound(err) {
		fmt.Printf("error in deletion of Validating Webhook. Error: %v", err)
	}

	// delete mutating Webhook
	if err := f.kubeClient.AdmissionregistrationV1beta1().MutatingWebhookConfigurations().DeleteCollection(deleteInForeground(), metav1.ListOptions{
		LabelSelector: "app=kubedb",
	}); err != nil && !kerr.IsNotFound(err) {
		fmt.Printf("error in deletion of Mutating Webhook. Error: %v", err)
	}

	// Delete APIService
	if err := f.kaClient.ApiregistrationV1beta1().APIServices().DeleteCollection(deleteInForeground(), metav1.ListOptions{
		LabelSelector: "app=kubedb",
	}); err != nil && !kerr.IsNotFound(err) {
		fmt.Printf("error in deletion of APIService. Error: %v", err)
	}

	// Delete Service
	if err := f.kubeClient.CoreV1().Services("kube-system").Delete("kubedb-operator", &metav1.DeleteOptions{}); err != nil && !kerr.IsNotFound(err) {
		fmt.Printf("error in deletion of Service. Error: %v", err)
	}

	// Delete EndPoints
	if err := f.kubeClient.CoreV1().Endpoints("kube-system").DeleteCollection(deleteInForeground(), metav1.ListOptions{
		LabelSelector: "app=kubedb",
	}); err != nil && !kerr.IsNotFound(err) {
		fmt.Printf("error in deletion of Endpoints. Error: %v", err)
	}

	time.Sleep(time.Second * 1) // let the kube-server know it!!
}

//func (f *Framework) DeleteOperatorAndServer() {
//	sh := shell.NewSession()
//	//args := []interface{}{"--minikube", fmt.Sprintf("--docker-registry=%v", DockerRegistry)
//	sh.ShowCMD = true
//	By("Deleteing Operator")
//	sh.SetDir("../../")
//	cmd := sh.Command("make", "uninstall")
//	err := cmd.Run()
//	Expect(err).ShouldNot(HaveOccurred())
//}
