package runtime

import (
	"testing"
)

func TestFormatKey(t *testing.T) {
	testCases := []struct {
		input    string
		expected string
	}{
		{"Content-Type", "Content_Type"},
		{"X-Request-ID", "X_Request_ID"},
		{"User.Agent", "User_Agent"},
		{"Accept-Language", "Accept_Language"},
		{"my-custom-header", "my_custom_header"},
		{"a-b-c-d-e", "a_b_c_d_e"},
		{"request.headers.X-API-Key", "request_headers_X_API_Key"},
	}

	for _, tc := range testCases {
		actual := FormatKey(tc.input)
		if actual != tc.expected {
			t.Errorf("FormatKey(%q) = %q, expected %q", tc.input, actual, tc.expected)
		}
	}
}

func TestFormatExpression(t *testing.T) {
	testCases := []struct {
		input    string
		expected string
	}{
		// Basic dot to underscore conversion
		{"a.b", "a_b"},
		{"A.B", "A_B"},
		{"a.B.c.D.E", "a_B_c_D_E"},
		{"request.headers.X-API-Key", "request_headers_X_API_Key"},

		// Optional chaining ?. should be preserved
		{"user?.name", "user?.name"},
		{"a?.b?.c", "a?.b?.c"},
		{"user.profile?.settings", "user_profile?.settings"},
		{"missing?.nested?.deep", "missing?.nested?.deep"},

		// Lambda element accessor #. should be preserved
		{"filter(items, {#.Age > 18})", "filter(items, {#.Age > 18})"},
		{"map(users, {#.Name})", "map(users, {#.Name})"},
		{"filter(users, {#.Age > 18 && #.Active})", "filter(users, {#.Age > 18 && #.Active})"},
		{"any(items, {#.Price > 100})", "any(items, {#.Price > 100})"},

		// Combined: optional chaining in lambda
		{"{#.user?.name}", "{#.user?.name}"},

		// String literals: dots inside quotes must NOT be replaced
		{`event_type == "payment_intent.succeeded"`, `event_type == "payment_intent.succeeded"`},
		{`a.b == "x.y.z"`, `a_b == "x.y.z"`},
		{`a.b != "payment_intent.succeeded" && a.c != "payment_intent.failed"`, `a_b != "payment_intent.succeeded" && a_c != "payment_intent.failed"`},
		{`"hello.world"`, `"hello.world"`},
		{`a.b == "x.y" ? "a.b" : "c.d"`, `a_b == "x.y" ? "a.b" : "c.d"`},

		// Escaped quotes inside strings
		{`a.b == "say \"hello.world\""`, `a_b == "say \"hello.world\""`},

		// Backtick strings
		{"a.b == `x.y.z`", "a_b == `x.y.z`"},

		// Hyphens inside string literals must NOT be replaced
		{`a.b == "Content-Type"`, `a_b == "Content-Type"`},

		// Float literals: dots between digits must NOT be replaced
		{"amount > 3.14", "amount > 3.14"},
		{"a.b > 0.5", "a_b > 0.5"},
		{"price == 19.99", "price == 19.99"},
		{"a.b * 1.15", "a_b * 1.15"},
		{"100.0", "100.0"},
		{"a.b > 0.5 && c.d < 3.14", "a_b > 0.5 && c_d < 3.14"},
	}

	for _, tc := range testCases {
		result := FormatExpression(tc.input)
		if result != tc.expected {
			t.Errorf("FormatExpression(%q) = %q; want %q", tc.input, result, tc.expected)
		}
	}
}
