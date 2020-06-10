package dbscan_test

import (
	"reflect"
	"testing"

	"github.com/georgysavva/dbscan"
	"github.com/stretchr/testify/mock"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type FooNested struct {
	FooNested string
}

type BarNested struct {
	BarNested string
}

type nestedUnexported struct {
	FooNested string
	BarNested string
}

type jsonObj struct {
	Key string
}

func TestRowScannerDoScan_StructDestination(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		query    string
		expected interface{}
	}{
		{
			name: "fields without tag are filled from column via snake case mapping",
			query: `
				SELECT 'foo val' AS foo_column, 'bar val' AS bar_column
			`,
			expected: struct {
				FooColumn string
				BarColumn string
			}{
				FooColumn: "foo val",
				BarColumn: "bar val",
			},
		},
		{
			name: "fields with tag are filled from columns via tag",
			query: `
				SELECT 'foo val' AS foo_column, 'bar val' AS bar_column
			`,
			expected: struct {
				Foo string `db:"foo_column"`
				Bar string `db:"bar_column"`
			}{
				Foo: "foo val",
				Bar: "bar val",
			},
		},
		{
			name: "string field by ptr",
			query: `
				SELECT 'foo val' AS foo, 'bar val' AS bar
			`,
			expected: struct {
				Foo *string
				Bar string
			}{
				Foo: makeStrPtr("foo val"),
				Bar: "bar val",
			},
		},
		{
			name: "field with ignore tag isn't filled",
			query: `
				SELECT 'foo val' AS foo
			`,
			expected: struct {
				Foo string `db:"-"`
				Bar string `db:"foo"`
			}{
				Foo: "",
				Bar: "foo val",
			},
		},
		{
			name: "embedded struct is filled from columns without prefix",
			query: `
				SELECT 'foo val' AS foo, 'bar val' AS bar,
					'foo nested val' as foo_nested, 'bar nested val' as bar_nested
			`,
			expected: struct {
				FooNested
				BarNested
				Foo string
				Bar string
			}{
				FooNested: FooNested{
					FooNested: "foo nested val",
				},
				BarNested: BarNested{
					BarNested: "bar nested val",
				},
				Foo: "foo val",
				Bar: "bar val",
			},
		},
		{
			name: "embedded struct with tag is filled from columns with prefix",
			query: `
				SELECT 'foo val' AS foo, 'bar val' AS bar,
					'foo nested val' as "nested.foo_nested"
			`,
			expected: struct {
				FooNested `db:"nested"`
				Foo       string
				Bar       string
			}{
				FooNested: FooNested{
					FooNested: "foo nested val",
				},
				Foo: "foo val",
				Bar: "bar val",
			},
		},
		{
			name: "embedded struct by ptr is initialized and filled",
			query: `
				SELECT 'foo val' AS foo, 'bar val' AS bar,
					'foo nested val' as foo_nested
			`,
			expected: struct {
				*FooNested
				Foo string
				Bar string
			}{
				FooNested: &FooNested{
					FooNested: "foo nested val",
				},
				Foo: "foo val",
				Bar: "bar val",
			},
		},
		{
			name: "embedded struct by ptr isn't initialized if not filled",
			query: `
				SELECT 'foo val' AS foo, 'bar val' AS bar
			`,
			expected: struct {
				*FooNested
				Foo string
				Bar string
			}{
				FooNested: nil,
				Foo:       "foo val",
				Bar:       "bar val",
			},
		},
		{
			name: "embedded struct with ignore tag isn't filled",
			query: `
				SELECT 'foo nested val' as "nested.foo_nested", 
					'bar nested val' as "nested.bar_nested"
			`,
			expected: struct {
				FooNested `db:"-"`
				Foo       string `db:"nested.foo_nested"`
				Bar       string `db:"nested.bar_nested"`
			}{
				FooNested: FooNested{},
				Foo:       "foo nested val",
				Bar:       "bar nested val",
			},
		},
		{
			name: "nested struct is filled from a json column",
			query: `
				SELECT '{"key": "key val"}'::JSON AS foo_json, 'foo val' AS foo
			`,
			expected: struct {
				FooJSON jsonObj
				Foo     string
			}{
				FooJSON: jsonObj{Key: "key val"},
				Foo:     "foo val",
			},
		},
		{
			name: "nested struct by ptr is filled from a json column",
			query: `
				SELECT '{"key": "key val"}'::JSON AS foo_json, 'foo val' AS foo
			`,
			expected: struct {
				FooJSON *jsonObj
				Foo     string
			}{
				FooJSON: &jsonObj{Key: "key val"},
				Foo:     "foo val",
			},
		},
		{
			name: "map field is filled from a json column",
			query: `
				SELECT '{"key": "key val"}'::JSON AS foo_json, 'foo val' AS foo
			`,
			expected: struct {
				FooJSON map[string]interface{}
				Foo     string
			}{
				FooJSON: map[string]interface{}{"key": "key val"},
				Foo:     "foo val",
			},
		},
		{
			name: "map field by ptr is filled from a json column",
			query: `
				SELECT '{"key": "key val"}'::JSON AS foo_json, 'foo val' AS foo
			`,
			expected: struct {
				FooJSON *map[string]interface{}
				Foo     string
			}{
				FooJSON: &map[string]interface{}{"key": "key val"},
				Foo:     "foo val",
			},
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			rows := queryRows(t, tc.query)
			dstVal := newDstValue(tc.expected)
			err := doScan(t, dstVal, rows)
			require.NoError(t, err)
			assertDstValueEqual(t, tc.expected, dstVal)
		})
	}
}

