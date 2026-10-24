package control

import "testing"

// Windows writes a DACL back with aliases for well-known SIDs, such as LA for
// the local Administrator (RID 500): the check compares it with the wanted
// DACL in that same canonical form, never with one built from the raw SID.
func TestIsPrivateComparesCanonicalForms(t *testing.T) {
	const (
		user  = "D:P(A;OICI;FA;;;S-1-5-21-1-2-3-1001)"
		admin = "D:P(A;OICI;FA;;;LA)" // how Windows writes S-1-5-21-…-500
	)
	for _, tc := range []struct {
		got, want string
		private   bool
	}{
		{user, user, true},
		{"D:PAI(A;OICI;FA;;;S-1-5-21-1-2-3-1001)", user, true},
		{"D:PAI(A;OICI;FA;;;LA)", admin, true},                                // windows-latest runs as the local Administrator
		{"D:(A;OICI;FA;;;S-1-5-21-1-2-3-1001)", user, false},                  // inherits from the parent
		{"D:P(A;OICI;FA;;;S-1-5-21-1-2-3-1001)(A;OICI;FA;;;SY)", user, false}, // SYSTEM too
		{"D:P(A;OICI;FR;;;S-1-5-21-1-2-3-1001)", user, false},                 // read only
		{"D:P(A;OICI;FA;;;S-1-5-21-1-2-3-1002)", user, false},                 // another user
		{"", user, false},
	} {
		if got := isPrivate(tc.got, tc.want); got != tc.private {
			t.Errorf("isPrivate(%q, %q) = %v, want %v", tc.got, tc.want, got, tc.private)
		}
	}
}
