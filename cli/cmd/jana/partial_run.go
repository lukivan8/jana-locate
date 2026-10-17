package main

import "example.com/jana-locate-cli/internal/api"

// Continue only the latest journal for this exact source, never an older
// publication superseded by an explicit normalize --new.
func partialRun(runs []api.Run, source api.Source, apiURL string) string {
	if source.ConnectionId == nil {
		return ""
	}
	for _, run := range runs {
		s := run.Source
		if s.ConnectionId == nil || *s.ConnectionId != *source.ConnectionId {
			continue
		}
		if s.Schema != source.Schema || s.Table != source.Table || s.Key != source.Key || s.KeyType != source.KeyType || s.Column != source.Column || s.TargetSchema != source.TargetSchema || s.TargetTable != source.TargetTable {
			continue
		}
		if (run.Status == api.Review || run.Status == api.Normalizing || run.Status == api.Failed) && run.ApiUrl == apiURL {
			return run.Id
		}
		return ""
	}
	return ""
}