func TestRowScannerDoScan_InvalidStructDestination_ReturnsErr(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name        string
		query       string
		dst         interface{}
		expectedErr string
	}{
		{
			name: "doesn't have a corresponding field",
			query: `
				SELECT 'foo val' AS foo, 'bar val' AS bar
			`,
			dst: struct {
				Bar string
			}{},
			expectedErr: "dbscan: column: 'foo': no corresponding field found or it's unexported in " +
				"struct { Bar string }",
		},
		{
			name: "the corresponding field is unexported",
			query: `
				SELECT 'foo val' AS foo, 'bar val' AS bar
			`,
			dst: struct {
				foo string
				Bar string
			}{},
			expectedErr: "dbscan: column: 'foo': no corresponding field found or it's unexported in " +
				"struct { foo string; Bar string }",
		},
		{
			name: "embedded struct is unexported",
			query: `
				SELECT 'foo val' AS foo, 'bar val' AS bar,
					'foo nested val' as foo_nested, 'bar nested val' as bar_nested
			`,
			dst: struct {
				nestedUnexported
				Foo string
				Bar string
			}{},
			expectedErr: "dbscan: column: 'foo_nested': no corresponding field found or it's unexported in " +
				"struct { dbscan_test.nestedUnexported; Foo string; Bar string }",
		},
		{
			name: "nested non embedded structs aren't allowed",
			query: `
				SELECT 'foo val' AS foo, 'bar val' AS bar,
					'foo nested val' as foo_nested, 'bar nested val' as bar_nested
			`,
			dst: struct {
				Nested FooNested
				Foo    string
				Bar    string
			}{},
			expectedErr: "dbscan: column: 'foo_nested': no corresponding field found or it's unexported in " +
				"struct { Nested dbscan_test.FooNested; Foo string; Bar string }",
		},
		{
			name: "fields contain duplicated tag",
			query: `
				SELECT 'foo val' AS foo_column, 'bar val' AS bar
			`,
			dst: struct {
				Foo string `db:"foo_column"`
				Bar string `db:"foo_column"`
			}{},
			expectedErr: "dbscan: Column must have exactly one field pointing to it; " +
				"found 2 fields with indexes [0] and [1] pointing to 'foo_column' in " +
				"struct { Foo string \"db:\\\"foo_column\\\"\"; Bar string \"db:\\\"foo_column\\\"\" }",
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			rows := queryRows(t, tc.query)
			dstVal := newDstValue(tc.dst)
			err := doScan(t, dstVal, rows)
			assert.EqualError(t, err, tc.expectedErr)
		})
	}
}

