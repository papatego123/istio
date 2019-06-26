// Copyright 2019 Istio Authors
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

package inmemory

import (
	"testing"

	. "github.com/onsi/gomega"

	"istio.io/istio/galley/pkg/config/event"
	"istio.io/istio/galley/pkg/config/resource"
	"istio.io/istio/galley/pkg/config/testing/basicmeta"
	"istio.io/istio/galley/pkg/config/testing/data"
	"istio.io/istio/galley/pkg/config/testing/data/builtin"
	"istio.io/istio/galley/pkg/config/testing/fixtures"
	"istio.io/istio/galley/pkg/config/testing/k8smeta"
	"istio.io/istio/galley/pkg/config/util/kubeyaml"
)

func TestKubeSource_ApplyContent(t *testing.T) {
	g := NewGomegaWithT(t)

	s, acc := setupKubeSource()
	defer s.Stop()

	err := s.ApplyContent("foo", data.YamlN1I1V1)
	g.Expect(err).To(BeNil())

	g.Expect(s.ContentNames()).To(Equal(map[string]struct{}{"foo": {}}))

	actual := s.Get(data.Collection1).AllSorted()
	g.Expect(actual).To(HaveLen(1))

	g.Expect(actual[0].Metadata.Name).To(Equal(data.EntryN1I1V1.Metadata.Name))

	g.Expect(acc.Events()).To(HaveLen(2))
	g.Expect(acc.Events()[0].Kind).To(Equal(event.FullSync))
	g.Expect(acc.Events()[1].Kind).To(Equal(event.Added))
	g.Expect(acc.Events()[1].Entry.Metadata.Name).To(Equal(data.EntryN1I1V1.Metadata.Name))
}

func TestKubeSource_ApplyContent_Unchanged0Add1(t *testing.T) {
	g := NewGomegaWithT(t)

	s, acc := setupKubeSource()
	defer s.Stop()

	err := s.ApplyContent("foo", kubeyaml.JoinString(data.YamlN1I1V1, data.YamlN2I2V1))
	g.Expect(err).To(BeNil())

	actual := s.Get(data.Collection1).AllSorted()
	g.Expect(actual).To(HaveLen(2))
	g.Expect(actual[0].Metadata.Name).To(Equal(data.EntryN1I1V1.Metadata.Name))
	g.Expect(actual[1].Metadata.Name).To(Equal(data.EntryN2I2V1.Metadata.Name))

	err = s.ApplyContent("foo", kubeyaml.JoinString(data.YamlN2I2V2, data.YamlN3I3V1))
	g.Expect(err).To(BeNil())

	g.Expect(s.ContentNames()).To(Equal(map[string]struct{}{"foo": {}}))

	actual = s.Get(data.Collection1).AllSorted()
	g.Expect(actual).To(HaveLen(2))
	g.Expect(actual[0].Metadata.Name).To(Equal(data.EntryN2I2V2.Metadata.Name))
	g.Expect(actual[1].Metadata.Name).To(Equal(data.EntryN3I3V1.Metadata.Name))

	g.Expect(acc.Events()).To(HaveLen(6))
	g.Expect(acc.Events()[0].Kind).To(Equal(event.FullSync))
	g.Expect(acc.Events()[1].Kind).To(Equal(event.Added))
	g.Expect(acc.Events()[1].Entry).To(Equal(data.EntryN1I1V1))
	g.Expect(acc.Events()[2].Kind).To(Equal(event.Added))
	g.Expect(acc.Events()[2].Entry).To(Equal(withVersion(data.EntryN2I2V1, "v2")))
	g.Expect(acc.Events()[3].Kind).To(Equal(event.Updated))
	g.Expect(acc.Events()[3].Entry).To(Equal(withVersion(data.EntryN2I2V2, "v3")))
	g.Expect(acc.Events()[4].Kind).To(Equal(event.Added))
	g.Expect(acc.Events()[4].Entry).To(Equal(withVersion(data.EntryN3I3V1, "v4")))
	g.Expect(acc.Events()[5].Kind).To(Equal(event.Deleted))
	g.Expect(acc.Events()[5].Entry.Metadata.Name).To(Equal(data.EntryN1I1V1.Metadata.Name))
}

