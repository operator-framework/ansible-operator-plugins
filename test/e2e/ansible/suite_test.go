// Copyright 2020 The Operator-SDK Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package e2e_ansible_test

import (
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/operator-framework/ansible-operator-plugins/hack/generate/samples/ansible"
	"github.com/operator-framework/ansible-operator-plugins/pkg/testutils/command"
	"github.com/operator-framework/ansible-operator-plugins/pkg/testutils/e2e/kind"
	"github.com/operator-framework/ansible-operator-plugins/pkg/testutils/e2e/operator"
	"github.com/operator-framework/ansible-operator-plugins/pkg/testutils/e2e/prometheus"
	"github.com/operator-framework/ansible-operator-plugins/pkg/testutils/kubernetes"
	"github.com/operator-framework/ansible-operator-plugins/pkg/testutils/sample"
	kbutil "sigs.k8s.io/kubebuilder/v4/pkg/plugin/util"
)

//TODO: update this to use the new PoC api

// TestE2EAnsible ensures the ansible projects built with the SDK tool by using its binary.
func TestE2EAnsible(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping Operator SDK E2E Ansible Suite testing in short mode")
	}
	RegisterFailHandler(Fail)
	RunSpecs(t, "E2EAnsible Suite")
}

var (
	kctl                       kubernetes.Kubectl
	isPrometheusManagedBySuite = true
	ansibleSample              sample.Sample
	testdir                    = "e2e-test-ansible"
	image                      = "e2e-test-ansible:temp"
)

// BeforeSuite run before any specs are run to perform the required actions for all e2e ansible tests.
var _ = BeforeSuite(func() {
	wd, err := os.Getwd()
	Expect(err).NotTo(HaveOccurred())

	samples := ansible.GenerateMoleculeSample(path.Join(wd, testdir))
	ansibleSample = samples[0]

	kctl = kubernetes.NewKubectlUtil(
		kubernetes.WithCommandContext(
			command.NewGenericCommandContext(
				command.WithDir(ansibleSample.Dir()),
			),
		),
		kubernetes.WithNamespace(ansibleSample.Name()+"-system"),
		kubernetes.WithServiceAccount(ansibleSample.Name()+"-controller-manager"),
	)

	// TODO(everettraven): IMO this should be moved to the logic for implementing the sample. Keeping as is for now to make finish the PoC easier
	// ---------------------------------------------------

	By("enabling debug logging in the manager")
	err = kbutil.InsertCode(filepath.Join(ansibleSample.Dir(), "config", "manager", "manager.yaml"),
		"--health-probe-bind-address=:6789", "\n          - --zap-log-level=2")
	Expect(err).NotTo(HaveOccurred())

	// ---------------------------------------------------

	By("checking API resources applied on Cluster")
	output, err := kctl.Command("api-resources")
	fmt.Println("output:", output)
	Expect(err).NotTo(HaveOccurred())
	if strings.Contains(output, "servicemonitors") {
		isPrometheusManagedBySuite = false
	}

	if isPrometheusManagedBySuite {
		By("installing Prometheus")
		Expect(prometheus.InstallPrometheusOperator(kctl)).To(Succeed())

		By("ensuring provisioned Prometheus Manager Service")
		Eventually(func() error {
			_, err := kctl.Get(
				false,
				"Service", "prometheus-operator")
			return err
		}, 3*time.Minute, time.Second).Should(Succeed())
	}

	By("building the project image")
	err = operator.BuildOperatorImage(ansibleSample, image)
	Expect(err).NotTo(HaveOccurred())

	onKind, err := kind.IsRunningOnKind(kctl)
	Expect(err).NotTo(HaveOccurred())
	if onKind {
		By("loading the required images into Kind cluster")
		Expect(kind.LoadImageToKindCluster(ansibleSample.CommandContext(), image)).To(Succeed())
	}

})

// AfterSuite run after all the specs have run, regardless of whether any tests have failed to ensures that
// all be cleaned up
var _ = AfterSuite(func() {
	By("uninstalling prerequisites")
	if isPrometheusManagedBySuite {
		By("uninstalling Prometheus")
		Expect(prometheus.UninstallPrometheusOperator(kctl)).To(Succeed())
	}

	By("destroying container image and work dir")
	cmd := exec.Command("docker", "rmi", "-f", image)
	if _, err := ansibleSample.CommandContext().Run(cmd); err != nil {
		Expect(err).To(BeNil())
	}
	if err := os.RemoveAll(testdir); err != nil {
		Expect(err).To(BeNil())
	}
})
