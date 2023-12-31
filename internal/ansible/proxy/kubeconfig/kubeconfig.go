// Copyright 2018 The Operator-SDK Authors
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

package kubeconfig

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"html/template"
	"net/url"
	"os"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

var log = logf.Log.WithName("kubeconfig")

// kubectl, as of 1.10.5, only does basic auth if the username is present in
// the URL. The python client used by ansible, as of 6.0.0, only does basic
// auth if the username and password are provided under the "user" key within
// "users".
const kubeConfigTemplate = `---
apiVersion: v1
kind: Config
clusters:
- cluster:
    insecure-skip-tls-verify: true
    server: {{.ProxyURL}}
  name: proxy-server
contexts:
- context:
    cluster: proxy-server
    user: admin/proxy-server
  name: {{.Namespace}}/proxy-server
current-context: {{.Namespace}}/proxy-server
preferences: {}
users:
- name: admin/proxy-server
  user:
    username: {{.Username}}
    password: unused
`

// values holds the data used to render the template
type values struct {
	Username  string
	ProxyURL  string
	Namespace string
}

type NamespacedOwnerReference struct {
	metav1.OwnerReference
	Namespace string
}

// EncodeOwnerRef takes an ownerReference and a namespace and returns a base64 encoded
// string that can be used in the username field of a request to associate the
// owner with the request being made.
func EncodeOwnerRef(ownerRef metav1.OwnerReference, namespace string) (string, error) {
	nsOwnerRef := NamespacedOwnerReference{OwnerReference: ownerRef, Namespace: namespace}
	ownerRefJSON, err := json.Marshal(nsOwnerRef)
	if err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(ownerRefJSON), nil
}

// Create renders a kubeconfig template and writes it to disk
func Create(ownerRef metav1.OwnerReference, proxyURL string, namespace string) (*os.File, error) {
	parsedURL, err := url.Parse(proxyURL)
	if err != nil {
		return nil, err
	}
	username, err := EncodeOwnerRef(ownerRef, namespace)
	if err != nil {
		return nil, err
	}
	parsedURL.User = url.User(username)
	v := values{
		Username:  username,
		ProxyURL:  parsedURL.String(),
		Namespace: namespace,
	}

	var parsed bytes.Buffer

	t := template.Must(template.New("kubeconfig").Parse(kubeConfigTemplate))
	if err := t.Execute(&parsed, v); err != nil {
		return nil, err
	}

	file, err := os.CreateTemp("", "kubeconfig")
	if err != nil {
		return nil, err
	}
	// multiple calls to close file will not hurt anything,
	// but we don't want to lose the error because we are
	// writing to the file, so we will call close twice.
	defer func() {
		if err := file.Close(); err != nil && !errors.Is(err, os.ErrClosed) {
			log.Error(err, "Failed to close generated kubeconfig file")
		}
	}()

	if _, err := file.WriteString(parsed.String()); err != nil {
		return nil, err
	}
	if err := file.Close(); err != nil {
		return nil, err
	}
	return file, nil
}
