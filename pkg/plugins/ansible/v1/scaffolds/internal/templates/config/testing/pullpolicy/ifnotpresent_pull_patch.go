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

package pullpolicy

import (
	"path/filepath"

	"sigs.k8s.io/kubebuilder/v4/pkg/machinery"
)

var _ machinery.Template = &IfNotPresentPullPatch{}

// IfNotPresentPullPatch scaffolds the patch file for overriding the
// default image pull policy during Ansible testing
type IfNotPresentPullPatch struct {
	machinery.TemplateMixin
}

// SetTemplateDefaults implements machinery.Template
func (f *IfNotPresentPullPatch) SetTemplateDefaults() error {
	if f.Path == "" {
		f.Path = filepath.Join("config", "testing", "pull_policy", "IfNotPresent.yaml")
	}

	f.TemplateBody = ifNotPresentPullPatchTemplate

	f.IfExistsAction = machinery.Error

	return nil
}

const ifNotPresentPullPatchTemplate = `---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: controller-manager
  namespace: system
spec:
  template:
    spec:
      containers:
        - name: manager
          imagePullPolicy: IfNotPresent
`
