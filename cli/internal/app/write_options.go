package app

// WriteOptions selects an explicitly requested publication scope.
// Progress reports staged writes; success is only durable after the method returns nil.
type WriteOptions struct {
	ReadyOnly bool
	Progress  func(completed, total int)
}

const readyRecords = "(reviewed=1 OR json_extract(result,'$.status')='matched')"

func writeFilter(ready bool) string {
	if ready {
		return " AND " + readyRecords
	}
	return ""
}
