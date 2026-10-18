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
	"encoding/json"
	"fmt"
	"mime"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer/protobuf"
)

// mediaType returns the lower-cased media type portion of a Content-Type
// header value, without any parameters (e.g. "; charset=utf-8"). If the
// value cannot be parsed as a media type, the trimmed raw value is returned.
func mediaType(contentType string) string {
	mt, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return strings.TrimSpace(contentType)
	}
	return mt
}

// decodeRequestBody decodes an HTTP request body into an unstructured object
// based on the request's Content-Type.
//
// Protobuf-encoded bodies (application/vnd.kubernetes.protobuf) are decoded
// with the k8s protobuf serializer, which requires gvk to be registered in
// scheme with protobuf support. All other content types (including missing
// or unknown ones) are decoded as JSON, preserving the proxy's historical
// behavior.
func decodeRequestBody(scheme *runtime.Scheme, gvk schema.GroupVersionKind, contentType string, body []byte) (*unstructured.Unstructured, error) {
	un := &unstructured.Unstructured{}
	if mediaType(contentType) != runtime.ContentTypeProtobuf {
		if err := json.Unmarshal(body, un); err != nil {
			return nil, err
		}
		return un, nil
	}

	if scheme == nil {
		return nil, fmt.Errorf("cannot decode a %q encoded body without a scheme", runtime.ContentTypeProtobuf)
	}
	obj, _, err := protobuf.NewSerializer(scheme, scheme).Decode(body, &gvk, nil)
	if err != nil {
		return nil, fmt.Errorf("could not decode %q encoded body: %w", runtime.ContentTypeProtobuf, err)
	}
	object, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
	if err != nil {
		return nil, fmt.Errorf("could not convert decoded object to unstructured: %w", err)
	}
	un.Object = object
	un.SetGroupVersionKind(obj.GetObjectKind().GroupVersionKind())
	return un, nil
}
