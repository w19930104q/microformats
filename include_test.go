// Copyright (c) The microformats project authors.
// SPDX-License-Identifier: MIT

package microformats

import (
	"encoding/json"
	"net/url"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"golang.org/x/net/html"
)

// jsonRoundTrip serializes parsed data through JSON so comparisons only see
// the public output contract (the unexported parser fields are excluded).
func jsonRoundTrip(t *testing.T, d *Data) any {
	t.Helper()
	b, err := json.Marshal(d)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	var out any
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	return out
}

// parseDocument parses a full HTML document.
func parseDocument(t *testing.T, s string) *html.Node {
	t.Helper()
	doc, err := html.Parse(strings.NewReader(s))
	if err != nil {
		t.Fatalf("html.Parse: %v", err)
	}
	return doc
}

func mustURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("url.Parse(%q): %v", raw, err)
	}
	return u
}

// renderDocument serializes a node tree for source-DOM comparison.
func renderDocument(t *testing.T, n *html.Node) string {
	t.Helper()
	var b strings.Builder
	if err := html.Render(&b, n); err != nil {
		t.Fatalf("html.Render: %v", err)
	}
	return b.String()
}

// propStrings returns the string values of a property, failing on non-string
// values.
func propStrings(t *testing.T, m *Microformat, name string) []string {
	t.Helper()
	var out []string
	for _, v := range m.Properties[name] {
		s, ok := v.(string)
		if !ok {
			t.Fatalf("property %q has non-string value %#v", name, v)
		}
		out = append(out, s)
	}
	return out
}

// Test_Include_Cycle verifies that cyclic include references terminate and
// that only the back edge on the current path is ignored while every other
// field is still extracted.
func Test_Include_Cycle(t *testing.T) {
	const src = `<!DOCTYPE html><html><body>
<div class="vcard" id="a" itemref="b"><span class="fn">Alice</span></div>
<div class="vcard" id="b" itemref="a"><span class="org">B Org</span></div>
</body></html>`
	doc := parseDocument(t, src)
	before := renderDocument(t, doc)

	data := ParseNode(doc, mustURL(t, "http://example.com/"))

	if got, want := len(data.Items), 2; got != want {
		t.Fatalf("got %d items, want %d", got, want)
	}
	if got, want := propStrings(t, data.Items[0], "name"), []string{"Alice"}; !cmp.Equal(got, want) {
		t.Errorf("item[0] name = %v, want %v", got, want)
	}
	if got, want := propStrings(t, data.Items[1], "org"), []string{"B Org"}; !cmp.Equal(got, want) {
		t.Errorf("item[1] org = %v, want %v", got, want)
	}
	// the back edge must not pull the counterpart's fields into either item
	if vals := data.Items[0].Properties["org"]; len(vals) != 0 {
		t.Errorf("item[0] should not inherit org across back edge, got %v", vals)
	}
	if vals := data.Items[1].Properties["name"]; len(vals) != 0 {
		t.Errorf("item[1] should not inherit name across back edge, got %v", vals)
	}
	if got := renderDocument(t, doc); got != before {
		t.Errorf("source DOM mutated:\n got  %q\n want %q", got, before)
	}
}

// Test_Include_ThreeNodeCycle exercises a chain a -> b -> c -> a.
func Test_Include_ThreeNodeCycle(t *testing.T) {
	const src = `<!DOCTYPE html><html><body>
<div class="vcard" id="a" itemref="b"><span class="fn">A</span></div>
<div class="vcard" id="b" itemref="c"><span class="org">B Org</span></div>
<div class="vcard" id="c" itemref="a"><span class="note">C Note</span></div>
</body></html>`
	doc := parseDocument(t, src)

	data := ParseNode(doc, mustURL(t, "http://example.com/"))
	if got, want := len(data.Items), 3; got != want {
		t.Fatalf("got %d items, want %d", got, want)
	}
	want := [][]string{{"A"}, {"B Org"}, {"C Note"}}
	names := []string{"name", "org", "note"}
	for i, item := range data.Items {
		if got := propStrings(t, item, names[i]); !cmp.Equal(got, want[i]) {
			t.Errorf("item[%d] %s = %v, want %v", i, names[i], got, want[i])
		}
	}
}

