package tests_test

import (
	"context"
	"flag"
	"fmt"
	"testing"
	"time"

	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/ginkgo/v2"
	ginkgo_reporters "github.com/onsi/ginkgo/v2/reporters"
	. "github.com/onsi/gomega"
	qe_reporters "kubevirt.io/qe-tools/pkg/ginkgo-reporters"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"

	"kubevirt.io/containerized-data-importer/tests/framework"
	"kubevirt.io/containerized-data-importer/tests/utils"
)

const (
	pollInterval     = 2 * time.Second
	nsDeletedTimeout = 270 * time.Second
)

var (
	kubectlPath    = flag.String("kubectl-path", "kubectl", "The path to the kubectl binary")
	ocPath         = flag.String("oc-path", "oc", "The path to the oc binary")
	cdiInstallNs   = flag.String("cdi-namespace", "cdi", "The namespace of the CDI controller")
	kubeConfig     = flag.String("kubeconfig", "/var/run/kubernetes/admin.kubeconfig", "The absolute path to the kubeconfig file")
	kubeURL        = flag.String("kubeurl", "", "kube URL url:port")
	goCLIPath      = flag.String("gocli-path", "cli.sh", "The path to cli script")
	snapshotSCName = flag.String("snapshot-sc", "", "The Storage Class supporting snapshots")
	blockSCName    = flag.String("block-sc", "", "The Storage Class supporting block mode volumes")
	csiCloneSCName = flag.String("csiclone-sc", "", "The Storage Class supporting CSI Volume Cloning")
	dockerPrefix   = flag.String("docker-prefix", "", "The docker host:port")
	dockerTag      = flag.String("docker-tag", "", "The docker tag")
)

// cdiFailHandler call ginkgo.Fail with printing the additional information
func cdiFailHandler(message string, callerSkip ...int) {
	if len(callerSkip) > 0 {
		callerSkip[0]++
	}
	ginkgo.Fail(message, callerSkip...)
}

//nolint:staticcheck // Ignore SA1019. Need to keep deprecated function for compatibility.
var afterSuiteReporters []ginkgo.Reporter
var _ = ReportAfterSuite("TestTests", func(report Report) {
	for _, reporter := range afterSuiteReporters {
		//nolint:staticcheck // Ignore SA1019. Need to keep deprecated function for compatibility.
		ginkgo_reporters.ReportViaDeprecatedReporter(reporter, report)
	}
})

func TestTests(t *testing.T) {
	if qe_reporters.Polarion.Run {
		afterSuiteReporters = append(afterSuiteReporters, &qe_reporters.Polarion)
	}
	defer GinkgoRecover()
	RegisterFailHandler(cdiFailHandler)
	BuildTestSuite()
	RunSpecs(t, "Tests Suite")
}

