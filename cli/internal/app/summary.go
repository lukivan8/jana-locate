package app

import (
	"context"
	"example.com/jana-locate-cli/internal/api"
)

func (s *Store) Summary(ctx context.Context, id string) (api.RunSummary, error) {
	var out api.RunSummary
	if _, err := s.Get(ctx, id); err != nil {
		return out, err
	}
	err := s.DB.QueryRowContext(ctx, `SELECT
 COALESCE(SUM(CASE WHEN reviewed=0 AND result IS NOT NULL AND json_extract(result,'$.status')='matched' THEN 1 ELSE 0 END),0),
 COALESCE(SUM(CASE WHEN reviewed=1 THEN 1 ELSE 0 END),0),
 COALESCE(SUM(CASE WHEN reviewed=0 AND result IS NOT NULL AND json_extract(result,'$.status')!='matched' THEN 1 ELSE 0 END),0),
 COALESCE(SUM(CASE WHEN result IS NULL THEN 1 ELSE 0 END),0)
 FROM records WHERE run_id=?`, id).Scan(&out.Automatic, &out.Reviewed, &out.NeedsReview, &out.Pending)
	return out, err
}