// Test_Include_SelfReference verifies a node that references itself.
func Test_Include_SelfReference(t *testing.T) {
	const src = `<!DOCTYPE html><html><body>
<div class="vcard" id="self" itemref="self"><span class="fn">Self</span></div>
</body></html>`
	doc := parseDocument(t, src)
	before := renderDocument(t, doc)

	data := ParseNode(doc, mustURL(t, "http://example.com/"))
	if got, want := propStrings(t, data.Items[0], "name"), []string{"Self"}; !cmp.Equal(got, want) {
		t.Errorf("name = %v, want %v", got, want)
	}
	if got := renderDocument(t, doc); got != before {
		t.Errorf("source DOM mutated:\n got  %q\n want %q", got, before)
	}
}

// Test_Include_Diamond verifies that two independent entries referencing the
// same node each receive its full value, and that references resolve in
// declaration order.
func Test_Include_Diamond(t *testing.T) {
	const src = `<!DOCTYPE html><html><body>
<div class="vcard" itemref="shared r2"><span class="fn">One</span></div>
<div class="vcard" itemref="shared"><span class="fn">Two</span></div>
<p id="shared" class="org">Shared Org</p>
<p id="r2" class="note">Second Note</p>
</body></html>`
	doc := parseDocument(t, src)
	before := renderDocument(t, doc)

	data := ParseNode(doc, mustURL(t, "http://example.com/"))
	if got, want := len(data.Items), 2; got != want {
		t.Fatalf("got %d items, want %d", got, want)
	}

	// both independent entries get the shared node's full value
	for i, item := range data.Items {
		if got, want := propStrings(t, item, "org"), []string{"Shared Org"}; !cmp.Equal(got, want) {
			t.Errorf("item[%d] org = %v, want %v", i, got, want)
		}
	}
	if got, want := propStrings(t, data.Items[0], "name"), []string{"One"}; !cmp.Equal(got, want) {
		t.Errorf("item[0] name = %v, want %v", got, want)
	}
	if got, want := propStrings(t, data.Items[1], "name"), []string{"Two"}; !cmp.Equal(got, want) {
		t.Errorf("item[1] name = %v, want %v", got, want)
	}
	// declaration order: shared (org) before r2 (note)
	if got, want := propStrings(t, data.Items[0], "note"), []string{"Second Note"}; !cmp.Equal(got, want) {
		t.Errorf("item[0] note = %v, want %v", got, want)
	}
	if got := renderDocument(t, doc); got != before {
		t.Errorf("source DOM mutated:\n got  %q\n want %q", got, before)
	}
}

// Test_Include_MissingAndDuplicateIDs verifies missing ids are ignored and
// duplicate ids resolve to the first node in the original document.
func Test_Include_MissingAndDuplicateIDs(t *testing.T) {
	const src = `<!DOCTYPE html><html><body>
<div class="vcard" itemref="missing dup"><span class="fn">C</span></div>
<p id="dup" class="org">First</p>
<p id="dup" class="org">Second</p>
</body></html>`
	doc := parseDocument(t, src)
	before := renderDocument(t, doc)

	data := ParseNode(doc, mustURL(t, "http://example.com/"))
	if got, want := propStrings(t, data.Items[0], "org"), []string{"First"}; !cmp.Equal(got, want) {
		t.Errorf("org = %v, want %v (duplicate id must use first, missing id ignored)", got, want)
	}
	if got := renderDocument(t, doc); got != before {
		t.Errorf("source DOM mutated:\n got  %q\n want %q", got, before)
	}
}

// Test_Include_Replacing verifies the class="include" replacement pattern and
// a cyclic replacement reference.
func Test_Include_Replacing(t *testing.T) {
	t.Run("resolves target", func(t *testing.T) {
		const src = `<!DOCTYPE html><html><body>
<div class="vcard"><span class="fn">Dan</span><a class="include" href="#org">tw</a></div>
<p id="org" class="org">The Org</p>
</body></html>`
		doc := parseDocument(t, src)
		before := renderDocument(t, doc)

		data := ParseNode(doc, mustURL(t, "http://example.com/"))
		if got, want := propStrings(t, data.Items[0], "org"), []string{"The Org"}; !cmp.Equal(got, want) {
			t.Errorf("org = %v, want %v", got, want)
		}
		if got := renderDocument(t, doc); got != before {
			t.Errorf("source DOM mutated:\n got  %q\n want %q", got, before)
		}
	})

	t.Run("cyclic replacement terminates", func(t *testing.T) {
		const src = `<!DOCTYPE html><html><body>
<div class="vcard" id="inc"><a class="include" href="#inc">x</a><span class="fn">Carol</span></div>
</body></html>`
		doc := parseDocument(t, src)
		before := renderDocument(t, doc)

		data := ParseNode(doc, mustURL(t, "http://example.com/"))
		if len(data.Items) == 0 {
			t.Fatal("expected at least one item")
		}
		if got := renderDocument(t, doc); got != before {
			t.Errorf("source DOM mutated:\n got  %q\n want %q", got, before)
		}
	})
}