func TestKubeSource_RemoveContent(t *testing.T) {
	g := NewGomegaWithT(t)

	s, acc := setupKubeSource()
	defer s.Stop()

	err := s.ApplyContent("foo", kubeyaml.JoinString(data.YamlN1I1V1, data.YamlN2I2V1))
	g.Expect(err).To(BeNil())
	err = s.ApplyContent("bar", kubeyaml.JoinString(data.YamlN3I3V1))
	g.Expect(err).To(BeNil())

	g.Expect(s.ContentNames()).To(Equal(map[string]struct{}{"bar": {}, "foo": {}}))

	s.RemoveContent("foo")
	g.Expect(s.ContentNames()).To(Equal(map[string]struct{}{"bar": {}}))

	actual := s.Get(data.Collection1).AllSorted()
	g.Expect(actual).To(HaveLen(1))

	g.Expect(acc.Events()).To(HaveLen(6))
	g.Expect(acc.Events()[0:4]).To(ConsistOf(
		event.FullSyncFor(data.Collection1),
		event.AddFor(data.Collection1, data.EntryN1I1V1),
		event.AddFor(data.Collection1, withVersion(data.EntryN2I2V1, "v2")),
		event.AddFor(data.Collection1, withVersion(data.EntryN3I3V1, "v3"))))

	//  Delete events can appear out of order.
	g.Expect(acc.Events()[4].Kind).To(Equal(event.Deleted))
	g.Expect(acc.Events()[5].Kind).To(Equal(event.Deleted))

	if acc.Events()[4].Entry.Metadata.Name == data.EntryN1I1V1.Metadata.Name {
		g.Expect(acc.Events()[4:]).To(ConsistOf(
			event.DeleteForResource(data.Collection1, data.EntryN1I1V1),
			event.DeleteForResource(data.Collection1, withVersion(data.EntryN2I2V1, "v2"))))
	} else {
		g.Expect(acc.Events()[4:]).To(ConsistOf(
			event.DeleteForResource(data.Collection1, withVersion(data.EntryN2I2V1, "v2")),
			event.DeleteForResource(data.Collection1, data.EntryN1I1V1)))
	}
}

func TestKubeSource_Clear(t *testing.T) {
	g := NewGomegaWithT(t)

	s, acc := setupKubeSource()
	defer s.Stop()

	err := s.ApplyContent("foo", kubeyaml.JoinString(data.YamlN1I1V1, data.YamlN2I2V1))
	g.Expect(err).To(BeNil())

	s.Clear()

	actual := s.Get(data.Collection1).AllSorted()
	g.Expect(actual).To(HaveLen(0))

	g.Expect(acc.Events()).To(HaveLen(5))
	g.Expect(acc.Events()[0].Kind).To(Equal(event.FullSync))
	g.Expect(acc.Events()[1].Kind).To(Equal(event.Added))
	g.Expect(acc.Events()[1].Entry).To(Equal(data.EntryN1I1V1))
	g.Expect(acc.Events()[2].Kind).To(Equal(event.Added))
	g.Expect(acc.Events()[2].Entry).To(Equal(withVersion(data.EntryN2I2V1, "v2")))

	g.Expect(acc.Events()[3].Kind).To(Equal(event.Deleted))
	g.Expect(acc.Events()[4].Kind).To(Equal(event.Deleted))

	if acc.Events()[3].Entry.Metadata.Name == data.EntryN1I1V1.Metadata.Name {
		g.Expect(acc.Events()[3].Entry.Metadata.Name).To(Equal(data.EntryN1I1V1.Metadata.Name))
		g.Expect(acc.Events()[4].Entry.Metadata.Name).To(Equal(data.EntryN2I2V1.Metadata.Name))
	} else {
		g.Expect(acc.Events()[3].Entry.Metadata.Name).To(Equal(data.EntryN2I2V1.Metadata.Name))
		g.Expect(acc.Events()[4].Entry.Metadata.Name).To(Equal(data.EntryN1I1V1.Metadata.Name))
	}
}