// To understand the order in which things are run, read http://onsi.github.io/ginkgo/#understanding-ginkgos-lifecycle
// flag parsing happens AFTER ginkgo has constructed the entire testing tree. So anything that uses information from flags
// cannot work when called during test tree construction.
func BuildTestSuite() {
	BeforeSuite(func() {
		fmt.Fprintf(ginkgo.GinkgoWriter, "Reading parameters\n")
		// Read flags, and configure client instances
		framework.ClientsInstance.KubectlPath = *kubectlPath
		framework.ClientsInstance.OcPath = *ocPath
		framework.ClientsInstance.CdiInstallNs = *cdiInstallNs
		framework.ClientsInstance.KubeConfig = *kubeConfig
		framework.ClientsInstance.KubeURL = *kubeURL
		framework.ClientsInstance.GoCLIPath = *goCLIPath
		framework.ClientsInstance.SnapshotSCName = *snapshotSCName
		framework.ClientsInstance.BlockSCName = *blockSCName
		framework.ClientsInstance.CsiCloneSCName = *csiCloneSCName
		framework.ClientsInstance.DockerPrefix = *dockerPrefix
		framework.ClientsInstance.DockerTag = *dockerTag

		fmt.Fprintf(ginkgo.GinkgoWriter, "Kubectl path: %s\n", framework.ClientsInstance.KubectlPath)
		fmt.Fprintf(ginkgo.GinkgoWriter, "OC path: %s\n", framework.ClientsInstance.OcPath)
		fmt.Fprintf(ginkgo.GinkgoWriter, "CDI install NS: %s\n", framework.ClientsInstance.CdiInstallNs)
		fmt.Fprintf(ginkgo.GinkgoWriter, "Kubeconfig: %s\n", framework.ClientsInstance.KubeConfig)
		fmt.Fprintf(ginkgo.GinkgoWriter, "KubeURL: %s\n", framework.ClientsInstance.KubeURL)
		fmt.Fprintf(ginkgo.GinkgoWriter, "GO CLI path: %s\n", framework.ClientsInstance.GoCLIPath)
		fmt.Fprintf(ginkgo.GinkgoWriter, "Snapshot SC: %s\n", framework.ClientsInstance.SnapshotSCName)
		fmt.Fprintf(ginkgo.GinkgoWriter, "Block SC: %s\n", framework.ClientsInstance.BlockSCName)
		fmt.Fprintf(ginkgo.GinkgoWriter, "CSI Volume Cloning SC: %s\n", framework.ClientsInstance.CsiCloneSCName)
		fmt.Fprintf(ginkgo.GinkgoWriter, "DockerPrefix: %s\n", framework.ClientsInstance.DockerPrefix)
		fmt.Fprintf(ginkgo.GinkgoWriter, "DockerTag: %s\n", framework.ClientsInstance.DockerTag)

		restConfig, err := framework.ClientsInstance.LoadConfig()
		if err != nil {
			// Can't use Expect here due this being called outside of an It block, and Expect
			// requires any calls to it to be inside an It block.
			ginkgo.Fail("ERROR, unable to load RestConfig")
		}
		framework.ClientsInstance.RestConfig = restConfig
		// clients
		kcs, err := framework.ClientsInstance.GetKubeClient()
		if err != nil {
			ginkgo.Fail(fmt.Sprintf("ERROR, unable to create K8SClient: %v", err))
		}
		framework.ClientsInstance.K8sClient = kcs

		cs, err := framework.ClientsInstance.GetCdiClient()
		if err != nil {
			ginkgo.Fail(fmt.Sprintf("ERROR, unable to create CdiClient: %v", err))
		}
		framework.ClientsInstance.CdiClient = cs

		extcs, err := framework.ClientsInstance.GetExtClient()
		if err != nil {
			ginkgo.Fail(fmt.Sprintf("ERROR, unable to create CsiClient: %v", err))
		}
		framework.ClientsInstance.ExtClient = extcs

		crClient, err := framework.ClientsInstance.GetCrClient()
		if err != nil {
			ginkgo.Fail(fmt.Sprintf("ERROR, unable to create CrClient: %v", err))
		}
		framework.ClientsInstance.CrClient = crClient

		dyn, err := framework.ClientsInstance.GetDynamicClient()
		if err != nil {
			ginkgo.Fail(fmt.Sprintf("ERROR, unable to create DynamicClient: %v", err))
		}
		framework.ClientsInstance.DynamicClient = dyn

		utils.CacheTestsData(framework.ClientsInstance.K8sClient, framework.ClientsInstance.CdiInstallNs)
	})

	AfterSuite(func() {
		client := framework.ClientsInstance.K8sClient

		Eventually(func() []corev1.Namespace {
			nsList, _ := utils.GetTestNamespaceList(client, framework.NsPrefixLabel)
			fmt.Fprintf(ginkgo.GinkgoWriter, "DEBUG: AfterSuite nsList: %v\n", nsList.Items)
			return nsList.Items
		}, nsDeletedTimeout, pollInterval).Should(BeEmpty())

		// Delete temp storage classes
		labelSelector := metav1.LabelSelector{MatchLabels: map[string]string{"cdi.kubevirt.io/testing": ""}}
		scList, err := client.StorageV1().StorageClasses().List(context.TODO(), metav1.ListOptions{
			LabelSelector: labels.Set(labelSelector.MatchLabels).String(),
		})
		Expect(err).ToNot(HaveOccurred())
		for _, sc := range scList.Items {
			err = client.StorageV1().StorageClasses().Delete(context.TODO(), sc.Name, metav1.DeleteOptions{})
			Expect(err).ToNot(HaveOccurred())
		}
	})
}
