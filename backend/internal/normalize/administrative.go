package normalize

// Explicit landmarks are not settlement/street mentions. Keep them in the
// original parser input and result, but exclude their spans from reference
// lookup and administrative evidence. A repeated name elsewhere remains usable.
func administrativeText(work string) string {
	return landmarkRE.ReplaceAllString(work, " ")
}
func administrativeSignals(work, administrative string, signals []evidence) []evidence {
	kept := make([]evidence, 0, len(signals))
	for _, signal := range signals {
		switch signal.Field {
		case "city", "province", "district", "living_area", "street_name":
			if contains(work, signal.Value) && !contains(administrative, signal.Value) {
				continue
			}
		}
		kept = append(kept, signal)
	}
	return kept
}
