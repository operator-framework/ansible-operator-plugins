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

package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer/protobuf"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/cache"

	"github.com/operator-framework/ansible-operator-plugins/internal/ansible/proxy/controllermap"
	"github.com/operator-framework/ansible-operator-plugins/internal/ansible/proxy/kubeconfig"
)

var _ = Describe("injectOwnerReferenceHandler", func() {

	Describe("ServeHTTP", func() {
		It("Should inject ownerReferences even when namespace is not explicitly set", func() {
			if testing.Short() {
				Skip("skipping ansible owner reference injection testing in short mode")
			}
			cm := corev1.ConfigMap{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "v1",
					Kind:       "ConfigMap",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-owner-ref-injection",
				},
				Data: map[string]string{
					"hello": "world",
				},
			}

			body, err := json.Marshal(cm)
			if err != nil {
				Fail("Failed to marshal body")
			}

			po, err := createTestPod("test-injection", "default", testClient)
			if err != nil {
				Fail(fmt.Sprintf("Failed to create pod: %v", err))
			}
			defer func() {
				if err := testClient.Delete(context.Background(), po); err != nil {
					Fail(fmt.Sprintf("Failed to delete the pod: %v", err))
				}
			}()

			req, err := http.NewRequest("POST", "http://localhost:8888/api/v1/namespaces/default/configmaps", bytes.NewReader(body))
			if err != nil {
				Fail(fmt.Sprintf("Failed to create http request: %v", err))
			}

			username, err := kubeconfig.EncodeOwnerRef(
				metav1.OwnerReference{
					APIVersion: "v1",
					Kind:       "Pod",
					Name:       po.GetName(),
					UID:        po.GetUID(),
				}, "default")
			if err != nil {
				Fail("Failed to encode owner reference")
			}
			req.SetBasicAuth(username, "unused")

			httpClient := http.Client{}

			defer func() {
				cleanupReq, err := http.NewRequest("DELETE", "http://localhost:8888/api/v1/namespaces/default/configmaps/test-owner-ref-injection", bytes.NewReader([]byte{}))
				if err != nil {
					Fail(fmt.Sprintf("Failed to delete configmap: %v", err))
				}
				_, err = httpClient.Do(cleanupReq)
				if err != nil {
					Fail(fmt.Sprintf("Failed to delete configmap: %v", err))
				}
			}()

			resp, err := httpClient.Do(req)
			if err != nil {
				Fail(fmt.Sprintf("Failed to create configmap: %v", err))
			}
			respBody, err := io.ReadAll(resp.Body)
			if err != nil {
				Fail(fmt.Sprintf("Failed to read response body: %v", err))
			}
			var modifiedCM corev1.ConfigMap
			err = json.Unmarshal(respBody, &modifiedCM)
			if err != nil {
				Fail(fmt.Sprintf("Failed to unmarshal configmap: %v", err))
			}
			ownerRefs := modifiedCM.ObjectMeta.OwnerReferences

			Expect(ownerRefs).To(HaveLen(1))

			ownerRef := ownerRefs[0]

			Expect(ownerRef.APIVersion).To(Equal("v1"))
			Expect(ownerRef.Kind).To(Equal("Pod"))
			Expect(ownerRef.Name).To(Equal(po.GetName()))
			Expect(ownerRef.UID).To(Equal(po.GetUID()))
		})

		It("Should inject ownerReferences for protobuf encoded request bodies", func() {
			if testing.Short() {
				Skip("skipping ansible owner reference injection testing in short mode")
			}
			cm := corev1.ConfigMap{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "v1",
					Kind:       "ConfigMap",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-protobuf-owner-ref-injection",
				},
				Data: map[string]string{
					"hello": "world",
				},
			}

			body := &bytes.Buffer{}
			if err := protobuf.NewSerializer(clientgoscheme.Scheme, clientgoscheme.Scheme).Encode(&cm, body); err != nil {
				Fail(fmt.Sprintf("Failed to marshal protobuf body: %v", err))
			}

			po, err := createTestPod("test-injection", "default", testClient)
			if err != nil {
				Fail(fmt.Sprintf("Failed to create pod: %v", err))
			}
			defer func() {
				if err := testClient.Delete(context.Background(), po); err != nil {
					Fail(fmt.Sprintf("Failed to delete pod: %v", err))
				}
			}()

			req, err := http.NewRequest("POST", "http://localhost:8888/api/v1/namespaces/default/configmaps", body)
			if err != nil {
				Fail(fmt.Sprintf("Failed to create http request: %v", err))
			}
			req.Header.Set("Content-Type", runtime.ContentTypeProtobuf)

			username, err := kubeconfig.EncodeOwnerRef(
				metav1.OwnerReference{
					APIVersion: "v1",
					Kind:       "Pod",
					Name:       po.GetName(),
					UID:        po.GetUID(),
				}, "default")
			if err != nil {
				Fail("Failed to encode owner reference")
			}
			req.SetBasicAuth(username, "unused")

			httpClient := http.Client{}

			defer func() {
				cleanupReq, err := http.NewRequest("DELETE", "http://localhost:8888/api/v1/namespaces/default/configmaps/test-protobuf-owner-ref-injection", bytes.NewReader([]byte{}))
				if err != nil {
					Fail(fmt.Sprintf("Failed to delete configmap: %v", err))
				}
				_, err = httpClient.Do(cleanupReq)
				if err != nil {
					Fail(fmt.Sprintf("Failed to delete configmap: %v", err))
				}
			}()

			resp, err := httpClient.Do(req)
			if err != nil {
				Fail(fmt.Sprintf("Failed to create configmap: %v", err))
			}
			respBody, err := io.ReadAll(resp.Body)
			if err != nil {
				Fail(fmt.Sprintf("Failed to read response body: %v", err))
			}
			var modifiedCM corev1.ConfigMap
			err = json.Unmarshal(respBody, &modifiedCM)
			if err != nil {
				Fail(fmt.Sprintf("Failed to unmarshal configmap: %v", err))
			}
			ownerRefs := modifiedCM.ObjectMeta.OwnerReferences

			Expect(ownerRefs).To(HaveLen(1))

			ownerRef := ownerRefs[0]

			Expect(ownerRef.APIVersion).To(Equal("v1"))
			Expect(ownerRef.Kind).To(Equal("Pod"))
			Expect(ownerRef.Name).To(Equal(po.GetName()))
			Expect(ownerRef.UID).To(Equal(po.GetUID()))
		})
	})

	Describe("ServeHTTP with protobuf encoded request bodies", func() {
		It("Should forward an injected, JSON-encoded body to the next handler", func() {
			scheme := clientgoscheme.Scheme
			restMapper := meta.NewDefaultRESTMapper([]schema.GroupVersion{
				{Version: "v1"},
				{Group: "example.com", Version: "v1"},
			})
			restMapper.Add(schema.GroupVersionKind{Version: "v1", Kind: "ConfigMap"}, meta.RESTScopeNamespace)
			restMapper.Add(schema.GroupVersionKind{Version: "v1", Kind: "Pod"}, meta.RESTScopeNamespace)
			configMapGVK := schema.GroupVersionKind{Version: "v1", Kind: "ConfigMap"}
			apiRes := &apiResources{
				mu: &sync.RWMutex{},
				gvkToAPIResource: map[string]metav1.APIResource{
					configMapGVK.String(): {
						Name:  "configmaps",
						Kind:  "ConfigMap",
						Verbs: []string{"create", "get", "list", "watch"},
					},
				},
			}

			var gotReq *http.Request
			var gotBody []byte
			next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, err := io.ReadAll(r.Body)
				Expect(err).NotTo(HaveOccurred())
				gotReq = r
				gotBody = body
				w.WriteHeader(http.StatusOK)
			})
			h := &injectOwnerReferenceHandler{
				next:       next,
				cMap:       controllermap.NewControllerMap(),
				restMapper: restMapper,
				scheme:     scheme,
				// An empty set of watched namespaces skips registering a
				// watch for the created resource.
				watchedNamespaces: map[string]cache.Config{},
				apiResources:      apiRes,
			}

			cm := corev1.ConfigMap{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "v1",
					Kind:       "ConfigMap",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-protobuf-injection",
				},
				Data: map[string]string{
					"hello": "world",
				},
			}
			body := &bytes.Buffer{}
			Expect(protobuf.NewSerializer(scheme, scheme).Encode(&cm, body)).To(Succeed())

			req := httptest.NewRequest(http.MethodPost,
				"http://localhost:8888/api/v1/namespaces/default/configmaps", body)
			req.Header.Set("Content-Type", runtime.ContentTypeProtobuf)

			username, err := kubeconfig.EncodeOwnerRef(
				metav1.OwnerReference{
					APIVersion: "v1",
					Kind:       "Pod",
					Name:       "owner-pod",
					UID:        "00000000-0000-0000-0000-000000000000",
				}, "default")
			Expect(err).NotTo(HaveOccurred())
			req.SetBasicAuth(username, "unused")

			w := httptest.NewRecorder()
			h.ServeHTTP(w, req)

			Expect(w.Code).To(Equal(http.StatusOK))
			Expect(gotReq).NotTo(BeNil())

			// The injected body must be forwarded as JSON regardless of the
			// content type the client used.
			Expect(gotReq.Header.Get("Content-Type")).To(Equal(runtime.ContentTypeJSON))
			Expect(gotReq.ContentLength).To(Equal(int64(len(gotBody))))

			var gotCM corev1.ConfigMap
			Expect(json.Unmarshal(gotBody, &gotCM)).To(Succeed())
			Expect(gotCM.GetName()).To(Equal("test-protobuf-injection"))
			Expect(gotCM.Data).To(Equal(map[string]string{"hello": "world"}))

			Expect(gotCM.OwnerReferences).To(HaveLen(1))
			ownerRef := gotCM.OwnerReferences[0]
			Expect(ownerRef.APIVersion).To(Equal("v1"))
			Expect(ownerRef.Kind).To(Equal("Pod"))
			Expect(ownerRef.Name).To(Equal("owner-pod"))
			Expect(ownerRef.UID).To(Equal(types.UID("00000000-0000-0000-0000-000000000000")))
		})
	})
})