// Test_Include_TableHeaders covers the td headers include pattern.
func Test_Include_TableHeaders(t *testing.T) {
	const src = `<!DOCTYPE html><html><body><table>
<tr><th id="org"><a class="url org" href="http://dev.opera.com/">Opera</a></th></tr>
<tr><td class="vcard" headers="org"><span class="fn">Chris</span></td></tr>
<tr><td class="vcard" headers="org"><span class="fn">Erik</span></td></tr>
</table></body></html>`
	doc := parseDocument(t, src)
	before := renderDocument(t, doc)

	data := ParseNode(doc, mustURL(t, "http://example.com/"))
	if got, want := len(data.Items), 2; got != want {
		t.Fatalf("got %d items, want %d", got, want)
	}
	wantNames := []string{"Chris", "Erik"}
	for i, item := range data.Items {
		if got, want := propStrings(t, item, "name"), []string{wantNames[i]}; !cmp.Equal(got, want) {
			t.Errorf("item[%d] name = %v, want %v", i, got, want)
		}
		if got, want := propStrings(t, item, "org"), []string{"Opera"}; !cmp.Equal(got, want) {
			t.Errorf("item[%d] org = %v, want %v", i, got, want)
		}
		if got, want := propStrings(t, item, "url"), []string{"http://dev.opera.com/"}; !cmp.Equal(got, want) {
			t.Errorf("item[%d] url = %v, want %v", i, got, want)
		}
	}
	if got := renderDocument(t, doc); got != before {
		t.Errorf("source DOM mutated:\n got  %q\n want %q", got, before)
	}
}

// Test_ParseNode_Repeated verifies that parsing the same tree repeatedly is
// equivalent to parsing fresh copies: no accumulated state and no mutation.
func Test_ParseNode_Repeated(t *testing.T) {
	const src = `<!DOCTYPE html><html><body>
<div class="vcard" itemref="x"><span class="fn">A</span></div>
<p id="x" class="org">Org</p>
</body></html>`

	fresh := func() *Data {
		return ParseNode(parseDocument(t, src), mustURL(t, "http://example.com/"))
	}

	shared := parseDocument(t, src)
	before := renderDocument(t, shared)

	firstData := ParseNode(shared, mustURL(t, "http://example.com/"))
	first := jsonRoundTrip(t, firstData)
	for i := 0; i < 5; i++ {
		again := jsonRoundTrip(t, ParseNode(shared, mustURL(t, "http://example.com/")))
		if diff := cmp.Diff(again, first); diff != "" {
			t.Fatalf("repeat parse %d differs from first (-got +want):\n%s", i, diff)
		}
		if diff := cmp.Diff(again, jsonRoundTrip(t, fresh())); diff != "" {
			t.Fatalf("repeat parse %d differs from fresh parse (-got +want):\n%s", i, diff)
		}
	}
	if got := renderDocument(t, shared); got != before {
		t.Errorf("source DOM mutated after repeated parses:\n got  %q\n want %q", got, before)
	}
	// no duplicated referenced values across parses
	if got, want := propStrings(t, firstData.Items[0], "org"), []string{"Org"}; !cmp.Equal(got, want) {
		t.Errorf("org = %v, want %v", got, want)
	}
}

// Test_ParseNode_DifferentBases verifies that re-parsing the same tree with
// different base URLs yields independently resolved relative URLs and leaves
// the source attributes untouched.
func Test_ParseNode_DifferentBases(t *testing.T) {
	const src = `<!DOCTYPE html><html><body>
<a class="h-card" href="rel/path"><span class="p-name">N</span></a>
<div class="h-entry"><div class="e-content"><a href="rel">link</a></div></div>
</body></html>`
	doc := parseDocument(t, src)
	before := renderDocument(t, doc)

	a := ParseNode(doc, mustURL(t, "http://example.com/a/"))
	b := ParseNode(doc, mustURL(t, "http://other.test/x/"))

	if got, want := propStrings(t, a.Items[0], "url"), []string{"http://example.com/a/rel/path"}; !cmp.Equal(got, want) {
		t.Errorf("base a url = %v, want %v", got, want)
	}
	if got, want := propStrings(t, b.Items[0], "url"), []string{"http://other.test/x/rel/path"}; !cmp.Equal(got, want) {
		t.Errorf("base b url = %v, want %v", got, want)
	}
	if got := renderDocument(t, doc); got != before {
		t.Errorf("source DOM mutated:\n got  %q\n want %q", got, before)
	}
}

