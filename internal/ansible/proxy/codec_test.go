// Copyright 2026 The Operator-SDK Authors
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
	"encoding/json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer/protobuf"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
)

var _ = Describe("mediaType", func() {
	It("should strip parameters from a media type", func() {
		Expect(mediaType("application/json; charset=utf-8")).To(Equal("application/json"))
		Expect(mediaType("application/vnd.kubernetes.protobuf")).To(Equal("application/vnd.kubernetes.protobuf"))
	})
	It("should return the trimmed raw value for unparseable input", func() {
		Expect(mediaType(" application/json ")).To(Equal("application/json"))
	})
})

var _ = Describe("decodeRequestBody", func() {
	var cm *corev1.ConfigMap
	var cmGVK schema.GroupVersionKind

	BeforeEach(func() {
		cm = &corev1.ConfigMap{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "v1",
				Kind:       "ConfigMap",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-cm",
				Namespace: "default",
			},
			Data: map[string]string{
				"hello": "world",
			},
		}
		cmGVK = schema.GroupVersionKind{Version: "v1", Kind: "ConfigMap"}
	})

	expectDecodedConfigMap := func(un *unstructured.Unstructured) {
		Expect(un.GetAPIVersion()).To(Equal("v1"))
		Expect(un.GetKind()).To(Equal("ConfigMap"))
		Expect(un.GetName()).To(Equal("test-cm"))
		Expect(un.GetNamespace()).To(Equal("default"))
		data, found, err := unstructured.NestedStringMap(un.Object, "data")
		Expect(err).NotTo(HaveOccurred())
		Expect(found).To(BeTrue())
		Expect(data).To(Equal(map[string]string{"hello": "world"}))
	}

	protobufBody := func(obj runtime.Object) []byte {
		GinkgoHelper()
		buf := &bytes.Buffer{}
		err := protobuf.NewSerializer(clientgoscheme.Scheme, clientgoscheme.Scheme).Encode(obj, buf)
		Expect(err).NotTo(HaveOccurred())
		return buf.Bytes()
	}

	It("should decode a JSON body", func() {
		body, err := json.Marshal(cm)
		Expect(err).NotTo(HaveOccurred())

		un, err := decodeRequestBody(clientgoscheme.Scheme, cmGVK, "application/json", body)
		Expect(err).NotTo(HaveOccurred())
		expectDecodedConfigMap(un)
	})

	It("should decode a body with a JSON content type containing parameters", func() {
		body, err := json.Marshal(cm)
		Expect(err).NotTo(HaveOccurred())

		un, err := decodeRequestBody(clientgoscheme.Scheme, cmGVK, "application/json; charset=utf-8", body)
		Expect(err).NotTo(HaveOccurred())
		expectDecodedConfigMap(un)
	})

	It("should decode a JSON body when the content type is missing or unknown", func() {
		body, err := json.Marshal(cm)
		Expect(err).NotTo(HaveOccurred())

		un, err := decodeRequestBody(clientgoscheme.Scheme, cmGVK, "", body)
		Expect(err).NotTo(HaveOccurred())
		expectDecodedConfigMap(un)

		un, err = decodeRequestBody(clientgoscheme.Scheme, cmGVK, "application/octet-stream", body)
		Expect(err).NotTo(HaveOccurred())
		expectDecodedConfigMap(un)
	})

	It("should decode a protobuf body", func() {
		un, err := decodeRequestBody(clientgoscheme.Scheme, cmGVK, runtime.ContentTypeProtobuf, protobufBody(cm))
		Expect(err).NotTo(HaveOccurred())
		expectDecodedConfigMap(un)
	})

	It("should decode a protobuf body when the default gvk is empty", func() {
		un, err := decodeRequestBody(clientgoscheme.Scheme, schema.GroupVersionKind{}, runtime.ContentTypeProtobuf, protobufBody(cm))
		Expect(err).NotTo(HaveOccurred())
		expectDecodedConfigMap(un)
	})

	It("should error on a protobuf body when the scheme is nil", func() {
		_, err := decodeRequestBody(nil, cmGVK, runtime.ContentTypeProtobuf, protobufBody(cm))
		Expect(err).To(HaveOccurred())
	})

	It("should error on a protobuf body when the kind is not registered in the scheme", func() {
		_, err := decodeRequestBody(runtime.NewScheme(), cmGVK, runtime.ContentTypeProtobuf, protobufBody(cm))
		Expect(err).To(HaveOccurred())
	})

	It("should error on a malformed protobuf body", func() {
		_, err := decodeRequestBody(clientgoscheme.Scheme, cmGVK, runtime.ContentTypeProtobuf, []byte("this is not protobuf"))
		Expect(err).To(HaveOccurred())
	})

	It("should error on a malformed JSON body", func() {
		_, err := decodeRequestBody(clientgoscheme.Scheme, cmGVK, "application/json", []byte("{not json"))
		Expect(err).To(HaveOccurred())
	})
})

var _ = Describe("requestBodyLogValue", func() {
	It("should log protobuf bodies as a size placeholder", func() {
		Expect(requestBodyLogValue(runtime.ContentTypeProtobuf, []byte{0x6b, 0x38, 0x73, 0x00})).
			To(Equal("<4 bytes of application/vnd.kubernetes.protobuf>"))
	})
	It("should log other bodies verbatim", func() {
		Expect(requestBodyLogValue("application/json", []byte(`{"hello":"world"}`))).To(Equal(`{"hello":"world"}`))
		Expect(requestBodyLogValue("", nil)).To(Equal(""))
	})
})
