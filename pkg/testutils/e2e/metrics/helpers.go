package metrics

import (
	"fmt"
	"time"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"

	"github.com/operator-framework/ansible-operator-plugins/pkg/testutils/kubernetes"
	"github.com/operator-framework/ansible-operator-plugins/pkg/testutils/sample"
)

// GetMetrics creates a pod with the permissions to `curl` metrics. It will then return the output of the `curl` pod
func GetMetrics(sample sample.Sample, kubectl kubernetes.Kubectl, metricsClusterRoleBindingName string) string {
	ginkgo.By("creating a curl pod")
	cmdOpts := []string{
		"run", "curl", "--image=curlimages/curl:7.68.0", "--restart=OnFailure", "--",
		"curl", "-v",
		fmt.Sprintf("http://%s-controller-manager-metrics-service.%s.svc:8080/metrics", sample.Name(), kubectl.Namespace()),
	}
	out, err := kubectl.CommandInNamespace(cmdOpts...)
	fmt.Println("OUT --", out)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	ginkgo.By("validating that the curl pod is running as expected")
	verifyCurlUp := func() error {
		// Validate pod status
		status, err := kubectl.Get(
			true,
			"pods", "curl", "-o", "jsonpath={.status.phase}")
		if err != nil {
			return err
		}
		if status != "Completed" && status != "Succeeded" {
			return fmt.Errorf("curl pod in %s status", status)
		}
		return nil
	}
	gomega.Eventually(verifyCurlUp, 2*time.Minute, time.Second).Should(gomega.Succeed())

	ginkgo.By("validating that the metrics endpoint is serving as expected")
	var metricsOutput string
	getCurlLogs := func() string {
		metricsOutput, err = kubectl.Logs(true, "curl")
		gomega.ExpectWithOffset(1, err).NotTo(gomega.HaveOccurred())
		return metricsOutput
	}
	gomega.Eventually(getCurlLogs, time.Minute, time.Second).Should(gomega.ContainSubstring("< HTTP/1.1 200"))

	return metricsOutput
}

// CleanUpMetrics with clean up the resources created to gather metrics information
func CleanUpMetrics(kubectl kubernetes.Kubectl, metricsClusterRoleBindingName string) error {
	_, err := kubectl.Delete(true, "pod", "curl")
	if err != nil {
		return fmt.Errorf("encountered an error when deleting the metrics pod: %w", err)
	}

	return nil
}
