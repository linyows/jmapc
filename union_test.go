package jmapc

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestUnionMarshalsTheShapeItHolds(t *testing.T) {
	u := FilterOperatorOrEmailFilterCondition{
		EmailFilterCondition: &EmailFilterCondition{Text: "invoice"},
	}
	got, err := json.Marshal(u)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if want := `{"text":"invoice"}`; string(got) != want {
		t.Errorf("Marshal = %s, want %s", got, want)
	}
}

func TestUnionRefusesToMarshalNothing(t *testing.T) {
	_, err := json.Marshal(FilterOperatorOrEmailFilterCondition{})
	if err == nil {
		t.Fatal("Marshal of an empty union succeeded, want an error")
	}
	if !strings.Contains(err.Error(), "no alternative is set") {
		t.Errorf("error = %v, want it to say no alternative is set", err)
	}
}

func TestUnionRefusesToMarshalTwoShapes(t *testing.T) {
	u := FilterOperatorOrEmailFilterCondition{
		FilterOperator:       &FilterOperator{Operator: "AND"},
		EmailFilterCondition: &EmailFilterCondition{Text: "invoice"},
	}
	_, err := json.Marshal(u)
	if err == nil {
		t.Fatal("Marshal of two shapes succeeded, want an error")
	}
	if !strings.Contains(err.Error(), "2 alternatives are set") {
		t.Errorf("error = %v, want it to say two alternatives are set", err)
	}
}

func TestUnionUnmarshalsTheShapeThatFits(t *testing.T) {
	tests := []struct {
		name     string
		json     string
		operator bool
	}{
		{"operator", `{"operator":"AND","conditions":[{"text":"invoice"}]}`, true},
		{"condition", `{"text":"invoice"}`, false},
		{"empty object", `{}`, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var u FilterOperatorOrEmailFilterCondition
			if err := json.Unmarshal([]byte(tt.json), &u); err != nil {
				t.Fatalf("Unmarshal: %v", err)
			}
			if tt.operator && u.FilterOperator == nil {
				t.Fatalf("Unmarshal(%s) did not fill FilterOperator", tt.json)
			}
			if !tt.operator && u.EmailFilterCondition == nil {
				t.Fatalf("Unmarshal(%s) did not fill EmailFilterCondition", tt.json)
			}
			if u.FilterOperator != nil && u.EmailFilterCondition != nil {
				t.Errorf("Unmarshal(%s) filled both shapes", tt.json)
			}
		})
	}
}

// A server may add a property to a shape jmapc knows, and that must not make
// the value unreadable: the shape is still the one its required properties say.
func TestUnionKeepsReadingAShapeThatGrewAProperty(t *testing.T) {
	var u FilterOperatorOrEmailFilterCondition
	if err := json.Unmarshal([]byte(`{"operator":"OR","conditions":[],"weight":3}`), &u); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if u.FilterOperator == nil {
		t.Fatal("Unmarshal did not fill FilterOperator")
	}
	if u.FilterOperator.Operator != "OR" {
		t.Errorf("operator = %q, want OR", u.FilterOperator.Operator)
	}
}

func TestUnionReportsAValueThatFitsNoShape(t *testing.T) {
	var u FilterOperatorOrEmailFilterCondition
	err := json.Unmarshal([]byte(`"invoice"`), &u)
	if err == nil {
		t.Fatal("Unmarshal of a string succeeded, want an error")
	}
	if !strings.Contains(err.Error(), "fits none of the shapes") {
		t.Errorf("error = %v, want it to say the value fits no shape", err)
	}
}

func TestUnionReadsNullAsNothing(t *testing.T) {
	u := FilterOperatorOrEmailFilterCondition{FilterOperator: &FilterOperator{Operator: "AND"}}
	if err := u.UnmarshalJSON([]byte("null")); err != nil {
		t.Fatalf("UnmarshalJSON: %v", err)
	}
	if u.FilterOperator != nil || u.EmailFilterCondition != nil {
		t.Errorf("null left a shape set: %+v", u)
	}
}

// The point of the union is that an argument holding one is typed, so a filter
// travels as itself rather than as an any the caller has to get right.
func TestQueryArgumentsCarryATypedFilter(t *testing.T) {
	args := EmailQueryArguments{
		AccountID: "a",
		Filter: &FilterOperatorOrEmailFilterCondition{
			FilterOperator: &FilterOperator{
				Operator:   "AND",
				Conditions: []any{EmailFilterCondition{Text: "invoice"}},
			},
		},
	}
	got, err := json.Marshal(args)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	want := `{"accountId":"a","filter":{"operator":"AND","conditions":[{"text":"invoice"}]}}`
	if string(got) != want {
		t.Errorf("Marshal = %s, want %s", got, want)
	}
}
