// Copyright 2016 The OPA Authors.  All rights reserved.
// Use of this source code is governed by an Apache2
// license that can be found in the LICENSE file.

package storage

import (
	"reflect"
	"strings"
	"testing"

	"context"

	"github.com/open-policy-agent/opa/ast"
)

func TestStorageReadPlugin(t *testing.T) {

	ctx := context.Background()

	mem1 := NewDataStoreFromReader(strings.NewReader(`
    {
        "foo": {
            "bar": {
                "baz": [1,2,3,4]
            }
        }
    }`))

	mem2 := NewDataStoreFromReader(strings.NewReader(`
	{
		"corge": [5,6,7,8]
	}
	`))

	mountPath := MustParsePath("/foo/bar/qux")
	store := New(Config{
		Builtin: mem1,
	})
	if err := store.Mount(mem2, mountPath); err != nil {
		t.Fatalf("Unexpected mount error: %v", err)
	}

	txn, err := store.NewTransaction(ctx)
	if err != nil {
		panic(err)
	}

	tests := []struct {
		note     string
		path     string
		expected string
	}{
		{"plugin", "/foo/bar/qux/corge/1", "6"},
		{"multiple", "/foo/bar", `{"baz": [1,2,3,4], "qux": {"corge": [5,6,7,8]}}`},
	}

	for i, tc := range tests {

		expected := loadExpectedResult(tc.expected)
		result, err := store.Read(ctx, txn, MustParsePath(tc.path))

		if err != nil {
			t.Errorf("Test #%d (%v): Unexpected read error: %v", i+1, tc.note, err)
		} else if !reflect.DeepEqual(result, expected) {
			t.Errorf("Test #%d (%v): Expected %v from built-in store but got: %v", i+1, tc.note, expected, result)
		}

	}

}

func TestStorageIndexingBasicUpdate(t *testing.T) {

	refA := ast.MustParseRef("data.a[i]")
	refB := ast.MustParseRef("data.b[x]")
	store, ds := newStorageWithIndices(refA, refB)
	ds.Write(context.Background(), nil, AddOp, MustParsePath("/a/-"), nil)

	if store.IndexExists(refA) {
		t.Errorf("Expected index to be removed after patch")
	}
}

func TestStorageTransactionManagement(t *testing.T) {

	store := New(Config{
		Builtin: NewDataStoreFromReader(strings.NewReader(`
			{
				"foo": {
					"bar": {
						"baz": [1,2,3,4]
					}
				}
			}`)),
	})

	mock := mockStore{}
	ctx := context.Background()

	mountPath := MustParsePath("/foo/bar/qux")
	if err := store.Mount(mock, mountPath); err != nil {
		t.Fatalf("Unexpected mount error: %v", err)
	}

	params := NewTransactionParams().
		WithPaths([]Path{Path{"foo", "bar", "qux", "corge"}})
	txn, err := store.NewTransactionWithParams(ctx, params)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if !reflect.DeepEqual(store.active, map[string]struct{}{mock.ID(): struct{}{}}) {
		t.Fatalf("Expected active to contain exactly one element but got: %v", store.active)
	}

	store.Close(ctx, txn)

	if len(store.active) != 0 {
		t.Fatalf("Expected active to be reset but got: %v", store.active)
	}

}

type mockStore struct {
	WritesNotSupported
	TriggersNotSupported
	id string
}

func (mockStore) ID() string {
	return "mock-store"
}

func (mockStore) Read(ctx context.Context, txn Transaction, path Path) (interface{}, error) {
	return nil, nil
}

func (mockStore) Begin(ctx context.Context, txn Transaction, params TransactionParams) error {
	return nil
}

func (mockStore) Close(ctx context.Context, txn Transaction) {

}

func TestGroupPathsByStore(t *testing.T) {

	root := MustParsePath("/")
	foo := MustParsePath("/foo")
	fooBarQux := MustParsePath("/foo/bar/qux")
	fooBarQuxGrault := MustParsePath("/foo/bar/qux/grault")
	fooBaz := MustParsePath("/foo/baz")
	corge := MustParsePath("/corge")
	grault := MustParsePath("/grault")

	mounts := map[string]Path{
		"mount-1": fooBarQux,
		"mount-2": fooBaz,
		"mount-3": corge,
	}

	result := groupPathsByStore("built-in", mounts, []Path{root})
	expected := map[string][]Path{
		"built-in": {
			root,
		},
		"mount-1": {
			root,
		},
		"mount-2": {
			root,
		},
		"mount-3": {
			root,
		},
	}

	if !reflect.DeepEqual(expected, result) {
		t.Errorf("Expected:\n%v\n\nGot:\n%v", expected, result)
	}

	result = groupPathsByStore("built-in", mounts, []Path{foo})
	expected = map[string][]Path{
		"built-in": {
			foo,
		},
		"mount-1": {
			root,
		},
		"mount-2": {
			root,
		},
	}

	if !reflect.DeepEqual(expected, result) {
		t.Errorf("Expected:\n%v\n\nGot:\n%v", expected, result)
	}

	result = groupPathsByStore("built-in", mounts, []Path{fooBarQuxGrault, corge})
	expected = map[string][]Path{
		"mount-1": {
			grault,
		},
		"mount-3": {
			root,
		},
	}

	if !reflect.DeepEqual(expected, result) {
		t.Errorf("Expected:\n%v\n\nGot:\n%v", expected, result)
	}

}

func mustBuild(store *Storage, ref ast.Ref) {
	err := store.BuildIndex(context.Background(), invalidTXN, ref)
	if err != nil {
		panic(err)
	}
	if !store.IndexExists(ref) {
		panic(err)
	}
}

func newStorageWithIndices(r ...ast.Ref) (*Storage, *DataStore) {

	data := loadSmallTestData()
	ds := NewDataStoreFromJSONObject(data)
	store := New(Config{
		Builtin: ds,
	})

	for _, x := range r {
		mustBuild(store, x)
	}

	return store, ds
}
