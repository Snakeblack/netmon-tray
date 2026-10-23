package domain

// Classify attributes a sample to the closest layer that fails. A failing DNS
// with a reachable internet is degraded service, not an outage, so it has no
// cause.
func Classify(s Sample) (outage bool, cause Cause) {
	switch {
	case !s.Link:
		return true, CauseLink
	case !s.Router:
		return true, CauseRouter
	case !s.Internet:
		return true, CauseLine
	default:
		return false, CauseNone
	}
}

// healthy reports whether every layer answers.
func healthy(s Sample) bool {
	return s.Link && s.Router && s.Internet && s.DNS
}