func TestKubeSource_UnparseableSegment(t *testing.T) {
	g := NewGomegaWithT(t)

	s, _ := setupKubeSource()
	defer s.Stop()

	err := s.ApplyContent("foo", kubeyaml.JoinString(data.YamlN1I1V1, "	\n", data.YamlN2I2V1))
	g.Expect(err).To(BeNil())

	actual := s.Get(data.Collection1).AllSorted()
	g.Expect(actual).To(HaveLen(2))
	g.Expect(actual[0]).To(Equal(data.EntryN1I1V1))
	g.Expect(actual[1]).To(Equal(withVersion(data.EntryN2I2V1, "v2")))
}

func TestKubeSource_Unrecognized(t *testing.T) {
	g := NewGomegaWithT(t)

	s, _ := setupKubeSource()
	defer s.Stop()

	err := s.ApplyContent("foo", kubeyaml.JoinString(data.YamlN1I1V1, data.YamlUnrecognized))
	g.Expect(err).To(BeNil())

	actual := s.Get(data.Collection1).AllSorted()
	g.Expect(actual).To(HaveLen(1))
	g.Expect(actual[0]).To(Equal(data.EntryN1I1V1))
}

func TestKubeSource_Unparseable(t *testing.T) {
	g := NewGomegaWithT(t)

	s, _ := setupKubeSource()
	defer s.Stop()

	err := s.ApplyContent("foo", kubeyaml.JoinString(data.YamlN1I1V1, data.YamlUnparseableResource))
	g.Expect(err).To(BeNil())

	actual := s.Get(data.Collection1).AllSorted()
	g.Expect(actual).To(HaveLen(1))
	g.Expect(actual[0]).To(Equal(data.EntryN1I1V1))
}

func TestKubeSource_NonStringKey(t *testing.T) {
	g := NewGomegaWithT(t)

	s, _ := setupKubeSource()
	defer s.Stop()

	err := s.ApplyContent("foo", kubeyaml.JoinString(data.YamlN1I1V1, data.YamlNonStringKey))
	g.Expect(err).To(BeNil())

	actual := s.Get(data.Collection1).AllSorted()
	g.Expect(actual).To(HaveLen(1))
	g.Expect(actual[0]).To(Equal(data.EntryN1I1V1))
}

func TestKubeSource_Service(t *testing.T) {
	g := NewGomegaWithT(t)

	s, _ := setupKubeSourceWithK8sMeta()
	defer s.Stop()

	err := s.ApplyContent("foo", builtin.GetService())
	g.Expect(err).To(BeNil())

	actual := s.Get(k8smeta.K8SCoreV1Services).AllSorted()
	g.Expect(actual).To(HaveLen(1))
	g.Expect(actual[0].Metadata.Name).To(Equal(resource.NewName("kube-system", "kube-dns")))
}

func setupKubeSource() (*KubeSource, *fixtures.Accumulator) {
	s := NewKubeSource(basicmeta.MustGet().KubeSource().Resources())

	acc := &fixtures.Accumulator{}
	s.Dispatch(acc)

	s.Start()
	return s, acc
}

func setupKubeSourceWithK8sMeta() (*KubeSource, *fixtures.Accumulator) {
	s := NewKubeSource(k8smeta.MustGet().KubeSource().Resources())

	acc := &fixtures.Accumulator{}
	s.Dispatch(acc)

	s.Start()
	return s, acc
}

func withVersion(r *resource.Entry, v string) *resource.Entry {
	r = r.Clone()
	r.Metadata.Version = resource.Version(v)
	return r
}
