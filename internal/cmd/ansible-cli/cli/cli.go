// Copyright 2025 The Operator-SDK Authors
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

package cli

import (
	"fmt"
	"log"
	"runtime"

	"github.com/operator-framework/ansible-operator-plugins/pkg/plugins/ansible/v1"

	ver "github.com/operator-framework/ansible-operator-plugins/internal/version"
	"sigs.k8s.io/kubebuilder/v4/pkg/cli"
	cfgv3 "sigs.k8s.io/kubebuilder/v4/pkg/config/v3"
	"sigs.k8s.io/kubebuilder/v4/pkg/plugin"
	kustomizev2 "sigs.k8s.io/kubebuilder/v4/pkg/plugins/common/kustomize/v2"
)

func Run() error {
	c := GetPluginsCLI()
	return c.Run()
}

func GetPluginsCLI() *cli.CLI {
	ansibleBundle, _ := plugin.NewBundleWithOptions(
		plugin.WithName(ansible.Plugin{}.Name()),
		plugin.WithVersion(ansible.Plugin{}.Version()),
		plugin.WithPlugins(
			kustomizev2.Plugin{},
			ansible.Plugin{},
		),
	)

	c, err := cli.New(
		cli.WithCommandName("ansible-cli"),
		cli.WithVersion(makeVersionString()),
		cli.WithPlugins(
			ansibleBundle,
		),
		cli.WithDefaultPlugins(cfgv3.Version, ansibleBundle),
		cli.WithDefaultProjectVersion(cfgv3.Version),
		cli.WithCompletion(),
	)

	if err != nil {
		log.Fatal(err)
	}

	return c
}

func makeVersionString() string {
	return fmt.Sprintf("ansible-cli version: %q, commit: %q, kubernetes version: %q, go version: %q, GOOS: %q, GOARCH: %q",
		ver.GitVersion, ver.GitCommit, ver.KubernetesVersion, runtime.Version(), runtime.GOOS, runtime.GOARCH)
}
