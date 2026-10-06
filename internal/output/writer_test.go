package output

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

// stepResult is a representative success value, modeled on the small result
// structs cmd/ passes to Writer.WriteResult (e.g. StepsResult, Result).
type stepResult struct {
	Step        string `json:"step"`
	Instruction string `json:"instruction"`
}

func TestNewErrorWithBuilderChainSetsAllFields(t *testing.T) {
	er := NewError("bad_input", "resource not found").
		WithResource("spec.md").
		WithState("draft", []string{"submit", "cancel"}).
		WithNextAction("run spek fix")

	require.True(t, er.IsError)
	require.Equal(t, "bad_input", er.Code)
	require.Equal(t, "resource not found", er.Message)
	require.Equal(t, "spec.md", er.Resource)
	require.NotNil(t, er.State)
	require.Equal(t, "draft", er.State.Current)
	require.Equal(t, []string{"submit", "cancel"}, er.State.ValidActions)
	require.Equal(t, "run spek fix", er.NextAction)

	data, err := json.Marshal(er)
	require.NoError(t, err)

	want := `{
		"error": true,
		"code": "bad_input",
		"message": "resource not found",
		"resource": "spec.md",
		"state": {"current": "draft", "valid_actions": ["submit", "cancel"]},
		"next_action": "run spek fix"
	}`
	require.JSONEq(t, want, string(data))
}

func TestNewErrorWithoutOptionalFieldsOmitsThem(t *testing.T) {
	er := NewError("bad_input", "resource not found")

	data, err := json.Marshal(er)
	require.NoError(t, err)

	want := `{
		"error": true,
		"code": "bad_input",
		"message": "resource not found",
		"next_action": ""
	}`
	require.JSONEq(t, want, string(data))

	var m map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(data, &m))
	_, hasResource := m["resource"]
	require.False(t, hasResource, "resource should be omitted (omitempty)")
	_, hasState := m["state"]
	require.False(t, hasState, "state should be omitted (omitempty)")
	_, hasNextAction := m["next_action"]
	require.True(t, hasNextAction, "next_action has no omitempty and should always be present")
}

func TestErrorResponseErrorReturnsMessage(t *testing.T) {
	er := NewError("bad_input", "resource not found")

	var err error = er
	require.Equal(t, "resource not found", err.Error())
}

func TestWriteInjectsFalseDiscriminantOnSuccessValue(t *testing.T) {
	var buf bytes.Buffer
	err := Write(&buf, stepResult{Step: "foo", Instruction: "do the thing"}, "")
	require.NoError(t, err)

	var m map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &m))
	require.Equal(t, false, m["error"])
	require.Equal(t, "foo", m["step"])
	require.Equal(t, "do the thing", m["instruction"])
}

func TestWriteInjectsFalseDiscriminantOnMapValue(t *testing.T) {
	var buf bytes.Buffer
	err := Write(&buf, map[string]any{"step": "foo"}, "")
	require.NoError(t, err)

	want := `{"error": false, "step": "foo"}`
	require.JSONEq(t, want, buf.String())
}

func TestWriteOnErrorResponseHasNoConflictingDiscriminant(t *testing.T) {
	var buf bytes.Buffer
	er := NewError("bad_input", "resource not found")
	err := Write(&buf, er, "")
	require.NoError(t, err)

	var m map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &m))
	require.Equal(t, true, m["error"])

	// The "error" key must appear exactly once in the raw JSON object (no
	// double-injection alongside IsError's own marshaled value).
	require.Equal(t, 1, bytes.Count(buf.Bytes(), []byte(`"error"`)))
}

func TestWriteFailureWritesErrorResponseWithTrueDiscriminant(t *testing.T) {
	var buf bytes.Buffer
	er := NewError("bad_input", "resource not found").WithResource("spec.md")
	err := WriteFailure(&buf, er, "")
	require.NoError(t, err)

	want := `{
		"error": true,
		"code": "bad_input",
		"message": "resource not found",
		"resource": "spec.md",
		"next_action": ""
	}`
	require.JSONEq(t, want, buf.String())
}

func TestWriteFieldsFilterAppliesAfterDiscriminantInjectionOnSuccess(t *testing.T) {
	var buf bytes.Buffer
	err := Write(&buf, stepResult{Step: "foo", Instruction: "do the thing"}, `["error", "step"]`)
	require.NoError(t, err)

	want := `{"error": false, "step": "foo"}`
	require.JSONEq(t, want, buf.String())
}

func TestWriteFailureFieldsFilterSeesIsErrorAsErrorField(t *testing.T) {
	var buf bytes.Buffer
	er := NewError("bad_input", "resource not found").WithResource("spec.md")
	err := WriteFailure(&buf, er, `["error", "code", "resource"]`)
	require.NoError(t, err)

	want := `{"error": true, "code": "bad_input", "resource": "spec.md"}`
	require.JSONEq(t, want, buf.String())
}

func TestWriteUnknownFieldErrorsOnSuccessValue(t *testing.T) {
	var buf bytes.Buffer
	err := Write(&buf, stepResult{Step: "foo", Instruction: "do the thing"}, `["not_a_real_field"]`)
	require.Error(t, err)
	require.Contains(t, err.Error(), "unknown field")
}

func TestWriteFailureUnknownFieldErrorsOnErrorResponse(t *testing.T) {
	var buf bytes.Buffer
	er := NewError("bad_input", "resource not found")
	err := WriteFailure(&buf, er, `["not_a_real_field"]`)
	require.Error(t, err)
	require.Contains(t, err.Error(), "unknown field")
}
