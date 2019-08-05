package framework

import (
	"fmt"
	"time"

	shell "github.com/codeskyblue/go-sh"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	kerr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	kutil "kmodules.xyz/client-go"
	api "kubedb.dev/apimachinery/apis/kubedb/v1alpha1"
)

const (
	kubeDbOperator           = "kubedb-pg-operator"
	pgbouncerOperator        = "kubedb-pgbouncer-operator"
	operatorGetRetryInterval = time.Second * 5
)

func (f *Framework) RunPgBouncerOperatorAndServer() {

}

func (f *Framework) RunKubeDBOperators(kubeconfigPath string) {
	sh := shell.NewSession()
	sh.ShowCMD = true

	By("Creating Posgtres Operator")
	sh.SetDir("../../../postgres")
	//By("Starting Server and Operator")
	//cmd := sh.Command("bash", "kubedb.sh")
	err := sh.Command("env", "REGISTRY=rezoan", "make", "install").Run()
	Expect(err).ShouldNot(HaveOccurred())
	By("::::::::::: Postgres Operator created")

	By(">>>>>>>>>>>Create postgres")
	f.setupPostgres()
	postgres := f.Invoke().Postgres()
	println(":::::::::::::::::Postgres Name = ", postgres.Name)
	err = f.CreatePostgres(postgres)
	Expect(err).ShouldNot(HaveOccurred())
	By("Waiting for running Postgres")
	err = f.WaitUntilPostgresReady(postgres.Name)
	Expect(err).ShouldNot(HaveOccurred())
	println("Postgres is Ready.")
	By("Uninstall Postgres Operator")
	//err = sh.Command("bash", "kubedb.sh", "--uninstall", "--purge", "--kubeconfig="+kubeconfigPath).Run()
	err = sh.Command("env", "REGISTRY=rezoan", "make", "uninstall").Run()
	Expect(err).NotTo(HaveOccurred())

	By("Installing PgBouncer operator")
	sh = shell.NewSession()
	sh.SetDir("../../")
	cmd := sh.Command("env", "REGISTRY=rezoan", "make", "install")
	By("Starting Server and Operator")
	err = cmd.Run()
	Expect(err).ShouldNot(HaveOccurred())
	By("PgBouncer operator installed")
}
func (f *Framework) WaitUntilPostgresReady(name string) error {
	return wait.PollImmediate(operatorGetRetryInterval, kutil.ReadinessTimeout, func() (bool, error) {
		if pg, err := f.dbClient.KubedbV1alpha1().Postgreses(f.Namespace()).Get(name, metav1.GetOptions{}); err == nil {
			if pg.Status.Phase == api.DatabasePhaseRunning {
				println("____________Postgres is running._________")
				return true, nil
			}
			println("____________waiting for Postgres_________")
		}
		return false, nil
	})
}
func (f *Framework) setupPostgres() {

	//time.Sleep(time.Minute*3)
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

func (f *Framework) DeleteOperatorAndServer() {
	sh := shell.NewSession()
	//args := []interface{}{"--minikube", fmt.Sprintf("--docker-registry=%v", DockerRegistry)
	sh.ShowCMD = true
	By("Creating API server and webhook stuffs")
	sh.SetDir("../../")
	cmd := sh.Command("make", "uninstall")
	By("Starting Server and Operator")
	err := cmd.Run()
	Expect(err).ShouldNot(HaveOccurred())
}
