// Copyright 2021 The Operator-SDK Authors
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

package ansible

import (
	"os"
	"path/filepath"

	"github.com/operator-framework/ansible-operator-plugins/hack/generate/samples/internal/pkg"
	"github.com/operator-framework/ansible-operator-plugins/pkg/plugins/ansible/v1"
	"github.com/operator-framework/ansible-operator-plugins/pkg/testutils/command"
	"github.com/operator-framework/ansible-operator-plugins/pkg/testutils/e2e"
	"github.com/operator-framework/ansible-operator-plugins/pkg/testutils/sample"
	samplecli "github.com/operator-framework/ansible-operator-plugins/pkg/testutils/sample/cli"
	log "github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/kubebuilder/v3/pkg/cli"
	cfgv3 "sigs.k8s.io/kubebuilder/v3/pkg/config/v3"
	"sigs.k8s.io/kubebuilder/v3/pkg/plugin"
	kustomizev2 "sigs.k8s.io/kubebuilder/v3/pkg/plugins/common/kustomize/v2"
	"sigs.k8s.io/kubebuilder/v3/pkg/plugins/golang"
)

const bundleImage = "quay.io/example/memcached-operator:v0.0.1"

var memcachedGVK = schema.GroupVersionKind{
	Group:   "cache",
	Version: "v1alpha1",
	Kind:    "Memcached",
}

func getCli() *cli.CLI {
	ansibleBundle, _ := plugin.NewBundleWithOptions(
		plugin.WithName(golang.DefaultNameQualifier),
		plugin.WithVersion(ansible.Plugin{}.Version()),
		plugin.WithPlugins(kustomizev2.Plugin{}, ansible.Plugin{}),
	)

	c, err := cli.New(
		cli.WithCommandName("cli"),
		cli.WithVersion("v0.0.0"),
		cli.WithPlugins(
			ansibleBundle,
		),
		cli.WithDefaultPlugins(cfgv3.Version, ansibleBundle),
		cli.WithDefaultProjectVersion(cfgv3.Version),
		cli.WithCompletion(),
	)
	pkg.CheckError("getting cli implementation:", err)
	return c
}

func GenerateMemcachedSamples(rootPath string) []sample.Sample {
	ansibleCC := command.NewGenericCommandContext(
		command.WithEnv("GO111MODULE=on"),
		command.WithDir(filepath.Join(rootPath, "ansible")),
	)

	ansibleMemcached, err := samplecli.NewCliSample(
		samplecli.WithCLI(getCli()),
		samplecli.WithCommandContext(ansibleCC),
		samplecli.WithDomain("example.com"),
		samplecli.WithGvk(memcachedGVK),
		samplecli.WithPlugins("ansible"),
		samplecli.WithExtraApiOptions("--generate-role", "--generate-playbook"),
		samplecli.WithName("memcached-operator"),
	)
	pkg.CheckError("attempting to create sample cli", err)

	// remove sample directory if it already exists
	err = os.RemoveAll(ansibleMemcached.Dir())
	pkg.CheckError("attempting to remove sample dir", err)

	gen := sample.NewGenerator(
		sample.WithNoWebhook(),
	)

	err = gen.GenerateSamples(ansibleMemcached)
	pkg.CheckError("generating ansible samples", err)

	ImplementMemcached(ansibleMemcached, bundleImage)
	return []sample.Sample{ansibleMemcached}
}

