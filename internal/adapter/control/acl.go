package control

import "strings"

// privateACE grants sid full access, inherited by files and subdirectories.
func privateACE(sid string) string {
	return "(A;OICI;FA;;;" + sid + ")"
}

// isPrivate reports whether the DACL got is the protected DACL want. Both
// are in the SDDL form Windows writes, which uses aliases for well-known SIDs
// (LA for the local Administrator), so want must come from that same
// conversion. Windows may add the auto-inherited flag (AI) when it stores it.
func isPrivate(got, want string) bool {
	norm := func(s string) string {
		if rest, ok := strings.CutPrefix(s, "D:PAI("); ok {
			return "D:P(" + rest
		}
		return s
	}
	return strings.HasPrefix(want, "D:P(") && norm(got) == norm(want)
}