func TestRowScannerDoScan_MapDestination(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		query    string
		expected interface{}
	}{
		{
			name: "map[string]interface{}",
			query: `
				SELECT 'foo val' AS foo, 'bar val' AS bar
			`,
			expected: map[string]interface{}{
				"foo": "foo val",
				"bar": "bar val",
			},
		},
		{
			name: "map[string]string{}",
			query: `
				SELECT 'foo val' AS foo, 'bar val' AS bar
			`,
			expected: map[string]string{
				"foo": "foo val",
				"bar": "bar val",
			},
		},
		{
			name: "map[string]*string{}",
			query: `
				SELECT 'foo val' AS foo, NULL AS bar
			`,
			expected: map[string]*string{
				"foo": makeStrPtr("foo val"),
				"bar": nil,
			},
		},
		{
			name: "map[string]struct{}",
			query: `
				SELECT '{"key": "key val"}'::JSON AS foo_json, '{"key": "key val 2"}'::JSON AS bar_json
			`,
			expected: map[string]jsonObj{
				"foo_json": {Key: "key val"},
				"bar_json": {Key: "key val 2"},
			},
		},
		{
			name: "map[string]*struct{}",
			query: `
				SELECT '{"key": "key val"}'::JSON AS foo_json, NULL AS bar_json
			`,
			expected: map[string]*jsonObj{
				"foo_json": {Key: "key val"},
				"bar_json": nil,
			},
		},
		{
			name: "map[string]map[string]interface{}",
			query: `
				SELECT '{"key": "key val"}'::JSON AS foo_json, '{"key": "key val 2"}'::JSON AS bar_json
			`,
			expected: map[string]map[string]interface{}{
				"foo_json": {"key": "key val"},
				"bar_json": {"key": "key val 2"},
			},
		},
		{
			name: "map[string]*map[string]interface{}",
			query: `
				SELECT '{"key": "key val"}'::JSON AS foo_json, NULL AS bar_json
			`,
			expected: map[string]*map[string]interface{}{
				"foo_json": {"key": "key val"},
				"bar_json": nil,
			},
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			rows := queryRows(t, tc.query)
			dstVal := newDstValue(tc.expected)
			err := doScan(t, dstVal, rows)
			require.NoError(t, err)
			assertDstValueEqual(t, tc.expected, dstVal)
		})
	}
}

func TestRowScannerDoScan_MapDestinationWithNonStringKey_ReturnsErr(t *testing.T) {
	t.Parallel()
	query := `
		SELECT 'foo val' AS foo, 'bar val' AS bar
	`
	rows := queryRows(t, query)
	expectedErr := "dbscan: invalid type map[int]interface {}: map must have string key, got: int"
	dstVal := newDstValue(map[int]interface{}{})

	err := doScan(t, dstVal, rows)

	assert.EqualError(t, err, expectedErr)
}

func TestRowScannerDoScan_PrimitiveTypeDestination(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		query    string
		expected interface{}
	}{
		{
			name: "string",
			query: `
				SELECT 'foo val' AS foo 
			`,
			expected: "foo val",
		},
		{
			name: "string by ptr",
			query: `
				SELECT 'foo val' AS foo 
			`,
			expected: "foo val",
		},
		{
			name: "slice",
			query: `
				SELECT ARRAY('foo val', 'foo val 2', 'foo val 3') AS foo 
			`,
			expected: []string{"foo val", "foo val 2", "foo val 3"},
		},
		{
			name: "slice by ptr",
			query: `
				SELECT ARRAY('foo val', 'foo val 2', 'foo val 3') AS foo 
			`,
			expected: &[]string{"foo val", "foo val 2", "foo val 3"},
		},
		{
			name: "struct by ptr treated as primitive type",
			query: `
				SELECT '{"key": "key val"}'::JSON AS foo_json
			`,

			expected: &jsonObj{Key: "key val"},
		},
		{
			name: "map by ptr treated as primitive type",
			query: `
				SELECT '{"key": "key val"}'::JSON AS foo_json
			`,
			expected: &map[string]interface{}{"key": "key val"},
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			rows := queryRows(t, tc.query)
			dstVal := newDstValue(tc.expected)
			err := doScan(t, dstVal, rows)
			require.NoError(t, err)
			assertDstValueEqual(t, tc.expected, dstVal)
		})
	}
}