// GenerateMoleculeSample will call all actions to create the directory and generate the sample
// The Context to run the samples are not the same in the e2e test. In this way, note that it should NOT
// be called in the e2e tests since it will call the Prepare() to set the sample context and generate the files
// in the testdata directory. The e2e tests only ought to use the Run() method with the TestContext.
func GenerateMoleculeSample(samplesPath string) []sample.Sample {
	ansibleCC := command.NewGenericCommandContext(
		command.WithEnv("GO111MODULE=on"),
		command.WithDir(filepath.Join(samplesPath, "")),
	)

	ansibleMoleculeMemcached, err := samplecli.NewCliSample(
		samplecli.WithCLI(getCli()),
		samplecli.WithCommandContext(ansibleCC),
		samplecli.WithDomain("example.com"),
		samplecli.WithGvk(
			memcachedGVK,
			schema.GroupVersionKind{
				Group:   memcachedGVK.Group,
				Version: memcachedGVK.Version,
				Kind:    "Foo",
			},
			schema.GroupVersionKind{
				Group:   memcachedGVK.Group,
				Version: memcachedGVK.Version,
				Kind:    "Memfin",
			},
		),
		samplecli.WithPlugins("ansible"),
		samplecli.WithExtraApiOptions("--generate-role", "--generate-playbook"),
		samplecli.WithName("memcached-molecule-operator"),
	)
	pkg.CheckError("attempting to create sample cli", err)

	addIgnore, err := samplecli.NewCliSample(
		samplecli.WithCLI(getCli()),
		samplecli.WithCommandContext(ansibleMoleculeMemcached.CommandContext()),
		samplecli.WithGvk(
			schema.GroupVersionKind{
				Group:   "ignore",
				Version: "v1",
				Kind:    "Secret",
			},
		),
		samplecli.WithPlugins("ansible"),
		samplecli.WithExtraApiOptions("--generate-role"),
		samplecli.WithName(ansibleMoleculeMemcached.Name()),
	)
	pkg.CheckError("creating ignore samples", err)

	// remove sample directory if it already exists
	err = os.RemoveAll(ansibleMoleculeMemcached.Dir())
	pkg.CheckError("attempting to remove sample dir", err)

	gen := sample.NewGenerator(
		sample.WithNoWebhook(),
	)

	err = gen.GenerateSamples(ansibleMoleculeMemcached)
	pkg.CheckError("generating ansible molecule sample", err)

	log.Infof("enabling multigroup support")
	err = e2e.AllowProjectBeMultiGroup(ansibleMoleculeMemcached)
	pkg.CheckError("updating PROJECT file", err)

	ignoreGen := sample.NewGenerator(sample.WithNoInit(), sample.WithNoWebhook())
	err = ignoreGen.GenerateSamples(addIgnore)
	pkg.CheckError("generating ansible molecule sample - ignore", err)

	ImplementMemcached(ansibleMoleculeMemcached, bundleImage)

	ImplementMemcachedMolecule(ansibleMoleculeMemcached, bundleImage)
	return []sample.Sample{ansibleMoleculeMemcached}
}

// GenerateAdvancedMoleculeSample will call all actions to create the directory and generate the sample
// The Context to run the samples are not the same in the e2e test. In this way, note that it should NOT
// be called in the e2e tests since it will call the Prepare() to set the sample context and generate the files
// in the testdata directory. The e2e tests only ought to use the Run() method with the TestContext.
func GenerateAdvancedMoleculeSample(samplesPath string) {
	ansibleCC := command.NewGenericCommandContext(
		command.WithEnv("GO111MODULE=on"),
		command.WithDir(filepath.Join(samplesPath, "")),
	)

	gv := schema.GroupVersion{
		Group:   "test",
		Version: "v1alpha1",
	}

	kinds := []string{
		"ArgsTest",
		"CaseTest",
		"CollectionTest",
		"ClusterAnnotationTest",
		"FinalizerConcurrencyTest",
		"ReconciliationTest",
		"SelectorTest",
		"SubresourcesTest",
	}

	var gvks []schema.GroupVersionKind

	for _, kind := range kinds {
		gvks = append(gvks, schema.GroupVersionKind{Group: gv.Group, Version: gv.Version, Kind: kind})
	}

	advancedMoleculeMemcached, err := samplecli.NewCliSample(
		samplecli.WithCLI(getCli()),
		samplecli.WithCommandContext(ansibleCC),
		samplecli.WithDomain("example.com"),
		samplecli.WithGvk(gvks...),
		samplecli.WithPlugins("ansible"),
		samplecli.WithExtraApiOptions("--generate-playbook"),
		samplecli.WithName("advanced-molecule-operator"),
	)
	pkg.CheckError("attempting to create sample cli", err)

	addInventory, err := samplecli.NewCliSample(
		samplecli.WithCLI(getCli()),
		samplecli.WithCommandContext(advancedMoleculeMemcached.CommandContext()),
		samplecli.WithGvk(
			schema.GroupVersionKind{
				Group:   gv.Group,
				Version: gv.Version,
				Kind:    "InventoryTest",
			},
		),
		samplecli.WithPlugins("ansible"),
		samplecli.WithExtraApiOptions("--generate-role", "--generate-playbook"),
		samplecli.WithName(advancedMoleculeMemcached.Name()),
	)
	pkg.CheckError("creating inventory samples", err)

	// remove sample directory if it already exists
	err = os.RemoveAll(advancedMoleculeMemcached.Dir())
	pkg.CheckError("attempting to remove sample dir", err)

	gen := sample.NewGenerator(sample.WithNoWebhook())

	err = gen.GenerateSamples(advancedMoleculeMemcached)
	pkg.CheckError("generating ansible advanced molecule sample", err)

	ignoreGen := sample.NewGenerator(sample.WithNoInit(), sample.WithNoWebhook())
	err = ignoreGen.GenerateSamples(addInventory)
	pkg.CheckError("generating ansible molecule sample - ignore", err)

	ImplementAdvancedMolecule(advancedMoleculeMemcached, bundleImage)
}