// Test_ParseNode_EContentContract verifies the text and HTML output of
// e-content, including absolute URL expansion in the HTML copy while the
// source DOM keeps its relative attributes.
func Test_ParseNode_EContentContract(t *testing.T) {
	const src = `<!DOCTYPE html><html><body>
<div class="h-entry"><div class="e-content"><a href="rel">link</a><img src="img.png"></div></div>
</body></html>`
	doc := parseDocument(t, src)
	before := renderDocument(t, doc)

	data := ParseNode(doc, mustURL(t, "http://example.com/"))
	content, ok := data.Items[0].Properties["content"][0].(map[string]string)
	if !ok {
		t.Fatalf("e-content value is not a map: %#v", data.Items[0].Properties["content"])
	}
	wantHTML := `<a href="http://example.com/rel">link</a><img src="http://example.com/img.png">`
	if content["html"] != wantHTML {
		t.Errorf("e-content html = %q, want %q", content["html"], wantHTML)
	}
	if wantValue := "link http://example.com/img.png"; content["value"] != wantValue {
		t.Errorf("e-content value = %q, want %q", content["value"], wantValue)
	}
	if got := renderDocument(t, doc); got != before {
		t.Errorf("source DOM mutated:\n got  %q\n want %q", got, before)
	}
}

// Test_ParseNode_MixedV1V2Nested verifies v1/v2 mixing, nested entries and
// include references still cooperate after the refactor.
func Test_ParseNode_MixedV1V2Nested(t *testing.T) {
	const src = `<!DOCTYPE html><html><body>
<div class="h-feed">
  <article class="hentry" itemref="shared-author">
    <h1 class="entry-title">Post</h1>
    <div class="entry-content">Body <b>bold</b></div>
  </article>
</div>
<p id="shared-author" class="vcard"><span class="fn">Author</span></p>
</body></html>`
	doc := parseDocument(t, src)
	before := renderDocument(t, doc)

	data := ParseNode(doc, mustURL(t, "http://example.com/"))
	// the referenced vcard is also a standalone body node, so it appears as a
	// top-level item in addition to being pulled into the entry.
	if got, want := len(data.Items), 2; got != want {
		t.Fatalf("got %d top-level items, want %d", got, want)
	}
	feed := data.Items[0]
	if got, want := feed.Type, []string{"h-feed"}; !cmp.Equal(got, want) {
		t.Fatalf("feed type = %v, want %v", got, want)
	}
	if got, want := len(feed.Children), 1; got != want {
		t.Fatalf("got %d feed children, want %d", got, want)
	}
	entry := feed.Children[0]
	if got, want := entry.Type, []string{"h-entry"}; !cmp.Equal(got, want) {
		t.Errorf("entry type = %v, want %v", got, want)
	}
	if got, want := propStrings(t, entry, "name"), []string{"Post"}; !cmp.Equal(got, want) {
		t.Errorf("entry name = %v, want %v", got, want)
	}
	content, ok := entry.Properties["content"][0].(map[string]string)
	if !ok {
		t.Fatalf("entry content is not a map: %#v", entry.Properties["content"])
	}
	if wantHTML := "Body <b>bold</b>"; content["html"] != wantHTML {
		t.Errorf("entry content html = %q, want %q", content["html"], wantHTML)
	}
	if wantValue := "Body bold"; content["value"] != wantValue {
		t.Errorf("entry content value = %q, want %q", content["value"], wantValue)
	}
	// the v1 vcard referenced by itemref is parsed as a nested h-card child
	if got, want := len(entry.Children), 1; got != want {
		t.Fatalf("got %d entry children, want %d", got, want)
	}
	author := entry.Children[0]
	if got, want := author.Type, []string{"h-card"}; !cmp.Equal(got, want) {
		t.Errorf("author type = %v, want %v", got, want)
	}
	if got, want := propStrings(t, author, "name"), []string{"Author"}; !cmp.Equal(got, want) {
		t.Errorf("author name = %v, want %v", got, want)
	}
	// and the standalone vcard remains a distinct top-level item
	if got, want := data.Items[1].Type, []string{"h-card"}; !cmp.Equal(got, want) {
		t.Errorf("standalone vcard type = %v, want %v", got, want)
	}
	if got := renderDocument(t, doc); got != before {
		t.Errorf("source DOM mutated:\n got  %q\n want %q", got, before)
	}
}