func TestRowScannerDoScan_PrimitiveTypeDestinationRowsContainMoreThanOneColumn_ReturnsErr(t *testing.T) {
	t.Parallel()
	query := `
		SELECT 'foo val' AS foo, 'bar val' AS bar
	`
	rows := queryRows(t, query)
	expectedErr := "dbscan: to scan into a primitive type, columns number must be exactly 1, got: 2"
	dstVal := newDstValue("")

	err := doScan(t, dstVal, rows)

	assert.EqualError(t, err, expectedErr)
}

// It seems that there is no way to select result set with 0 columns from crdb server.
// So this type exists in order to check that dbscan handles this cases properly.
type emptyRow struct{}

func (er emptyRow) Scan(_ ...interface{}) error { return nil }
func (er emptyRow) Next() bool                  { return true }
func (er emptyRow) Columns() ([]string, error)  { return []string{}, nil }
func (er emptyRow) Close() error                { return nil }
func (er emptyRow) Err() error                  { return nil }

func TestRowScannerDoScan_PrimitiveTypeDestinationRowsContainZeroColumns_ReturnsErr(t *testing.T) {
	t.Parallel()
	rows := emptyRow{}
	var dst string
	expectedErr := "dbscan: to scan into a primitive type, columns number must be exactly 1, got: 0"
	dstVal := newDstValue(dst)
	err := doScan(t, dstVal, rows)
	assert.EqualError(t, err, expectedErr)
}

func TestRowScannerDoScan_RowsContainDuplicatedColumn_ReturnsErr(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		dst  interface{}
	}{
		{
			name: "struct destination",
			dst: struct {
				Foo string
			}{},
		},
		{
			name: "map destination",
			dst:  map[string]interface{}{},
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			query := `
				SELECT 'foo val' AS foo, 'foo val' AS foo
			`
			rows := queryRows(t, query)
			dstVal := newDstValue(tc.dst)
			expectedErr := "dbscan: rows contain duplicated column 'foo'"

			err := doScan(t, dstVal, rows)

			assert.EqualError(t, err, expectedErr)
		})
	}
}

type RowScannerMock struct {
	mock.Mock
	*dbscan.RowScanner
}

func (rsm *RowScannerMock) start(dstValue reflect.Value) error {
	_ = rsm.Called(dstValue)
	return rsm.RowScanner.Start(dstValue)
}

func TestRowScannerDoScan_AfterFirstScan_StartNotCalled(t *testing.T) {
	t.Parallel()
	query := `
		SELECT *
		FROM (
			VALUES ('foo val'), ('foo val 2'), ('foo val 3')
		) AS t (foo)
	`
	rows := queryRows(t, query)
	defer rows.Close()
	rs := dbscan.NewRowScanner(rows)
	rsMock := &RowScannerMock{RowScanner: rs}
	rsMock.On("start", mock.Anything)
	rs.SetStartFn(rsMock.start)

	for rows.Next() {
		var dst struct {
			Foo string
		}
		dstVal := newDstValue(dst)
		err := rs.DoScan(dstVal)
		require.NoError(t, err)
	}
	requireNoRowsErrors(t, rows)

	rsMock.AssertNumberOfCalls(t, "start", 1)
}