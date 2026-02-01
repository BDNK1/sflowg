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
	}

	for _, tc := range testCases {
		result := FormatExpression(tc.input)
		if result != tc.expected {
			t.Errorf("FormatExpression(%q) = %q; want %q", tc.input, result, tc.expected)
		}
	}
}
